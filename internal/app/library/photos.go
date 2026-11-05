package library

import (
	"context"

	"github.com/magpeek/mivo/internal/domain/library"
	"github.com/magpeek/mivo/internal/domain/photo"
)

// PhotoReader reads photo albums and photos (ADR 0029). Implemented by adapters/sqlite
// (M3-15e); the HTTP adapter serves it from M3-15h.
//
// Error contract as ChildReader: an unknown item wraps domain.ErrNotFound; an item of the
// wrong kind, an invalid limit and a cursor the reader did not issue for this album wrap
// domain.ErrInvalid.
type PhotoReader interface {
	// ListPhotoChildren returns one page of the children of a photo album: its sub-albums by
	// sort title, then its photos by capture time and file name (case-insensitive), ties by
	// item ID.
	ListPhotoChildren(ctx context.Context, albumID string, page library.PageRequest) (PhotoChildPage, error)
	// ListLibraryPhotos returns one page of every photo of a library, whatever album it is
	// in, in the photo order of ListPhotoChildren (the timeline). An unknown library yields
	// an empty page.
	ListLibraryPhotos(ctx context.Context, libraryID string, page library.PageRequest) (PhotoChildPage, error)
	// PhotoDetail returns the photo block of a photo item.
	PhotoDetail(ctx context.Context, itemID string) (PhotoDetail, error)
}

// PhotoDetail is what the catalog knows about one photo. Info.TakenAt holds the capture
// time: the EXIF time, or the modification time of the file when it has none, as
// TakenAtSource says.
type PhotoDetail struct {
	Info           photo.Info
	TakenAtSource  photo.TakenAtSource
	ThumbnailState photo.ThumbnailState
}

// PhotoChild is one child of a photo album: a sub-album (Photo nil) or a photo with its
// detail. A photo is its own thumbnail, so the image endpoint takes Item.ID. For a
// sub-album, ChildCount counts its direct children as ListPhotoChildren lists them
// (sub-albums and photos), LeafCount the photos in it and all its sub-albums, and
// CoverPhotoID names the newest of those photos whose thumbnail is ready, the album's tile
// ("" when there is none).
type PhotoChild struct {
	Item         library.Item
	ChildCount   int
	LeafCount    int
	CoverPhotoID string
	Photo        *PhotoDetail
}

// PhotoChildPage is one page of an album's children. NextCursor is empty on the last page.
type PhotoChildPage struct {
	Children   []PhotoChild
	NextCursor string
}
