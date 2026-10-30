//go:build windows

package libraryfs_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/magpeek/mivo/internal/domain"
)

func TestResolveAndOpenRejectWindowsDeviceNamesAndAliases(t *testing.T) {
	t.Parallel()

	root, _ := newLibrary(t)
	writeFiles(t, root, "Movies/movie.mkv", "Movies/clip.mp4")

	// The stream exists, so only the path rule can stop the access.
	stream := filepath.Join(root, "Movies", "movie.mkv:stream")
	if err := os.WriteFile(stream, []byte("hidden"), testFileMode); err != nil {
		t.Fatalf("write alternate data stream %s: %v", stream, err)
	}

	for _, relativePath := range []string{
		// Reserved device names, also with trailing spaces and inside folders.
		"NUL", "nul", "CON", "AUX", "PRN", "COM1", "LPT9", "CONIN$", "CONOUT$", "NUL ", "Movies/aux", "COM1/movie.mkv",
		// Trailing dots and spaces, which Win32 strips so that the name aliases another file.
		"Movies/movie.mkv.", "Movies/movie.mkv ", "Movies/movie.mkv. .", "Movies./movie.mkv", "Movies /movie.mkv",
		// Alternate data streams.
		"Movies/movie.mkv:stream", "Movies/movie.mkv::$DATA", "Movies/movie.mkv:stream:$DATA", "Movies:x/movie.mkv",
	} {
		t.Run(relativePath, func(t *testing.T) {
			t.Parallel()

			assertAccessFails(t, root, relativePath, domain.ErrForbidden)
		})
	}
}

func TestResolveAndOpenRefuseJunctions(t *testing.T) {
	t.Parallel()

	root, outside := newLibrary(t)
	writeFiles(t, root, "real/movie.mkv")
	mklinkJunction(t, filepath.Join(root, "junction-out"), outside)
	mklinkJunction(t, filepath.Join(root, "junction-in"), filepath.Join(root, "real"))

	for _, relativePath := range []string{"junction-out/secret.mkv", "junction-in/movie.mkv"} {
		t.Run(relativePath, func(t *testing.T) {
			t.Parallel()

			assertAccessFails(t, root, relativePath, domain.ErrForbidden)
		})
	}
}

func TestResolveComparesTheRootCaseInsensitively(t *testing.T) {
	t.Parallel()

	fileSystem, _ := newFileSystem(t)
	root, _ := newLibrary(t)
	writeFiles(t, root, "Movies/movie.mkv")

	want := evalSymlinks(t, filepath.Join(root, "Movies", "movie.mkv"))

	for _, libraryRoot := range []string{strings.ToUpper(root), strings.ToLower(root)} {
		got, err := fileSystem.Resolve(libraryRoot, "MOVIES/Movie.MKV")
		if err != nil {
			t.Errorf("Resolve(%s) error = %v", libraryRoot, err)
			continue
		}

		if !strings.EqualFold(got, want) {
			t.Errorf("Resolve(%s) = %q, want %q", libraryRoot, got, want)
		}
	}
}

func TestResolveAcceptsAReservedNameWithAnExtensionOnlyWhereWindowsAllowsIt(t *testing.T) {
	t.Parallel()

	fileSystem, _ := newFileSystem(t)
	root, _ := newLibrary(t)

	// Windows 11 no longer treats "CON.mkv" as a device; older versions do. Whatever the OS
	// says, Resolve must never return a device path.
	writeVerbatimFile(t, root, "folder", "CON.mkv")

	got, err := fileSystem.Resolve(root, "folder/CON.mkv")
	if err == nil && !strings.HasSuffix(got, `\folder\CON.mkv`) {
		t.Errorf("Resolve = %q, want a path to the file folder\\CON.mkv", got)
	}
}
