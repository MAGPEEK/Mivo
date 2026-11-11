package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/magpeek/mivo/internal/adapters/sqlite/sqlcgen"
	applibrary "github.com/magpeek/mivo/internal/app/library"
	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/access"
	"github.com/magpeek/mivo/internal/domain/library"
)

// The reader also counts facets and letters for the library listing.
var _ applibrary.ListingReader = (*CatalogReader)(nil)

// ListItems returns one page of a library's top-level items (movies and shows) in the order
// of query.Order, narrowed by query.Filter, each with its summary data (see summariesOf), and
// the number of matching items. page.Limit is normalised by library.NormalizeLimit; page.Cursor
// is empty for the first page and otherwise the NextCursor of the previous page.
//
// An invalid limit, a cursor this reader did not issue for the sort and direction, and a
// seed that differs from the cursor's wrap domain.ErrInvalid; an unknown library wraps
// domain.ErrNotFound.
func (catalog *CatalogReader) ListItems(ctx context.Context, libraryID string, page library.PageRequest, query library.ListingQuery) (applibrary.ItemPage, error) {
	request, scope, err := listingRequestOf(libraryID, page, query)
	if err != nil {
		return applibrary.ItemPage{}, fmt.Errorf("list items of library %q: %w", libraryID, err)
	}

	var result applibrary.ItemPage

	err = inListingTransaction(ctx, catalog.reader, func(transaction *sql.Tx, queries *sqlcgen.Queries) error {
		var readErr error

		result, readErr = readListingPage(ctx, listingSession{transaction: transaction, queries: queries}, request, scope)

		return readErr
	})
	if err != nil {
		return applibrary.ItemPage{}, fmt.Errorf("list items of library %q: %w", libraryID, err)
	}

	return result, nil
}

// listingRequestOf checks limit, cursor and seed. Sort "random" takes the seed from the
// cursor when the request has none; a request seed must match the cursor's.
func listingRequestOf(libraryID string, page library.PageRequest, query library.ListingQuery) (listingRequest, listingCursorScope, error) {
	limit, err := library.NormalizeLimit(page.Limit)
	if err != nil {
		return listingRequest{}, listingCursorScope{}, err
	}

	order := query.Order
	scope := listingCursorScope{sort: order.Key(), descending: order.IsDescending()}

	after, cursorSeed, hasAfter, err := decodeListingCursor(page.Cursor, scope)
	if err != nil {
		return listingRequest{}, listingCursorScope{}, err
	}

	if order.Key() == library.SortByRandom {
		if order.HasSeed && hasAfter && order.Seed != cursorSeed {
			return listingRequest{}, listingCursorScope{}, fmt.Errorf("cursor was issued for another seed: %w", domain.ErrInvalid)
		}

		if !order.HasSeed {
			order.Seed = cursorSeed
		}

		scope.seed = order.Seed
		query.Order = order
	}

	return listingRequest{
		target: listingTarget{libraryID: libraryID, kind: query.Filter.Kind, restriction: query.Restriction},
		query:  query, after: after, hasAfter: hasAfter, rowLimit: limit + 1,
	}, scope, nil
}

// listingSession is the read transaction of a listing: the raw transaction for the queries
// built at run time and the generated queries on it.
type listingSession struct {
	transaction *sql.Tx
	queries     *sqlcgen.Queries
}

// inListingTransaction is inReadTransaction with access to the transaction itself.
func inListingTransaction(ctx context.Context, reader *sql.DB, work func(*sql.Tx, *sqlcgen.Queries) error) error {
	transaction, err := reader.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin read transaction: %w", err)
	}

	return errors.Join(work(transaction, sqlcgen.New(transaction)), endReadTransaction(transaction))
}

// libraryFolderCountSQL reads the folder count of a library; no row means no such library.
const libraryFolderCountSQL = `SELECT (SELECT COUNT(*) FROM library_roots WHERE library_roots.library_id = libraries.id)
FROM libraries WHERE libraries.id = ?`

// completeTarget reads whether the library has several folders (see primaryFileSQL). An
// unknown library wraps domain.ErrNotFound, and so does a library the viewer may not see: its
// existence must not leak (ADR 0032 §7), and the answer needs no database read.
func completeTarget(ctx context.Context, transaction *sql.Tx, target listingTarget) (listingTarget, error) {
	if !libraryAllowed(target.restriction, target.libraryID) {
		return listingTarget{}, domain.ErrNotFound
	}

	var folders int

	err := transaction.QueryRowContext(ctx, libraryFolderCountSQL, target.libraryID).Scan(&folders)
	if errors.Is(err, sql.ErrNoRows) {
		return listingTarget{}, domain.ErrNotFound
	}

	if err != nil {
		return listingTarget{}, fmt.Errorf("read library: %w", err)
	}

	target.multiFolder = folders > 1

	return target, nil
}

