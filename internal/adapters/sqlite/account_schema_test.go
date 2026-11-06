package sqlite_test

import (
	"os"
	"testing"

	"github.com/pressly/goose/v3"
)

// accountsSchemaVersion is the migration that creates the account tables.
const accountsSchemaVersion = 11

// accountTables returns the tables migration 00011 creates, in creation order.
func accountTables() []string {
	return []string{"users", "profiles", "user_libraries", "profile_libraries", "auth_sessions"}
}

// Rolling back 00011 alone removes exactly the account tables and leaves the catalog.
func TestAccountMigrationRollsBackAndReapplies(t *testing.T) {
	t.Parallel()

	db, path := openMigratedTestDB(t)
	inspection := openInspectionDB(t, path)

	provider, err := goose.NewProvider(goose.DialectSQLite3, inspection, os.DirFS("migrations"),
		goose.WithDisableGlobalRegistry(true))
	if err != nil {
		t.Fatalf("NewProvider error = %v", err)
	}

	if _, err := provider.DownTo(t.Context(), accountsSchemaVersion-1); err != nil {
		t.Fatalf("DownTo(%d) error = %v", accountsSchemaVersion-1, err)
	}

	for _, table := range accountTables() {
		if tableExists(t, inspection, table) {
			t.Errorf("table %s exists after rolling back 00011", table)
		}
	}

	if !tableExists(t, inspection, "libraries") {
		t.Error("table libraries is gone after rolling back 00011")
	}

	if err := db.Migrate(t.Context()); err != nil {
		t.Fatalf("Migrate after rollback error = %v", err)
	}

	for _, table := range accountTables() {
		if !tableExists(t, inspection, table) {
			t.Errorf("table %s missing after re-applying 00011", table)
		}
	}
}

// The partial unique index allows one primary profile per account and any number of
// managed ones.
func TestAccountSchemaAllowsOnePrimaryProfilePerAccount(t *testing.T) {
	t.Parallel()

	_, path := openMigratedTestDB(t)
	inspection := openInspectionDB(t, path)
	ctx := t.Context()

	execute(t, inspection, `INSERT INTO users (id, name, name_key, role, password_hash, created_at, updated_at)
		VALUES ('u1', 'Anna', 'anna', 'admin', 'hash', 0, 0)`)

	insertProfile := `INSERT INTO profiles (id, user_id, name, is_primary, avatar_color, created_at, updated_at)
		VALUES (?, 'u1', 'P', ?, 'coral', 0, 0)`
	execute(t, inspection, insertProfile, "p1", 1)
	execute(t, inspection, insertProfile, "p2", 0)
	execute(t, inspection, insertProfile, "p3", 0)

	if _, err := inspection.ExecContext(ctx, insertProfile, "p4", 1); err == nil {
		t.Error("second primary profile was accepted, want a unique violation")
	}
}
