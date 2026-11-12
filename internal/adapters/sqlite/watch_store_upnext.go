package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"slices"

	"github.com/magpeek/mivo/internal/adapters/sqlite/sqlcgen"
	"github.com/magpeek/mivo/internal/domain/watch"
)

// upNextShowBatch is how many candidate shows Up next reads the episodes of at once: about
// one row's worth, so a typical row needs one episode query and finished shows only cost
// the next batch.
const upNextShowBatch = watch.UpNextLimit

// UpNext returns the episode IDs of the profile's Up next row (HO-04) in row order: per show
// with an episode watched at or after query.PlayedSince, the episode watch.NextUp picks;
// shows with an episode in Continue watching are left out, so a show appears in at most one
// of the two rows. Newest show activity first. Rows of items that no longer exist are
// skipped. The episodes are chosen among those query.Restriction allows; the show and its library
// are filtered by the caller, as for ContinueWatching.
//
// SQL finds and orders the candidate shows; the next episode of each comes from the domain
// rule, so the row cannot drift from watch.NextUp. Reads share one snapshot.
func (store *WatchStore) UpNext(ctx context.Context, query WatchRowQuery) ([]string, error) {
	itemIDs := []string{}
	if query.Limit <= 0 || query.Restriction.DeniesAll() {
		return itemIDs, nil
	}

	err := inReadTransaction(ctx, store.reader, func(queries *sqlcgen.Queries) error {
		shows, err := queries.ListUpNextShows(ctx, sqlcgen.ListUpNextShowsParams{
			ProfileID: query.ProfileID, LibraryID: query.LibraryID, PlayedSince: query.PlayedSince.UnixMilli(),
		})
		if err != nil {
			return fmt.Errorf("list up next shows: %w", err)
		}

		for batch := range slices.Chunk(shows, upNextShowBatch) {
			next, err := nextEpisodes(ctx, queries, query, batch)
			if err != nil {
				return err
			}

			if itemIDs = append(itemIDs, next...); len(itemIDs) >= query.Limit {
				itemIDs = itemIDs[:query.Limit]

				break
			}
		}

		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("list up next of profile %s: %w", query.ProfileID, err)
	}

	return itemIDs, nil
}

// nextEpisodes returns the next episode of each show that has one, in the order of showIDs.
func nextEpisodes(ctx context.Context, queries *sqlcgen.Queries, query WatchRowQuery, showIDs []string) ([]string, error) {
	content := contentLimitOf(query.Restriction)
	parentIDs := make([]sql.NullString, len(showIDs))
	for index, showID := range showIDs {
		parentIDs[index] = sql.NullString{String: showID, Valid: true}
	}

	rows, err := queries.ListShowEpisodeStates(ctx, sqlcgen.ListShowEpisodeStatesParams{
		ProfileID: query.ProfileID, ShowIds: parentIDs, MaxAge: content.maxAge, AllowUnrated: content.allowUnrated,
	})
	if err != nil {
		return nil, fmt.Errorf("list show episodes: %w", err)
	}

	episodes := make(map[string][]watch.EpisodeRef, len(showIDs))
	states := make(map[string]watch.State, len(rows))

	for _, row := range rows {
		episodes[row.ShowID] = append(episodes[row.ShowID], watch.EpisodeRef{
			ItemID: row.ItemID, Season: int(row.SeasonNumber), Episode: int(row.EpisodeNumber), EpisodeEnd: int(row.EpisodeEnd),
		})
		states[row.ItemID] = watch.State{Watched: row.Watched == 1}
	}

	next := make([]string, 0, len(showIDs))
	for _, showID := range showIDs {
		if episode, found := watch.NextUp(episodes[showID], states); found {
			next = append(next, episode.ItemID)
		}
	}

	return next, nil
}
