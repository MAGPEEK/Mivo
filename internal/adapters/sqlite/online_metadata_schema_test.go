package sqlite_test

import (
	"database/sql"
	"fmt"
	"slices"
	"strings"
	"testing"
)

// libraryManagementVersion is the schema version of library management (00004).
const libraryManagementVersion = 4

// onlineMetadataTables returns the tables migration 00005 adds.
func onlineMetadataTables() []string {
	return []string{"item_match", "provider_cache", "enrichment_queue"}
}

func onlineMetadataConstraintCases() []catalogConstraintCase {
	const (
		insertMatch = `INSERT INTO item_match (item_id, state, provider, confidence, matched_at) VALUES `
		insertCache = `INSERT INTO provider_cache (provider, cache_key, payload, fetched_at, expires_at, not_found) VALUES `
		insertJob   = `INSERT INTO enrichment_queue (item_id, reason, not_before, enqueued_at) VALUES `
	)

	return []catalogConstraintCase{
		{name: "match of a missing item", statement: insertMatch + `('item-9', 'unmatched', '', 0, NULL)`},
		{name: "unknown match state", statement: insertMatch + `('item-1', 'maybe', '', 0, NULL)`},
		{name: "match without provider", statement: insertMatch + `('item-1', 'matched', '', 1, 5)`},
		{name: "match without time", statement: insertMatch + `('item-1', 'matched', 'tmdb', 1, NULL)`},
		{name: "confidence above one", statement: insertMatch + `('item-1', 'unmatched', '', 1.5, NULL)`},
		{name: "negative confidence", statement: insertMatch + `('item-1', 'unmatched', '', -0.5, NULL)`},
		{name: "cache without provider", statement: insertCache + `('', 'k', '{}', 0, 0, 0)`},
		{name: "cache without key", statement: insertCache + `('tmdb', '', '{}', 0, 0, 0)`},
		{name: "cache payload not JSON", statement: insertCache + `('tmdb', 'k', '{', 0, 0, 0)`},
		{name: "found entry without payload", statement: insertCache + `('tmdb', 'k', '', 0, 0, 0)`},
		{name: "not-found entry with payload", statement: insertCache + `('tmdb', 'k', '{}', 0, 0, 1)`},
		{name: "not-found flag other than 0 or 1", statement: insertCache + `('tmdb', 'k', '{}', 0, 0, 2)`},
		{name: "expiry before the fetch", statement: insertCache + `('tmdb', 'k', '{}', 5, 4, 0)`},
		{name: "duplicate cache entry", statement: insertCache + `('tmdb', 'k', '{}', 0, 0, 0), ('tmdb', 'k', '[]', 0, 0, 0)`},
		{name: "job of a missing item", statement: insertJob + `('item-9', 'scan', 0, 0)`},
		{name: "unknown job reason", statement: insertJob + `('item-1', 'manual', 0, 0)`},
		{name: "second job of an item", statement: insertJob + `('item-1', 'scan', 0, 0), ('item-1', 'refresh', 0, 0)`},
		{name: "negative attempts", statement: `INSERT INTO enrichment_queue (item_id, reason, not_before, enqueued_at, attempts)
			VALUES ('item-1', 'scan', 0, 0, -1)`},
		{name: "online metadata other than 0 or 1", statement: `UPDATE libraries SET online_metadata = 2`},
	}
}

func TestOnlineMetadataConstraintsRejectInvalidRows(t *testing.T) {
	t.Parallel()

	for _, test := range onlineMetadataConstraintCases() {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			inspection := openCatalogV2Fixture(t)

			if _, err := inspection.ExecContext(t.Context(), test.statement); err == nil {
				t.Errorf("statement accepted, want a constraint error:\n%s", test.statement)
			}
		})
	}
}

// Deleting an item removes its match and its queued job.
func TestDeletingAnItemCascadesToItsMatchAndJob(t *testing.T) {
	t.Parallel()

	inspection := openCatalogV2Fixture(t)
	execute(t, inspection, `INSERT INTO item_match VALUES ('item-1', 'matched', 'tmdb', 0.9, 5), ('show-1', 'disabled', '', 0, NULL)`)
	execute(t, inspection, `INSERT INTO enrichment_queue (item_id, reason, not_before, enqueued_at)
		VALUES ('item-1', 'scan', 0, 0), ('show-1', 'periodic', 0, 0)`)
	execute(t, inspection, `INSERT INTO provider_cache VALUES ('tmdb', 'movie:603', '{}', 0, 1, 0)`)

	execute(t, inspection, "DELETE FROM items WHERE id = 'item-1'")

	want := map[string]int{"item_match": 1, "enrichment_queue": 1, "provider_cache": 1}
	for table, count := range want {
		if got := rowCount(t, inspection, table); got != count {
			t.Errorf("rows of %s after deleting the item = %d, want %d", table, got, count)
		}
	}
}

