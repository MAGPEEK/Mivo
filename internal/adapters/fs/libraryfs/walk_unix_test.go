//go:build unix

package libraryfs_test

import (
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"syscall"
	"testing"
	"time"

	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/library"
)

const (
	// lockedDirMode removes every permission, so an unprivileged user can neither list nor enter.
	lockedDirMode = 0o000
	// listOnlyDirMode allows listing names but not looking up the entries.
	listOnlyDirMode = 0o400
)

func TestWalkLogsAndSkipsNamesTheCatalogRejects(t *testing.T) {
	t.Parallel()

	fileSystem, logs := newFileSystem(t)
	root, _ := newLibrary(t)

	// Legal Linux names that library.ParseRelativePath rejects: a drive-letter prefix at the top
	// level and a backslash. A colon further down is fine on Linux.
	writeFiles(t, root, "M:I-2.mkv", `back\slash.mkv`, `dir\name/movie.mkv`, "Sub/M:I-2.mkv", "ok.mkv")

	assertPaths(t, walkPaths(t, fileSystem, root), "Sub/M:I-2.mkv", "ok.mkv")

	records := logs()
	assertLogged(t, records, slog.LevelWarn, "M:I-2.mkv")
	assertLogged(t, records, slog.LevelWarn, `back\slash.mkv`)
	// The directory is reported once instead of once per file inside it.
	assertLogged(t, records, slog.LevelWarn, `dir\name`)
}

func TestWalkLogsAndSkipsUnreadableDirectories(t *testing.T) {
	t.Parallel()
	skipAsRoot(t)

	fileSystem, logs := newFileSystem(t)
	root, _ := newLibrary(t)
	writeFiles(t, root, "open/movie.mkv", "locked/hidden-from-us.mkv", "searchable/movie.mkv")
	// A link into a folder we may not enter is unreadable, not a link that leaves the root.
	symlinkOrSkip(t, filepath.Join("..", "locked", "hidden-from-us.mkv"), filepath.Join(root, "links", "locked.mkv"))
	lock(t, filepath.Join(root, "locked"))
	// Readable but not searchable: the names can be listed, their metadata cannot be read.
	chmod(t, filepath.Join(root, "searchable"), listOnlyDirMode)

	paths, skipped := walkWithSkipped(t, fileSystem, root)

	assertPaths(t, paths, "open/movie.mkv")

	// Where ReadDir needs the entries' metadata (no d_type), the whole folder is unreadable;
	// otherwise only the file is. Either way its catalog entry is kept.
	searchable := "searchable/movie.mkv"
	if slices.Contains(skipped, "searchable") {
		searchable = "searchable"
	}

	assertPaths(t, skipped, "links/locked.mkv", "locked", searchable)

	records := logs()
	for _, path := range skipped {
		assertLogged(t, records, slog.LevelWarn, path)
	}

	assertNoAbsolutePathsLogged(t, records, filepath.Dir(root))
}

func TestWalkReportsARootThatBecomesUnreadableMidWalkAsUnavailable(t *testing.T) {
	t.Parallel()
	skipAsRoot(t)

	fileSystem, _ := newFileSystem(t)
	root, _ := newLibrary(t)
	writeFiles(t, root, "a.mkv", "b/movie.mkv", "c/movie.mkv")

	// A NAS share that drops mid-walk must not look like a library without "b" and "c":
	// the scan would delete their catalog entries.
	_, err := fileSystem.Walk(t.Context(), root, func(library.FileState) error {
		lock(t, root)
		return nil
	})

	if !errors.Is(err, domain.ErrUnavailable) {
		t.Errorf("Walk error = %v, want ErrUnavailable", err)
	}
}

func TestWalkReportsAnUnreadableRootAsUnavailable(t *testing.T) {
	t.Parallel()
	skipAsRoot(t)

	fileSystem, _ := newFileSystem(t)
	root, _ := newLibrary(t)
	writeFiles(t, root, "movie.mkv")
	lock(t, root)

	_, err := fileSystem.Walk(t.Context(), root, noVisit)
	if !errors.Is(err, domain.ErrUnavailable) {
		t.Errorf("Walk error = %v, want ErrUnavailable", err)
	}
}

func TestWalkSkipsSpecialFilesWithAVideoName(t *testing.T) {
	t.Parallel()

	fileSystem, _ := newFileSystem(t)
	root, _ := newLibrary(t)
	writeFiles(t, root, "movie.mkv")

	// Opening a FIFO blocks until a writer appears; the walker must never try.
	makeFifo(t, filepath.Join(root, "pipe.mkv"))

	assertPaths(t, walkPaths(t, fileSystem, root), "movie.mkv")
}

func TestResolveAndOpenRejectAFifoWithoutBlocking(t *testing.T) {
	t.Parallel()

	// A blocking open would hang until a writer appears; the test fails long before that.
	const timeout = 5 * time.Second

	fileSystem, _ := newFileSystem(t)
	root, _ := newLibrary(t)
	makeFifo(t, filepath.Join(root, "pipe.mkv"))

	done := make(chan struct{})

	go func() {
		defer close(done)

		assertAccessFailsWith(t, fileSystem, accessAttempt{root: root, relativePath: "pipe.mkv", want: domain.ErrNotFound})
	}()

	select {
	case <-done:
	case <-time.After(timeout):
		t.Fatalf("Resolve/Open of a FIFO did not return within %v", timeout)
	}
}

func skipAsRoot(t *testing.T) {
	t.Helper()

	if os.Geteuid() == 0 {
		t.Skip("root ignores directory permissions; run as an unprivileged user")
	}
}

// lock removes all permissions from a directory and restores them before t.TempDir cleanup.
func lock(t *testing.T, path string) {
	t.Helper()

	chmod(t, path, lockedDirMode)
}

// chmod changes the mode of a directory and restores it before t.TempDir cleanup.
func chmod(t *testing.T, path string, mode os.FileMode) {
	t.Helper()

	if err := os.Chmod(path, mode); err != nil {
		t.Fatalf("chmod %s: %v", path, err)
	}

	t.Cleanup(func() { _ = os.Chmod(path, testDirMode) })
}

// makeFifo creates a named pipe or skips the test where that is not possible.
func makeFifo(t *testing.T, path string) {
	t.Helper()

	if err := syscall.Mkfifo(path, testFileMode); err != nil {
		t.Skipf("mkfifo not supported here: %v", err)
	}
}
