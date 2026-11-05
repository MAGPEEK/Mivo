package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"path"
	"slices"
	"strings"

	"github.com/magpeek/mivo/internal/adapters/sqlite/sqlcgen"
	applibrary "github.com/magpeek/mivo/internal/app/library"
	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/library"
)

// ListArtistAlbums returns one page of the albums of an artist ordered by (sort title, ID),
// each with its track count as ChildCount. Limit and cursor work as for ListItems. An unknown
// item wraps domain.ErrNotFound; an item that is no artist, an invalid limit and a malformed
// cursor wrap domain.ErrInvalid.
func (catalog *CatalogReader) ListArtistAlbums(ctx context.Context, artistID string, page library.PageRequest) (applibrary.ItemPage, error) {
	result, err := catalog.readAlbumPage(ctx, page, func(queries *sqlcgen.Queries, request albumPageRequest) ([]listedItem, error) {
		if _, err := itemOfKind(ctx, queries, artistID, library.ItemKindArtist); err != nil {
			return nil, err
		}

		rows, err := queries.ListArtistAlbums(ctx, sqlcgen.ListArtistAlbumsParams{
			ParentID:       sql.NullString{String: artistID, Valid: true},
			AfterSortTitle: request.after.sortTitle, AfterID: request.after.id, RowLimit: request.rowLimit,
		})
		if err != nil {
			return nil, fmt.Errorf("read albums: %w", err)
		}

		listed := make([]listedItem, 0, len(rows))
		for _, row := range rows {
			listed = append(listed, listedItem{item: row.Item, childCount: row.ChildCount})
		}

		return listed, nil
	})
	if err != nil {
		return applibrary.ItemPage{}, fmt.Errorf("list albums of artist %q: %w", artistID, err)
	}

	return result, nil
}

// ListLibraryAlbums returns one page of every album of a library ordered by (sort title, ID),
// like ListArtistAlbums. A library without albums yields an empty page; an unknown library
// wraps domain.ErrNotFound.
func (catalog *CatalogReader) ListLibraryAlbums(ctx context.Context, libraryID string, page library.PageRequest) (applibrary.ItemPage, error) {
	result, err := catalog.readAlbumPage(ctx, page, func(queries *sqlcgen.Queries, request albumPageRequest) ([]listedItem, error) {
		found, err := queries.LibraryExists(ctx, libraryID)
		if err != nil {
			return nil, fmt.Errorf("read library: %w", err)
		}

		if !found {
			return nil, domain.ErrNotFound
		}

		rows, err := queries.ListLibraryAlbums(ctx, sqlcgen.ListLibraryAlbumsParams{
			LibraryID: libraryID, AfterSortTitle: request.after.sortTitle, AfterID: request.after.id, RowLimit: request.rowLimit,
		})
		if err != nil {
			return nil, fmt.Errorf("read albums: %w", err)
		}

		listed := make([]listedItem, 0, len(rows))
		for _, row := range rows {
			listed = append(listed, listedItem{item: row.Item, childCount: row.ChildCount})
		}

		return listed, nil
	})
	if err != nil {
		return applibrary.ItemPage{}, fmt.Errorf("list albums of library %q: %w", libraryID, err)
	}

	return result, nil
}

// albumPageRequest is the keyset position and the row limit (one more than the page) of an
// album listing.
type albumPageRequest struct {
	after    itemKey
	rowLimit int64
}

// readAlbumPage validates the page request and runs list in a read transaction; list reads
// one row more than the limit, so pageOf learns whether another page follows.
func (catalog *CatalogReader) readAlbumPage(
	ctx context.Context, page library.PageRequest,
	list func(*sqlcgen.Queries, albumPageRequest) ([]listedItem, error),
) (applibrary.ItemPage, error) {
	limit, err := library.NormalizeLimit(page.Limit)
	if err != nil {
		return applibrary.ItemPage{}, err
	}

	after, err := decodeCursor(page.Cursor)
	if err != nil {
		return applibrary.ItemPage{}, err
	}

	var result applibrary.ItemPage

	err = inReadTransaction(ctx, catalog.reader, func(queries *sqlcgen.Queries) error {
		listed, listErr := list(queries, albumPageRequest{after: after, rowLimit: int64(limit) + 1})
		if listErr != nil {
			return listErr
		}

		result, listErr = pageOf(ctx, queries, pageSource{rows: listed, limit: limit}, func(last listedItem) string {
			return encodeCursor(itemKey{sortTitle: last.item.SortTitle, id: last.item.ID})
		})

		return listErr
	})
	if err != nil {
		return applibrary.ItemPage{}, err
	}

	return result, nil
}