// seedVersion4 migrates a database to 00004 and stores the catalog fixture rows.
func seedVersion4(t *testing.T) (*sql.DB, string) {
	t.Helper()

	_, path := openTestDB(t, nil)
	inspection := openForeignKeyInspectionDB(t, path)

	if _, err := newSourceMigrationProvider(t, inspection).UpTo(t.Context(), libraryManagementVersion); err != nil {
		t.Fatalf("UpTo(%d) error = %v", libraryManagementVersion, err)
	}

	for _, statement := range slices.Concat(constraintFixture(), catalogV2Fixture()) {
		execute(t, inspection, statement)
	}

	return inspection, path
}

// Migration 00005 leaves every existing row as it was and switches online metadata on for
// every library (ADR 0031 §2).
func TestMigrateTo00005KeepsExistingData(t *testing.T) {
	t.Parallel()

	inspection, _ := seedVersion4(t)
	before := dumpVersion4Rows(t, inspection)

	if _, err := newSourceMigrationProvider(t, inspection).Up(t.Context()); err != nil {
		t.Fatalf("Up error = %v", err)
	}

	if after := dumpVersion4Rows(t, inspection); !slices.Equal(after, before) {
		t.Errorf("rows after 00005 =\n%s\nwant\n%s", strings.Join(after, "\n"), strings.Join(before, "\n"))
	}

	rows := queryStrings(t, inspection, "SELECT id || '|' || online_metadata FROM libraries ORDER BY id")
	if want := []string{"lib-1|1", "lib-2|1"}; !slices.Equal(rows, want) {
		t.Errorf("online_metadata after 00005 = %q, want %q", rows, want)
	}
}

func TestMigrateFromVersion3SwitchesOnlineMetadataOnForCarriedLibraries(t *testing.T) {
	t.Parallel()

	db, inspection := seedVersion3(t)

	if err := db.Migrate(t.Context()); err != nil {
		t.Fatalf("Migrate error = %v", err)
	}

	rows := queryStrings(t, inspection, "SELECT id || '|' || online_metadata FROM libraries ORDER BY id")
	if want := []string{"lib-1|1", "lib-2|1", "lib-3|1"}; !slices.Equal(rows, want) {
		t.Errorf("online_metadata after the upgrade = %q, want %q", rows, want)
	}

	for _, table := range onlineMetadataTables() {
		if got := rowCount(t, inspection, table); got != 0 {
			t.Errorf("rows of %s after the upgrade = %d, want 0", table, got)
		}
	}
}

// goose down from 00005 restores exactly the 00004 schema and keeps the libraries.
func TestMigrationDownToVersion4RestoresTheVersion4Schema(t *testing.T) {
	t.Parallel()

	version4, _ := seedVersion4(t)
	inspection := openCatalogV2Fixture(t)
	execute(t, inspection, `INSERT INTO item_match VALUES ('item-1', 'matched', 'tmdb', 0.9, 5)`)
	execute(t, inspection, `INSERT INTO enrichment_queue (item_id, reason, not_before, enqueued_at) VALUES ('item-1', 'scan', 0, 0)`)

	if _, err := newSourceMigrationProvider(t, inspection).DownTo(t.Context(), libraryManagementVersion); err != nil {
		t.Fatalf("DownTo(%d) error = %v", libraryManagementVersion, err)
	}

	if got, want := schemaObjects(t, inspection), schemaObjects(t, version4); !slices.Equal(got, want) {
		t.Errorf("schema after goose down =\n%s\nwant the 00004 schema\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}

	if got, want := dumpVersion4Rows(t, inspection), dumpVersion4Rows(t, version4); !slices.Equal(got, want) {
		t.Errorf("rows after goose down =\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

// version4LibraryColumns selects the columns libraries had before 00005.
const version4LibraryColumns = `SELECT id, name, kind, metadata_language, scan_extras, read_nfo,
	local_artwork, imported_from_config, created_at, updated_at FROM libraries ORDER BY id`

// dumpVersion4Rows returns every row of the 00004 tables as "table: values" in a stable
// order, libraries with their 00004 columns only.
func dumpVersion4Rows(t *testing.T, inspection *sql.DB) []string {
	t.Helper()

	dump := dumpQuery(t, inspection, "libraries", version4LibraryColumns)

	for _, table := range slices.Concat(catalogTables(), catalogV2Tables(), libraryManagementTables()) {
		if table != "libraries" {
			dump = append(dump, dumpQuery(t, inspection, table, "SELECT * FROM "+table+" ORDER BY 1, 2")...)
		}
	}

	return dump
}

func dumpQuery(t *testing.T, inspection *sql.DB, label, query string) []string {
	t.Helper()

	rows, err := inspection.QueryContext(t.Context(), query)
	if err != nil {
		t.Fatalf("dump %s: %v", label, err)
	}

	defer func() { _ = rows.Close() }()

	columns, err := rows.Columns()
	if err != nil {
		t.Fatalf("columns of %s: %v", label, err)
	}

	var dump []string

	for rows.Next() {
		values := make([]any, len(columns))
		pointers := make([]any, len(columns))

		for index := range values {
			pointers[index] = &values[index]
		}

		if err := rows.Scan(pointers...); err != nil {
			t.Fatalf("scan %s: %v", label, err)
		}

		dump = append(dump, fmt.Sprintf("%s: %v", label, values))
	}

	if err := rows.Err(); err != nil {
		t.Fatalf("read %s: %v", label, err)
	}

	return dump
}