// readListingPage reads one row more than the limit to learn whether another page follows.
func readListingPage(ctx context.Context, session listingSession, request listingRequest, scope listingCursorScope) (applibrary.ItemPage, error) {
	target, err := completeTarget(ctx, session.transaction, request.target)
	if err != nil {
		return applibrary.ItemPage{}, err
	}

	request.target = target

	keys, total, err := readListingKeys(ctx, session.transaction, request)
	if err != nil {
		return applibrary.ItemPage{}, err
	}

	listed, err := listedItemsInOrder(ctx, session, keys, target.restriction)
	if err != nil {
		return applibrary.ItemPage{}, err
	}

	keyByID := make(map[string]listingKey, len(keys))
	for _, key := range keys {
		keyByID[key.id] = key
	}

	page, err := pageOf(ctx, session.queries, pageSource{rows: listed, limit: request.rowLimit - 1}, func(last listedItem) string {
		return encodeListingCursor(scope, keyByID[last.item.ID])
	})
	if err != nil {
		return applibrary.ItemPage{}, err
	}

	if total < 0 {
		total, err = countListing(ctx, session.transaction, request)
		if err != nil {
			return applibrary.ItemPage{}, err
		}
	}

	page.TotalCount, page.Counted = total, true

	return page, nil
}

// listedItemsInOrder reads the items of the keys that the restriction allows, with their
// counts of visible children and episodes, in the order of the keys.
func listedItemsInOrder(ctx context.Context, session listingSession, keys []listingKey, restriction access.Restriction) ([]listedItem, error) {
	if len(keys) == 0 || restriction.DeniesAll() {
		return nil, nil
	}

	itemIDs := make([]string, 0, len(keys))
	for _, key := range keys {
		itemIDs = append(itemIDs, key.id)
	}

	rows, err := session.queries.ListItemsWithCounts(ctx, itemIDs)
	if err != nil {
		return nil, fmt.Errorf("read listed items: %w", err)
	}

	byID := make(map[string]listedItem, len(rows))
	for _, row := range rows {
		byID[row.Item.ID] = listedItem{item: row.Item, childCount: row.ChildCount, leafCount: row.LeafCount}
	}

	if byID, err = restrictListedItems(ctx, session.transaction, byID, restriction); err != nil {
		return nil, err
	}

	listed := make([]listedItem, 0, len(keys))

	for _, id := range itemIDs {
		// The keys and the rows come from one snapshot, so every key has its row.
		if entry, found := byID[id]; found {
			listed = append(listed, entry)
		}
	}

	return listed, nil
}

func countListing(ctx context.Context, transaction *sql.Tx, request listingRequest) (int64, error) {
	text, args := countQuery(request.target, request.query.Filter)

	var count int64
	if err := transaction.QueryRowContext(ctx, text, args...).Scan(&count); err != nil {
		return 0, fmt.Errorf("count listing: %w", err)
	}

	return count, nil
}

// readListingKeys reads the keys of a page and, when the page query counts too, the total;
// the total is -1 when the caller has to count separately (see pageQuery).
func readListingKeys(ctx context.Context, transaction *sql.Tx, request listingRequest) ([]listingKey, int64, error) {
	text, args, counted := pageQuery(request)

	rows, err := transaction.QueryContext(ctx, text, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("read listing: %w", err)
	}

	defer closeRows(rows)

	var keys []listingKey

	total := int64(-1)

	for rows.Next() {
		var key listingKey

		destinations := []any{&key.id, &key.sortTitle, &key.value}
		if counted {
			destinations = append(destinations, &total)
		}

		if err := rows.Scan(destinations...); err != nil {
			return nil, 0, fmt.Errorf("read listing row: %w", err)
		}

		if request.query.Order.Key() == library.SortByTitle {
			key.value = nil
		}

		keys = append(keys, key)
	}

	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("read listing rows: %w", err)
	}

	return keys, total, nil
}

func closeRows(rows *sql.Rows) {
	_ = rows.Close()
}
