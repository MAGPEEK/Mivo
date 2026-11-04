package sqlite_test

import (
	"slices"
	"testing"

	"github.com/magpeek/mivo/internal/domain/library"
)

// A library the sync inserts gets its only folder, keyed like library.FolderKey, and an entry
// in the import ledger; it is flagged as imported from the configuration. A repeated sync
// and a deleted library leave the ledger as it is.
func TestSyncLibrariesStoresTheFolderAndRecordsTheImport(t *testing.T) {
	t.Parallel()

	repository, _, inspection := newTestLibraryRepository(t)
	newID, _ := idSequence(firstLibraryID)
	definitions := []library.Definition{{Name: "Movies", Root: moviesRoot, Kind: library.KindMovies}}

	mustSync(t, repository, definitions, newID)

	folders := queryStrings(t, inspection, `SELECT id || '|' || library_id || '|' || path || '|' || path_key
		|| '|' || position FROM library_roots`)
	if want := []string{firstLibraryID + "|" + firstLibraryID + "|" + moviesRoot + "|media/movies|0"}; !slices.Equal(folders, want) {
		t.Errorf("folders = %q, want %q", folders, want)
	}

	imported := queryStrings(t, inspection, `SELECT imported_from_config FROM libraries`)
	if want := []string{"1"}; !slices.Equal(imported, want) {
		t.Errorf("imported_from_config = %q, want %q", imported, want)
	}

	mustSync(t, repository, nil, newID)

	if got := rowCount(t, inspection, "library_roots"); got != 0 {
		t.Errorf("folders after removing the library = %d, want 0", got)
	}

	if got := rowCount(t, inspection, "library_imports"); got != 1 {
		t.Errorf("ledger entries after removing the library = %d, want 1 (the import must not come back)", got)
	}
}
