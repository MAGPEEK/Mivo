package sqlite_test

import (
	"errors"
	"testing"
	"time"

	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/library"
)

func sidecar(path string, class library.FileClass, size int64) library.FileState {
	return library.FileState{
		RelativePath: path, Class: class, Size: size, ModifiedAt: time.UnixMilli(fileModifiedMilli).UTC(),
	}
}

func TestSidecarStatesAreIdempotent(t *testing.T) {
	t.Parallel()

	fixture := newPlacementFixture(t)
	nfo := sidecar("M/movie.nfo", library.FileClassNFO, 10)
	poster := sidecar("M/poster.jpg", library.FileClassImage, 20)

	save := func(libraryID string, upserts []library.FileState, removals []string) {
		t.Helper()

		if err := fixture.catalog.SaveSidecarStates(t.Context(), libraryID, upserts, removals); err != nil {
			t.Fatalf("SaveSidecarStates error = %v", err)
		}
	}

	save(moviesLibraryID, []library.FileState{poster, nfo}, nil)
	save(moviesLibraryID, []library.FileState{poster, nfo}, nil)

	got, err := fixture.catalog.SidecarStates(t.Context(), moviesLibraryID)
	if err != nil {
		t.Fatal(err)
	}

	assertDeepEqual(t, "states", got, []library.FileState{nfo, poster})

	nfo.Size = 11
	save(moviesLibraryID, []library.FileState{nfo}, []string{"M/poster.jpg", "M/missing.nfo"})
	save(moviesLibraryID, nil, []string{"M/poster.jpg"})
	// The other library's call must not delete this library's path.
	save(showsLibraryID, nil, []string{"M/movie.nfo"})

	got, err = fixture.catalog.SidecarStates(t.Context(), moviesLibraryID)
	if err != nil {
		t.Fatal(err)
	}

	assertDeepEqual(t, "states after update", got, []library.FileState{nfo})

	if other, err := fixture.catalog.SidecarStates(t.Context(), showsLibraryID); err != nil || len(other) != 0 {
		t.Errorf("shows library = %v, %v; want none", other, err)
	}
}

func TestSaveSidecarStatesErrors(t *testing.T) {
	t.Parallel()

	fixture := newPlacementFixture(t)

	tests := []struct {
		name      string
		libraryID string
		state     library.FileState
		want      error
	}{
		{"unknown library", unknownID, sidecar("a.nfo", library.FileClassNFO, 1), domain.ErrNotFound},
		{"video class", moviesLibraryID, sidecar("a.mkv", library.FileClassVideo, 1), domain.ErrInvalid},
		{"escaping path", moviesLibraryID, sidecar("../a.nfo", library.FileClassNFO, 1), domain.ErrInvalid},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := fixture.catalog.SaveSidecarStates(t.Context(), test.libraryID, []library.FileState{test.state}, nil)
			if !errors.Is(err, test.want) {
				t.Errorf("error = %v, want %v", err, test.want)
			}
		})
	}
}
