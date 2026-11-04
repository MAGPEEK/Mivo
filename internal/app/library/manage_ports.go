package library

import (
	"context"

	"github.com/magpeek/mivo/internal/domain/library"
)

// ManagedWriter changes the managed libraries, each call in one transaction. Implemented by
// adapters/sqlite (M3-19g).
//
// Error contract: a folder that another library already holds (the unique index backstop)
// wraps domain.ErrConflict; an unknown ID wraps domain.ErrNotFound. The store assigns the
// IDs of new libraries and folders.
type ManagedWriter interface {
	// CreateLibrary stores a new library and returns it with its IDs.
	CreateLibrary(ctx context.Context, candidate library.ManagedLibrary) (library.ManagedLibrary, error)
	// UpdateLibrary replaces the name, language, options and folders of a library; folders
	// without an ID are new, stored folders missing from the list are deleted with their
	// catalog entries. The kind never changes.
	UpdateLibrary(ctx context.Context, updated library.ManagedLibrary) error
	// DeleteLibrary deletes a library; its catalog is deleted by cascade.
	DeleteLibrary(ctx context.Context, id string) error
}

// ManagedReader reads the managed libraries with their folders. Implemented by
// adapters/sqlite (M3-19g).
type ManagedReader interface {
	// ListManaged returns every library with its folders, ordered by name.
	ListManaged(ctx context.Context) ([]library.ManagedLibrary, error)
	// GetManaged returns one library. An unknown ID wraps domain.ErrNotFound.
	GetManaged(ctx context.Context, id string) (library.ManagedLibrary, error)
}

// ImportLedger remembers which configured roots were imported already, so a library the
// user deleted does not return at the next start. Implemented by adapters/sqlite (M3-19g).
type ImportLedger interface {
	// NeverImported returns those of roots that were not recorded yet, in the given order.
	NeverImported(ctx context.Context, roots []string) ([]string, error)
	// RecordImported records roots as handled.
	RecordImported(ctx context.Context, roots []string) error
}

// BrowseRoot is a folder root the user may browse.
type BrowseRoot struct {
	ID        string
	Path      string
	Available bool
}

// FolderListing is the result of listing one folder: the names of its readable
// sub-directories, how many could not be read and whether the list was cut short.
type FolderListing struct {
	Names           []string
	UnreadableCount int
	Truncated       bool
}

// FolderBrowser lists and checks folders inside the browse roots. Implemented by
// adapters/fs/folderbrowse (M3-19d).
//
// Error contract of Check: domain.ErrNotFound (missing), domain.ErrForbidden (unreadable),
// domain.ErrInvalid (bad path or unknown root) and domain.ErrUnavailable (root missing or
// timed out).
type FolderBrowser interface {
	// Roots returns the configured browse roots.
	Roots(ctx context.Context) ([]BrowseRoot, error)
	// List returns the sub-directories of relativePath inside the root.
	List(ctx context.Context, rootID, relativePath string) (FolderListing, error)
	// Check verifies that the folder exists and is readable and returns its absolute path.
	Check(ctx context.Context, rootID, relativePath string) (string, error)
}

// ScanControl starts and stops library scans. Implemented by *scan.Service (M3-19i); cmd
// passes it in.
type ScanControl interface {
	// Start starts a scan unless one runs already.
	Start(ctx context.Context, libraryID string) error
	// Cancel asks a running scan of the library to stop; it does nothing when none runs.
	Cancel(libraryID string)
	// WaitIdle returns once no scan of the library runs or ctx is done.
	WaitIdle(ctx context.Context, libraryID string) error
}

// ChangeEvents announces library changes (for the scan triggers, M3-19j). Calls must not
// block.
type ChangeEvents interface {
	// Added reports a created library.
	Added(libraryID string)
	// Changed reports a library whose folders, options, name or language changed.
	Changed(libraryID string)
	// Removed reports a deleted library.
	Removed(libraryID string)
}
