package scan

import (
	"context"
	"time"

	"github.com/magpeek/mivo/internal/domain/library"
	"github.com/magpeek/mivo/internal/domain/media"
)

// LibrarySource looks up the configured libraries. Implemented by adapters/sqlite, whose
// library repository returns summaries; the scan only uses their Library.
type LibrarySource interface {
	// GetLibrary returns one library or an error wrapping domain.ErrNotFound.
	GetLibrary(ctx context.Context, libraryID string) (library.Summary, error)
	// ListLibraries returns every library.
	ListLibraries(ctx context.Context) ([]library.Summary, error)
}

// Walker lists the video files below a library root. Implemented by adapters/fs/libraryfs.
type Walker interface {
	// Walk calls visit once per video file with its root-relative state. It returns the
	// root-relative paths it could not read (directories, entries, directories beyond the
	// depth limit): catalog entries equal to or below them must be kept. A root that is
	// missing or lost during the walk returns an error wrapping domain.ErrUnavailable; a
	// cancelled ctx returns ctx.Err().
	Walk(ctx context.Context, root string, visit func(library.FileState) error) (skipped []string, err error)
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

// CatalogFile is a media file as the catalog knows it, for change detection.
type CatalogFile struct {
	// ID is the media file ID, ItemID the ID of the item it belongs to.
	ID     string
	ItemID string
	State  library.FileState
}

// ProbedFile is the result of probing one new or changed file. For a new file Item and
// File carry new IDs; for a changed file they carry the stored IDs and the catalog keeps
// the stored item (and its creation time) while it replaces the file row and its streams.
//
// The catalog rejects a record with an error wrapping domain.ErrInvalid unless:
//   - ProbedAt is not zero and File passes media.File.Validate,
//   - Item.LibraryID equals File.LibraryID (for changed files too) and Item.ID equals
//     File.ItemID,
//   - for a new item: CreatedAt is not zero, Title is not blank and Kind equals
//     library.ItemKindForLibrary of the library's kind.
type ProbedFile struct {
	Item     library.Item
	File     media.File
	ProbedAt time.Time
}

// CatalogWriter persists scan results. Implemented by adapters/sqlite.
type CatalogWriter interface {
	// FileStates returns every media file of the library.
	FileStates(ctx context.Context, libraryID string) ([]CatalogFile, error)
	// SaveProbedFile creates or updates one file, its item and its streams atomically. It
	// changes nothing and returns an error wrapping
	//   - domain.ErrInvalid for a record that breaks the rules of ProbedFile,
	//   - domain.ErrNotFound for an unknown library,
	//   - domain.ErrConflict when another file ID is stored at the same relative path of
	//     the library,
	//   - domain.ErrInvalid when a stored file ID belongs to another item or library, or a
	//     stored item to another library.
	SaveProbedFile(ctx context.Context, record ProbedFile) error
	// RemoveFiles deletes the files and the items left without files, atomically. It is
	// idempotent: unknown IDs and IDs of another library are ignored, so callers cannot
	// learn from it whether a file existed.
	RemoveFiles(ctx context.Context, libraryID string, fileIDs []string) error
}

// IDGenerator creates new random IDs in canonical lowercase UUID text form. Implemented by
// adapters/uuidgen.
type IDGenerator interface {
	// NewID returns a new random ID.
	NewID() (string, error)
}
