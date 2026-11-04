package library

import (
	"context"
	"errors"
	"fmt"

	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/artwork"
	"github.com/magpeek/mivo/internal/domain/library"
	"github.com/magpeek/mivo/internal/domain/media"
	"github.com/magpeek/mivo/internal/domain/metadata"
)

// ItemSummary is an entry of a listing: a library's top-level items, the children of a
// container or the extras of an item. The milestone-3 fields are additive (ADR 0027): Poster
// and Thumb are nil when the item has no such artwork; ChildCount is the number of seasons of
// a show or episodes of a season, LeafCount the number of episodes of a show; both are 0 for
// leaves. The year is Item.Year.
//
// PrimaryFile is the item's primary file with streams, whose technical description the list
// shows ("MKV · H.264 · AAC"). Items that own files (movies, episodes, extras) always have
// one, because the catalog removes an item with its last file; containers (shows, seasons)
// own no file and leave it zero (HasPrimaryFile reports false).
type ItemSummary struct {
	Item        library.Item
	PrimaryFile media.File
	Poster      *artwork.Ref
	Thumb       *artwork.Ref
	ChildCount  int
	LeafCount   int
}

// HasPrimaryFile reports whether the summary carries a primary file; containers have none.
func (summary ItemSummary) HasPrimaryFile() bool {
	return summary.PrimaryFile.ID != ""
}

// ItemPage is one page of items in the order of the listing that produced it. NextCursor is
// empty on the last page; otherwise it is passed unchanged as the cursor of the next request.
type ItemPage struct {
	Items      []ItemSummary
	NextCursor string
}

// ItemRef names an ancestor of an item for the breadcrumb and the episode header
// ("Show > Season 1"). Number is the season number of a season and absent otherwise.
type ItemRef struct {
	ID     string
	Kind   library.ItemKind
	Title  string
	Number library.OptionalNumber
}

// ItemFile is a media file of an item with its place among the item's files: VersionLabel
// names an alternative version ("" for the only or an unnamed one), PartNumber the part of a
// multi-part version (absent for a single-part file).
type ItemFile struct {
	File         media.File
	VersionLabel string
	PartNumber   library.OptionalNumber
}

// ItemDetail is an item with everything its detail page needs.
//
// Files is the milestone-2 view: the media files with streams, the primary one first.
// Milestone 2 has exactly one file per item and leaves every other field zero, which the
// current web app relies on. The milestone-3 fields are additive until M3-05c and M3-06b
// switch the adapters over (M3-11b removes the milestone-2 path):
//   - Metadata is the merged metadata record (ADR 0027 §6).
//   - Ancestors are the items above this one, root first; empty for top-level items.
//   - PreviousID and NextID are the neighbouring seasons or episodes in ascending child
//     order (see ChildReader); empty at either end and for other kinds.
//   - PlayItemID is the item Play starts: the item itself for a leaf, the first episode of
//     the first regular season for a show, the first episode for a season; empty when there
//     is nothing to play.
//   - Artwork holds the current artwork reference per kind; a missing kind has none.
//   - Extras are the extras of a movie or show, ordered by type and title.
//   - ItemFiles are the files with version label and part number, the primary one first;
//     containers have none.
//   - ChildCount and LeafCount are counted as in ItemSummary.
type ItemDetail struct {
	Item       library.Item
	Files      []media.File
	Metadata   metadata.Record
	Ancestors  []ItemRef
	PreviousID string
	NextID     string
	PlayItemID string
	Artwork    map[artwork.Kind]artwork.Ref
	Extras     []ItemSummary
	ItemFiles  []ItemFile
	ChildCount int
	LeafCount  int
}

// ListItems returns one page of a library's top-level items (movies and shows; seasons,
// episodes and extras are reached through ListChildren and GetItem). A limit of 0 means the
// default; a limit outside 1 to library.MaxPageLimit and a cursor the catalog did not produce
// wrap domain.ErrInvalid, an unknown library wraps domain.ErrNotFound. Broken stored library
// data is reported as a plain error, never as domain.ErrInvalid.
func (service *Service) ListItems(ctx context.Context, libraryID string, page library.PageRequest) (ItemPage, error) {
	limit, err := library.NormalizeLimit(page.Limit)
	if err != nil {
		return ItemPage{}, fmt.Errorf("list items of library %q: %w", libraryID, err)
	}

	// The reader cannot tell an unknown library from an empty one.
	if _, err := service.libraries.GetManaged(ctx, libraryID); err != nil {
		return ItemPage{}, fmt.Errorf("list items of library %q: %w", libraryID, storedDataError(err))
	}

	items, err := service.items.ListItems(ctx, libraryID, library.PageRequest{Limit: limit, Cursor: page.Cursor})
	if err != nil {
		return ItemPage{}, fmt.Errorf("list items of library %q: %w", libraryID, err)
	}

	return items, nil
}

// GetItem returns an item with its detail (see ItemDetail). An unknown ID wraps
// domain.ErrNotFound.
func (service *Service) GetItem(ctx context.Context, itemID string) (ItemDetail, error) {
	detail, err := service.items.GetItem(ctx, itemID)
	if err != nil {
		return ItemDetail{}, fmt.Errorf("get item %q: %w", itemID, err)
	}

	return detail, nil
}

// storedDataError hides domain.ErrInvalid from a library store read. No client value reaches
// those reads (an unknown ID is ErrNotFound), so ErrInvalid can only mean a stored row the
// domain cannot read: a server fault that must not turn into a "bad request" for the client.
// The message is kept for the log; every other error passes through unchanged.
func storedDataError(err error) error {
	if !errors.Is(err, domain.ErrInvalid) {
		return err
	}

	return errors.New("unreadable stored library data: " + err.Error())
}
