package sqlite_test

import (
	"errors"
	"testing"
	"time"

	"github.com/magpeek/mivo/internal/adapters/sqlite"
	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/library"
)

func TestScanLibrarySourceReturnsOneFolderLibrariesWithDefaultOptions(t *testing.T) {
	t.Parallel()

	db, _ := openMigratedTestDB(t)
	repository := sqlite.NewLibraryRepository(db, time.Now)
	newID, _ := idSequence(firstLibraryID)

	mustSync(t, repository, []library.Definition{{Name: "Movies", Root: moviesRoot, Kind: library.KindMovies}}, newID)

	source := sqlite.NewScanLibrarySource(db, time.Now)
	want := library.ManagedLibrary{
		ID: firstLibraryID, Name: "Movies", Kind: library.KindMovies,
		Folders: []library.Folder{{ID: firstLibraryID, Path: moviesRoot}}, Options: library.DefaultOptions(),
	}

	got, err := source.GetLibrary(t.Context(), firstLibraryID)
	if err != nil {
		t.Fatalf("GetLibrary() error = %v", err)
	}

	assertDeepEqual(t, "library", got, want)

	listed, err := source.ListLibraries(t.Context())
	if err != nil {
		t.Fatalf("ListLibraries() error = %v", err)
	}

	assertDeepEqual(t, "libraries", listed, []library.ManagedLibrary{want})

	if _, err := source.GetLibrary(t.Context(), "unknown"); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("GetLibrary(unknown) error = %v, want ErrNotFound", err)
	}
}
