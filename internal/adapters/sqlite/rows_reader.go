package sqlite

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/magpeek/mivo/internal/adapters/sqlite/sqlcgen"
	applibrary "github.com/magpeek/mivo/internal/app/library"
	approws "github.com/magpeek/mivo/internal/app/rows"
	"github.com/magpeek/mivo/internal/domain/access"
	"github.com/magpeek/mivo/internal/domain/rows"
)

// The reader also serves the Home and Overview rows (task M5-13).
var (
	_ approws.RowReader     = (*CatalogReader)(nil)
	_ approws.SummaryReader = (*CatalogReader)(nil)
)

// ListRowItems returns the first items of a row of top-level items (see
// approws.RowReader.ListRowItems), each with its summary as listings carry it. An unknown
// library wraps domain.ErrNotFound.
func (catalog *CatalogReader) ListRowItems(ctx context.Context, request approws.ItemRequest) ([]applibrary.ItemSummary, error) {
	var summaries []applibrary.ItemSummary

	err := inListingTransaction(ctx, catalog.reader, func(transaction *sql.Tx, queries *sqlcgen.Queries) error {
		target, err := completeTarget(ctx, transaction, listingTarget{
			libraryID: request.LibraryID, kind: request.Query.Filter.Kind,
			restriction: access.Unrestricted(), // TODO(M7-13): the viewer's restriction.
		})
		if err != nil {
			return err
		}

		text, args := rowItemsQuery(target, request)

		keys, err := scanIDs(ctx, transaction, text, args)
		if err != nil {
			return err
		}

		listed, err := listedItemsInOrder(ctx, queries, keys)
		if err != nil {
			return err
		}

		summaries, err = summariesOf(ctx, queries, listed, sqlcgen.Item{})

		return err
	})
	if err != nil {
		return nil, fmt.Errorf("list row items of library %q: %w", request.LibraryID, err)
	}

	return summaries, nil
}

// scanIDs runs a query of item IDs and returns them as listing keys in order.
func scanIDs(ctx context.Context, transaction *sql.Tx, text string, args []any) ([]listingKey, error) {
	result, err := transaction.QueryContext(ctx, text, args...)
	if err != nil {
		return nil, fmt.Errorf("read row item ids: %w", err)
	}
	defer closeRows(result)

	var keys []listingKey

	for result.Next() {
		var key listingKey
		if err := result.Scan(&key.id); err != nil {
			return nil, fmt.Errorf("read row item ids: %w", err)
		}

		keys = append(keys, key)
	}

	if err := result.Err(); err != nil {
		return nil, fmt.Errorf("read row item ids: %w", err)
	}

	return keys, nil
}

// ListEpisodeGroups returns the recently added episodes per season of the shows with the
// newest episodes (see approws.RowReader.ListEpisodeGroups). An unknown library wraps
// domain.ErrNotFound.
func (catalog *CatalogReader) ListEpisodeGroups(ctx context.Context, request approws.EpisodeRequest) ([]rows.EpisodeGroup, error) {
	var groups []rows.EpisodeGroup

	err := catalog.inEpisodeQuery(ctx, request, episodeGroupsQuery, func(scan func(...any) error) error {
		var (
			group  rows.EpisodeGroup
			newest int64
		)

		if err := scan(&group.ShowID, &group.SeasonID, &group.EpisodeCount, &newest); err != nil {
			return err
		}

		group.NewestAdded = timeOfMillis(newest)
		groups = append(groups, group)

		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("list episode groups of library %q: %w", request.LibraryID, err)
	}

	return groups, nil
}

// ListRecentEpisodes returns the episodes with an air date, newest first (see
// approws.RowReader.ListRecentEpisodes). An unknown library wraps domain.ErrNotFound.
func (catalog *CatalogReader) ListRecentEpisodes(ctx context.Context, request approws.EpisodeRequest) ([]approws.EpisodeRef, error) {
	var refs []approws.EpisodeRef

	err := catalog.inEpisodeQuery(ctx, request, recentEpisodesQuery, func(scan func(...any) error) error {
		var ref approws.EpisodeRef
		if err := scan(&ref.EpisodeID, &ref.SeasonID, &ref.ShowID); err != nil {
			return err
		}

		refs = append(refs, ref)

		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("list recent episodes of library %q: %w", request.LibraryID, err)
	}

	return refs, nil
}

// episodeQuery builds the SQL of an episode query for the completed target.
type episodeQuery func(target listingTarget, request approws.EpisodeRequest) (string, []any)

// inEpisodeQuery runs the episode query in a read transaction and hands every row to read.
func (catalog *CatalogReader) inEpisodeQuery(ctx context.Context, request approws.EpisodeRequest, query episodeQuery,
	read func(scan func(...any) error) error,
) error {
	return inListingTransaction(ctx, catalog.reader, func(transaction *sql.Tx, _ *sqlcgen.Queries) error {
		// TODO(M7-13): the viewer's restriction.
		target, err := completeTarget(ctx, transaction, listingTarget{libraryID: request.LibraryID, restriction: access.Unrestricted()})
		if err != nil {
			return err
		}

		text, args := query(target, request)

		result, err := transaction.QueryContext(ctx, text, args...)
		if err != nil {
			return fmt.Errorf("query episodes: %w", err)
		}
		defer closeRows(result)

		for result.Next() {
			if err := read(result.Scan); err != nil {
				return fmt.Errorf("read episode row: %w", err)
			}
		}

		if err := result.Err(); err != nil {
			return fmt.Errorf("read episodes: %w", err)
		}

		return nil
	})
}
