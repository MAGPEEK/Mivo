package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"time"
	"uuid"

	"github.com/magpeek/mivo/internal/adapters/sqlite/sqlcgen"
	"github.com/magpeek/mivo/internal/app/enrich"
	"github.com/magpeek/mivo/internal/app/identify"
	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/artwork"
	"github.com/magpeek/mivo/internal/domain/metadata"
	"github.com/magpeek/mivo/internal/domain/metadata/match"
)

// Compile-time check: the identify use cases store their match changes here.
var _ identify.MatchWriter = (*EnrichmentStore)(nil)

// Artwork providers of the provider rows (artwork.provider), by the provider of the match:
// TMDB matches bring TMDB images, MusicBrainz matches Cover Art Archive images.
const (
	artworkProviderTMDB     = string(enrich.ArtworkProviderTMDB)
	artworkProviderCoverArt = string(enrich.ArtworkProviderCoverArt)
)

// SaveMatchState stores only the match state of an item (enrich.MetadataStore). A disabled
// match is never replaced by another state: "Remove match" may have run while an enrichment
// job was in flight (ADR 0031 §9). The error wraps domain.ErrNotFound for an unknown item.
func (store *EnrichmentStore) SaveMatchState(ctx context.Context, itemID string, result enrich.MatchResult) error {
	now := store.clock()

	err := inWriteTransaction(ctx, store.writer, func(queries *sqlcgen.Queries) error {
		stored, err := readItemMatch(ctx, queries, itemID)
		if err != nil {
			return err
		}

		if store.keepsDisabled(ctx, itemID, stored, result) {
			return nil
		}

		return upsertMatch(ctx, queries, itemID, matchStateChange(stored, result, now))
	})
	if err != nil {
		return fmt.Errorf("save match state of item %q: %w", itemID, err)
	}

	return nil
}

// matchStateChange applies a state-only change checked at now: a match keeps its provider
// and reference unless the result names new ones, any other state drops them.
func matchStateChange(stored ItemMatch, result enrich.MatchResult, now time.Time) ItemMatch {
	if result.State != match.MatchStateMatched {
		return ItemMatch{State: result.State, CheckedAt: now}
	}

	if result.Provider != "" {
		return matchOf(result, now)
	}

	stored.State, stored.CheckedAt = match.MatchStateMatched, now
	if stored.MatchedAt.IsZero() {
		stored.MatchedAt = now
	}

	return stored
}

// keepsDisabled reports a change an in-flight enrichment job must not make: the user
// removed the match meanwhile.
func (store *EnrichmentStore) keepsDisabled(ctx context.Context, itemID string, stored ItemMatch, result enrich.MatchResult) bool {
	if stored.State != match.MatchStateDisabled || result.State == match.MatchStateDisabled {
		return false
	}

	store.logger.LogAttrs(ctx, slog.LevelDebug, "enrichment result dropped: match was removed",
		slog.String("item_id", itemID))

	return true
}

// SaveEnrichment stores record, match, provider artwork and selection of an item together
// (enrich.MetadataStore). Locked fields and kinds keep their stored values. The provider rows
// not in the enrichment are deleted; their files stay in the data directory. A disabled
// match is kept and nothing is written (see SaveMatchState). Errors wrap domain.ErrNotFound
// for an unknown item and domain.ErrInvalid for an invalid record or picture.
func (store *EnrichmentStore) SaveEnrichment(ctx context.Context, itemID string, enrichment enrich.Enrichment) error {
	now := store.clock()

	err := inWriteTransaction(ctx, store.writer, func(queries *sqlcgen.Queries) error {
		stored, err := readItemMatch(ctx, queries, itemID)
		if err != nil {
			return err
		}

		if store.keepsDisabled(ctx, itemID, stored, enrichment.Match) {
			return nil
		}

		if err := seedLocalRecords(ctx, queries, itemID, now); err != nil {
			return err
		}

		if err := saveMetadata(ctx, queries, itemID, enrichment.Record); err != nil {
			return err
		}

		if err := upsertMatch(ctx, queries, itemID, matchOf(enrichment.Match, now)); err != nil {
			return err
		}

		return replaceProviderArtwork(ctx, queries, providerArtworkRequest{
			itemID: itemID, provider: artworkProviderOf(enrichment.Match.Provider), now: now,
			candidates: enrichment.ProviderArtwork, selection: enrichment.Selection,
		})
	})
	if err != nil {
		return fmt.Errorf("save enrichment of item %q: %w", itemID, err)
	}

	return nil
}

