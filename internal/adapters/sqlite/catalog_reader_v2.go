package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/magpeek/mivo/internal/adapters/sqlite/sqlcgen"
	applibrary "github.com/magpeek/mivo/internal/app/library"
	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/artwork"
	"github.com/magpeek/mivo/internal/domain/library"
)

// ListChildren returns one page of the children of an item: the seasons of a show or the
// episodes of a season in child order, reversed for request.Descending; extras are never
// children. A leaf yields an empty page. An unknown item wraps domain.ErrNotFound, an invalid
// limit and a cursor that was not issued for this item and direction domain.ErrInvalid. The
// request is validated before the item is read, so a bad cursor on an unknown item reports
// domain.ErrInvalid, not domain.ErrNotFound.
func (catalog *CatalogReader) ListChildren(ctx context.Context, itemID string, request applibrary.ChildPageRequest) (applibrary.ItemPage, error) {
	limit, err := library.NormalizeLimit(request.Page.Limit)
	if err != nil {
		return applibrary.ItemPage{}, fmt.Errorf("list children of item %q: %w", itemID, err)
	}

	scope := childCursorScope{parentID: itemID, descending: request.Descending}

	after, hasAfter, err := decodeChildCursor(request.Page.Cursor, scope)
	if err != nil {
		return applibrary.ItemPage{}, fmt.Errorf("list children of item %q: %w", itemID, err)
	}

	var result applibrary.ItemPage

	err = inReadTransaction(ctx, catalog.reader, func(queries *sqlcgen.Queries) error {
		var readErr error

		result, readErr = readChildrenPage(ctx, queries, childrenRequest{
			scope: scope, after: after, hasAfter: hasAfter, limit: limit,
		})

		return readErr
	})
	if err != nil {
		return applibrary.ItemPage{}, fmt.Errorf("list children of item %q: %w", itemID, err)
	}

	return result, nil
}

// listedItem is a row of any listing: the item, its counts and, for children, its position
// in child order.
type listedItem struct {
	item       sqlcgen.Item
	childCount int64
	leafCount  int64
	key        childKey
}

type topLevelRequest struct {
	libraryID string
	after     itemKey
	limit     int
}

// readTopLevelPage reads one row more than the limit to learn whether another page follows.
func readTopLevelPage(ctx context.Context, queries *sqlcgen.Queries, request topLevelRequest) (applibrary.ItemPage, error) {
	found, err := queries.LibraryExists(ctx, request.libraryID)
	if err != nil {
		return applibrary.ItemPage{}, fmt.Errorf("read library: %w", err)
	}

	if !found {
		return applibrary.ItemPage{}, domain.ErrNotFound
	}

	rows, err := queries.ListTopLevelItems(ctx, sqlcgen.ListTopLevelItemsParams{
		LibraryID:      request.libraryID,
		AfterSortTitle: request.after.sortTitle,
		AfterID:        request.after.id,
		RowLimit:       int64(request.limit) + 1,
	})
	if err != nil {
		return applibrary.ItemPage{}, fmt.Errorf("read items: %w", err)
	}

	listed := make([]listedItem, 0, len(rows))
	for _, row := range rows {
		listed = append(listed, listedItem{item: row.Item, childCount: row.ChildCount, leafCount: row.LeafCount})
	}

	return pageOf(ctx, queries, pageSource{rows: listed, limit: request.limit}, func(last listedItem) string {
		return encodeCursor(itemKey{sortTitle: last.item.SortTitle, id: last.item.ID})
	})
}

type childrenRequest struct {
	scope    childCursorScope
	after    childKey
	hasAfter bool
	limit    int
}

// readChildrenPage reads one row more than the limit, like readTopLevelPage.
func readChildrenPage(ctx context.Context, queries *sqlcgen.Queries, request childrenRequest) (applibrary.ItemPage, error) {
	parentID := request.scope.parentID

	parent, err := queries.GetItem(ctx, parentID)
	if errors.Is(err, sql.ErrNoRows) {
		return applibrary.ItemPage{}, domain.ErrNotFound
	}

	if err != nil {
		return applibrary.ItemPage{}, fmt.Errorf("read item: %w", err)
	}

	listed, err := listChildren(ctx, queries, request, int64(request.limit)+1)
	if err != nil {
		return applibrary.ItemPage{}, err
	}

	page := pageSource{rows: listed, limit: request.limit}

	// Only the children of a show are seasons; reading the parent's artwork for episodes
	// would be wasted.
	if parent.Kind == library.ItemKindShow.String() {
		page.fallbackParent = parent
	}

	return pageOf(ctx, queries, page, func(last listedItem) string {
		return encodeChildCursor(request.scope, last.key)
	})
}

