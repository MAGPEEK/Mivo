package sqlite_test

import (
	"database/sql"
	"slices"
	"testing"
)

// catalogTables returns the tables of migration 00002 in creation order.
func catalogTables() []string {
	return []string{"libraries", "items", "media_files", "media_streams"}
}

// openForeignKeyInspectionDB is openInspectionDB with foreign key enforcement, so constraint
// tests see what the adapter's connections see.
func openForeignKeyInspectionDB(t *testing.T, path string) *sql.DB {
	t.Helper()

	return openInspectionDB(t, path+"?_foreign_keys=1")
}

func TestMediaCatalogTablesAreStrict(t *testing.T) {
	t.Parallel()

	_, path := openMigratedTestDB(t)
	inspection := openInspectionDB(t, path)

	for _, table := range catalogTables() {
		var strict int

		err := inspection.QueryRowContext(t.Context(),
			"SELECT strict FROM pragma_table_list WHERE schema = 'main' AND name = ?", table).Scan(&strict)
		if err != nil {
			t.Fatalf("look up table %s: %v", table, err)
		}

		if strict != 1 {
			t.Errorf("table %s is not STRICT", table)
		}
	}
}

// foreignKey is one column reference as PRAGMA foreign_key_list reports it.
type foreignKey struct {
	table, column, parent, onDelete string
}

// Every reference cascades, and every referencing column leads an index: without one,
// SQLite scans the whole child table for each deleted parent row.
func TestMediaCatalogForeignKeysCascadeAndAreIndexed(t *testing.T) {
	t.Parallel()

	_, path := openMigratedTestDB(t)
	inspection := openInspectionDB(t, path)

	want := []foreignKey{
		{table: "items", column: "library_id", parent: "libraries", onDelete: "CASCADE"},
		{table: "items", column: "parent_id", parent: "items", onDelete: "CASCADE"},
		{table: "media_files", column: "item_id", parent: "items", onDelete: "CASCADE"},
		{table: "media_files", column: "library_id", parent: "libraries", onDelete: "CASCADE"},
		{table: "media_streams", column: "media_file_id", parent: "media_files", onDelete: "CASCADE"},
	}

	if got := foreignKeys(t, inspection); !slices.Equal(got, want) {
		t.Errorf("foreign keys = %+v, want %+v", got, want)
	}

	for _, key := range want {
		if !hasIndexLeadingWith(t, inspection, key.table, key.column) {
			t.Errorf("no index on %s starts with the foreign key column %s", key.table, key.column)
		}
	}

	if !hasIndexLeadingWith(t, inspection, "items", "library_id") ||
		!indexHasColumns(t, inspection, "items_library_sort_title", "library_id", "sort_title", "id") {
		t.Error("items has no (library_id, sort_title, id) index for keyset pagination")
	}
}

func foreignKeys(t *testing.T, inspection *sql.DB) []foreignKey {
	t.Helper()

	rows, err := inspection.QueryContext(t.Context(), `
		SELECT tables.name, keys."from", keys."table", keys.on_delete
		FROM sqlite_schema AS tables, pragma_foreign_key_list(tables.name) AS keys
		WHERE tables.type = 'table'
		ORDER BY tables.name, keys."from"`)
	if err != nil {
		t.Fatalf("list foreign keys: %v", err)
	}

	defer func() { _ = rows.Close() }()

	keys := []foreignKey{}

	for rows.Next() {
		var key foreignKey
		if err := rows.Scan(&key.table, &key.column, &key.parent, &key.onDelete); err != nil {
			t.Fatalf("scan foreign key: %v", err)
		}

		keys = append(keys, key)
	}

	if err := rows.Err(); err != nil {
		t.Fatalf("list foreign keys: %v", err)
	}

	return keys
}

func hasIndexLeadingWith(t *testing.T, inspection *sql.DB, table, column string) bool {
	t.Helper()

	var count int

	err := inspection.QueryRowContext(t.Context(), `
		SELECT COUNT(*)
		FROM pragma_index_list(?) AS list, pragma_index_info(list.name) AS info
		WHERE info.seqno = 0 AND info.name = ?`, table, column).Scan(&count)
	if err != nil {
		t.Fatalf("look up indexes of %s: %v", table, err)
	}

	return count > 0
}

