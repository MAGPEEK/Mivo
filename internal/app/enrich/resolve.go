package enrich

import (
	"cmp"
	"context"
	"fmt"
	"slices"

	"github.com/magpeek/mivo/internal/domain/library"
	"github.com/magpeek/mivo/internal/domain/metadata"
	"github.com/magpeek/mivo/internal/domain/metadata/match"
)

// exactConfidence is the confidence of a link made by a known provider ID.
const exactConfidence = 1.0

// resolution is what matching found. details is nil when there is no new provider data;
// skip means nothing was asked; ownRef means result.Ref names the item itself (not the show
// of a season or episode), so its ID belongs in the item's record.
type resolution struct {
	skip    bool
	ownRef  bool
	result  MatchResult
	source  metadata.Source
	details *Details
}

// finder is one provider's search and details for a titled item (movie, show, album,
// artist), so the matching flow exists once.
type finder struct {
	kind      match.RefKind
	source    metadata.Source
	providers []match.RefProvider
	search    func(ctx context.Context, query match.Query) ([]match.Candidate, error)
	details   func(ctx context.Context, ref match.ProviderRef) (Details, error)
}

// resolveMatch dispatches on the item kind; mayLookUp has excluded kinds without provider.
func (enricher *Enricher) resolveMatch(ctx context.Context, current item) (resolution, error) {
	switch current.snapshot.Kind {
	case library.ItemKindSeason, library.ItemKindEpisode:
		return enricher.resolveShowChild(ctx, current)
	case library.ItemKindAlbum, library.ItemKindArtist:
		return resolveTitled(ctx, current, enricher.musicFinder(current))
	case library.ItemKindShow:
		return resolveTitled(ctx, current, enricher.showFinder(current))
	case library.ItemKindMovie:
		return resolveTitled(ctx, current, enricher.movieFinder(current))
	case library.ItemKindExtra, library.ItemKindTrack, library.ItemKindPhotoAlbum, library.ItemKindPhoto,
		library.ItemKindAudiobook:
		return resolution{skip: true}, nil
	default:
		return resolution{skip: true}, nil
	}
}

// resolveTitled uses a known provider ID when the item has one (ADR 0031 §8: trusted, no
// search); otherwise it searches and lets match.Resolve decide.
func resolveTitled(ctx context.Context, current item, find finder) (resolution, error) {
	if ref, ok := knownRef(current.snapshot, find); ok {
		return fetchDetails(ctx, find, ref, exactConfidence)
	}

	query := searchQuery(current.snapshot)

	candidates, err := find.search(ctx, query)
	if err != nil {
		return resolution{}, fmt.Errorf("search %s: %w", find.kind, err)
	}

	providerIDsLocked := current.snapshot.Locks.Has(metadata.FieldProviderIDs)
	decision := match.Resolve(current.snapshot.MatchState, providerIDsLocked, match.Rank(query, candidates))

	if decision.Chosen == nil {
		return resolution{result: MatchResult{State: decision.State}}, nil
	}

	return fetchDetails(ctx, find, decision.Chosen.Ref, match.Score(query, *decision.Chosen))
}

func fetchDetails(ctx context.Context, find finder, ref match.ProviderRef, confidence float64) (resolution, error) {
	details, err := find.details(ctx, ref)
	if err != nil {
		return resolution{}, fmt.Errorf("details %s %s %s: %w", ref.Provider, find.kind, ref.ID, err)
	}

	return resolution{
		ownRef:  true,
		result:  MatchResult{State: match.MatchStateMatched, Provider: ref.Provider, Ref: ref, Confidence: confidence},
		source:  find.source,
		details: &details,
	}, nil
}

// knownRef returns the reference the item is linked to: the first provider ID in the record
// the finder can use, else LinkedRef. The record wins because a trusted NFO ID (ADR 0031 §8)
// or an Identify edit (§10) changes it while LinkedRef still names the earlier entry;
// LinkedRef is the fallback for IDs a record cannot hold (MusicBrainz).
func knownRef(snapshot ItemSnapshot, find finder) (match.ProviderRef, bool) {
	if ref, ok := refFromIDs(snapshot.Record.ProviderIDs, find); ok {
		return ref, true
	}

	linked := snapshot.LinkedRef
	if !linked.IsZero() && slices.Contains(find.providers, linked.Provider) {
		return linked, true
	}

	return match.ProviderRef{}, false
}

