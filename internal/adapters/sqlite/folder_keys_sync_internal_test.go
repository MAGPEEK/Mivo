package sqlite

import (
	"testing"
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

func TestFilesystemRootGetsANonEmptyKey(t *testing.T) {
	t.Parallel()

	for _, caseInsensitive := range []bool{false, true} {
		if got := folderKey("/", caseInsensitive); got != "/" {
			t.Errorf("folderKey(/, %v) = %q, want %q", caseInsensitive, got, "/")
		}
	}
}
