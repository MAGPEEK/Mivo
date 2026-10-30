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
const latestSchemaVersion = 1

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

	if !tableExists(t, inspection, "server_identity") {
		t.Error("table server_identity does not exist after Migrate")
	}

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

// The Down section exists for development (ADR 0004). goose runs here on the source files,
// independent of the adapter, and must remove what Up created.
func TestMigrationsCanBeRolledBack(t *testing.T) {
	t.Parallel()

	_, path := openMigratedTestDB(t)
	inspection := openInspectionDB(t, path)

	provider, err := goose.NewProvider(goose.DialectSQLite3, inspection, os.DirFS("migrations"),
		goose.WithDisableGlobalRegistry(true))
	if err != nil {
		t.Fatalf("NewProvider error = %v", err)
	}

	if _, err := provider.DownTo(t.Context(), 0); err != nil {
		t.Fatalf("DownTo(0) error = %v", err)
	}

	if tableExists(t, inspection, "server_identity") {
		t.Error("table server_identity still exists after rolling back all migrations")
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