// SaveMatchChange replaces record, locks and match of an item in one transaction
// (identify.MatchWriter). The record is stored as given: the use case already kept the
// locked fields. The error wraps domain.ErrNotFound for an unknown item.
func (store *EnrichmentStore) SaveMatchChange(ctx context.Context, itemID string, change identify.MatchChange) error {
	now := store.clock()

	err := inWriteTransaction(ctx, store.writer, func(queries *sqlcgen.Queries) error {
		if _, err := readItemMatch(ctx, queries, itemID); err != nil {
			return err
		}

		if err := seedLocalRecords(ctx, queries, itemID, now); err != nil {
			return err
		}

		if err := queries.DeleteItemFieldLocks(ctx, itemID); err != nil {
			return fmt.Errorf("delete field locks: %w", err)
		}

		// Without locks the record replaces every field; the new locks follow.
		if err := saveMetadata(ctx, queries, itemID, change.Record); err != nil {
			return err
		}

		if err := insertLocks(ctx, queries, itemID, change.Locks); err != nil {
			return err
		}

		return upsertMatch(ctx, queries, itemID, matchOf(change.Match, now))
	})
	if err != nil {
		return fmt.Errorf("save match change of item %q: %w", itemID, err)
	}

	return nil
}

func insertLocks(ctx context.Context, queries *sqlcgen.Queries, itemID string, locks metadata.LockSet) error {
	for _, field := range locks.Fields() {
		err := queries.InsertItemFieldLock(ctx, sqlcgen.InsertItemFieldLockParams{ItemID: itemID, Field: field.String()})
		if err != nil {
			return fmt.Errorf("insert field lock %s: %w", field, err)
		}
	}

	return nil
}

// matchOf turns a match result into the stored match checked now; a match is dated now.
func matchOf(result enrich.MatchResult, now time.Time) ItemMatch {
	stored := ItemMatch{
		State: result.State, Provider: string(result.Provider), Confidence: result.Confidence, Ref: result.Ref,
		CheckedAt: now,
	}

	if result.State == match.MatchStateMatched {
		stored.MatchedAt = now
	}

	return stored
}

// upsertMatch stores the match with its CheckedAt.
func upsertMatch(ctx context.Context, queries *sqlcgen.Queries, itemID string, itemMatch ItemMatch) error {
	if err := validateItemMatch(itemMatch); err != nil {
		return err
	}

	err := queries.UpsertItemMatch(ctx, sqlcgen.UpsertItemMatchParams{
		ItemID: itemID, State: itemMatch.State.String(), Provider: itemMatch.Provider,
		Confidence: itemMatch.Confidence, MatchedAt: optionalMillis(itemMatch.MatchedAt),
		RefProvider: string(itemMatch.Ref.Provider), RefKind: string(itemMatch.Ref.Kind), RefID: itemMatch.Ref.ID,
		CheckedAt: optionalMillis(itemMatch.CheckedAt),
	})
	if err != nil {
		return fmt.Errorf("store match: %w", foreignKeyAsNotFound(err))
	}

	return nil
}

// nullString stores "" as NULL.
func nullString(value string) sql.NullString {
	return sql.NullString{String: value, Valid: value != ""}
}

func artworkProviderOf(provider match.RefProvider) string {
	if provider == match.RefMusicBrainz {
		return artworkProviderCoverArt
	}

	return artworkProviderTMDB
}

// providerArtworkRequest is the artwork part of SaveEnrichment.
type providerArtworkRequest struct {
	itemID     string
	provider   string
	now        time.Time
	candidates []artwork.Candidate
	selection  map[artwork.Kind]artwork.Candidate
}

