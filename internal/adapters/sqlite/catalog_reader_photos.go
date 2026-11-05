package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/magpeek/mivo/internal/adapters/sqlite/sqlcgen"
	applibrary "github.com/magpeek/mivo/internal/app/library"
	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/library"
	"github.com/magpeek/mivo/internal/domain/photo"
)

var _ applibrary.PhotoReader = (*CatalogReader)(nil)

// ListPhotoChildren returns one page of the children of a photo album: its sub-albums by sort
// title, then its photos by capture time and lower-case file name, ties by item ID. Limit
// works as for ListItems; a cursor is bound to its album. An unknown item wraps
// domain.ErrNotFound; an item that is no photo album, an invalid limit and a cursor not
// issued for this album wrap domain.ErrInvalid.
func (catalog *CatalogReader) ListPhotoChildren(
	ctx context.Context, albumID string, page library.PageRequest,
) (applibrary.PhotoChildPage, error) {
	limit, err := library.NormalizeLimit(page.Limit)
	if err != nil {
		return applibrary.PhotoChildPage{}, fmt.Errorf("list children of photo album %q: %w", albumID, err)
	}

	after, err := decodePhotoCursor(page.Cursor, albumID)
	if err != nil {
		return applibrary.PhotoChildPage{}, fmt.Errorf("list children of photo album %q: %w", albumID, err)
	}

	var result applibrary.PhotoChildPage

	err = inReadTransaction(ctx, catalog.reader, func(queries *sqlcgen.Queries) error {
		var readErr error

		result, readErr = readPhotoChildren(ctx, queries, photoChildrenRequest{albumID: albumID, after: after, limit: limit})

		return readErr
	})
	if err != nil {
		return applibrary.PhotoChildPage{}, fmt.Errorf("list children of photo album %q: %w", albumID, err)
	}

	return result, nil
}

type photoChildrenRequest struct {
	albumID string
	after   photoPosition
	limit   int
}

// listedPhotoChild is one child of a page with its place in album order.
type listedPhotoChild struct {
	child    applibrary.PhotoChild
	position photoPosition
}

// readPhotoChildren reads one row more than the limit, sub-albums first, so it learns whether
// another page follows.
func readPhotoChildren(ctx context.Context, queries *sqlcgen.Queries, request photoChildrenRequest) (applibrary.PhotoChildPage, error) {
	if _, err := itemOfKind(ctx, queries, request.albumID, library.ItemKindPhotoAlbum); err != nil {
		return applibrary.PhotoChildPage{}, err
	}

	rowLimit := request.limit + 1

	var listed []listedPhotoChild

	if !request.after.photo {
		albums, err := listSubAlbums(ctx, queries, request, rowLimit)
		if err != nil {
			return applibrary.PhotoChildPage{}, err
		}

		listed = albums
	}

	if remaining := rowLimit - len(listed); remaining > 0 {
		photos, err := listAlbumPhotos(ctx, queries, request, remaining)
		if err != nil {
			return applibrary.PhotoChildPage{}, err
		}

		listed = append(listed, photos...)
	}

	return photoPageOf(request, listed), nil
}

func photoPageOf(request photoChildrenRequest, listed []listedPhotoChild) applibrary.PhotoChildPage {
	rows := listed[:min(len(listed), request.limit)]
	page := applibrary.PhotoChildPage{Children: make([]applibrary.PhotoChild, 0, len(rows))}

	for _, entry := range rows {
		page.Children = append(page.Children, entry.child)
	}

	if len(listed) > request.limit {
		page.NextCursor = encodePhotoCursor(request.albumID, rows[len(rows)-1].position)
	}

	return page
}

func listSubAlbums(ctx context.Context, queries *sqlcgen.Queries, request photoChildrenRequest, rowLimit int) ([]listedPhotoChild, error) {
	rows, err := queries.ListPhotoSubAlbums(ctx, sqlcgen.ListPhotoSubAlbumsParams{
		ParentID:       sql.NullString{String: request.albumID, Valid: true},
		AfterSortTitle: request.after.sortTitle, AfterID: request.after.id, RowLimit: int64(rowLimit),
	})
	if err != nil {
		return nil, fmt.Errorf("read sub-albums: %w", err)
	}

	listed := make([]listedPhotoChild, 0, len(rows))

	for _, row := range rows {
		item, err := itemFromRow(row.Item)
		if err != nil {
			return nil, err
		}

		totals, err := queries.GetPhotoAlbumTotals(ctx, item.ID)
		if err != nil {
			return nil, fmt.Errorf("read totals of album %q: %w", item.ID, err)
		}

		listed = append(listed, listedPhotoChild{
			child: applibrary.PhotoChild{
				Item: item, ChildCount: int(row.ChildCount), LeafCount: int(totals.LeafCount), CoverPhotoID: totals.CoverPhotoID,
			},
			position: photoPosition{id: item.ID, sortTitle: row.Item.SortTitle},
		})
	}

	return listed, nil
}

func listAlbumPhotos(ctx context.Context, queries *sqlcgen.Queries, request photoChildrenRequest, rowLimit int) ([]listedPhotoChild, error) {
	params := sqlcgen.ListAlbumPhotosParams{
		ParentID: sql.NullString{String: request.albumID, Valid: true}, AfterTakenAt: math.MinInt64, RowLimit: int64(rowLimit),
	}
	if after := request.after; after.photo {
		params.AfterTakenAt, params.AfterNameKey, params.AfterID = after.takenAt, after.nameKey, after.id
	}

	rows, err := queries.ListAlbumPhotos(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("read photos: %w", err)
	}

	listed := make([]listedPhotoChild, 0, len(rows))

	for _, row := range rows {
		entry, err := listedPhotoOf(row.Item, row.PhotoInfo)
		if err != nil {
			return nil, err
		}

		listed = append(listed, entry)
	}

	return listed, nil
}