func indexHasColumns(t *testing.T, inspection *sql.DB, index string, want ...string) bool {
	t.Helper()

	rows, err := inspection.QueryContext(t.Context(),
		"SELECT name FROM pragma_index_info(?) ORDER BY seqno", index)
	if err != nil {
		t.Fatalf("look up index %s: %v", index, err)
	}

	defer func() { _ = rows.Close() }()

	var columns []string

	for rows.Next() {
		var column string
		if err := rows.Scan(&column); err != nil {
			t.Fatalf("scan index column: %v", err)
		}

		columns = append(columns, column)
	}

	if err := rows.Err(); err != nil {
		t.Fatalf("look up index %s: %v", index, err)
	}

	return slices.Equal(columns, want)
}

// fileRow holds the SQL literals of a media_files insert; the other columns are fixed.
type fileRow struct {
	id, itemID, libraryID, relativePath, size, container, probeError string
}

// newFile returns the insert of file "file-2" (b.mkv, item-1, lib-1, probed) after change
// adjusted its literals.
func newFile(change func(row *fileRow)) string {
	row := fileRow{
		id: "'file-2'", itemID: "'item-1'", libraryID: "'lib-1'", relativePath: "'b.mkv'",
		size: "0", container: "'mkv'", probeError: "NULL",
	}
	change(&row)

	return `INSERT INTO media_files (id, item_id, library_id, relative_path, size_bytes, modified_at,
		container, format_name, duration_ms, bit_rate, probed_at, probe_error)
		VALUES (` + row.id + `, ` + row.itemID + `, ` + row.libraryID + `, ` + row.relativePath + `, ` +
		row.size + `, 0, ` + row.container + `, '', 0, 0, 0, ` + row.probeError + `)`
}

// streamRow holds the SQL literals of a media_streams insert; the other columns are fixed.
type streamRow struct {
	fileID, index, kind, codecTag, width, frameRate, isDefault string
}

// newStream returns the insert of audio stream 1 of "file-1" after change adjusted its
// literals.
func newStream(change func(row *streamRow)) string {
	row := streamRow{
		fileID: "'file-1'", index: "1", kind: "'audio'", codecTag: "NULL", width: "0",
		frameRate: "''", isDefault: "0",
	}
	change(&row)

	return `INSERT INTO media_streams (media_file_id, stream_index, kind, codec, codec_tag, profile,
		level, pixel_format, bit_depth, width, height, frame_rate, color_primaries, color_transfer,
		color_space, hdr, channels, channel_layout, sample_rate, language, title, is_default,
		is_forced, is_attached_picture, bit_rate)
		VALUES (` + row.fileID + `, ` + row.index + `, ` + row.kind + `, 'h264', ` + row.codecTag +
		`, '', 0, '', 0, ` + row.width + `, 0, ` + row.frameRate + `, '', '', '', '', 0, '', 0, '', '', ` +
		row.isDefault + `, 0, 0, 0)`
}

// constraintFixture returns the parent rows every constraint case builds on: library
// "lib-1" (root /media/movies) with item "item-1" and file "file-1" (a.mkv) with video
// stream 0, and an empty library "lib-2".
func constraintFixture() []string {
	return []string{
		`INSERT INTO libraries (id, name, kind, root_path, created_at, updated_at)
			VALUES ('lib-1', 'Movies', 'movies', '/media/movies', 0, 0),
			       ('lib-2', 'Shows', 'shows', '/media/shows', 0, 0)`,
		`INSERT INTO items (id, library_id, kind, title, sort_title, created_at, updated_at)
			VALUES ('item-1', 'lib-1', 'movie', 'A', 'a', 0, 0)`,
		newFile(func(row *fileRow) { row.id, row.relativePath = "'file-1'", "'a.mkv'" }),
		newStream(func(row *streamRow) {
			row.index, row.kind, row.width, row.frameRate = "0", "'video'", "1920", "'24000/1001'"
		}),
	}
}