// ListAlbumTracks returns one page of the tracks of an album ordered by library.CompareTracks
// (disc, track number with unnumbered tracks last, file name of the primary file), then ID.
// A track without stored disc counts as disc 1. Limit and cursor work as for ListItems; a
// cursor is bound to its album. An unknown item wraps domain.ErrNotFound; an item that is no
// album, an invalid limit and a cursor not issued for this album wrap domain.ErrInvalid.
func (catalog *CatalogReader) ListAlbumTracks(ctx context.Context, albumID string, page library.PageRequest) (applibrary.ItemPage, error) {
	limit, err := library.NormalizeLimit(page.Limit)
	if err != nil {
		return applibrary.ItemPage{}, fmt.Errorf("list tracks of album %q: %w", albumID, err)
	}

	scope := childCursorScope{parentID: albumID}

	after, hasAfter, err := decodeTrackCursor(page.Cursor, scope)
	if err != nil {
		return applibrary.ItemPage{}, fmt.Errorf("list tracks of album %q: %w", albumID, err)
	}

	var result applibrary.ItemPage

	err = inReadTransaction(ctx, catalog.reader, func(queries *sqlcgen.Queries) error {
		if _, err := itemOfKind(ctx, queries, albumID, library.ItemKindAlbum); err != nil {
			return err
		}

		var readErr error

		result, readErr = readTrackPage(ctx, queries, trackPageRequest{scope: scope, after: after, hasAfter: hasAfter, limit: limit})

		return readErr
	})
	if err != nil {
		return applibrary.ItemPage{}, fmt.Errorf("list tracks of album %q: %w", albumID, err)
	}

	return result, nil
}

// trackPageRequest asks for one page of the tracks of the album scope.parentID in the
// direction of scope.
type trackPageRequest struct {
	scope    childCursorScope
	after    trackPosition
	hasAfter bool
	limit    int
}

// readTrackPage reads every track of a known album, sorts them in Go (the order needs the
// file name, which SQL cannot cut from the path portably) and pages the sorted list,
// reversed for a descending scope. Every page pays for the whole album: usually a few dozen
// tracks, but the "Unknown Album" of untagged files in the root can hold thousands (keyset
// paging on a stored sort key: BACKLOG).
func readTrackPage(ctx context.Context, queries *sqlcgen.Queries, request trackPageRequest) (applibrary.ItemPage, error) {
	rows, err := queries.ListAlbumTracks(ctx, sql.NullString{String: request.scope.parentID, Valid: true})
	if err != nil {
		return applibrary.ItemPage{}, fmt.Errorf("read tracks: %w", err)
	}

	compare := compareTrackPositions
	if request.scope.descending {
		compare = func(left, right trackPosition) int { return compareTrackPositions(right, left) }
	}

	positions := make(map[string]trackPosition, len(rows))
	listed := make([]listedItem, 0, len(rows))

	for _, row := range rows {
		position := trackPositionOf(row)
		if request.hasAfter && compare(position, request.after) <= 0 {
			continue
		}

		positions[row.Item.ID] = position
		listed = append(listed, listedItem{item: row.Item})
	}

	slices.SortFunc(listed, func(left, right listedItem) int {
		return compare(positions[left.item.ID], positions[right.item.ID])
	})

	page := pageSource{rows: listed[:min(len(listed), request.limit+1)], limit: request.limit}

	return pageOf(ctx, queries, page, func(last listedItem) string {
		return encodeTrackCursor(request.scope, positions[last.item.ID])
	})
}

// trackPosition is the place of a track in album order.
type trackPosition struct {
	order library.TrackOrder
	id    string
}

func trackPositionOf(row sqlcgen.ListAlbumTracksRow) trackPosition {
	disc := firstDisc
	if row.Item.DiscNumber.Valid {
		disc = int(row.Item.DiscNumber.Int64)
	}

	var track library.OptionalNumber
	if row.Item.Number.Valid {
		track = library.NumberOf(int(row.Item.Number.Int64))
	}

	fileName := ""
	if row.PrimaryPath != "" {
		fileName = path.Base(row.PrimaryPath)
	}

	return trackPosition{order: library.TrackOrder{Disc: disc, Track: track, FileName: fileName}, id: row.Item.ID}
}

// compareTrackPositions orders by library.CompareTracks and then by ID, so no two tracks
// share a position and a cursor names exactly one place.
func compareTrackPositions(left, right trackPosition) int {
	if order := library.CompareTracks(left.order, right.order); order != 0 {
		return order
	}

	return strings.Compare(left.id, right.id)
}
