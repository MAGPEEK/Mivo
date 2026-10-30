package library

import (
	"context"
	"fmt"

	"github.com/magpeek/mivo/internal/domain/library"
	"github.com/magpeek/mivo/internal/domain/media"
)

// ItemSummary is an entry of a library listing: the item and its primary file with streams,
// whose technical description the list shows ("MKV · H.264 · AAC"). The catalog keeps no
// item without a file (removing a library's last file of an item removes the item), so
// PrimaryFile is always set.
type ItemSummary struct {
	Item        library.Item
	PrimaryFile media.File
}

// ItemPage is one page of a library's items ordered by sort title. NextCursor is empty on
// the last page; otherwise it is passed unchanged as the cursor of the next request.
type ItemPage struct {
	Items      []ItemSummary
	NextCursor string
}

// ItemDetail is an item with its media files and their streams; the first file is the
// primary one. Milestone 2 has exactly one file per item.
type ItemDetail struct {
	Item  library.Item
	Files []media.File
}

// ListItems returns one page of a library's items. A limit of 0 means the default; a limit
// outside 1 to library.MaxPageLimit and a cursor the catalog did not produce wrap
// domain.ErrInvalid, an unknown library wraps domain.ErrNotFound. Broken stored library data
// is reported as a plain error, never as domain.ErrInvalid.
func (service *Service) ListItems(ctx context.Context, libraryID string, page library.PageRequest) (ItemPage, error) {
	limit, err := library.NormalizeLimit(page.Limit)
	if err != nil {
		return ItemPage{}, fmt.Errorf("list items of library %q: %w", libraryID, err)
	}

	// The reader cannot tell an unknown library from an empty one.
	if _, err := service.libraries.GetLibrary(ctx, libraryID); err != nil {
		return ItemPage{}, fmt.Errorf("list items of library %q: %w", libraryID, storedDataError(err))
	}

	items, err := service.items.ListItems(ctx, libraryID, library.PageRequest{Limit: limit, Cursor: page.Cursor})
	if err != nil {
		return ItemPage{}, fmt.Errorf("list items of library %q: %w", libraryID, err)
	}

	return items, nil
}

// GetItem returns an item with its files and streams. An unknown ID wraps
// domain.ErrNotFound.
func (service *Service) GetItem(ctx context.Context, itemID string) (ItemDetail, error) {
	detail, err := service.items.GetItem(ctx, itemID)
	if err != nil {
		return ItemDetail{}, fmt.Errorf("get item %q: %w", itemID, err)
	}

	return detail, nil
}
