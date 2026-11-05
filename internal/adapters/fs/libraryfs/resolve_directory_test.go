package libraryfs_test

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/magpeek/mivo/internal/domain"
)

func TestResolveDirectoryReturnsTheEvaluatedDiscFolder(t *testing.T) {
	t.Parallel()

	fileSystem, _ := newFileSystem(t)
	root, _ := newLibrary(t)
	writeFiles(t, root, "Heat (1995)/VIDEO_TS/VIDEO_TS.IFO")

	got, err := fileSystem.ResolveDirectory(root, "Heat (1995)")
	if err != nil {
		t.Fatalf("ResolveDirectory error = %v", err)
	}

	if want := evalSymlinks(t, filepath.Join(root, "Heat (1995)")); got != want {
		t.Errorf("ResolveDirectory = %q, want %q", got, want)
	}
}

func TestResolveDirectoryRefusesFilesMissingAndEscapingPaths(t *testing.T) {
	t.Parallel()

	fileSystem, _ := newFileSystem(t)
	root, _ := newLibrary(t)
	writeFiles(t, root, "Heat (1995)/VIDEO_TS/VIDEO_TS.IFO")
	symlinkOrSkip(t, filepath.Join("..", "outside"), filepath.Join(root, "out"))

	tests := []struct {
		relativePath string
		want         error
	}{
		{relativePath: "Heat (1995)/VIDEO_TS/VIDEO_TS.IFO", want: domain.ErrNotFound},
		{relativePath: "Missing (2000)", want: domain.ErrNotFound},
		{relativePath: "../outside", want: domain.ErrForbidden},
		{relativePath: "out", want: domain.ErrForbidden},
	}

	for _, test := range tests {
		if got, err := fileSystem.ResolveDirectory(root, test.relativePath); !errors.Is(err, test.want) {
			t.Errorf("ResolveDirectory(%q) = (%q, %v), want %v", test.relativePath, got, err, test.want)
		}
	}
}
