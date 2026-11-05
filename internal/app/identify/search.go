package identify

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/magpeek/mivo/internal/app/enrich"
	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/artwork"
	"github.com/magpeek/mivo/internal/domain/metadata"
	"github.com/magpeek/mivo/internal/domain/metadata/match"
)

// ErrProvider marks a failure of the provider call itself, so callers can tell "the provider
// is unreachable or rate limiting" (it wraps domain.ErrUnavailable or ErrRateLimited too)
// from an unavailable store or a shutdown.
var ErrProvider = errors.New("metadata provider failed")

// MaxCandidates bounds the candidates one search returns (API contract: at most 25).
const MaxCandidates = 25

// exactConfidence is the confidence of the entry a pasted ID names.
const exactConfidence = 1.0

// Query is what the user searches with: a title and an optional year (0 when unknown), or
// a pasted provider ID or URL in ProviderRef, which wins over the title.
type Query struct {
	Text        string
	Year        int
	ProviderRef string
}

// Search returns provider candidates for the item, best first, at most MaxCandidates. Movies
// are searched as movies; shows, seasons and episodes as shows (a season or episode is
// matched through its show); albums and artists at the music provider. A pasted ID or URL is
// looked up directly and returned with confidence 1, or not at all when the provider does
// not know it. Errors wrap domain.ErrInvalid (empty query, unusable ID, kind without
// provider), domain.ErrConflict (online metadata switched off), domain.ErrNotFound (unknown
// item) or the lookup's domain.ErrUnavailable / domain.ErrRateLimited.
func (service *Service) Search(ctx context.Context, itemID string, query Query) ([]match.Scored, error) {
	current, err := service.loadOnlineTarget(ctx, itemID)
	if err != nil {
		return nil, fmt.Errorf("search candidates for item %s: %w", itemID, err)
	}

	var candidates []match.Scored

	if strings.TrimSpace(query.ProviderRef) != "" {
		candidates, err = service.lookUpRef(ctx, current, query.ProviderRef)
	} else {
		candidates, err = service.searchTitle(ctx, current, query)
	}

	if err != nil {
		return nil, fmt.Errorf("search candidates for item %s: %w", itemID, err)
	}

	return candidates[:min(len(candidates), MaxCandidates)], nil
}

func (service *Service) searchTitle(ctx context.Context, current target, query Query) ([]match.Scored, error) {
	title := strings.TrimSpace(query.Text)
	if title == "" {
		return nil, fmt.Errorf("empty query: %w", domain.ErrInvalid)
	}

	found, err := service.searchProvider(ctx, current, enrich.SearchQuery{
		Title: title, Year: query.Year, Language: current.language,
	})
	if err != nil {
		return nil, fmt.Errorf("search %s %q: %w: %w", current.family.kind, title, ErrProvider, err)
	}

	return match.Rank(match.Query{Title: title, Year: query.Year}, found), nil
}

func (service *Service) searchProvider(ctx context.Context, current target, query enrich.SearchQuery) ([]match.Candidate, error) {
	switch {
	case current.family.music:
		artist := current.snapshot.ArtistName
		if current.family.kind == match.RefKindArtist {
			artist = ""
		}

		return service.deps.Music.Search(ctx, enrich.MusicQuery{
			Kind: current.family.kind, Title: query.Title, Artist: artist, Year: query.Year,
		})
	case current.family.kind == match.RefKindMovie:
		return service.deps.Movies.Search(ctx, query)
	default:
		return service.deps.Shows.Search(ctx, query)
	}
}

// lookUpRef returns the entry a pasted ID or URL names as the only candidate, none when the
// provider does not know it.
func (service *Service) lookUpRef(ctx context.Context, current target, text string) ([]match.Scored, error) {
	parsed, err := match.ParseProviderRef(text)
	if err != nil {
		return nil, err
	}

	ref, err := fitRef(current.family, parsed)
	if err != nil {
		return nil, err
	}

	details, err := service.details(ctx, current, ref)
	if errors.Is(err, domain.ErrNotFound) {
		return []match.Scored{}, nil
	}

	if err != nil {
		return nil, fmt.Errorf("look up %s %s: %w: %w", ref.Provider, ref.ID, ErrProvider, err)
	}

	candidate := match.Candidate{
		Ref:           preferTMDB(ref, details.Record, current.family.kind),
		Title:         details.Record.Title,
		OriginalTitle: details.Record.OriginalTitle,
		Year:          details.Record.Year,
		Overview:      details.Record.Summary,
		Poster:        tmdbPoster(details.Artwork),
	}

	return []match.Scored{{Candidate: candidate, Confidence: exactConfidence, ExactID: true}}, nil
}

func (service *Service) details(ctx context.Context, current target, ref match.ProviderRef) (enrich.Details, error) {
	request := enrich.DetailsRequest{Ref: ref, Language: current.language}

	switch {
	case current.family.music:
		return service.deps.Music.Details(ctx, request)
	case current.family.kind == match.RefKindMovie:
		return service.deps.Movies.Details(ctx, request)
	default:
		return service.deps.Shows.Details(ctx, request)
	}
}

// fitRef checks that a reference can identify an item of the family and completes its
// kind. An IMDb ID keeps the unknown kind, like enrichment uses it. The error wraps
// domain.ErrInvalid.
func fitRef(itemFamily family, ref match.ProviderRef) (match.ProviderRef, error) {
	if !slices.Contains(itemFamily.providers, ref.Provider) {
		return match.ProviderRef{}, fmt.Errorf("a %s ID cannot identify a %s: %w", ref.Provider, itemFamily.kind, domain.ErrInvalid)
	}

	if ref.Kind != match.RefKindUnknown && ref.Kind != itemFamily.kind {
		return match.ProviderRef{}, fmt.Errorf("a %s %s ID cannot identify a %s: %w", ref.Provider, ref.Kind, itemFamily.kind, domain.ErrInvalid)
	}

	if ref.Provider != match.RefIMDb {
		ref.Kind = itemFamily.kind
	}

	return ref, nil
}

// tmdbPoster returns the best TMDB poster of the details, empty when there is none. Only TMDB
// pictures are offered as candidate previews (M6-13: the preview proxy fetches from the TMDB
// image host only).
func tmdbPoster(pictures []enrich.ArtworkRef) string {
	for _, picture := range pictures {
		if picture.Provider == enrich.ArtworkProviderTMDB && picture.Kind == artwork.KindPoster {
			return picture.Path
		}
	}

	return ""
}

// preferTMDB replaces an IMDb reference by the TMDB ID of the same entry when the details
// carry one, because candidates are applied by provider and the API knows no IMDb provider.
func preferTMDB(ref match.ProviderRef, record metadata.Record, kind match.RefKind) match.ProviderRef {
	if ref.Provider != match.RefIMDb {
		return ref
	}

	if id, ok := record.ProviderID(metadata.ProviderTMDB); ok && id != "" {
		return match.ProviderRef{Provider: match.RefTMDB, Kind: kind, ID: id}
	}

	return ref
}
