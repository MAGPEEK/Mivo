package sqlite_test

import (
	"database/sql"
	"slices"
	"strings"
	"testing"

	"github.com/magpeek/mivo/internal/adapters/sqlite"
)

// catalogV2Version is the schema version of the library model v2 (00003).
const catalogV2Version = 3

func libraryManagementConstraintCases() []catalogConstraintCase {
	const insertRoot = `INSERT INTO library_roots (id, library_id, path, path_key, position, created_at) VALUES `

	return []catalogConstraintCase{
		{name: "folder of a missing library", statement: insertRoot + `('root-9', 'lib-9', '/a', 'a', 0, 0)`},
		{name: "duplicate folder id", statement: insertRoot + `('root-1', 'lib-2', '/a', 'a', 1, 0)`},
		{name: "empty folder path", statement: insertRoot + `('root-9', 'lib-1', '', 'a', 1, 0)`},
		{name: "empty folder key", statement: insertRoot + `('root-9', 'lib-1', '/a', '', 1, 0)`},
		{name: "folder key used by another library", statement: insertRoot + `('root-9', 'lib-2', '/a', 'media/movies', 1, 0)`},
		{name: "second folder at one position", statement: insertRoot + `('root-9', 'lib-1', '/a', 'a', 0, 0)`},
		{name: "negative folder position", statement: insertRoot + `('root-9', 'lib-1', '/a', 'a', -1, 0)`},
		{name: "scan extras other than 0 or 1", statement: `UPDATE libraries SET scan_extras = 2`},
		{name: "read nfo other than 0 or 1", statement: `UPDATE libraries SET read_nfo = 2`},
		{name: "local artwork other than 0 or 1", statement: `UPDATE libraries SET local_artwork = -1`},
		{name: "imported flag other than 0 or 1", statement: `UPDATE libraries SET imported_from_config = 2`},
		{name: "empty ledger key", statement: `INSERT INTO library_imports VALUES ('', '/a', 0)`},
		{name: "empty ledger path", statement: `INSERT INTO library_imports VALUES ('a', '', 0)`},
		{name: "duplicate ledger entry", statement: `INSERT INTO library_imports VALUES ('a', '/a', 0), ('a', '/b', 0)`},
		{name: "file without a folder", statement: newFile(func(row *fileRow) { row.rootID = "NULL" })},
		{name: "file of a missing folder", statement: newFile(func(row *fileRow) { row.rootID = "'root-9'" })},
		{name: "same path in one folder", statement: newFile(func(row *fileRow) { row.relativePath = "'a.mkv'" })},
		{name: "sidecar without a folder", statement: `INSERT INTO library_sidecars VALUES ('lib-1', NULL, 'a.nfo', 'nfo', 0, 0)`},
	}
}

func TestLibraryManagementConstraintsRejectInvalidRows(t *testing.T) {
	t.Parallel()

	for _, test := range libraryManagementConstraintCases() {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			inspection := openCatalogV2Fixture(t)

			if _, err := inspection.ExecContext(t.Context(), test.statement); err == nil {
				t.Errorf("statement accepted, want a constraint error:\n%s", test.statement)
			}
		})
	}
}

func TestLibraryManagementConstraintsAcceptValidRows(t *testing.T) {
	t.Parallel()

	inspection := openCatalogV2Fixture(t)

	// The same relative path in a second folder of the same library is the point of root_id.
	execute(t, inspection, `INSERT INTO library_roots VALUES ('root-3', 'lib-1', '/media/more', 'media/more', 1, 0)`)
	execute(t, inspection, newFile(func(row *fileRow) { row.rootID, row.relativePath = "'root-3'", "'a.mkv'" }))
	execute(t, inspection, `UPDATE libraries SET metadata_language = 'de', scan_extras = 0, read_nfo = 0,
		local_artwork = 0, imported_from_config = 1 WHERE id = 'lib-1'`)
	execute(t, inspection, `INSERT INTO library_imports VALUES ('media/movies', '/media/movies', 5)`)
}

// Deleting a folder removes the files, sidecars and local artwork below it and keeps the
// library, the other folder and what lives there.
func TestDeletingAFolderCascadesToItsCatalogRows(t *testing.T) {
	t.Parallel()

	inspection := openCatalogV2Fixture(t)
	execute(t, inspection, `INSERT INTO library_roots VALUES ('root-3', 'lib-1', '/media/more', 'media/more', 1, 0)`)
	execute(t, inspection, newFile(func(row *fileRow) { row.id, row.rootID, row.relativePath = "'file-3'", "'root-3'", "'c.mkv'" }))
	execute(t, inspection, `INSERT INTO library_sidecars VALUES ('lib-1', 'root-3', 'c.nfo', 'nfo', 0, 0)`)
	execute(t, inspection, newArtwork(func(row *artworkRow) { row.id, row.rootID = "'art-3'", "'root-3'" }))

	execute(t, inspection, "DELETE FROM library_roots WHERE id = 'root-1'")

	want := map[string]int{
		"libraries": 2, "library_roots": 2, "media_files": 1, "library_sidecars": 1, "artwork": 1,
	}
	for table, count := range want {
		if got := rowCount(t, inspection, table); got != count {
			t.Errorf("rows of %s after deleting the folder = %d, want %d", table, got, count)
		}
	}
}

