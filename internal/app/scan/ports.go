package scan

import (
	"context"

	"github.com/magpeek/mivo/internal/domain/library"
	"github.com/magpeek/mivo/internal/domain/media"
)

// LibrarySource looks up the configured libraries with their folders and options, read at the
// start of each scan so edits apply to the next scan. Implemented by adapters/sqlite
// (ManagedLibraryStore).
type LibrarySource interface {
	// GetLibrary returns one library or an error wrapping domain.ErrNotFound.
	GetLibrary(ctx context.Context, libraryID string) (library.ManagedLibrary, error)
	// ListLibraries returns every library.
	ListLibraries(ctx context.Context) ([]library.ManagedLibrary, error)
}

// Walker lists the library files below a library root. Implemented by adapters/fs/libraryfs.
type Walker interface {
	// Walk calls visit once per file library.ClassifyFileIn accepts for the library kind (media
	// files and sidecars) with its root-relative state. It returns the root-relative paths it
	// could not read (directories, entries, directories beyond the depth limit): catalog
	// entries equal to or below them must be kept. A root that is missing or lost during the
	// walk returns an error wrapping domain.ErrUnavailable; a cancelled ctx returns ctx.Err().
	Walk(ctx context.Context, root string, kind library.Kind, visit func(library.FileState) error) (skipped []string, err error)
}

// PathResolver turns a root-relative path into the absolute path handed to the prober,
// confined to the root. Implemented by adapters/fs/libraryfs.
type PathResolver interface {
	// Resolve returns the absolute path. A vanished file wraps domain.ErrNotFound, a path
	// leaving the root domain.ErrForbidden, an unusable root domain.ErrUnavailable.
	Resolve(root, relativePath string) (string, error)
}

// Prober describes a media file. Implemented by adapters/ffmpeg.
type Prober interface {
	// Probe describes the file at absolutePath. Errors wrap context.DeadlineExceeded for a
	// per-file timeout, context.Canceled when ctx was cancelled, domain.ErrUnavailable when
	// the probing tool cannot run at all and domain.ErrUnsupported when the file could not
	// be described.
	Probe(ctx context.Context, absolutePath string) (media.Description, error)
}

// IDGenerator creates new random IDs in canonical lowercase UUID text form. Implemented by
// adapters/uuidgen.
type IDGenerator interface {
	// NewID returns a new random ID.
	NewID() (string, error)
}
