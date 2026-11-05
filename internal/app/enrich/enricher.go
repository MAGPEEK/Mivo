package enrich

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/library"
	"github.com/magpeek/mivo/internal/domain/metadata"
	"github.com/magpeek/mivo/internal/domain/metadata/match"
)

// Outcome says what EnrichItem did with an item.
type Outcome string

// Outcomes of EnrichItem.
const (
	// OutcomeSkipped: nothing was asked (switched off, disabled, kind without provider, or a
	// season or episode whose show has no provider ID yet).
	OutcomeSkipped Outcome = "skipped"
	// OutcomeUnmatched: the providers were asked but there was no confident match.
	OutcomeUnmatched Outcome = "unmatched"
	// OutcomeMatched: the item is linked; provider data was merged unless a kept match had
	// no clear winner this time.
	OutcomeMatched Outcome = "matched"
)

// Deps are the ports of the Enricher. A nil lookup counts as a provider that is switched
// off; a nil Artwork downloader registers no provider pictures; a nil Logger discards.
type Deps struct {
	Store    MetadataStore
	Settings SettingsSource
	Movies   MovieLookup
	Shows    ShowLookup
	Music    MusicLookup
	Artwork  ArtworkDownloader
	Logger   *slog.Logger
}

// Enricher runs the use case "enrich one item".
type Enricher struct {
	deps Deps
}

// NewEnricher returns an Enricher on deps.
func NewEnricher(deps Deps) *Enricher {
	if deps.Logger == nil {
		deps.Logger = slog.New(slog.DiscardHandler)
	}

	return &Enricher{deps: deps}
}

// item bundles what one enrichment run knows about the item.
type item struct {
	id       string
	snapshot ItemSnapshot
	language string
}

// ErrItemGone marks an error caused by the item itself being unknown (deleted since it was
// queued), as opposed to a provider's domain.ErrNotFound for a stale reference. The queue
// checks it first and drops the job instead of retrying it.
var ErrItemGone = errors.New("item gone")

// EnrichItem looks the item up at its provider and stores the merged result. Errors of a
// lookup keep their domain.ErrUnavailable, domain.ErrRateLimited or domain.ErrNotFound so
// the queue can retry; nothing is written then and the item keeps its local data. An
// unknown item wraps ErrItemGone in addition to domain.ErrNotFound.
func (enricher *Enricher) EnrichItem(ctx context.Context, itemID string) (Outcome, error) {
	settings, err := enricher.deps.Settings.OnlineSettings(ctx)
	if err != nil {
		return OutcomeSkipped, fmt.Errorf("enrich item %s: read settings: %w", itemID, err)
	}

	if !settings.Enabled {
		return OutcomeSkipped, nil
	}

	snapshot, err := enricher.deps.Store.ItemSnapshot(ctx, itemID)
	if err != nil {
		return OutcomeSkipped, fmt.Errorf("enrich item %s: load: %w", itemID, markItemGone(err))
	}

	if !enricher.mayLookUp(snapshot, settings) {
		return OutcomeSkipped, nil
	}

	current := item{id: itemID, snapshot: snapshot, language: languageOf(snapshot, settings)}

	resolved, err := enricher.resolveMatch(ctx, current)
	if err != nil {
		return OutcomeSkipped, fmt.Errorf("enrich item %s: %w", itemID, err)
	}

	return enricher.store(ctx, current, resolved)
}

// mayLookUp reports whether the item may be looked up at all; a disabled item, a library
// without online metadata and a provider switched off are skipped before any call.
func (enricher *Enricher) mayLookUp(snapshot ItemSnapshot, settings OnlineSettings) bool {
	if snapshot.MatchState == match.MatchStateDisabled || !snapshot.OnlineEnabled {
		return false
	}

	switch snapshot.Kind {
	case library.ItemKindMovie:
		return settings.TMDB && enricher.deps.Movies != nil
	case library.ItemKindShow, library.ItemKindSeason, library.ItemKindEpisode:
		return settings.TMDB && enricher.deps.Shows != nil
	case library.ItemKindAlbum, library.ItemKindArtist:
		return settings.MusicBrainz && enricher.deps.Music != nil
	case library.ItemKindExtra, library.ItemKindTrack, library.ItemKindPhotoAlbum, library.ItemKindPhoto,
		library.ItemKindAudiobook:
		return false // no provider in milestone 6 (ADR 0031)
	default:
		return false
	}
}

func languageOf(snapshot ItemSnapshot, settings OnlineSettings) string {
	if snapshot.Language != "" {
		return snapshot.Language
	}

	return settings.Language
}

// store writes what the resolution found: the full enrichment for new provider data, else
// only a changed match state.
func (enricher *Enricher) store(ctx context.Context, current item, resolved resolution) (Outcome, error) {
	if resolved.skip {
		return OutcomeSkipped, nil
	}

	if resolved.details == nil {
		return enricher.storeMatchState(ctx, current, resolved.result.State)
	}

	selection, providerArtwork, err := enricher.chooseArtwork(ctx, current, resolved.details.Artwork)
	if err != nil {
		return OutcomeSkipped, fmt.Errorf("enrich item %s: %w", current.id, err)
	}

	enrichment := Enrichment{
		Record:          mergeRecord(current.snapshot, resolved),
		Match:           resolved.result,
		ProviderArtwork: providerArtwork,
		Selection:       selection,
	}

	if err := enricher.deps.Store.SaveEnrichment(ctx, current.id, enrichment); err != nil {
		return OutcomeSkipped, fmt.Errorf("enrich item %s: save: %w", current.id, markItemGone(err))
	}

	return OutcomeMatched, nil
}

func (enricher *Enricher) storeMatchState(ctx context.Context, current item, state match.MatchState) (Outcome, error) {
	outcome := OutcomeUnmatched
	if state == match.MatchStateMatched {
		outcome = OutcomeMatched
	}

	if state == current.snapshot.MatchState {
		return outcome, nil
	}

	if err := enricher.deps.Store.SaveMatchState(ctx, current.id, MatchResult{State: state}); err != nil {
		return OutcomeSkipped, fmt.Errorf("enrich item %s: save match state: %w", current.id, markItemGone(err))
	}

	return outcome, nil
}

// markItemGone adds ErrItemGone to a store error about an unknown item.
func markItemGone(err error) error {
	if errors.Is(err, domain.ErrNotFound) {
		return fmt.Errorf("%w: %w", ErrItemGone, err)
	}

	return err
}

// mergeRecord merges the sources in the order of ADR 0031 §8: user (the current values of
// the locked fields), nfo, the provider, filename. Merge keeps every locked field anyway; the
// user source documents the order and is where the edit dialog's values will come from.
// Provider IDs are taken whole from one source, so the ID the item was linked by is added
// afterwards unless the field is locked: an NFO with only a TVDB ID must not push it out.
func mergeRecord(snapshot ItemSnapshot, resolved resolution) metadata.Record {
	userRecord := metadata.Merge(snapshot.Record, snapshot.Locks, nil)

	merged := metadata.Merge(snapshot.Record, snapshot.Locks, []metadata.SourcedRecord{
		{Source: metadata.SourceUser, Record: userRecord},
		{Source: metadata.SourceNFO, Record: snapshot.Local.NFO},
		{Source: resolved.source, Record: resolved.details.Record},
		{Source: metadata.SourceFilename, Record: snapshot.Local.Filename},
	})

	if resolved.ownRef && !snapshot.Locks.Has(metadata.FieldProviderIDs) {
		merged = withRefID(merged, resolved.result.Ref)
	}

	return merged
}
