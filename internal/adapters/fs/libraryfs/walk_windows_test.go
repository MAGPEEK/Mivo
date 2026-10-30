//go:build windows

package libraryfs_test

import (
	"errors"
	"log/slog"
	"path/filepath"
	"testing"

	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/library"
)

func TestWalkSkipsAndLogsJunctions(t *testing.T) {
	t.Parallel()

	fileSystem, logs := newFileSystem(t)
	root, outside := newLibrary(t)
	writeFiles(t, root, "real/movie.mkv")

	// A junction always stores an absolute target, which os.Root refuses even when the target
	// lies inside the root. Both are skipped; the files inside the root stay reachable directly.
	mklinkJunction(t, filepath.Join(root, "junction-out"), outside)
	mklinkJunction(t, filepath.Join(root, "nested", "junction-in"), filepath.Join(root, "real"))

	assertPaths(t, walkPaths(t, fileSystem, root), "real/movie.mkv")

	records := logs()
	assertLogged(t, records, slog.LevelInfo, "junction-out")
	assertLogged(t, records, slog.LevelInfo, "nested/junction-in")
	assertNoAbsolutePathsLogged(t, records, filepath.Dir(root))
}

func TestWalkLogsAndReportsADirectoryItsACLDoesNotLetUsList(t *testing.T) {
	t.Parallel()

	fileSystem, logs := newFileSystem(t)
	root, _ := newLibrary(t)
	writeFiles(t, root, "open/movie.mkv", "denied/movie.mkv")
	denyListing(t, filepath.Join(root, "denied"), listDirectory)

	paths, skipped := walkWithSkipped(t, fileSystem, root)

	assertPaths(t, paths, "open/movie.mkv")
	assertPaths(t, skipped, "denied")
	assertLogged(t, logs(), slog.LevelWarn, "denied")
	assertNoAbsolutePathsLogged(t, logs(), filepath.Dir(root))
}

func TestWalkReportsARootThatBecomesUnreadableMidWalkAsUnavailable(t *testing.T) {
	t.Parallel()

	fileSystem, _ := newFileSystem(t)
	root, _ := newLibrary(t)
	writeFiles(t, root, "a.mkv", "b/movie.mkv", "c/movie.mkv")

	denied := false

	_, err := fileSystem.Walk(t.Context(), root, func(library.FileState) error {
		if !denied {
			denied = true
			denyListing(t, root, listDirectoryInherited)
		}

		return nil
	})

	if !errors.Is(err, domain.ErrUnavailable) {
		t.Errorf("Walk error = %v, want ErrUnavailable", err)
	}
}

func TestWalkLogsAndSkipsNamesWindowsCannotOpenSafely(t *testing.T) {
	t.Parallel()

	fileSystem, logs := newFileSystem(t)
	root, _ := newLibrary(t)
	writeFiles(t, root, "ok/movie.mkv")

	// Directories whose names Win32 would turn into a device or strip to another name.
	for _, directory := range []string{"aux", "trailing-dot.", "trailing-space "} {
		writeVerbatimFile(t, root, directory, "movie.mkv")
	}

	assertPaths(t, walkPaths(t, fileSystem, root), "ok/movie.mkv")

	records := logs()
	for _, directory := range []string{"aux", "trailing-dot.", "trailing-space "} {
		assertLogged(t, records, slog.LevelWarn, directory)
	}
}
