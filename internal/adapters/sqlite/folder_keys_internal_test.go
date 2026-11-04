package sqlite

import (
	"context"
	"path/filepath"
	"testing"
)

func openRekeyTestDB(t *testing.T) *DB {
	t.Helper()

	db, err := Open(t.Context(), Options{Path: filepath.Join(t.TempDir(), "server.db")})
	if err != nil {
		t.Fatalf("Open error = %v", err)
	}

	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("Close error = %v", err)
		}
	})

	if err := db.Migrate(t.Context()); err != nil {
		t.Fatalf("Migrate error = %v", err)
	}

	return db
}

func queryOne(t *testing.T, db *DB, query string) string {
	t.Helper()

	var value string
	if err := db.writer.QueryRowContext(t.Context(), query).Scan(&value); err != nil {
		t.Fatalf("%s: %v", query, err)
	}

	return value
}

// Carried-over folders are keyed without case folding; the Windows fix-up folds the key of
// the folder and of its ledger entry together and is idempotent.
func TestRekeyFoldersFoldsCarriedOverKeysAndTheLedger(t *testing.T) {
	t.Parallel()

	db := openRekeyTestDB(t)

	statements := []string{
		`INSERT INTO libraries (id, name, kind, created_at, updated_at) VALUES ('lib-1', 'Movies', 'movies', 0, 0)`,
		`INSERT INTO library_roots VALUES ('lib-1', 'lib-1', 'C:\Media\Movies', 'C:\Media\Movies', 0, 0)`,
		`INSERT INTO library_imports VALUES ('C:\Media\Movies', 'C:\Media\Movies', 0)`,
	}
	for _, statement := range statements {
		if _, err := db.writer.ExecContext(t.Context(), statement); err != nil {
			t.Fatalf("%s: %v", statement, err)
		}
	}

	for range 2 {
		if err := db.rekeyFolders(t.Context(), true); err != nil {
			t.Fatalf("rekeyFolders error = %v", err)
		}
	}

	const want = "c:/media/movies"
	if got := queryOne(t, db, "SELECT path_key FROM library_roots"); got != want {
		t.Errorf("folder key = %q, want %q", got, want)
	}

	if got := queryOne(t, db, "SELECT path_key FROM library_imports"); got != want {
		t.Errorf("ledger key = %q, want %q", got, want)
	}

	if err := db.RekeyFolders(t.Context()); err != nil {
		t.Errorf("RekeyFolders error = %v", err)
	}
}

func TestRekeyFoldersReportsDatabaseErrors(t *testing.T) {
	t.Parallel()

	db := openRekeyTestDB(t)

	if err := db.RekeyFolders(cancelledTestContext(t)); err == nil {
		t.Error("RekeyFolders with a cancelled context error = nil, want an error")
	}
}

func cancelledTestContext(t *testing.T) context.Context {
	t.Helper()

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	return ctx
}