func TestMediaCatalogConstraintsRejectInvalidRows(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		statement string
	}{
		{name: "unknown library kind", statement: `INSERT INTO libraries VALUES ('lib-3', 'Music', 'music', '/media/music', 0, 0)`},
		{name: "duplicate library root", statement: `INSERT INTO libraries VALUES ('lib-3', 'Copy', 'movies', '/media/movies', 0, 0)`},
		{name: "empty library name", statement: `INSERT INTO libraries VALUES ('lib-3', '', 'movies', '/media/music', 0, 0)`},
		{name: "unknown item kind", statement: `INSERT INTO items (id, library_id, kind, title, sort_title, created_at, updated_at) VALUES ('item-2', 'lib-1', 'show', 'B', 'b', 0, 0)`},
		{name: "item of a missing library", statement: `INSERT INTO items (id, library_id, kind, title, sort_title, created_at, updated_at) VALUES ('item-2', 'lib-9', 'movie', 'B', 'b', 0, 0)`},
		{name: "item with a missing parent", statement: `INSERT INTO items VALUES ('item-2', 'lib-1', 'item-9', 'movie', 'B', 'b', 0, 0)`},
		{name: "negative file size", statement: newFile(func(row *fileRow) { row.size = "-1" })},
		{name: "empty container", statement: newFile(func(row *fileRow) { row.container = "''" })},
		{name: "empty probe error", statement: newFile(func(row *fileRow) { row.probeError = "''" })},
		{name: "duplicate relative path in a library", statement: newFile(func(row *fileRow) { row.relativePath = "'a.mkv'" })},
		{name: "file of a missing item", statement: newFile(func(row *fileRow) { row.itemID = "'item-9'" })},
		{name: "negative duration", statement: `UPDATE media_files SET duration_ms = -1`},
		{name: "negative file bit rate", statement: `UPDATE media_files SET bit_rate = -1`},
		{name: "unknown stream kind", statement: newStream(func(row *streamRow) { row.kind = "'data'" })},
		{name: "negative stream index", statement: newStream(func(row *streamRow) { row.index = "-1" })},
		{name: "duplicate stream index", statement: newStream(func(row *streamRow) { row.index = "0" })},
		{name: "stream of a missing file", statement: newStream(func(row *streamRow) { row.fileID = "'file-9'" })},
		{name: "empty codec tag", statement: newStream(func(row *streamRow) { row.codecTag = "''" })},
		{name: "negative width", statement: newStream(func(row *streamRow) { row.width = "-1" })},
		{name: "decimal frame rate", statement: newStream(func(row *streamRow) { row.frameRate = "'23.976'" })},
		{name: "unknown frame rate as 0/0", statement: newStream(func(row *streamRow) { row.frameRate = "'0/0'" })},
		{name: "flag other than 0 or 1", statement: newStream(func(row *streamRow) { row.isDefault = "2" })},
		{name: "text in an integer column", statement: `UPDATE media_streams SET width = 'wide'`},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			inspection := openConstraintFixture(t)

			if _, err := inspection.ExecContext(t.Context(), test.statement); err == nil {
				t.Errorf("statement succeeded, want a constraint error:\n%s", test.statement)
			}
		})
	}
}

// The open value sets and the nullable columns accept what the domain produces.
func TestMediaCatalogConstraintsAcceptDomainValues(t *testing.T) {
	t.Parallel()

	inspection := openConstraintFixture(t)

	// In order: later statements build on earlier ones.
	statements := []struct {
		name      string
		statement string
	}{
		{
			name: "unknown container with a probe error",
			statement: newFile(func(row *fileRow) {
				row.container, row.probeError = "'unknown'", "'probe_failed'"
			}),
		},
		{
			name:      "item in another library",
			statement: `INSERT INTO items (id, library_id, kind, title, sort_title, created_at, updated_at) VALUES ('item-2', 'lib-2', 'episode', 'A', 'a', 0, 0)`,
		},
		{
			name: "same relative path in another library",
			statement: newFile(func(row *fileRow) {
				row.id, row.itemID, row.libraryID, row.relativePath = "'file-3'", "'item-2'", "'lib-2'", "'a.mkv'"
			}),
		},
		{
			name: "stream with a codec tag",
			statement: newStream(func(row *streamRow) {
				row.kind, row.codecTag, row.width, row.frameRate, row.isDefault = "'video'", "'hvc1'", "3840", "'60/1'", "1"
			}),
		},
		{name: "subtitle stream", statement: newStream(func(row *streamRow) { row.index, row.kind = "2", "'subtitle'" })},
		{name: "stream of kind other", statement: newStream(func(row *streamRow) { row.index, row.kind = "3", "'other'" })},
		{name: "item with an existing parent", statement: `INSERT INTO items VALUES ('item-4', 'lib-1', 'item-1', 'episode', 'C', 'c', 0, 0)`},
	}

	for _, test := range statements {
		if _, err := inspection.ExecContext(t.Context(), test.statement); err != nil {
			t.Errorf("%s: error = %v, want the row accepted", test.name, err)
		}
	}
}

func openConstraintFixture(t *testing.T) *sql.DB {
	t.Helper()

	_, path := openMigratedTestDB(t)
	inspection := openForeignKeyInspectionDB(t, path)

	for _, statement := range constraintFixture() {
		if _, err := inspection.ExecContext(t.Context(), statement); err != nil {
			t.Fatalf("insert fixture: %v\n%s", err, statement)
		}
	}

	return inspection
}
