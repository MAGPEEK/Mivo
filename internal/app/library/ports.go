// Package library contains the catalog use cases (ADR 0022, 0028): managing libraries and
// importing the configured ones once, listing libraries with their availability, paging
// through a library's top-level items and an item's children, and opening one item with its
// detail. Scanning is a separate package (internal/app/scan). It declares the ports it needs;
// adapters implement them and cmd/mivo wires them (ADR 0003).
package library

import (
	"context"

	"github.com/magpeek/mivo/internal/domain/access"
	"github.com/magpeek/mivo/internal/domain/library"
)

// ItemReader reads the media catalog. Implemented by adapters/sqlite (M2-15).
//
// Error contract: domain.ErrInvalid means only a bad client value, that is a cursor the
// reader did not produce. A stored row the domain cannot read is a server fault and must be
// reported as an error that does not wrap domain.ErrInvalid; the service passes reader errors
// on unchanged, and the HTTP adapter answers ErrInvalid with 400.
type ItemReader interface {
	// ListItems returns one page of a library's top-level items (movies and shows, never
	// seasons, episodes or extras) in the order of query.Order (ties by sort title, then
	// ID; items without a value for the key last in both directions) and narrowed by
	// query.Filter, each with its primary file and streams (none for a show), plus the total
	// count (Counted true). page.Limit is already normalised (1 to library.MaxPageLimit);
	// query.Filter.Kind is the library's top-level kind or empty for libraries without one.
	// A cursor the reader did not produce, or one issued for another sort, direction or
	// seed, wraps domain.ErrInvalid. Sort "random" without HasSeed takes the seed from the
	// cursor. The service checks that the library exists, so an unknown library may simply
	// yield an empty page.
	ListItems(ctx context.Context, libraryID string, page library.PageRequest, query library.ListingQuery) (ItemPage, error)
	// GetItem returns an item with its detail (ItemDetail), the primary file first. An
	// unknown ID wraps domain.ErrNotFound.
	GetItem(ctx context.Context, itemID string) (ItemDetail, error)
}

// ChildReader pages through the children of a catalog item. It is a port of its own, not a
// method of ItemReader, so the milestone-2 sqlite reader keeps satisfying ItemReader until
// M3-05c implements this port and M3-11b wires it (M3-00 dual paths). Implemented by
// adapters/sqlite (M3-05c).
//
// Ordering (ADR 0027 §2, API listItemChildren): the children of a show are its seasons by
// ascending number with the specials (season 0) last; the children of a season are its
// episodes by ascending number with episodes without a number last; equal numbers and
// unnumbered items order by sort title, then by ID. The albums of an artist order by sort
// title, then ID; the tracks of an album follow library.CompareTracks (disc, track number with
// unnumbered tracks last, file name), then ID. The children of a photo album follow
// PhotoReader.ListPhotoChildren, with the photo details and tiles in the summaries (see
// ItemSummary). Extras are never children. Descending reverses the whole order, so specials
// and unnumbered episodes come first; a photo album lists ascending only and wraps
// domain.ErrInvalid for Descending.
//
// Error contract as ItemReader: an unknown item wraps domain.ErrNotFound, a cursor the reader
// did not produce (or one issued for the other direction) wraps domain.ErrInvalid, a stored
// row the domain cannot read is a plain error. A leaf (movie, episode, extra) yields an
// empty page.
type ChildReader interface {
	// ListChildren returns one page of the item's children. request.Page.Limit is already
	// normalised (1 to library.MaxPageLimit).
	ListChildren(ctx context.Context, itemID string, request ChildPageRequest) (ItemPage, error)
}

// RootChecker reports whether a library root is reachable now. Implemented by
// adapters/fs/libraryfs (FileSystem.RootAvailable).
type RootChecker interface {
	// RootAvailable reports whether root can be opened. It should return when ctx is done;
	// the service stops waiting at its timeout either way, because a stat on a hung network
	// mount may ignore cancellation.
	RootAvailable(ctx context.Context, root string) bool
}

// ListingReader counts what the filter menu and the A–Z bar of a library listing show.
// Implemented by adapters/sqlite (M5-11). Errors follow the ItemReader contract; the filter
// is complete (see Service.prepareFilter) and the library exists.
type ListingReader interface {
	// CountFacets returns one Facet per requested field, in the order of request.Fields;
	// each field is counted under request.Filter without that field's own values.
	CountFacets(ctx context.Context, libraryID string, request FacetRequest) ([]Facet, error)
	// CountFirstCharacters returns, in the order of the title-sorted listing narrowed by
	// request.Filter (descending when request.Descending), each first character of the sort
	// titles with the number of items starting with it.
	CountFirstCharacters(ctx context.Context, libraryID string, request LetterRequest) ([]CharacterCount, error)
}

// FolderItemReader reads the catalog side of the folder view. Implemented by adapters/sqlite
// (M5-11).
type FolderItemReader interface {
	// ListFolderItems returns the items (never extras) that own a file directly in the
	// directory of the library folder; directory is "/"-separated and empty for the folder
	// itself. Each item appears once, named after its first file there. Only items the
	// restriction allows are returned; its zero value allows none.
	ListFolderItems(ctx context.Context, rootID, directory string, restriction access.Restriction) ([]FolderItem, error)
	// ItemSummaries returns the summaries of the given items as listings carry them; unknown
	// IDs are left out.
	ItemSummaries(ctx context.Context, itemIDs []string) ([]ItemSummary, error)
}

// DirectoryLister lists directories inside a library folder. Implemented by
// adapters/fs/libraryfs (M5-11).
type DirectoryLister interface {
	// ListDirectories returns the names of the sub-directories of directory ("/"-separated,
	// empty for the library folder itself) below root, unordered. Symlinks are followed only
	// while they stay inside root; one that leaves it is left out, and a directory reached
	// only through one wraps domain.ErrForbidden. A missing directory wraps
	// domain.ErrNotFound, an unreachable root domain.ErrUnavailable.
	ListDirectories(ctx context.Context, root, directory string) ([]string, error)
}
