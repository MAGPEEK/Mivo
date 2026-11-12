package sqlite_test

import (
	"database/sql"
	"slices"
	"strings"
	"testing"
)

// catalogTables returns the core catalog tables (created by 00002, rebuilt by 00003) in
// creation order.
func catalogTables() []string {
	return []string{"libraries", "items", "media_files", "media_streams"}
}

// catalogV2Tables returns the tables migration 00003 adds, in creation order.
func catalogV2Tables() []string {
	return []string{
		"tags", "item_tags", "people", "item_people", "item_provider_ids", "item_field_locks",
		"artwork", "library_sidecars",
	}
}

// libraryManagementTables returns the tables migration 00004 adds.
func libraryManagementTables() []string {
	return []string{"library_roots", "library_imports"}
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

	for _, table := range slices.Concat(catalogTables(), catalogV2Tables(), libraryManagementTables(), onlineMetadataTables()) {
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

// Every reference cascades (or sets NULL where listed), and every referencing column leads a full index: without one,
// SQLite scans the whole child table for each deleted parent row. A partial index does not
// help a cascade, because it does not hold every row.
func TestMediaCatalogForeignKeysCascadeAndAreIndexed(t *testing.T) {
	t.Parallel()

	_, path := openMigratedTestDB(t)
	inspection := openInspectionDB(t, path)

	want := []foreignKey{
		{table: "artwork", column: "item_id", parent: "items", onDelete: "CASCADE"},
		{table: "artwork", column: "media_file_id", parent: "media_files", onDelete: "CASCADE"},
		{table: "artwork", column: "root_id", parent: "library_roots", onDelete: "CASCADE"},
		{table: "auth_sessions", column: "profile_id", parent: "profiles", onDelete: "SET NULL"},
		{table: "auth_sessions", column: "user_id", parent: "users", onDelete: "CASCADE"},
		{table: "enrichment_queue", column: "item_id", parent: "items", onDelete: "CASCADE"},
		{table: "external_subtitles", column: "media_file_id", parent: "media_files", onDelete: "CASCADE"},
		{table: "item_field_locks", column: "item_id", parent: "items", onDelete: "CASCADE"},
		{table: "item_local_records", column: "item_id", parent: "items", onDelete: "CASCADE"},
		{table: "item_match", column: "item_id", parent: "items", onDelete: "CASCADE"},
		{table: "item_people", column: "item_id", parent: "items", onDelete: "CASCADE"},
		{table: "item_people", column: "person_id", parent: "people", onDelete: "CASCADE"},
		{table: "item_provider_ids", column: "item_id", parent: "items", onDelete: "CASCADE"},
		{table: "item_tags", column: "item_id", parent: "items", onDelete: "CASCADE"},
		{table: "item_tags", column: "tag_id", parent: "tags", onDelete: "CASCADE"},
		{table: "items", column: "library_id", parent: "libraries", onDelete: "CASCADE"},
		{table: "items", column: "parent_id", parent: "items", onDelete: "CASCADE"},
		{table: "library_roots", column: "library_id", parent: "libraries", onDelete: "CASCADE"},
		{table: "library_sidecars", column: "library_id", parent: "libraries", onDelete: "CASCADE"},
		{table: "library_sidecars", column: "root_id", parent: "library_roots", onDelete: "CASCADE"},
		{table: "media_chapters", column: "media_file_id", parent: "media_files", onDelete: "CASCADE"},
		{table: "media_files", column: "item_id", parent: "items", onDelete: "CASCADE"},
		{table: "media_files", column: "library_id", parent: "libraries", onDelete: "CASCADE"},
		{table: "media_files", column: "root_id", parent: "library_roots", onDelete: "CASCADE"},
		{table: "media_streams", column: "media_file_id", parent: "media_files", onDelete: "CASCADE"},
		{table: "photo_info", column: "item_id", parent: "items", onDelete: "CASCADE"},
		{table: "photo_info", column: "library_id", parent: "libraries", onDelete: "CASCADE"},
		{table: "photo_info", column: "parent_id", parent: "items", onDelete: "CASCADE"},
		{table: "photo_info", column: "primary_file_id", parent: "media_files", onDelete: "SET NULL"},
		{table: "profile_libraries", column: "library_id", parent: "libraries", onDelete: "CASCADE"},
		{table: "profile_libraries", column: "profile_id", parent: "profiles", onDelete: "CASCADE"},
		{table: "profiles", column: "user_id", parent: "users", onDelete: "CASCADE"},
		{table: "search_docs", column: "item_id", parent: "items", onDelete: "CASCADE"},
		{table: "user_libraries", column: "library_id", parent: "libraries", onDelete: "CASCADE"},
		{table: "user_libraries", column: "user_id", parent: "users", onDelete: "CASCADE"},
		// watch_state.item_id has deliberately no foreign key (ADR 0032 §8).
		{table: "watch_state", column: "profile_id", parent: "profiles", onDelete: "CASCADE"},
	}

	if got := foreignKeys(t, inspection); !slices.Equal(got, want) {
		t.Errorf("foreign keys = %+v, want %+v", got, want)
	}

	for _, key := range want {
		if !hasFullIndexLeadingWith(t, inspection, key.table, key.column) {
			t.Errorf("no full index on %s starts with the foreign key column %s", key.table, key.column)
		}
	}
}

// The items indexes of ADR 0027 §5: match keys unique per (library, parent, kind), keyset
// pagination of the top-level items, and children in display order.
func TestItemsIndexesServeMatchKeysListingsAndChildren(t *testing.T) {
	t.Parallel()

	_, path := openMigratedTestDB(t)
	inspection := openInspectionDB(t, path)

	tests := []struct {
		index   string
		columns []string
		unique  bool
		where   string
	}{
		// An expression column (ifnull(parent_id, '')) has no name in pragma_index_info.
		{index: "items_match_key", columns: []string{"library_id", "", "kind", "match_key"}, unique: true},
		{
			index: "items_top_level", columns: []string{"library_id", "sort_title", "id"},
			where: "WHERE parent_id IS NULL AND kind <> 'extra'",
		},
		{index: "items_children", columns: []string{"parent_id", "number", "sort_title", "id"}},
	}

	for _, test := range tests {
		if got := indexColumns(t, inspection, test.index); !slices.Equal(got, test.columns) {
			t.Errorf("index %s columns = %q, want %q", test.index, got, test.columns)
		}

		definition := indexDefinition(t, inspection, test.index)

		if unique := strings.HasPrefix(definition, "CREATE UNIQUE INDEX"); unique != test.unique {
			t.Errorf("index %s unique = %t, want %t: %s", test.index, unique, test.unique, definition)
		}

		if test.where != "" && !strings.HasSuffix(strings.Join(strings.Fields(definition), " "), test.where) {
			t.Errorf("index %s = %s, want it partial with %s", test.index, definition, test.where)
		}
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

func hasFullIndexLeadingWith(t *testing.T, inspection *sql.DB, table, column string) bool {
	t.Helper()

	var count int

	err := inspection.QueryRowContext(t.Context(), `
		SELECT COUNT(*)
		FROM pragma_index_list(?) AS list, pragma_index_info(list.name) AS info
		WHERE list.partial = 0 AND info.seqno = 0 AND info.name = ?`, table, column).Scan(&count)
	if err != nil {
		t.Fatalf("look up indexes of %s: %v", table, err)
	}

	return count > 0
}

// indexColumns returns the columns of an index in order, an empty name for an expression.
func indexColumns(t *testing.T, inspection *sql.DB, index string) []string {
	t.Helper()

	rows, err := inspection.QueryContext(t.Context(),
		"SELECT ifnull(name, '') FROM pragma_index_info(?) ORDER BY seqno", index)
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

	return columns
}

func indexDefinition(t *testing.T, inspection *sql.DB, index string) string {
	t.Helper()

	var definition string

	err := inspection.QueryRowContext(t.Context(),
		"SELECT sql FROM sqlite_schema WHERE type = 'index' AND name = ?", index).Scan(&definition)
	if err != nil {
		t.Fatalf("look up index %s: %v", index, err)
	}

	return definition
}

// itemRow holds the SQL literals of an items insert.
type itemRow struct {
	id, libraryID, parentID, kind, matchKey, title, sortTitle, year, releaseDate string
	number, numberEnd, absoluteNumber, discNumber, extraType                     string
}

// newItem returns the insert of top-level movie "item-2" (key 'b', lib-1) after change
// adjusted its literals.
func newItem(change func(row *itemRow)) string {
	row := itemRow{
		id: "'item-2'", libraryID: "'lib-1'", parentID: "NULL", kind: "'movie'", matchKey: "'b'",
		title: "'B'", sortTitle: "'b'", year: "0", releaseDate: "''",
		number: "NULL", numberEnd: "NULL", absoluteNumber: "NULL", discNumber: "NULL", extraType: "NULL",
	}
	change(&row)

	return `INSERT INTO items (id, library_id, parent_id, kind, match_key, title, sort_title, year,
		release_date, number, number_end, absolute_number, disc_number, extra_type, created_at, updated_at)
		VALUES (` + strings.Join([]string{
		row.id, row.libraryID, row.parentID, row.kind, row.matchKey, row.title, row.sortTitle, row.year,
		row.releaseDate, row.number, row.numberEnd, row.absoluteNumber, row.discNumber, row.extraType,
	}, ", ") + `, 0, 0)`
}

// fileRow holds the SQL literals of a media_files insert; the other columns are fixed.
type fileRow struct {
	id, itemID, libraryID, rootID, relativePath, size, container, probeError, partNumber, layout string
}

// newFile returns the insert of file "file-2" (b.mkv, item-1, lib-1, probed) after change
// adjusted its literals.
func newFile(change func(row *fileRow)) string {
	row := fileRow{
		id: "'file-2'", itemID: "'item-1'", libraryID: "'lib-1'", rootID: "'root-1'", relativePath: "'b.mkv'",
		size: "0", container: "'mkv'", probeError: "NULL", partNumber: "0", layout: "'file'",
	}
	change(&row)

	return `INSERT INTO media_files (id, item_id, library_id, root_id, relative_path, size_bytes, modified_at,
		container, format_name, duration_ms, bit_rate, probed_at, probe_error, part_number, layout)
		VALUES (` + row.id + `, ` + row.itemID + `, ` + row.libraryID + `, ` + row.rootID + `, ` + row.relativePath + `, ` +
		row.size + `, 0, ` + row.container + `, '', 0, 0, 0, ` + row.probeError + `, ` + row.partNumber +
		`, ` + row.layout + `)`
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
// "lib-1" (root /media/movies) with movie "item-1" (key 'a') and file "file-1" (a.mkv) with
// video stream 0; library "lib-2" (shows) with show "show-1" and its season 1 "season-1".
func constraintFixture() []string {
	return []string{
		`INSERT INTO libraries (id, name, kind, created_at, updated_at)
			VALUES ('lib-1', 'Movies', 'movies', 0, 0), ('lib-2', 'Shows', 'shows', 0, 0)`,
		`INSERT INTO library_roots (id, library_id, path, path_key, position, created_at)
			VALUES ('root-1', 'lib-1', '/media/movies', 'media/movies', 0, 0),
			       ('root-2', 'lib-2', '/media/shows', 'media/shows', 0, 0)`,
		newItem(func(row *itemRow) { row.id, row.matchKey, row.title, row.sortTitle = "'item-1'", "'a'", "'A'", "'a'" }),
		newItem(func(row *itemRow) {
			row.id, row.libraryID, row.kind, row.matchKey, row.title, row.sortTitle = "'show-1'", "'lib-2'", "'show'", "'show'", "'Show'", "'show'"
		}),
		newItem(func(row *itemRow) {
			row.id, row.libraryID, row.parentID, row.kind = "'season-1'", "'lib-2'", "'show-1'", "'season'"
			row.matchKey, row.title, row.sortTitle, row.number = "'1'", "'Season 1'", "'season 1'", "1"
		}),
		newFile(func(row *fileRow) { row.id, row.relativePath = "'file-1'", "'a.mkv'" }),
		newStream(func(row *streamRow) {
			row.index, row.kind, row.width, row.frameRate = "0", "'video'", "1920", "'24000/1001'"
		}),
	}
}

// catalogConstraintCase is a statement the schema must reject.
type catalogConstraintCase struct {
	name      string
	statement string
}

func libraryAndItemConstraintCases() []catalogConstraintCase {
	return []catalogConstraintCase{
		{name: "unknown library kind", statement: `INSERT INTO libraries (id, name, kind, created_at, updated_at) VALUES ('lib-3', 'Mixed', 'mixed', 0, 0)`},
		{name: "empty library name", statement: `INSERT INTO libraries (id, name, kind, created_at, updated_at) VALUES ('lib-3', '', 'movies', 0, 0)`},
		{name: "unknown item kind", statement: newItem(func(row *itemRow) { row.kind = "'folder'" })},
		{name: "item of a missing library", statement: newItem(func(row *itemRow) { row.libraryID = "'lib-9'" })},
		{name: "item with a missing parent", statement: newItem(func(row *itemRow) { row.parentID, row.kind = "'item-9'", "'season'" })},
		{name: "empty match key", statement: newItem(func(row *itemRow) { row.matchKey = "''" })},
		{name: "empty title", statement: newItem(func(row *itemRow) { row.title = "''" })},
		{name: "empty sort title", statement: newItem(func(row *itemRow) { row.sortTitle = "''" })},
		{name: "duplicate top-level match key", statement: newItem(func(row *itemRow) { row.matchKey = "'a'" })},
		{name: "duplicate match key below one parent", statement: newItem(func(row *itemRow) {
			row.libraryID, row.parentID, row.kind, row.matchKey, row.number = "'lib-2'", "'show-1'", "'season'", "'1'", "1"
		})},
		{name: "negative year", statement: newItem(func(row *itemRow) { row.year = "-1" })},
		{name: "text in an integer column", statement: newItem(func(row *itemRow) { row.year = "'1999a'" })},
		{name: "release date without leading zeros", statement: newItem(func(row *itemRow) { row.releaseDate = "'2024-1-5'" })},
		{name: "release date in German order", statement: newItem(func(row *itemRow) { row.releaseDate = "'05.01.2024'" })},
		{name: "release date with a time", statement: newItem(func(row *itemRow) { row.releaseDate = "'2024-01-05T10:00'" })},
		{name: "negative number", statement: newItem(func(row *itemRow) { row.number = "-1" })},
		{name: "number end not after number", statement: newItem(func(row *itemRow) { row.number, row.numberEnd = "2", "2" })},
		{name: "number end without number", statement: newItem(func(row *itemRow) { row.numberEnd = "2" })},
		{name: "negative absolute number", statement: newItem(func(row *itemRow) { row.absoluteNumber = "-1" })},
		{name: "disc number 0", statement: newItem(func(row *itemRow) { row.discNumber = "0" })},
		{name: "extra without extra type", statement: newItem(func(row *itemRow) { row.parentID, row.kind = "'item-1'", "'extra'" })},
		{name: "extra type on a movie", statement: newItem(func(row *itemRow) { row.extraType = "'trailer'" })},
		{name: "unknown extra type", statement: newItem(func(row *itemRow) {
			row.parentID, row.kind, row.extraType = "'item-1'", "'extra'", "'blooper'"
		})},
		{name: "movie below another item", statement: newItem(func(row *itemRow) { row.parentID = "'item-1'" })},
		{name: "show below another item", statement: newItem(func(row *itemRow) { row.parentID, row.kind = "'item-1'", "'show'" })},
	}
}

func fileAndStreamConstraintCases() []catalogConstraintCase {
	return []catalogConstraintCase{
		{name: "negative file size", statement: newFile(func(row *fileRow) { row.size = "-1" })},
		{name: "empty container", statement: newFile(func(row *fileRow) { row.container = "''" })},
		{name: "empty probe error", statement: newFile(func(row *fileRow) { row.probeError = "''" })},
		{name: "duplicate relative path in a library", statement: newFile(func(row *fileRow) { row.relativePath = "'a.mkv'" })},
		{name: "file of a missing item", statement: newFile(func(row *fileRow) { row.itemID = "'item-9'" })},
		{name: "negative part number", statement: newFile(func(row *fileRow) { row.partNumber = "-1" })},
		{name: "unknown layout", statement: newFile(func(row *fileRow) { row.layout = "'dvd'" })},
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
		{name: "frame rate with a letter", statement: newStream(func(row *streamRow) { row.frameRate = "'1x/2'" })},
		{name: "frame rate with two slashes", statement: newStream(func(row *streamRow) { row.frameRate = "'24/1/1'" })},
		{name: "flag other than 0 or 1", statement: newStream(func(row *streamRow) { row.isDefault = "2" })},
		{name: "text in a stream integer column", statement: `UPDATE media_streams SET width = 'wide'`},
	}
}

func TestMediaCatalogConstraintsRejectInvalidRows(t *testing.T) {
	t.Parallel()

	assertConstraintCasesFail(t, append(libraryAndItemConstraintCases(), fileAndStreamConstraintCases()...))
}

// assertConstraintCasesFail runs every case on its own fresh constraint fixture.
func assertConstraintCasesFail(t *testing.T, tests []catalogConstraintCase) {
	t.Helper()

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

// The open value sets, the new kinds and the nullable columns accept what the domain
// produces.
func TestMediaCatalogConstraintsAcceptDomainValues(t *testing.T) {
	t.Parallel()

	inspection := openConstraintFixture(t)

	// In order: later statements build on earlier ones.
	statements := []catalogConstraintCase{
		{
			name: "every library kind",
			statement: `INSERT INTO libraries (id, name, kind, created_at, updated_at)
				VALUES ('lib-3', 'Music', 'music', 0, 0),
				       ('lib-4', 'Books', 'audiobooks', 0, 0),
				       ('lib-5', 'Photos', 'photos', 0, 0)`,
		},
		{
			name: "unknown container with a probe error",
			statement: newFile(func(row *fileRow) {
				row.container, row.probeError = "'unknown'", "'probe_failed'"
			}),
		},
		{name: "same match key in another library", statement: newItem(func(row *itemRow) {
			row.libraryID, row.kind, row.matchKey = "'lib-2'", "'episode'", "'a'"
		})},
		{name: "same match key with another kind", statement: newItem(func(row *itemRow) {
			row.id, row.kind, row.matchKey = "'item-3'", "'show'", "'a'"
		})},
		{name: "trailer below a movie", statement: newItem(func(row *itemRow) {
			row.id, row.parentID, row.kind, row.matchKey, row.extraType = "'extra-1'", "'item-1'", "'extra'", "'a/trailer.mkv'", "'trailer'"
		})},
		{
			name: "same relative path in another library",
			statement: newFile(func(row *fileRow) {
				row.id, row.itemID, row.libraryID, row.rootID, row.relativePath = "'file-3'", "'item-2'", "'lib-2'", "'root-2'", "'a.mkv'"
			}),
		},
		{name: "second part of a disc image", statement: newFile(func(row *fileRow) {
			row.id, row.relativePath, row.partNumber, row.layout = "'file-4'", "'a.iso'", "2", "'iso'"
		})},
		{
			name: "stream with a codec tag",
			statement: newStream(func(row *streamRow) {
				row.kind, row.codecTag, row.width, row.frameRate, row.isDefault = "'video'", "'hvc1'", "3840", "'60/1'", "1"
			}),
		},
		{name: "subtitle stream", statement: newStream(func(row *streamRow) { row.index, row.kind = "2", "'subtitle'" })},
		{name: "stream of kind other", statement: newStream(func(row *streamRow) { row.index, row.kind = "3", "'other'" })},
		{name: "specials season 0", statement: newItem(func(row *itemRow) {
			row.id, row.libraryID, row.parentID, row.kind, row.matchKey, row.number = "'season-0'", "'lib-2'", "'show-1'", "'season'", "'0'", "0"
		})},
		{name: "multi-episode file with an absolute number and an air date", statement: newItem(func(row *itemRow) {
			row.id, row.libraryID, row.parentID, row.kind, row.matchKey = "'episode-1'", "'lib-2'", "'season-1'", "'episode'", "'1-2'"
			row.number, row.numberEnd, row.absoluteNumber, row.releaseDate = "1", "2", "130", "'2024-01-05'"
		})},
		{name: "episode without a number", statement: newItem(func(row *itemRow) {
			row.id, row.libraryID, row.parentID, row.kind, row.matchKey = "'episode-2'", "'lib-2'", "'season-1'", "'episode'", "'x'"
		})},
		{name: "track with a disc number", statement: newItem(func(row *itemRow) {
			row.id, row.libraryID, row.kind, row.number, row.discNumber, row.year = "'track-1'", "'lib-3'", "'track'", "3", "1", "1999"
		})},
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
