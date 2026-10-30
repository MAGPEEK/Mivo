package libraryfs

import (
	"cmp"
	"context"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"slices"

	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/library"
)

const (
	// readAllEntries makes ReadDir return every entry of a directory at once.
	readAllEntries = -1
	// probeEntries is enough to tell a readable root from a lost one.
	probeEntries = 1
	// maxDirectoryDepth backstops the walk against pathological trees, including directories
	// with an unknown identity, which are always listed; real libraries nest a handful of
	// levels. Deeper directories are logged and reported as skipped.
	maxDirectoryDepth = 64
	// Log keys; paths are always relative to the library root.
	pathLogKey  = "path"
	errorLogKey = "error"
	limitLogKey = "limit"
	// alreadyWalkedMessage is logged at debug level for a directory reached again through a link.
	alreadyWalkedMessage = "skipping directory that was already walked"
	// collisionMessage is logged when two directories of the real tree report the same identity.
	collisionMessage = "skipping directory whose identity matches another directory; the file system may not report unique ids"
	// linkModes are the entry types that point elsewhere: symlinks, and on Windows junctions and
	// other name-surrogate reparse points, which Go reports as irregular files.
	linkModes = fs.ModeSymlink | fs.ModeIrregular
)

// FileSystem walks library roots and opens or resolves the files below them, confined to the
// root (see the package documentation). It holds no open handles and is safe for concurrent
// use.
type FileSystem struct {
	logger *slog.Logger
}

// New returns a FileSystem that logs skipped entries to logger.
func New(logger *slog.Logger) (*FileSystem, error) {
	if logger == nil {
		return nil, fmt.Errorf("new library file system: logger is nil: %w", domain.ErrInvalid)
	}

	return &FileSystem{logger: logger}, nil
}

// Walk calls visit once for every video file below the absolute library root.
//
// The real directory tree is walked first, directories in name order; directory links are
// followed afterwards, and every physical directory is listed once, so a file keeps the path
// without links and cross-linked folders cannot multiply the walk. File links are reported
// under their own path, so one file may appear under several paths. Hidden entries, NAS and OS
// system folders (library.IsIgnoredDirectory), non-video files, special files and links that
// leave the root are skipped. Entries whose path the catalog cannot store are logged and
// skipped.
//
// Walk returns the root-relative paths it could not read: unreadable directories and entries,
// directories below the depth limit and directories of the real tree whose identity collides
// with another one. The caller must keep catalog entries equal to or below these paths: their
// files were not seen, but they were not deleted either. On error the result is nil and the
// caller must not delete anything.
//
// A root that cannot be opened or listed, or that becomes unreadable during the walk, returns
// an error wrapping domain.ErrUnavailable. Walk stops with ctx.Err() when ctx ends (checked
// before each entry) and with the error of visit when visit fails.
func (fileSystem *FileSystem) Walk(ctx context.Context, root string, visit func(library.FileState) error) ([]string, error) {
	if visit == nil {
		return nil, fmt.Errorf("walk library root %s: visit is nil: %w", root, domain.ErrInvalid)
	}

	libraryRoot, err := openLibraryRoot(root)
	if err != nil {
		return nil, fmt.Errorf("walk: %w", err)
	}

	defer closeRoot(libraryRoot)

	walker := newWalk(libraryRoot, fileSystem.logger, visit)
	if err := walker.run(ctx); err != nil {
		return nil, fmt.Errorf("walk library root %s: %w", root, err)
	}

	return walker.skipped, nil
}

// location is where the walk is: a root-relative path, its depth and whether a link was
// followed to get there.
type location struct {
	relativePath string
	depth        int
	throughLink  bool
}

func (at location) child(name string) location {
	return location{relativePath: joinRelative(at.relativePath, name), depth: at.depth + 1, throughLink: at.throughLink}
}

// walk is the state of one Walk call.
type walk struct {
	root   *os.Root
	logger *slog.Logger
	visit  func(library.FileState) error
	// identify reads the identity of an open directory; tests inject colliding or unknown ids.
	identify func(*os.File) (directoryID, error)
	visited  map[directoryID]struct{}
	// pendingLinks are directory links, followed in discovery order once the real tree is done.
	pendingLinks []location
	skipped      []string
}

func newWalk(root *os.Root, logger *slog.Logger, visit func(library.FileState) error) *walk {
	return &walk{
		root:     root,
		logger:   logger,
		visit:    visit,
		identify: identifyDirectory,
		visited:  map[directoryID]struct{}{},
	}
}

func (w *walk) run(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	rootLocation := location{relativePath: rootDirectory}

	entries, err := w.list(rootLocation)
	if err != nil {
		return fmt.Errorf("list root: %w: %w", domain.ErrUnavailable, err)
	}

	if err := w.walkEntries(ctx, rootLocation, entries); err != nil {
		return err
	}

	for len(w.pendingLinks) > 0 {
		if err := ctx.Err(); err != nil {
			return err
		}

		link := w.pendingLinks[0]
		w.pendingLinks = w.pendingLinks[1:]

		if err := w.directory(ctx, link); err != nil {
			return err
		}
	}

	return nil
}