// replaceProviderArtwork makes the provider rows of the unlocked kinds the given candidates
// and stores the selection. The row has no remote address: the candidate carries only the
// stored file, so remote_url repeats the data path (the file name is derived from the
// provider reference and identifies the picture).
func replaceProviderArtwork(ctx context.Context, queries *sqlcgen.Queries, request providerArtworkRequest) error {
	locks, err := readFieldLocks(ctx, queries, request.itemID)
	if err != nil {
		return err
	}

	locked, err := lockedArtworkKinds(locks)
	if err != nil {
		return err
	}

	wanted, err := wantedProviderArtwork(request.candidates, locked)
	if err != nil {
		return err
	}

	run := artworkSave{queries: queries, itemID: request.itemID, now: request.now, locked: locked, ids: map[artworkKey]string{}}

	if err := run.reconcileProviderRows(ctx, wanted); err != nil {
		return err
	}

	for key, candidate := range wanted {
		if _, stored := run.ids[key]; stored {
			continue
		}

		if err := run.insertProvider(ctx, key, candidate, request.provider); err != nil {
			return err
		}
	}

	return run.applySelection(ctx, request.selection)
}

func wantedProviderArtwork(candidates []artwork.Candidate, locked map[artwork.Kind]bool) (map[artworkKey]artwork.Candidate, error) {
	wanted := map[artworkKey]artwork.Candidate{}

	for _, candidate := range candidates {
		dataPath := candidate.Location.DataPath
		if candidate.Source != artwork.SourceProvider || !candidate.Kind.IsValid() || candidate.Size < 0 ||
			dataPath == "" || !isLocalDataPath(dataPath) {
			return nil, fmt.Errorf("provider artwork (kind %q, source %q): %w", candidate.Kind, candidate.Source, domain.ErrInvalid)
		}

		if !locked[candidate.Kind] {
			wanted[candidateKey(candidate)] = candidate
		}
	}

	return wanted, nil
}

// reconcileProviderRows maps every stored row to its key, deletes the provider rows of
// unlocked kinds that are not wanted any more and refreshes the file data of the others.
func (run artworkSave) reconcileProviderRows(ctx context.Context, wanted map[artworkKey]artwork.Candidate) error {
	rows, err := run.queries.ListItemArtwork(ctx, run.itemID)
	if err != nil {
		return fmt.Errorf("read artwork: %w", err)
	}

	for _, row := range rows {
		key, known := storedArtworkKey(row)
		if !known {
			continue
		}

		run.ids[key] = row.ID

		if key.source != artwork.SourceProvider || run.locked[key.kind] {
			continue
		}

		if err := run.reconcileProviderRow(ctx, row.ID, key, wanted); err != nil {
			return err
		}
	}

	return nil
}

func (run artworkSave) reconcileProviderRow(ctx context.Context, rowID string, key artworkKey, wanted map[artworkKey]artwork.Candidate) error {
	candidate, keep := wanted[key]
	if !keep {
		delete(run.ids, key)

		if err := run.queries.DeleteArtwork(ctx, rowID); err != nil {
			return fmt.Errorf("delete artwork: %w", err)
		}

		return nil
	}

	err := run.queries.UpdateArtworkFile(ctx, sqlcgen.UpdateArtworkFileParams{
		SizeBytes: candidate.Size, ModifiedAt: candidate.ModifiedAt.UnixMilli(), Tag: candidate.Tag(), ID: rowID,
	})
	if err != nil {
		return fmt.Errorf("update artwork: %w", err)
	}

	return nil
}

func (run artworkSave) insertProvider(ctx context.Context, key artworkKey, candidate artwork.Candidate, provider string) error {
	dataPath := candidate.Location.DataPath
	params := sqlcgen.InsertProviderArtworkParams{
		ID: uuid.NewV4().String(), ItemID: run.itemID, Kind: candidate.Kind.String(),
		DataPath:   nullString(dataPath),
		Provider:   nullString(provider),
		RemoteUrl:  nullString(dataPath),
		SizeBytes:  candidate.Size,
		ModifiedAt: candidate.ModifiedAt.UnixMilli(),
		Tag:        candidate.Tag(),
		CreatedAt:  run.now.UnixMilli(),
	}

	if err := run.queries.InsertProviderArtwork(ctx, params); err != nil {
		return fmt.Errorf("insert provider artwork: %w", uniqueAsConflict(err))
	}

	run.ids[key] = params.ID

	return nil
}
