package sqlite

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/magpeek/mivo/internal/adapters/sqlite/sqlcgen"
	appwatch "github.com/magpeek/mivo/internal/app/watch"
	"github.com/magpeek/mivo/internal/domain/access"
	"github.com/magpeek/mivo/internal/domain/watch"
)

// WatchRowQuery selects a profile's Continue watching or Up next row.
type WatchRowQuery struct {
	ProfileID string
	// LibraryID limits the row to one library; empty means all libraries.
	LibraryID string
	// PlayedSince is the start of the row's window (now minus watch.ContinueWatchingWindow).
	PlayedSince time.Time
	// Limit is the most item IDs returned; 0 or less returns none.
	Limit int
	// Restriction is what the viewer may see: Up next picks only the next episode among the
	// episodes it allows. Its zero value allows nothing. Continue watching leaves the filter to
	// the restricted summary reader.
	Restriction access.Restriction
}

// States returns the profile's stored states of the items; an item without a row is absent
// from the map. The batches of a long list are read in one transaction, so they share a
// snapshot.
func (store *WatchStore) States(ctx context.Context, profileID string, itemIDs []string) (map[string]watch.State, error) {
	states := make(map[string]watch.State, len(itemIDs))

	err := inReadTransaction(ctx, store.reader, func(queries *sqlcgen.Queries) error {
		for batch := range slices.Chunk(itemIDs, watchBatchSize) {
			rows, err := readRows(ctx, queries, profileID, batch)
			if err != nil {
				return err
			}

			for itemID, row := range rows {
				states[itemID] = row.state()
			}
		}

		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("read watch states of profile %s: %w", profileID, err)
	}

	return states, nil
}

// EpisodeCounts returns per show or season the episodes below it (as ItemSummary.LeafCount
// and ChildCount count them; a show's specials included) and how many of them the profile
// has not watched; a container without episodes, an unknown ID and an item of another kind
// are absent.
//
// Only episodes the restriction allows are counted: the content limit applies to the stored
// visibility of each episode, so an episode hidden by its own or an ancestor's rating is neither
// counted nor unwatched (ADR 0032 section 7). The library is the caller's check: it passes only
// containers the viewer can see, and a restriction that allows no library counts nothing.
func (store *WatchStore) EpisodeCounts(
	ctx context.Context, profileID string, containerIDs []string, restriction access.Restriction,
) (map[string]appwatch.EpisodeCount, error) {
	counts := make(map[string]appwatch.EpisodeCount, len(containerIDs))

	if restriction.DeniesAll() {
		return counts, nil
	}

	content := contentLimitOf(restriction)

	err := inReadTransaction(ctx, store.reader, func(queries *sqlcgen.Queries) error {
		for batch := range slices.Chunk(containerIDs, watchBatchSize) {
			params := sqlcgen.CountSeasonEpisodesParams{
				ProfileID: profileID, ContainerIds: batch, MaxAge: content.maxAge, AllowUnrated: content.allowUnrated,
			}
			if err := countBatch(ctx, queries, params, counts); err != nil {
				return err
			}
		}

		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("count episodes for profile %s: %w", profileID, err)
	}

	return counts, nil
}

// countBatch adds the counts of the containers of params (at most watchBatchSize) to counts.
func countBatch(ctx context.Context, queries *sqlcgen.Queries, params sqlcgen.CountSeasonEpisodesParams, counts map[string]appwatch.EpisodeCount) error {
	seasons, err := queries.CountSeasonEpisodes(ctx, params)
	if err != nil {
		return fmt.Errorf("count season episodes: %w", err)
	}

	for _, row := range seasons {
		counts[row.ContainerID] = appwatch.EpisodeCount{Total: int(row.Total), Unwatched: int(row.Unwatched)}
	}

	shows, err := queries.CountShowEpisodes(ctx, sqlcgen.CountShowEpisodesParams(params))
	if err != nil {
		return fmt.Errorf("count show episodes: %w", err)
	}

	for _, row := range shows {
		counts[row.ContainerID] = appwatch.EpisodeCount{Total: int(row.Total), Unwatched: int(row.Unwatched)}
	}

	return nil
}

// ContinueWatching returns the item IDs of the profile's Continue watching row (HO-03) in row
// order: items in progress, not hidden, played at or after query.PlayedSince, newest first;
// of a show only its most recently played episode. Rows of items that no longer exist are
// skipped. Restrictions are not applied here: the caller loads the items through the
// restricted summary reader (M7-13), which drops what the viewer may not see.
func (store *WatchStore) ContinueWatching(ctx context.Context, query WatchRowQuery) ([]string, error) {
	if query.Limit <= 0 {
		return []string{}, nil
	}

	itemIDs, err := sqlcgen.New(store.reader).ListContinueWatching(ctx, sqlcgen.ListContinueWatchingParams{
		ProfileID: query.ProfileID, LibraryID: query.LibraryID,
		PlayedSince: query.PlayedSince.UnixMilli(), RowLimit: int64(query.Limit),
	})
	if err != nil {
		return nil, fmt.Errorf("list continue watching of profile %s: %w", query.ProfileID, err)
	}

	return itemIDs, nil
}
