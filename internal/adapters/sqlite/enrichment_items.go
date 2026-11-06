package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"

	"github.com/magpeek/mivo/internal/adapters/sqlite/sqlcgen"
	"github.com/magpeek/mivo/internal/app/enrich"
	"github.com/magpeek/mivo/internal/app/identify"
	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/library"
)

// enqueueChunk bounds the IDs of one ListEnqueueableItems call, well below SQLite's limit of
// bound parameters.
const enqueueChunk = 500

// Compile-time checks: the identify use cases and the queue read and queue items here.
var (
	_ identify.ShowChildren           = (*EnrichmentStore)(nil)
	_ identify.LibraryItems           = (*EnrichmentStore)(nil)
	_ enrich.BatchEnqueuer            = (*EnrichmentStore)(nil)
	_ enrich.FailedJobRetrier         = (*EnrichmentStore)(nil)
	_ identify.ProviderArtworkRemover = (*EnrichmentStore)(nil)
)

// SeasonsAndEpisodes returns the item IDs of the seasons and episodes of a show
// (identify.ShowChildren).
func (store *EnrichmentStore) SeasonsAndEpisodes(ctx context.Context, showID string) ([]string, error) {
	ids, err := sqlcgen.New(store.reader).ListShowDescendants(ctx, sql.NullString{String: showID, Valid: true})
	if err != nil {
		return nil, fmt.Errorf("list seasons and episodes of show %q: %w", showID, err)
	}

	return ids, nil
}

// LibraryOnlineEnabled returns the online_metadata switch of a library
// (identify.LibraryItems). The error wraps domain.ErrNotFound for an unknown library.
func (store *EnrichmentStore) LibraryOnlineEnabled(ctx context.Context, libraryID string) (bool, error) {
	enabled, err := sqlcgen.New(store.reader).GetLibraryOnlineMetadata(ctx, libraryID)
	if errors.Is(err, sql.ErrNoRows) {
		return false, fmt.Errorf("read online metadata switch of library %q: %w", libraryID, domain.ErrNotFound)
	}

	if err != nil {
		return false, fmt.Errorf("read online metadata switch of library %q: %w", libraryID, err)
	}

	return enabled == storedTrue, nil
}

// MatchableItems returns the items of a library a provider can match whose match is not
// disabled (identify.LibraryItems).
func (store *EnrichmentStore) MatchableItems(ctx context.Context, libraryID string) ([]string, error) {
	ids, err := sqlcgen.New(store.reader).ListMatchableItems(ctx, libraryID)
	if err != nil {
		return nil, fmt.Errorf("list matchable items of library %q: %w", libraryID, err)
	}

	return ids, nil
}

// EnqueueItems queues those of itemIDs that are of one of kinds, belong to a library with
// online metadata and whose match is not disabled, due now (enrich.BatchEnqueuer). Unknown
// IDs are skipped. It returns how many it queued.
func (store *EnrichmentStore) EnqueueItems(ctx context.Context, itemIDs []string, kinds []library.ItemKind,
	reason enrich.Reason,
) (int, error) {
	if !reason.IsValid() {
		return 0, fmt.Errorf("enqueue items: unknown reason %q: %w", reason, domain.ErrInvalid)
	}

	batch := enqueueBatch{kinds: kinds, reason: reason, now: store.clock().UnixMilli()}
	queued := 0

	err := inWriteTransaction(ctx, store.writer, func(queries *sqlcgen.Queries) error {
		for chunk := range slices.Chunk(itemIDs, enqueueChunk) {
			count, err := batch.enqueue(ctx, queries, chunk)
			if err != nil {
				return err
			}

			queued += count
		}

		return nil
	})
	if err != nil {
		return 0, fmt.Errorf("enqueue items: %w", err)
	}

	return queued, nil
}

// enqueueBatch is one EnqueueItems call.
type enqueueBatch struct {
	kinds  []library.ItemKind
	reason enrich.Reason
	now    int64
}

// enqueue queues the enqueueable items of one chunk of IDs and returns how many.
func (batch enqueueBatch) enqueue(ctx context.Context, queries *sqlcgen.Queries, itemIDs []string) (int, error) {
	rows, err := queries.ListEnqueueableItems(ctx, itemIDs)
	if err != nil {
		return 0, fmt.Errorf("list enqueueable items: %w", err)
	}

	queued := 0

	for _, row := range rows {
		if !slices.Contains(batch.kinds, library.ItemKind(row.Kind)) {
			continue
		}

		err := queries.EnqueueEnrichment(ctx, sqlcgen.EnqueueEnrichmentParams{
			ItemID: row.ID, Reason: string(batch.reason), NotBefore: batch.now, EnqueuedAt: batch.now,
		})
		if err != nil {
			return queued, fmt.Errorf("enqueue item %q: %w", row.ID, err)
		}

		queued++
	}

	return queued, nil
}

// RetryFailedJobs makes every failed job that is not running due now (enrich.
// FailedJobRetrier) and returns how many it changed.
func (store *EnrichmentStore) RetryFailedJobs(ctx context.Context) (int, error) {
	now := store.clock().UnixMilli()

	changed, err := sqlcgen.New(store.writer).RetryFailedEnrichment(ctx, sqlcgen.RetryFailedEnrichmentParams{
		NotBefore: now, NotBefore_2: now,
	})
	if err != nil {
		return 0, fmt.Errorf("retry failed enrichment jobs: %w", err)
	}

	return int(changed), nil
}
