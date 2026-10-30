package sqlite_test

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"testing"

	"github.com/pressly/goose/v3"
)

// latestSchemaVersion is the version of the newest file in migrations/.
const latestSchemaVersion = 2

// schemaTables returns all tables the migrations create, in creation order.
func schemaTables() []string {
	return append([]string{"server_identity"}, catalogTables()...)
}

func TestMigrateCreatesTheSchemaAndASecondRunIsANoOp(t *testing.T) {
	t.Parallel()

	logger, records := newRecordingLogger()
	db, path := openTestDB(t, logger)

	for run := 1; run <= 2; run++ {
		if err := db.Migrate(t.Context()); err != nil {
			t.Fatalf("Migrate run %d error = %v", run, err)
		}
	}

	inspection := openInspectionDB(t, path)

	if got := schemaVersion(t, inspection); got != latestSchemaVersion {
		t.Errorf("schema version = %d, want %d", got, latestSchemaVersion)
	}

	if got := appliedMigrationRows(t, inspection); got != latestSchemaVersion {
		t.Errorf("applied migration rows = %d, want %d (second run must not re-apply)", got, latestSchemaVersion)
	}

	assertTablesExist(t, inspection, true)

	// Only the first run applies something, and it logs each applied migration once.
	if got := len(records()); got != latestSchemaVersion {
		t.Errorf("log records = %+v, want %d applied-migration records", records(), latestSchemaVersion)
	}
}

func TestMigrateFailsWithACancelledContext(t *testing.T) {
	t.Parallel()

	db, _ := openTestDB(t, nil)

	err := db.Migrate(cancelledContext(t))
	if !errors.Is(err, context.Canceled) {
		t.Errorf("Migrate error = %v, want context.Canceled", err)
	}
}

func TestServerIdentityTableHoldsASingleRow(t *testing.T) {
	t.Parallel()

	_, path := openMigratedTestDB(t)
	inspection := openInspectionDB(t, path)

	_, err := inspection.ExecContext(t.Context(),
		"INSERT INTO server_identity (id, server_id, created_at) VALUES (2, 'x', 0)")
	if err == nil {
		t.Error("insert with id 2 succeeded, want the CHECK (id = 1) constraint to reject it")
	}
}

// The Down sections exist for development (ADR 0004). goose runs here on the source files,
// independent of the adapter, and must remove what Up created; the adapter's Migrate then
// rebuilds the same schema (up, down, up).
func TestMigrationsCanBeRolledBackAndReapplied(t *testing.T) {
	t.Parallel()

	db, path := openMigratedTestDB(t)
	inspection := openInspectionDB(t, path)

	provider, err := goose.NewProvider(goose.DialectSQLite3, inspection, os.DirFS("migrations"),
		goose.WithDisableGlobalRegistry(true))
	if err != nil {
		t.Fatalf("NewProvider error = %v", err)
	}

	if _, err := provider.DownTo(t.Context(), 0); err != nil {
		t.Fatalf("DownTo(0) error = %v", err)
	}

	assertTablesExist(t, inspection, false)

	if got := userIndexCount(t, inspection); got != 0 {
		t.Errorf("indexes after rolling back all migrations = %d, want 0", got)
	}

	if err := db.Migrate(t.Context()); err != nil {
		t.Fatalf("Migrate after rollback error = %v", err)
	}

	assertTablesExist(t, inspection, true)

	if got := schemaVersion(t, inspection); got != latestSchemaVersion {
		t.Errorf("schema version after re-applying = %d, want %d", got, latestSchemaVersion)
	}
}

func assertTablesExist(t *testing.T, inspection *sql.DB, want bool) {
	t.Helper()

	for _, table := range schemaTables() {
		if got := tableExists(t, inspection, table); got != want {
			t.Errorf("table %s exists = %t, want %t", table, got, want)
		}
	}
}

func schemaVersion(t *testing.T, inspection *sql.DB) int64 {
	t.Helper()

	var version int64

	err := inspection.QueryRowContext(t.Context(),
		"SELECT MAX(version_id) FROM goose_db_version WHERE is_applied").Scan(&version)
	if err != nil {
		t.Fatalf("read schema version: %v", err)
	}

	return version
}

func appliedMigrationRows(t *testing.T, inspection *sql.DB) int {
	t.Helper()

	var count int

	err := inspection.QueryRowContext(t.Context(),
		"SELECT COUNT(*) FROM goose_db_version WHERE version_id > 0").Scan(&count)
	if err != nil {
		t.Fatalf("count applied migrations: %v", err)
	}

	return count
}

func tableExists(t *testing.T, inspection *sql.DB, table string) bool {
	t.Helper()

	var count int

	err := inspection.QueryRowContext(t.Context(),
		"SELECT COUNT(*) FROM sqlite_schema WHERE type = 'table' AND name = ?", table).Scan(&count)
	if err != nil {
		t.Fatalf("look up table %s: %v", table, err)
	}

	return count == 1
}

// userIndexCount counts indexes the migrations created; SQLite's own autoindexes and those
// of goose's version table are not counted.
func userIndexCount(t *testing.T, inspection *sql.DB) int {
	t.Helper()

	var count int

	err := inspection.QueryRowContext(t.Context(), `
		SELECT COUNT(*) FROM sqlite_schema
		WHERE type = 'index' AND name NOT LIKE 'sqlite_autoindex_%' AND tbl_name <> 'goose_db_version'`).Scan(&count)
	if err != nil {
		t.Fatalf("count indexes: %v", err)
	}

	return count
}
