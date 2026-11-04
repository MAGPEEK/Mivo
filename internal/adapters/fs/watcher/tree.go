package watcher

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/fsnotify/fsnotify"

	"github.com/magpeek/mivo/internal/domain/library"
)

// maxDirectoryDepth matches the depth limit of the library walker: deeper directories are
// not scanned, so watching them would only cost watches.
const maxDirectoryDepth = 64

// libraryChanges are the operations that can change what a scan finds. Chmod is left out:
// it fires for attribute and timestamp updates, and on Linux before every delete.
const libraryChanges = fsnotify.Create | fsnotify.Write | fsnotify.Remove | fsnotify.Rename

var (
	// errWatchLimit explains the inotify ENOSPC error, whose own text ("no space left on
	// device") misleads.
	errWatchLimit = errors.New("the limit of file system watches is reached; raise fs.inotify.max_user_watches")
	// errTooDeep reports a directory below maxDirectoryDepth.
	errTooDeep = errors.New("directory is nested too deep to be scanned")
	// errEventsLost reports a queue or buffer overflow: some changes were not delivered.
	errEventsLost = errors.New("file system events were lost")
)

// treeOptions are the inputs of newTree; add and remove are the notifier's, replaceable in
// tests.
type treeOptions struct {
	root string
	// identity is the root as it was when the watch started.
	identity os.FileInfo
	// recursive means that watching the root covers every directory below it.
	recursive bool
	add       func(path string) error
	remove    func(path string) error
	report    func(err error)
	changed   func()
}

// tree is the state of one Watch call: the watched directories and whether a failure was
// reported already. Only the watching goroutine uses it.
type tree struct {
	treeOptions
	watched  map[string]struct{}
	reported bool
}

func newTree(options treeOptions) *tree {
	return &tree{treeOptions: options, watched: map[string]struct{}{}}
}

// watchRoot watches the root, which must succeed, and every directory below it, which may
// fail directory by directory. A cancelled ctx stops the walk early.
func (watched *tree) watchRoot(ctx context.Context) error {
	if err := watched.add(watched.root); err != nil {
		return describeAddError(err)
	}

	watched.watched[watched.root] = struct{}{}

	if !watched.recursive {
		watched.watchBelow(ctx, watched.root, 0)
	}

	return nil
}

// watchBelow watches the real subdirectories of directory. DirEntry.IsDir is false for
// symlinks and Windows junctions, so links are never followed.
func (watched *tree) watchBelow(ctx context.Context, directory string, depth int) {
	entries, err := os.ReadDir(directory)
	if err != nil {
		watched.fail(directory, err)
		return
	}

	for _, entry := range entries {
		if ctx.Err() != nil {
			return
		}

		if entry.IsDir() && !isSkippedName(entry.Name()) {
			watched.watchDirectory(ctx, filepath.Join(directory, entry.Name()), depth+1)
		}
	}
}

func (watched *tree) watchDirectory(ctx context.Context, directory string, depth int) {
	if depth > maxDirectoryDepth {
		watched.fail(directory, errTooDeep)
		return
	}

	if err := watched.add(directory); err != nil {
		watched.fail(directory, describeAddError(err))
		return
	}

	watched.watched[directory] = struct{}{}
	watched.watchBelow(ctx, directory, depth)
}

// watchCreated watches a new real directory and the directories already inside it (a
// folder moved into the library arrives with its content).
func (watched *tree) watchCreated(ctx context.Context, path, relative string) {
	info, err := os.Lstat(path)
	if err != nil || !info.IsDir() {
		return
	}

	watched.watchDirectory(ctx, path, strings.Count(relative, string(filepath.Separator))+1)
}

// forget drops the watches of a removed or renamed directory and of everything below it.
// fsnotify drops some of them itself, so failures of Remove are expected and ignored.
func (watched *tree) forget(directory string) {
	if _, ok := watched.watched[directory]; !ok {
		return
	}

	prefix := directory + string(filepath.Separator)

	for path := range watched.watched {
		if path == directory || strings.HasPrefix(path, prefix) {
			_ = watched.remove(path)

			delete(watched.watched, path)
		}
	}
}

// fail reports the first failure of this watch; a directory that vanished meanwhile is not
// a failure. Later failures are dropped: the caller only needs to know that the tree is not
// fully watched, and a reached watch limit would otherwise report every remaining folder.
func (watched *tree) fail(path string, err error) {
	if errors.Is(err, fs.ErrNotExist) || watched.reported {
		return
	}

	watched.reported = true
	watched.report(fmt.Errorf("watch %s: %w", watched.describe(path), err))
}

// describe names a path relative to the root, the way the library walker logs paths.
func (watched *tree) describe(path string) string {
	relative, ok := watched.relative(path)
	if !ok || relative == "." {
		return "library root"
	}

	return filepath.ToSlash(relative)
}

func describeAddError(err error) error {
	if errors.Is(err, syscall.ENOSPC) {
		return fmt.Errorf("%w: %w", errWatchLimit, err)
	}

	return err
}

// relative returns path relative to the root, or false for a path outside it.
func (watched *tree) relative(path string) (string, bool) {
	relative, err := filepath.Rel(watched.root, path)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", false
	}

	return relative, true
}

// isSkippedPath reports a path with a hidden or system element anywhere: the scan skips
// the whole subtree, so changes inside it do not matter.
func isSkippedPath(relative string) bool {
	for element := range strings.SplitSeq(relative, string(filepath.Separator)) {
		if isSkippedName(element) {
			return true
		}
	}

	return false
}

// isSkippedName applies the skip rules of the library walker (ADR 0022).
func isSkippedName(name string) bool {
	return library.IsHiddenEntry(name) || library.IsIgnoredDirectory(name)
}