// refFromIDs prefers TMDB over IMDb because TMDB IDs name the kind they belong to.
func refFromIDs(ids []metadata.ProviderID, find finder) (match.ProviderRef, bool) {
	candidates := []struct {
		provider    metadata.Provider
		refProvider match.RefProvider
		kind        match.RefKind
	}{
		{metadata.ProviderTMDB, match.RefTMDB, find.kind},
		{metadata.ProviderIMDb, match.RefIMDb, match.RefKindUnknown},
	}

	for _, candidate := range candidates {
		if !slices.Contains(find.providers, candidate.refProvider) {
			continue
		}

		for _, id := range ids {
			if id.Provider == candidate.provider && id.Value != "" {
				return match.ProviderRef{Provider: candidate.refProvider, Kind: candidate.kind, ID: id.Value}, true
			}
		}
	}

	return match.ProviderRef{}, false
}

// withRefID adds the ID the item was linked by when the record lacks it, so the next run
// skips the search. MusicBrainz IDs have no metadata.Provider; they travel in MatchResult.Ref.
func withRefID(record metadata.Record, ref match.ProviderRef) metadata.Record {
	id, ok := ref.ProviderID()
	if !ok {
		return record
	}

	if _, present := record.ProviderID(id.Provider); present {
		return record
	}

	record.ProviderIDs = append(slices.Clone(record.ProviderIDs), id)

	return record
}

// searchQuery searches with the title and year the user locked, else the NFO ones, else the
// parsed file name ones (the same priority as the merge).
func searchQuery(snapshot ItemSnapshot) match.Query {
	var query match.Query

	if snapshot.Locks.Has(metadata.FieldTitle) {
		query.Title = snapshot.Record.Title
	}

	if snapshot.Locks.Has(metadata.FieldYear) {
		query.Year = snapshot.Record.Year
	}

	for _, local := range []metadata.Record{snapshot.Local.NFO, snapshot.Local.Filename} {
		query.Title = cmp.Or(query.Title, local.Title)
		query.Year = cmp.Or(query.Year, local.Year)
	}

	return query
}

func (enricher *Enricher) movieFinder(current item) finder {
	movies := enricher.deps.Movies

	return finder{
		kind:      match.RefKindMovie,
		source:    metadata.SourceTMDB,
		providers: []match.RefProvider{match.RefTMDB, match.RefIMDb},
		search: func(ctx context.Context, query match.Query) ([]match.Candidate, error) {
			return movies.Search(ctx, SearchQuery{Title: query.Title, Year: query.Year, Language: current.language})
		},
		details: func(ctx context.Context, ref match.ProviderRef) (Details, error) {
			return movies.Details(ctx, DetailsRequest{Ref: ref, Language: current.language})
		},
	}
}

func (enricher *Enricher) showFinder(current item) finder {
	shows := enricher.deps.Shows

	return finder{
		kind:      match.RefKindShow,
		source:    metadata.SourceTMDB,
		providers: []match.RefProvider{match.RefTMDB, match.RefIMDb},
		search: func(ctx context.Context, query match.Query) ([]match.Candidate, error) {
			return shows.Search(ctx, SearchQuery{Title: query.Title, Year: query.Year, Language: current.language})
		},
		details: func(ctx context.Context, ref match.ProviderRef) (Details, error) {
			return shows.Details(ctx, DetailsRequest{Ref: ref, Language: current.language})
		},
	}
}

func (enricher *Enricher) musicFinder(current item) finder {
	music := enricher.deps.Music
	kind := match.RefKindRelease

	artist := current.snapshot.ArtistName
	if current.snapshot.Kind == library.ItemKindArtist {
		kind, artist = match.RefKindArtist, ""
	}

	return finder{
		kind:      kind,
		source:    metadata.SourceMusicBrainz,
		providers: []match.RefProvider{match.RefMusicBrainz},
		search: func(ctx context.Context, query match.Query) ([]match.Candidate, error) {
			return music.Search(ctx, MusicQuery{Kind: kind, Title: query.Title, Artist: artist, Year: query.Year})
		},
		details: func(ctx context.Context, ref match.ProviderRef) (Details, error) {
			return music.Details(ctx, DetailsRequest{Ref: ref, Language: current.language})
		},
	}
}