// seedVersion3 migrates a database to 00003 and stores two libraries with catalog rows.
func seedVersion3(t *testing.T) (*sqlite.DB, *sql.DB) {
	t.Helper()

	db, path := openTestDB(t, nil)
	inspection := openForeignKeyInspectionDB(t, path)

	if _, err := newSourceMigrationProvider(t, inspection).UpTo(t.Context(), catalogV2Version); err != nil {
		t.Fatalf("UpTo(%d) error = %v", catalogV2Version, err)
	}

	execute(t, inspection, `INSERT INTO libraries VALUES ('lib-1', 'Movies', 'movies', '/media/Movies', 10, 20),
		('lib-2', 'Shows', 'shows', '/media/shows', 11, 21), ('lib-3', 'Disk', 'movies', '/', 12, 22)`)
	execute(t, inspection, `INSERT INTO items VALUES ('item-1', 'lib-1', NULL, 'movie', 'a', 'A', 'a', '', 0, '',
		'', '', '', NULL, NULL, NULL, NULL, NULL, '', 0, 0)`)
	execute(t, inspection, `INSERT INTO media_files (id, item_id, library_id, relative_path, size_bytes,
		modified_at, container, format_name, duration_ms, bit_rate, probed_at)
		VALUES ('file-1', 'item-1', 'lib-1', 'a.mkv', 0, 0, 'mkv', '', 0, 0, 0)`)
	execute(t, inspection, `INSERT INTO library_sidecars VALUES ('lib-1', 'a.nfo', 'nfo', 0, 0)`)
	execute(t, inspection, `INSERT INTO tags VALUES ('tag-1', 'genre', 'Action')`)

	return db, inspection
}

// Migrating a 00003 database keeps every library with one folder and one ledger entry, and
// rebuilds the catalog (ADR 0028).
func TestMigrateCarriesLibrariesOverToFoldersAndTheImportLedger(t *testing.T) {
	t.Parallel()

	db, inspection := seedVersion3(t)

	// The second run proves the re-keying after the migrations is idempotent.
	for range 2 {
		if err := db.Migrate(t.Context()); err != nil {
			t.Fatalf("Migrate error = %v", err)
		}
	}

	if got := schemaVersion(t, inspection); got != latestSchemaVersion {
		t.Errorf("schema version = %d, want %d", got, latestSchemaVersion)
	}

	rows := queryStrings(t, inspection, `SELECT libraries.id || '|' || libraries.name || '|' || libraries.metadata_language
		|| '|' || libraries.scan_extras || libraries.read_nfo || libraries.local_artwork
		|| '|' || libraries.imported_from_config || '|' || libraries.created_at || '|' || libraries.updated_at
		|| '|' || library_roots.id || '|' || library_roots.path || '|' || library_roots.path_key
		|| '|' || library_roots.position
		FROM libraries JOIN library_roots ON library_roots.library_id = libraries.id ORDER BY libraries.id`)
	want := []string{
		"lib-1|Movies||111|1|10|20|lib-1|/media/Movies|" + carriedKey("media/Movies") + "|0",
		"lib-2|Shows||111|1|11|21|lib-2|/media/shows|media/shows|0",
		"lib-3|Disk||111|1|12|22|lib-3|/|/|0",
	}

	if !slices.Equal(rows, want) {
		t.Errorf("carried-over libraries =\n%q\nwant\n%q", rows, want)
	}

	ledger := queryStrings(t, inspection, `SELECT path_key || '|' || path || '|' || imported_at FROM library_imports ORDER BY path_key`)
	if wantLedger := []string{"/|/|12", carriedKey("media/Movies") + "|/media/Movies|10", "media/shows|/media/shows|11"}; !slices.Equal(ledger, wantLedger) {
		t.Errorf("import ledger = %q, want %q", ledger, wantLedger)
	}

	for _, table := range []string{"items", "media_files", "library_sidecars", "tags"} {
		if got := rowCount(t, inspection, table); got != 0 {
			t.Errorf("rows of %s after the upgrade = %d, want 0 (catalog rebuilt)", table, got)
		}
	}
}

// goose down from 00004 restores exactly the 00003 schema and the libraries with their
// first folder as root_path.
func TestMigrationDownToVersion3RestoresTheVersion3Schema(t *testing.T) {
	t.Parallel()

	_, version3Path := openTestDB(t, nil)
	version3 := openInspectionDB(t, version3Path)

	if _, err := newSourceMigrationProvider(t, version3).UpTo(t.Context(), catalogV2Version); err != nil {
		t.Fatalf("UpTo(%d) error = %v", catalogV2Version, err)
	}

	inspection := openCatalogV2Fixture(t)
	execute(t, inspection, `INSERT INTO library_roots VALUES ('root-3', 'lib-1', '/media/more', 'media/more', 1, 0)`)

	if _, err := newSourceMigrationProvider(t, inspection).DownTo(t.Context(), catalogV2Version); err != nil {
		t.Fatalf("DownTo(%d) error = %v", catalogV2Version, err)
	}

	if got, want := schemaObjects(t, inspection), schemaObjects(t, version3); !slices.Equal(got, want) {
		t.Errorf("schema after goose down =\n%s\nwant the 00003 schema\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}

	rows := queryStrings(t, inspection, `SELECT id || '|' || root_path FROM libraries ORDER BY id`)
	if want := []string{"lib-1|/media/movies", "lib-2|/media/shows"}; !slices.Equal(rows, want) {
		t.Errorf("libraries after goose down = %q, want %q", rows, want)
	}
}
