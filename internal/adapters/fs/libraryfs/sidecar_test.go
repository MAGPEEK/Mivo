package libraryfs_test

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/library"
)

const (
	// nfdName is "Amélie (2001)" with a decomposed é (e + combining acute), as macOS writes it.
	nfdName = "Amélie (2001)"
	// nfcName is the same name in NFC.
	nfcName      = "Amélie (2001)"
	sidecarLimit = 1024
)

func TestWalkReportsSidecarClassesAndNormalizedPaths(t *testing.T) {
	t.Parallel()

	fileSystem, _ := newFileSystem(t)
	root, _ := newLibrary(t)

	writeFiles(t, root,
		nfdName+"/"+nfdName+".mkv",
		nfdName+"/movie.nfo",
		nfdName+"/poster.JPG",
		"Show/tvshow.nfo",
		"Show/Season 1/S01E01-thumb.png",
		"Show/fanart.webp",
		"Show/notes.txt",
	)

	want := map[string]library.FileState{
		nfdName + "/" + nfdName + ".mkv": {Class: library.FileClassVideo, NormalizedPath: nfcName + "/" + nfcName + ".mkv"},
		nfdName + "/movie.nfo":           {Class: library.FileClassNFO, NormalizedPath: nfcName + "/movie.nfo"},
		nfdName + "/poster.JPG":          {Class: library.FileClassImage, NormalizedPath: nfcName + "/poster.JPG"},
		"Show/tvshow.nfo":                {Class: library.FileClassNFO},
		"Show/Season 1/S01E01-thumb.png": {Class: library.FileClassImage},
		"Show/fanart.webp":               {Class: library.FileClassImage},
	}

	got := map[string]library.FileState{}

	if _, err := fileSystem.Walk(t.Context(), root, func(state library.FileState) error {
		got[state.RelativePath] = state
		return nil
	}); err != nil {
		t.Fatalf("Walk error = %v", err)
	}

	if len(got) != len(want) {
		t.Errorf("walked %d files, want %d: %v", len(got), len(want), got)
	}

	for path, expected := range want {
		state, found := got[path]
		if !found {
			t.Errorf("%q not reported", path)
			continue
		}

		if state.Class != expected.Class || state.NormalizedPath != expected.NormalizedPath {
			t.Errorf("%q: class %q, normalized %q, want %q, %q",
				path, state.Class, state.NormalizedPath, expected.Class, expected.NormalizedPath)
		}
	}
}

func TestWalkReportsLinkedSidecarsInsideTheRootOnly(t *testing.T) {
	t.Parallel()

	fileSystem, _ := newFileSystem(t)
	root, outside := newLibrary(t)

	writeFile(t, filepath.Join(outside, "poster.jpg"), "secret")
	writeFiles(t, root, "Movie/movie.nfo")
	symlinkOrSkip(t, filepath.Join(outside, "poster.jpg"), filepath.Join(root, "Movie", "poster.jpg"))
	// Relative: os.Root refuses absolute link targets even when they point inside the root.
	symlinkOrSkip(t, "movie.nfo", filepath.Join(root, "Movie", "linked.nfo"))

	assertPaths(t, walkPaths(t, fileSystem, root), "Movie/movie.nfo", "Movie/linked.nfo")
}

func TestReadSidecarReturnsTheContent(t *testing.T) {
	t.Parallel()

	fileSystem, _ := newFileSystem(t)
	root, _ := newLibrary(t)

	content := strings.Repeat("<movie/>", 20000) // larger than one read chunk
	writeFile(t, filepath.Join(root, nfdName, "movie.nfo"), content)

	got, err := fileSystem.ReadSidecar(t.Context(), root, nfdName+"/movie.nfo", int64(len(content)))
	if err != nil {
		t.Fatalf("ReadSidecar error = %v", err)
	}

	if string(got) != content {
		t.Errorf("ReadSidecar returned %d bytes, want %d", len(got), len(content))
	}
}

func TestReadSidecarRefusals(t *testing.T) {
	t.Parallel()

	fileSystem, _ := newFileSystem(t)
	root, _ := newLibrary(t)

	writeFile(t, filepath.Join(root, "big.nfo"), strings.Repeat("x", sidecarLimit+1))
	mkdir(t, filepath.Join(root, "folder.nfo"))

	cancelled, cancel := context.WithCancel(t.Context())
	cancel()

	tests := []struct {
		name         string
		ctx          context.Context
		relativePath string
		limit        int64
		want         error
	}{
		{name: "larger than the limit", ctx: t.Context(), relativePath: "big.nfo", limit: sidecarLimit, want: domain.ErrUnsupported},
		{name: "vanished", ctx: t.Context(), relativePath: "gone.nfo", limit: sidecarLimit, want: domain.ErrNotFound},
		{name: "directory", ctx: t.Context(), relativePath: "folder.nfo", limit: sidecarLimit, want: domain.ErrNotFound},
		{name: "leaves the root", ctx: t.Context(), relativePath: "../outside/secret.mkv", limit: sidecarLimit, want: domain.ErrForbidden},
		{name: "zero limit", ctx: t.Context(), relativePath: "big.nfo", limit: 0, want: domain.ErrInvalid},
		{name: "cancelled", ctx: cancelled, relativePath: "big.nfo", limit: sidecarLimit * 2, want: context.Canceled},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			content, err := fileSystem.ReadSidecar(test.ctx, root, test.relativePath, test.limit)
			if !errors.Is(err, test.want) || content != nil {
				t.Errorf("ReadSidecar(%q) = %d bytes, %v, want %v", test.relativePath, len(content), err, test.want)
			}
		})
	}
}

func TestReadSidecarRefusesASymlinkLeavingTheRoot(t *testing.T) {
	t.Parallel()

	fileSystem, _ := newFileSystem(t)
	root, outside := newLibrary(t)

	writeFile(t, filepath.Join(outside, "poster.jpg"), "secret")
	symlinkOrSkip(t, filepath.Join(outside, "poster.jpg"), filepath.Join(root, "poster.jpg"))

	content, err := fileSystem.ReadSidecar(t.Context(), root, "poster.jpg", sidecarLimit)
	if !errors.Is(err, domain.ErrForbidden) || content != nil {
		t.Errorf("ReadSidecar(link outside) = %q, %v, want ErrForbidden", content, err)
	}
}