// listedPhotoOf converts the item and photo_info row of a photo.
func listedPhotoOf(itemRow sqlcgen.Item, infoRow sqlcgen.PhotoInfo) (listedPhotoChild, error) {
	item, err := itemFromRow(itemRow)
	if err != nil {
		return listedPhotoChild{}, err
	}

	detail, err := photoDetailFromRow(infoRow)
	if err != nil {
		return listedPhotoChild{}, err
	}

	return listedPhotoChild{
		child:    applibrary.PhotoChild{Item: item, Photo: &detail},
		position: photoPosition{photo: true, id: item.ID, takenAt: infoRow.TakenAt, nameKey: infoRow.NameKey},
	}, nil
}

// libraryCursorScope binds the cursors of a library-wide photo listing to the library; the
// prefix keeps them apart from the cursors of an album.
const libraryCursorScope = "library:"

// ListLibraryPhotos returns one page of every photo of a library, whatever album it is in,
// by capture time and lower-case file name, ties by item ID (the timeline). Limit works as
// for ListItems; a cursor is bound to its library. An unknown library yields an empty page;
// an invalid limit and a cursor not issued for this library wrap domain.ErrInvalid.
func (catalog *CatalogReader) ListLibraryPhotos(
	ctx context.Context, libraryID string, page library.PageRequest,
) (applibrary.PhotoChildPage, error) {
	limit, err := library.NormalizeLimit(page.Limit)
	if err != nil {
		return applibrary.PhotoChildPage{}, fmt.Errorf("list photos of library %q: %w", libraryID, err)
	}

	scope := libraryCursorScope + libraryID

	after, err := decodePhotoCursor(page.Cursor, scope)
	if err == nil && page.Cursor != "" && !after.photo {
		err = fmt.Errorf("album cursor for a photo listing: %w", domain.ErrInvalid)
	}

	if err != nil {
		return applibrary.PhotoChildPage{}, fmt.Errorf("list photos of library %q: %w", libraryID, err)
	}

	params := sqlcgen.ListLibraryPhotosParams{LibraryID: libraryID, AfterTakenAt: math.MinInt64, RowLimit: int64(limit) + 1}
	if after.photo {
		params.AfterTakenAt, params.AfterNameKey, params.AfterID = after.takenAt, after.nameKey, after.id
	}

	rows, err := sqlcgen.New(catalog.reader).ListLibraryPhotos(ctx, params)
	if err != nil {
		return applibrary.PhotoChildPage{}, fmt.Errorf("list photos of library %q: %w", libraryID, err)
	}

	listed := make([]listedPhotoChild, 0, len(rows))

	for _, row := range rows {
		entry, err := listedPhotoOf(row.Item, row.PhotoInfo)
		if err != nil {
			return applibrary.PhotoChildPage{}, fmt.Errorf("list photos of library %q: %w", libraryID, err)
		}

		listed = append(listed, entry)
	}

	return photoPageOf(photoChildrenRequest{albumID: scope, limit: limit}, listed), nil
}

// PhotoDetail returns the photo block of a photo item. An unknown item, and a photo the
// photo writer has not stored yet, wrap domain.ErrNotFound; an item that is no photo wraps
// domain.ErrInvalid.
func (catalog *CatalogReader) PhotoDetail(ctx context.Context, itemID string) (applibrary.PhotoDetail, error) {
	var detail applibrary.PhotoDetail

	err := inReadTransaction(ctx, catalog.reader, func(queries *sqlcgen.Queries) error {
		if _, err := itemOfKind(ctx, queries, itemID, library.ItemKindPhoto); err != nil {
			return err
		}

		row, err := queries.GetPhotoInfo(ctx, itemID)
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("photo info: %w", domain.ErrNotFound)
		}

		if err != nil {
			return fmt.Errorf("read photo info: %w", err)
		}

		detail, err = photoDetailFromRow(row)

		return err
	})
	if err != nil {
		return applibrary.PhotoDetail{}, fmt.Errorf("read photo %q: %w", itemID, err)
	}

	return detail, nil
}

func photoDetailFromRow(row sqlcgen.PhotoInfo) (applibrary.PhotoDetail, error) {
	state, err := photo.ParseThumbnailState(row.ThumbnailState)
	if err != nil {
		return applibrary.PhotoDetail{}, undecodable(fmt.Sprintf("photo %q: stored thumbnail state", row.ItemID), err)
	}

	source, err := photo.ParseTakenAtSource(row.TakenAtSource)
	if err != nil {
		return applibrary.PhotoDetail{}, undecodable(fmt.Sprintf("photo %q: stored capture time source", row.ItemID), err)
	}

	info := photo.Info{
		TakenAt: time.UnixMilli(row.TakenAt).UTC(), Make: row.Make, Model: row.Model, Lens: row.Lens,
		ExposureSeconds: row.Exposure, FNumber: row.FNumber, ISO: int(row.Iso), FocalLengthMM: row.FocalMm,
		Orientation: int(row.Orientation), Width: int(row.Width), Height: int(row.Height),
	}
	if row.Lat.Valid && row.Lon.Valid {
		info.GPS = &photo.Coordinates{Latitude: row.Lat.Float64, Longitude: row.Lon.Float64}
	}

	return applibrary.PhotoDetail{Info: info, TakenAtSource: source, ThumbnailState: state}, nil
}
