package sqlite

import (
	"testing"

	"github.com/magpeek/mivo/internal/domain/library"
)

// Ledger entries are re-keyed on their own: one without a folder is folded too, and an entry
// whose folded key another entry already holds is dropped.
func TestRekeyFoldersHandlesOrphanAndCollidingLedgerEntries(t *testing.T) {
	t.Parallel()

	db := openRekeyTestDB(t)

	statements := []string{
		`INSERT INTO library_imports VALUES ('C:\X', 'C:\X', 0)`,
		`INSERT INTO library_imports VALUES ('C:\Y', 'C:\Y', 0)`,
		`INSERT INTO library_imports VALUES ('c:/y', 'c:\y', 0)`,
	}
	for _, statement := range statements {
		if _, err := db.writer.ExecContext(t.Context(), statement); err != nil {
			t.Fatalf("%s: %v", statement, err)
		}
	}

	if err := db.rekeyFolders(t.Context(), true); err != nil {
		t.Fatalf("rekeyFolders error = %v", err)
	}

	const query = "SELECT group_concat(path_key, ',') FROM (SELECT path_key FROM library_imports ORDER BY path_key)"
	if got := queryOne(t, db, query); got != "c:/x,c:/y" {
		t.Errorf("ledger keys = %q, want %q", got, "c:/x,c:/y")
	}
}

// A root that differs from the stored one only in case is the same library on Windows: the
// sync keeps it instead of inserting a second folder with the same key.
func TestSyncLibrariesMatchesRootsByFolderKey(t *testing.T) {
	t.Parallel()

	db := openRekeyTestDB(t)
	repository := NewLibraryRepository(db, nil)
	repository.caseInsensitive = true

	ids := []string{"lib-1", "lib-2"}
	newID := func() (string, error) {
		id := ids[0]
		ids = ids[1:]

		return id, nil
	}

	for _, definition := range []library.Definition{
		{Name: "Movies", Root: "/Media", Kind: library.KindMovies},
		{Name: "Films", Root: "/media", Kind: library.KindMovies},
	} {
		result, err := repository.SyncLibraries(t.Context(), []library.Definition{definition}, newID)
		if err != nil {
			t.Fatalf("SyncLibraries(%q) error = %v", definition.Root, err)
		}

		if len(result.Removed) != 0 || len(result.Libraries) != 1 || result.Libraries[0].ID != "lib-1" {
			t.Fatalf("SyncLibraries(%q) = %+v, want library lib-1 kept and nothing removed", definition.Root, result)
		}
	}

	if got := queryOne(t, db, "SELECT name FROM libraries"); got != "Films" {
		t.Errorf("library name = %q, want %q", got, "Films")
	}

	_, err := repository.SyncLibraries(t.Context(), []library.Definition{
		{Name: "A", Root: "/A", Kind: library.KindMovies}, {Name: "B", Root: "/a", Kind: library.KindMovies},
	}, newID)
	if err == nil {
		t.Error("SyncLibraries with roots differing only in case error = nil, want an error")
	}
}

func TestFilesystemRootGetsANonEmptyKey(t *testing.T) {
	t.Parallel()

	for _, caseInsensitive := range []bool{false, true} {
		if got := folderKey("/", caseInsensitive); got != "/" {
			t.Errorf("folderKey(/, %v) = %q, want %q", caseInsensitive, got, "/")
		}
	}
}
