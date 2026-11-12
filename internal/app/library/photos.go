package library

import (
	"context"

	"github.com/magpeek/mivo/internal/domain/access"
	"github.com/magpeek/mivo/internal/domain/library"
	"github.com/magpeek/mivo/internal/domain/photo"
)

// PhotoReader reads photo albums and photos (ADR 0029). Implemented by adapters/sqlite
// (M3-15e), whose ChildReader and ItemReader serve the same order and details through the
// generic item endpoints (M3-15h); the port stays for photo-only views such as the timeline.
//
// Error contract as ChildReader: an unknown item and one the restriction hides wrap
// domain.ErrNotFound; an item of the wrong kind, an invalid limit and a cursor the reader did
// not issue for this album wrap domain.ErrInvalid. Every read returns and counts only what
// the restriction allows; its zero value allows nothing.
type PhotoReader interface {
	// ListPhotoChildren returns one page of the children of a photo album: its sub-albums by
	// sort title, then its photos by capture time and file name (case-insensitive), ties by
	// item ID.
	ListPhotoChildren(ctx context.Context, albumID string, page library.PageRequest, restriction access.Restriction) (PhotoChildPage, error)
	// ListLibraryPhotos returns one page of every photo of a library, whatever album it is
	// in, in the photo order of ListPhotoChildren (the timeline). An unknown library and one
	// the restriction does not allow yield an empty page.
	ListLibraryPhotos(ctx context.Context, libraryID string, page library.PageRequest, restriction access.Restriction) (PhotoChildPage, error)
	// PhotoDetail returns the photo block of a photo item.
	PhotoDetail(ctx context.Context, itemID string, restriction access.Restriction) (PhotoDetail, error)
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
// CoverPhotoID names the newest of those photos that is a still image (no video) and whose
// thumbnail is ready, the album's tile ("" when there is none).
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