// listChildren runs the child query of the requested direction.
func listChildren(ctx context.Context, queries *sqlcgen.Queries, request childrenRequest, rowLimit int64) ([]listedItem, error) {
	parentID := sql.NullString{String: request.scope.parentID, Valid: true}
	after := sqlcgen.ListChildrenAscendingParams{
		ParentID: parentID, HasAfter: storedFlag(request.hasAfter), AfterGroup: request.after.group,
		AfterNumber: request.after.number, AfterSortTitle: request.after.sortTitle, AfterID: request.after.id,
		RowLimit: rowLimit,
	}

	var listed []listedItem

	if request.scope.descending {
		rows, err := queries.ListChildrenDescending(ctx, sqlcgen.ListChildrenDescendingParams(after))
		if err != nil {
			return nil, fmt.Errorf("read children: %w", err)
		}

		for _, row := range rows {
			listed = append(listed, childListedItem(sqlcgen.ListChildrenAscendingRow(row)))
		}

		return listed, nil
	}

	rows, err := queries.ListChildrenAscending(ctx, after)
	if err != nil {
		return nil, fmt.Errorf("read children: %w", err)
	}

	for _, row := range rows {
		listed = append(listed, childListedItem(row))
	}

	return listed, nil
}

// childListedItem takes the row of the ascending query; the descending row has the same shape.
func childListedItem(row sqlcgen.ListChildrenAscendingRow) listedItem {
	return listedItem{
		item: row.Item, childCount: row.ChildCount, leafCount: row.LeafCount,
		key: childKey{group: row.OrderGroup, number: row.OrderNumber, sortTitle: row.Item.SortTitle, id: row.Item.ID},
	}
}

// pageSource is the rows of a listing, one more than limit when another page follows.
// fallbackParent is the parent of a children listing: its poster stands in for a season
// without one.
type pageSource struct {
	rows           []listedItem
	limit          int
	fallbackParent sqlcgen.Item
}

// pageOf cuts the extra row, builds the summaries and the cursor behind the last row.
func pageOf(ctx context.Context, queries *sqlcgen.Queries, source pageSource, cursorAfter func(listedItem) string) (applibrary.ItemPage, error) {
	hasMore := len(source.rows) > source.limit
	rows := source.rows[:min(len(source.rows), source.limit)]

	items, err := summariesOf(ctx, queries, rows, source.fallbackParent)
	if err != nil {
		return applibrary.ItemPage{}, err
	}

	page := applibrary.ItemPage{Items: items}
	if hasMore {
		page.NextCursor = cursorAfter(rows[len(rows)-1])
	}

	return page, nil
}

// summariesOf builds the summaries of listed items with three queries whatever the page size:
// the primary files with their streams, and the selected artwork. A season without a poster
// shows the poster of fallbackParent, its show.
func summariesOf(ctx context.Context, queries *sqlcgen.Queries, listed []listedItem, fallbackParent sqlcgen.Item) ([]applibrary.ItemSummary, error) {
	itemIDs := make([]string, 0, len(listed)+1)
	for _, entry := range listed {
		itemIDs = append(itemIDs, entry.item.ID)
	}

	if fallbackParent.ID != "" {
		itemIDs = append(itemIDs, fallbackParent.ID)
	}

	primaryFiles, err := primaryFilesByItem(ctx, queries, itemIDs)
	if err != nil {
		return nil, err
	}

	artworkByItem, err := selectedArtworkByItem(ctx, queries, itemIDs)
	if err != nil {
		return nil, err
	}

	summaries := make([]applibrary.ItemSummary, 0, len(listed))

	for _, entry := range listed {
		item, err := itemFromRow(entry.item)
		if err != nil {
			return nil, err
		}

		refs := withSeasonFallback(item.Kind, artworkByItem[item.ID], artworkByItem[fallbackParent.ID])

		summaries = append(summaries, applibrary.ItemSummary{
			Item: item, PrimaryFile: primaryFiles[item.ID],
			Poster: refPointer(refs, artwork.KindPoster), Thumb: refPointer(refs, artwork.KindThumb),
			ChildCount: int(entry.childCount), LeafCount: int(entry.leafCount),
		})
	}

	return summaries, nil
}

func refPointer(refs map[artwork.Kind]artwork.Ref, kind artwork.Kind) *artwork.Ref {
	ref, found := refs[kind]
	if !found {
		return nil
	}

	return &ref
}
