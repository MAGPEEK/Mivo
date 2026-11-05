package libraryfs_test

import (
	"context"
	"errors"
	"path/filepath"
	"slices"
	"testing"

	"github.com/magpeek/mivo/internal/domain"
)

func TestListDirectoriesListsOnlyVisibleDirectories(t *testing.T) {
	t.Parallel()

	fileSystem, _ := newFileSystem(t)
	root, _ := newLibrary(t)
	writeFiles(t, root, "Movies/Up (2009)/Up.mkv", "Movies/Alien/Alien.mkv", "Movies/file.mkv", "Shows/x.mkv",
		"Movies/.hidden/a.mkv", "Movies/@eaDir/thumb.jpg")

	tests := []struct {
		directory string
		want      []string
	}{
		{directory: "", want: []string{"Movies", "Shows"}},
		{directory: "Movies", want: []string{"Alien", "Up (2009)"}},
		{directory: "Movies/Alien"},
	}
	for _, test := range tests {
		names, err := fileSystem.ListDirectories(t.Context(), root, test.directory)
		if err != nil {
			t.Fatalf("ListDirectories(%q) error = %v", test.directory, err)
		}

		slices.Sort(names)

		if !slices.Equal(names, test.want) {
			t.Errorf("ListDirectories(%q) = %v, want %v", test.directory, names, test.want)
		}
	}
}

func TestListDirectoriesRefusesPathsOutsideTheRoot(t *testing.T) {
	t.Parallel()

	fileSystem, _ := newFileSystem(t)
	root, _ := newLibrary(t)
	writeFiles(t, root, "Movies/file.mkv")

	tests := []struct {
		directory string
		want      error
	}{
		{directory: "../outside", want: domain.ErrForbidden},
		{directory: "Missing", want: domain.ErrNotFound},
		{directory: "Movies/file.mkv", want: domain.ErrNotFound},
		{directory: "/etc", want: domain.ErrForbidden},
	}
	for _, test := range tests {
		_, err := fileSystem.ListDirectories(t.Context(), root, test.directory)
		if !errors.Is(err, test.want) {
			t.Errorf("ListDirectories(%q) error = %v, want %v", test.directory, err, test.want)
		}
	}

	_, err := fileSystem.ListDirectories(t.Context(), filepath.Join(root, "gone"), "")
	if !errors.Is(err, domain.ErrUnavailable) {
		t.Errorf("missing root: error = %v, want domain.ErrUnavailable", err)
	}

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	if _, err := fileSystem.ListDirectories(ctx, root, ""); !errors.Is(err, context.Canceled) {
		t.Errorf("cancelled: error = %v, want context.Canceled", err)
	}
}

func TestListDirectoriesConfinesSymlinksToTheRoot(t *testing.T) {
	t.Parallel()

	fileSystem, _ := newFileSystem(t)
	root, outside := newLibrary(t)
	writeFiles(t, root, "Movies/Alien/Alien.mkv")
	symlinkOrSkip(t, outside, filepath.Join(root, "Escape"))
	// A relative target: os.Root, like the scan, follows only links that resolve inside it.
	symlinkOrSkip(t, "Movies", filepath.Join(root, "Inside"))

	names, err := fileSystem.ListDirectories(t.Context(), root, "")
	if err != nil {
		t.Fatalf("ListDirectories error = %v", err)
	}

	slices.Sort(names)

	if !slices.Equal(names, []string{"Inside", "Movies"}) {
		t.Errorf("ListDirectories = %v, want the link leaving the root left out", names)
	}

	if _, err := fileSystem.ListDirectories(t.Context(), root, "Escape"); !errors.Is(err, domain.ErrForbidden) {
		t.Errorf("Escape: error = %v, want domain.ErrForbidden", err)
	}

	inside, err := fileSystem.ListDirectories(t.Context(), root, "Inside")
	if err != nil || !slices.Equal(inside, []string{"Alien"}) {
		t.Errorf("Inside = %v, %v; want the linked directory inside the root", inside, err)
	}
}
