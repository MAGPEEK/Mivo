// Package library contains the catalog use cases (ADR 0022): syncing the configured
// libraries into the catalog at startup, listing libraries with their availability, paging
// through a library's top-level items and an item's children, and opening one item with its
// detail. Scanning is a
// separate package (internal/app/scan). It declares the ports it needs; adapters implement
// them and cmd/mivo wires them (ADR 0003).
package library

import (
	"context"

	"github.com/magpeek/mivo/internal/domain/library"
)

// Store persists the configured libraries. Implemented by adapters/sqlite
// (LibraryRepository).
//
// Error contract: domain.ErrInvalid from SyncLibraries means invalid definitions or a
// missing ID generator. The reads receive no client value except an ID (unknown →
// domain.ErrNotFound), so ErrInvalid from a read can only mean a stored row the domain cannot
// read; the service reports it as a plain error, because it is a server fault and not a bad
// request.
type Store interface {
	// SyncLibraries makes the stored libraries match definitions in one transaction. The
	// root is a library's identity: a new root gets an ID from newID, and a stored root that
	// is no longer configured is deleted with its catalog and reported in Removed.
	SyncLibraries(ctx context.Context, definitions []library.Definition,
		newID func() (string, error)) (library.SyncResult, error)
	// ListLibraries returns every library with its item count, ordered by name.
	ListLibraries(ctx context.Context) ([]library.Summary, error)
	// GetLibrary returns one library with its item count. An unknown ID wraps
	// domain.ErrNotFound.
	GetLibrary(ctx context.Context, id string) (library.Summary, error)
}

// ItemReader reads the media catalog. Implemented by adapters/sqlite (M2-15).
//
// Error contract: domain.ErrInvalid means only a bad client value, that is a cursor the
// reader did not produce. A stored row the domain cannot read is a server fault and must be
// reported as an error that does not wrap domain.ErrInvalid; the service passes reader errors
// on unchanged, and the HTTP adapter answers ErrInvalid with 400.
type ItemReader interface {
	// ListItems returns one page of a library's top-level items (movies and shows, never
	// seasons, episodes or extras) ordered by sort title, each with its primary file and
	// streams (none for a show). page.Limit is already normalised (1 to
	// library.MaxPageLimit). A cursor the reader did not produce wraps domain.ErrInvalid.
	// The service checks that the library exists, so an unknown library may simply yield an
	// empty page.
	ListItems(ctx context.Context, libraryID string, page library.PageRequest) (ItemPage, error)
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
// unnumbered items order by sort title, then by ID. Extras are never children. Descending
// reverses the whole order, so specials and unnumbered episodes come first.
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

// IDGenerator creates new random library IDs in canonical lowercase UUID text form.
// Implemented by adapters/uuidgen.
type IDGenerator interface {
	// NewID returns a new random ID.
	NewID() (string, error)
}

// RootChecker reports whether a library root is reachable now. Implemented by
// adapters/fs/libraryfs (FileSystem.RootAvailable).
type RootChecker interface {
	// RootAvailable reports whether root can be opened. It should return when ctx is done;
	// the service stops waiting at its timeout either way, because a stat on a hung network
	// mount may ignore cancellation.
	RootAvailable(ctx context.Context, root string) bool
}