func (w *walk) directory(ctx context.Context, at location) error {
	entries, err := w.list(at)
	if err != nil {
		return w.unreadable(at.relativePath, err)
	}

	return w.walkEntries(ctx, at, entries)
}

func (w *walk) walkEntries(ctx context.Context, at location, entries []fs.DirEntry) error {
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return err
		}

		if err := w.entry(ctx, at, entry); err != nil {
			return err
		}
	}

	return nil
}

// list opens a directory through the root and reads its entries, sorted by name. It returns no
// entries for a directory that admit refuses. The identity comes from the open handle, so it
// describes the directory that is listed even if the path changes meanwhile.
func (w *walk) list(at location) ([]fs.DirEntry, error) {
	directory, err := w.root.OpenFile(at.relativePath, openDirectoryFlags, 0)
	if err != nil {
		return nil, fmt.Errorf("open directory: %w", err)
	}

	defer closeFile(directory)

	identity, err := w.identify(directory)
	if err != nil {
		return nil, err
	}

	if !w.admit(at, identity) {
		return nil, nil
	}

	// Where the file system leaves the entry type unknown (DT_UNKNOWN), Go's ReadDir looks it
	// up relative to the open directory; that reads metadata only and opens nothing.
	entries, err := directory.ReadDir(readAllEntries)
	if err != nil {
		return nil, fmt.Errorf("read directory: %w", err)
	}

	slices.SortFunc(entries, func(first, second fs.DirEntry) int { return cmp.Compare(first.Name(), second.Name()) })

	return entries, nil
}

// admit decides whether a directory is listed. The identity is checked before the depth limit,
// so a link back to a walked directory stays a harmless duplicate at any depth. Unknown
// identities are never recorded: such a directory is always listed, bounded by the depth limit.
func (w *walk) admit(at location, identity directoryID) bool {
	if _, seen := w.visited[identity]; seen && identity.known() {
		w.duplicate(at)
		return false
	}

	if at.depth > maxDirectoryDepth {
		w.logger.Warn("skipping directory below the depth limit",
			pathAttr(at.relativePath), slog.Int(limitLogKey, maxDirectoryDepth))
		w.skipped = append(w.skipped, at.relativePath)

		return false
	}

	if identity.known() {
		w.visited[identity] = struct{}{}
	}

	return true
}

// duplicate handles a directory whose identity was seen before. Through a link that is the
// expected result of following links. In the real tree, without links, two directories cannot
// be the same, so the file system reported a non-unique id (ReFS, SMB, FUSE): the directory is
// reported as skipped, because dropping it silently would delete its catalog entries.
func (w *walk) duplicate(at location) {
	if at.throughLink {
		w.logger.Debug(alreadyWalkedMessage, pathAttr(at.relativePath))
		return
	}

	w.logger.Warn(collisionMessage, pathAttr(at.relativePath))
	w.skipped = append(w.skipped, at.relativePath)
}

func (w *walk) entry(ctx context.Context, parent location, entry fs.DirEntry) error {
	name := entry.Name()
	if isSkippedName(name) || isPlainNonVideoFile(entry) {
		return nil
	}

	at := parent.child(name)
	if _, err := nativeRelativePath(at.relativePath); err != nil {
		w.logger.Warn("skipping entry whose path the catalog cannot store", pathAttr(at.relativePath), errorAttr(err))
		return nil
	}

	switch {
	case entry.IsDir():
		return w.directory(ctx, at)
	case entry.Type()&linkModes != 0:
		return w.link(at)
	case entry.Type().IsRegular():
		return w.videoFile(at.relativePath, entry)
	default:
		// FIFOs, sockets and devices: never opened, never reported.
		return nil
	}
}

func (w *walk) videoFile(relativePath string, entry fs.DirEntry) error {
	info, err := videoFileInfo(w.root, relativePath, entry)
	if err != nil {
		return w.unreadable(relativePath, err)
	}

	// The entry may have been replaced since the listing; only a regular file is reported.
	if !info.Mode().IsRegular() {
		return nil
	}

	return w.file(relativePath, info)
}

// link follows a symlink or junction as far as os.Root allows: inside the root. Directory
// links are queued and walked after the real tree.
func (w *walk) link(at location) error {
	info, err := w.root.Stat(at.relativePath)
	if err != nil {
		return w.unresolvedLink(at.relativePath, err)
	}

	switch {
	case info.IsDir():
		at.throughLink = true
		w.pendingLinks = append(w.pendingLinks, at)

		return nil
	case info.Mode().IsRegular():
		if !library.IsVideoFileName(at.relativePath) {
			return nil
		}

		return w.file(at.relativePath, info)
	default:
		// For example cloud-sync placeholders on Windows, which Go reports as irregular.
		w.logger.Info("skipping link that is not a regular file or directory", pathAttr(at.relativePath))
		return nil
	}
}

func (w *walk) file(relativePath string, info fs.FileInfo) error {
	state := library.FileState{RelativePath: relativePath, Size: info.Size(), ModifiedAt: info.ModTime()}
	if err := w.visit(state); err != nil {
		return fmt.Errorf("visit %s: %w", relativePath, err)
	}

	return nil
}
