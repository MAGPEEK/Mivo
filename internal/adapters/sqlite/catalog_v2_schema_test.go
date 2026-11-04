package sqlite_test

import (
	"database/sql"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/pressly/goose/v3"
)

// catalogV1Version is the schema version of the milestone-2 catalog (00002).
const catalogV1Version = 2

// artworkRow holds the SQL literals of an artwork insert; the other columns are fixed.
type artworkRow struct {
	id, itemID, kind, source, rootID, relativePath, mediaFileID, streamIndex string
	dataPath, provider, remoteURL, size, width, tag, selected                string
}

// newArtwork returns the insert of the local, unselected poster "art-2" (poster.jpg in
// lib-1) of item-1 after change adjusted its literals.
func newArtwork(change func(row *artworkRow)) string {
	row := artworkRow{
		id: "'art-2'", itemID: "'item-1'", kind: "'poster'", source: "'local'", rootID: "'root-1'",
		relativePath: "'poster.jpg'", mediaFileID: "NULL", streamIndex: "NULL", dataPath: "NULL",
		provider: "NULL", remoteURL: "NULL", size: "10", width: "0", tag: "'0123456789abcdef'", selected: "0",
	}
	change(&row)

	return `INSERT INTO artwork (id, item_id, kind, source, root_id, relative_path, media_file_id,
		stream_index, data_path, provider, remote_url, size_bytes, modified_at, width, height, tag,
		selected, created_at)
		VALUES (` + strings.Join([]string{
		row.id, row.itemID, row.kind, row.source, row.rootID, row.relativePath, row.mediaFileID,
		row.streamIndex, row.dataPath, row.provider, row.remoteURL, row.size,
	}, ", ") + `, 0, ` + row.width + `, 0, ` + row.tag + `, ` + row.selected + `, 0)`
}

// embedded, provider and upload turn the default local artwork row into one of the other
// sources with a complete location group.
func embedded(row *artworkRow) {
	row.source, row.rootID, row.relativePath, row.mediaFileID, row.streamIndex = "'embedded'", "NULL", "NULL", "'file-1'", "2"
}

func provider(row *artworkRow) {
	row.source, row.rootID, row.relativePath = "'provider'", "NULL", "NULL"
	row.provider, row.remoteURL = "'tmdb'", "'https://image.tmdb.org/t/p/original/a.jpg'"
}

func upload(row *artworkRow) {
	row.source, row.rootID, row.relativePath, row.dataPath = "'upload'", "NULL", "NULL", "'artwork/a.jpg'"
}

// catalogV2Fixture adds to constraintFixture: genre "tag-1" (Action) and person "person-1"
// linked to item-1, a provider ID, a lock, a selected local poster "art-1" of item-1 and
// the sidecar poster.jpg of lib-1.
func catalogV2Fixture() []string {
	return []string{
		`INSERT INTO tags (id, kind, name) VALUES ('tag-1', 'genre', 'Action')`,
		`INSERT INTO item_tags (item_id, tag_id, position) VALUES ('item-1', 'tag-1', 0)`,
		`INSERT INTO people (id, name, sort_name) VALUES ('person-1', 'Jane Doe', 'doe jane')`,
		`INSERT INTO item_people (item_id, person_id, role, character, position)
			VALUES ('item-1', 'person-1', 'actor', 'Hero', 0)`,
		`INSERT INTO item_provider_ids (item_id, provider, external_id) VALUES ('item-1', 'tmdb', '603')`,
		`INSERT INTO item_field_locks (item_id, field) VALUES ('item-1', 'title')`,
		newArtwork(func(row *artworkRow) { row.id, row.selected = "'art-1'", "1" }),
		`INSERT INTO library_sidecars (library_id, root_id, relative_path, kind, size_bytes, modified_at)
			VALUES ('lib-1', 'root-1', 'poster.jpg', 'image', 10, 0)`,
	}
}

func openCatalogV2Fixture(t *testing.T) *sql.DB {
	t.Helper()

	inspection := openConstraintFixture(t)

	for _, statement := range catalogV2Fixture() {
		if _, err := inspection.ExecContext(t.Context(), statement); err != nil {
			t.Fatalf("insert fixture: %v\n%s", err, statement)
		}
	}

	return inspection
}

func metadataConstraintCases() []catalogConstraintCase {
	return []catalogConstraintCase{
		{name: "unknown tag kind", statement: `INSERT INTO tags VALUES ('tag-2', 'mood', 'Dark')`},
		{name: "empty tag name", statement: `INSERT INTO tags VALUES ('tag-2', 'genre', '')`},
		{name: "duplicate tag", statement: `INSERT INTO tags VALUES ('tag-2', 'genre', 'Action')`},
		{name: "tag link of a missing item", statement: `INSERT INTO item_tags VALUES ('item-9', 'tag-1', 0)`},
		{name: "tag link of a missing tag", statement: `INSERT INTO item_tags VALUES ('item-1', 'tag-9', 0)`},
		{name: "duplicate tag link", statement: `INSERT INTO item_tags VALUES ('item-1', 'tag-1', 1)`},
		{name: "negative tag position", statement: `INSERT INTO item_tags VALUES ('show-1', 'tag-1', -1)`},
		{name: "empty person name", statement: `INSERT INTO people VALUES ('person-2', '', 'x')`},
		{name: "empty person sort name", statement: `INSERT INTO people VALUES ('person-2', 'John Roe', '')`},
		{name: "duplicate person name", statement: `INSERT INTO people VALUES ('person-2', 'Jane Doe', 'doe jane')`},
		{name: "empty role", statement: `INSERT INTO item_people VALUES ('show-1', 'person-1', '', '', 0)`},
		{name: "credit of a missing person", statement: `INSERT INTO item_people VALUES ('show-1', 'person-9', 'actor', '', 0)`},
		{name: "duplicate credit", statement: `INSERT INTO item_people VALUES ('item-1', 'person-1', 'actor', 'Villain', 1)`},
		{name: "negative credit position", statement: `INSERT INTO item_people VALUES ('show-1', 'person-1', 'actor', '', -1)`},
		{name: "empty provider", statement: `INSERT INTO item_provider_ids VALUES ('show-1', '', '1')`},
		{name: "empty external id", statement: `INSERT INTO item_provider_ids VALUES ('show-1', 'tvdb', '')`},
		{name: "second id of one provider", statement: `INSERT INTO item_provider_ids VALUES ('item-1', 'tmdb', '604')`},
		{name: "provider id of a missing item", statement: `INSERT INTO item_provider_ids VALUES ('item-9', 'tmdb', '1')`},
		{name: "empty locked field", statement: `INSERT INTO item_field_locks VALUES ('item-1', '')`},
		{name: "duplicate lock", statement: `INSERT INTO item_field_locks VALUES ('item-1', 'title')`},
		{name: "unknown sidecar kind", statement: `INSERT INTO library_sidecars VALUES ('lib-1', 'root-1', 'a.txt', 'text', 0, 0)`},
		{name: "empty sidecar path", statement: `INSERT INTO library_sidecars VALUES ('lib-1', 'root-1', '', 'nfo', 0, 0)`},
		{name: "duplicate sidecar", statement: `INSERT INTO library_sidecars VALUES ('lib-1', 'root-1', 'poster.jpg', 'image', 0, 0)`},
		{name: "negative sidecar size", statement: `INSERT INTO library_sidecars VALUES ('lib-1', 'root-1', 'a.nfo', 'nfo', -1, 0)`},
		{name: "sidecar of a missing library", statement: `INSERT INTO library_sidecars VALUES ('lib-9', 'root-1', 'a.nfo', 'nfo', 0, 0)`},
		{name: "sidecar of a missing folder", statement: `INSERT INTO library_sidecars VALUES ('lib-1', 'root-9', 'a.nfo', 'nfo', 0, 0)`},
	}
}

func artworkConstraintCases() []catalogConstraintCase {
	return []catalogConstraintCase{
		{name: "unknown artwork source", statement: newArtwork(func(row *artworkRow) { row.source = "'web'" })},
		{name: "empty artwork kind", statement: newArtwork(func(row *artworkRow) { row.kind = "''" })},
		{name: "empty tag", statement: newArtwork(func(row *artworkRow) { row.tag = "''" })},
		{name: "selected other than 0 or 1", statement: newArtwork(func(row *artworkRow) { row.selected = "2" })},
		{name: "negative artwork size", statement: newArtwork(func(row *artworkRow) { row.size = "-1" })},
		{name: "negative width", statement: newArtwork(func(row *artworkRow) { row.width = "-1" })},
		{name: "artwork of a missing item", statement: newArtwork(func(row *artworkRow) { row.itemID = "'item-9'" })},
		{name: "second selected poster", statement: newArtwork(func(row *artworkRow) { row.selected = "1" })},
		{name: "local without a path", statement: newArtwork(func(row *artworkRow) { row.relativePath = "NULL" })},
		{name: "local without a folder", statement: newArtwork(func(row *artworkRow) { row.rootID = "NULL" })},
		{name: "local with an empty path", statement: newArtwork(func(row *artworkRow) { row.relativePath = "''" })},
		{name: "local with a media file", statement: newArtwork(func(row *artworkRow) { row.mediaFileID, row.streamIndex = "'file-1'", "2" })},
		{name: "local with a data path", statement: newArtwork(func(row *artworkRow) { row.dataPath = "'artwork/a.jpg'" })},
		{name: "local with a provider", statement: newArtwork(func(row *artworkRow) { row.provider = "'tmdb'" })},
		{name: "embedded without a stream", statement: newArtwork(func(row *artworkRow) { embedded(row); row.streamIndex = "NULL" })},
		{name: "embedded without a file", statement: newArtwork(func(row *artworkRow) { embedded(row); row.mediaFileID = "NULL" })},
		{name: "embedded with a folder path", statement: newArtwork(func(row *artworkRow) {
			embedded(row)
			row.rootID, row.relativePath = "'root-1'", "'poster.jpg'"
		})},
		{name: "embedded of a missing file", statement: newArtwork(func(row *artworkRow) { embedded(row); row.mediaFileID = "'file-9'" })},
		{name: "embedded with a negative stream", statement: newArtwork(func(row *artworkRow) { embedded(row); row.streamIndex = "-1" })},
		{name: "provider without a URL", statement: newArtwork(func(row *artworkRow) { provider(row); row.remoteURL = "NULL" })},
		{name: "provider without a provider", statement: newArtwork(func(row *artworkRow) { provider(row); row.provider = "NULL" })},
		{name: "provider with a media file", statement: newArtwork(func(row *artworkRow) {
			provider(row)
			row.mediaFileID, row.streamIndex = "'file-1'", "2"
		})},
		{name: "upload without a data path", statement: newArtwork(func(row *artworkRow) { upload(row); row.dataPath = "NULL" })},
		{name: "upload with an empty data path", statement: newArtwork(func(row *artworkRow) { upload(row); row.dataPath = "''" })},
		{name: "upload with a URL", statement: newArtwork(func(row *artworkRow) { upload(row); row.remoteURL = "'https://a/b.jpg'" })},
	}
}

func TestCatalogV2ConstraintsRejectInvalidRows(t *testing.T) {
	t.Parallel()

	for _, test := range append(metadataConstraintCases(), artworkConstraintCases()...) {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			inspection := openCatalogV2Fixture(t)

			if _, err := inspection.ExecContext(t.Context(), test.statement); err == nil {
				t.Errorf("statement succeeded, want a constraint error:\n%s", test.statement)
			}
		})
	}
}

// Growing sets take values the schema does not know yet, every source stores its own
// location group, and only one candidate per (item, kind) is selected.
func TestCatalogV2ConstraintsAcceptDomainValues(t *testing.T) {
	t.Parallel()

	inspection := openCatalogV2Fixture(t)

	// In order: later statements build on earlier ones.
	statements := []catalogConstraintCase{
		{name: "same tag name of another kind", statement: `INSERT INTO tags VALUES ('tag-2', 'collection', 'Action')`},
		{name: "second tag of an item", statement: `INSERT INTO item_tags VALUES ('item-1', 'tag-2', 0)`},
		{name: "same person in another role", statement: `INSERT INTO item_people VALUES ('item-1', 'person-1', 'director', '', 0)`},
		{name: "role of a later task", statement: `INSERT INTO item_people VALUES ('show-1', 'person-1', 'narrator', '', 0)`},
		{name: "provider of a later task", statement: `INSERT INTO item_provider_ids VALUES ('item-1', 'musicbrainz_album', 'x')`},
		{name: "artwork lock", statement: `INSERT INTO item_field_locks VALUES ('item-1', 'artwork.poster')`},
		{name: "nfo sidecar", statement: `INSERT INTO library_sidecars VALUES ('lib-1', 'root-1', 'a.nfo', 'nfo', 0, 0)`},
		{name: "unselected local candidate", statement: newArtwork(func(*artworkRow) {})},
		{name: "embedded candidate", statement: newArtwork(func(row *artworkRow) { embedded(row); row.id = "'art-3'" })},
		{name: "provider candidate before its download", statement: newArtwork(func(row *artworkRow) { provider(row); row.id = "'art-4'" })},
		{name: "downloaded provider candidate", statement: newArtwork(func(row *artworkRow) {
			provider(row)
			row.id, row.dataPath = "'art-5'", "'artwork/b.jpg'"
		})},
		{name: "upload candidate", statement: newArtwork(func(row *artworkRow) { upload(row); row.id = "'art-6'" })},
		{name: "selected kind of a later task", statement: newArtwork(func(row *artworkRow) { row.id, row.kind, row.selected = "'art-7'", "'square'", "1" })},
		{name: "selected poster of another item", statement: newArtwork(func(row *artworkRow) {
			row.id, row.itemID, row.selected = "'art-8'", "'show-1'", "1"
		})},
	}

	for _, test := range statements {
		if _, err := inspection.ExecContext(t.Context(), test.statement); err != nil {
			t.Errorf("%s: error = %v, want the row accepted", test.name, err)
		}
	}

	// The partial index admits a new selection once the old one is cleared.
	execute(t, inspection, `UPDATE artwork SET selected = 0 WHERE id = 'art-1'`)
	execute(t, inspection, `UPDATE artwork SET selected = 1 WHERE id = 'art-3'`)
}

// Deleting a media file drops its embedded artwork, an item everything attached to it, and
// a library its whole catalog including its sidecars. Shared tags and people stay.
func TestCatalogV2RowsCascadeFromFilesItemsAndLibraries(t *testing.T) {
	t.Parallel()

	inspection := openCatalogV2Fixture(t)
	execute(t, inspection, newArtwork(func(row *artworkRow) { embedded(row); row.id = "'art-3'" }))

	execute(t, inspection, "DELETE FROM media_files WHERE id = 'file-1'")

	if got := rowCount(t, inspection, "artwork"); got != 1 {
		t.Errorf("artwork after deleting the file = %d, want the local poster only", got)
	}

	execute(t, inspection, "DELETE FROM libraries WHERE id = 'lib-1'")

	want := map[string]int{
		"tags": 1, "item_tags": 0, "people": 1, "item_people": 0, "item_provider_ids": 0,
		"item_field_locks": 0, "artwork": 0, "library_sidecars": 0,
	}

	for _, table := range catalogV2Tables() {
		if got := rowCount(t, inspection, table); got != want[table] {
			t.Errorf("rows of %s after deleting the library = %d, want %d", table, got, want[table])
		}
	}
}

// OD-M3-1 A: upgrading a milestone-2 database drops its catalog; the server identity stays.
func TestMigrateDropsTheCatalogOfAVersion2Database(t *testing.T) {
	t.Parallel()

	db, path := openTestDB(t, nil)
	inspection := openForeignKeyInspectionDB(t, path)

	sourceProvider := newSourceMigrationProvider(t, inspection)
	if _, err := sourceProvider.UpTo(t.Context(), catalogV1Version); err != nil {
		t.Fatalf("UpTo(%d) error = %v", catalogV1Version, err)
	}

	execute(t, inspection, `INSERT INTO server_identity VALUES (1, 'server', 0)`)
	execute(t, inspection, `INSERT INTO libraries VALUES ('lib-1', 'Movies', 'movies', '/media/movies', 0, 0)`)
	execute(t, inspection, `INSERT INTO items VALUES ('item-1', 'lib-1', NULL, 'movie', 'A', 'a', 0, 0)`)
	execute(t, inspection, `INSERT INTO items VALUES ('item-2', 'lib-1', 'item-1', 'episode', 'B', 'b', 0, 0)`)
	execute(t, inspection, `INSERT INTO media_files VALUES ('file-1', 'item-1', 'lib-1', 'a.mkv', 0, 0, 'mkv', '', 0, 0, 0, NULL)`)
	execute(t, inspection, newStream(func(*streamRow) {}))

	if err := db.Migrate(t.Context()); err != nil {
		t.Fatalf("Migrate error = %v", err)
	}

	if got := schemaVersion(t, inspection); got != latestSchemaVersion {
		t.Errorf("schema version = %d, want %d", got, latestSchemaVersion)
	}

	for table, count := range catalogRowCounts(t, inspection) {
		if count != 0 {
			t.Errorf("rows of %s after the upgrade = %d, want 0", table, count)
		}
	}

	if got := rowCount(t, inspection, "server_identity"); got != 1 {
		t.Errorf("server identity rows = %d, want 1", got)
	}
}

// goose down from 00003 restores exactly the schema 00002 creates, also from a catalog with
// v2 rows.
func TestMigrationDownToVersion2RestoresTheVersion2Schema(t *testing.T) {
	t.Parallel()

	_, version2Path := openTestDB(t, nil)
	version2 := openInspectionDB(t, version2Path)

	if _, err := newSourceMigrationProvider(t, version2).UpTo(t.Context(), catalogV1Version); err != nil {
		t.Fatalf("UpTo(%d) error = %v", catalogV1Version, err)
	}

	inspection := openCatalogV2Fixture(t)

	if _, err := newSourceMigrationProvider(t, inspection).DownTo(t.Context(), catalogV1Version); err != nil {
		t.Fatalf("DownTo(%d) error = %v", catalogV1Version, err)
	}

	if got, want := schemaObjects(t, inspection), schemaObjects(t, version2); !slices.Equal(got, want) {
		t.Errorf("schema after goose down =\n%s\nwant the 00002 schema\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}

	if got := rowCount(t, inspection, "libraries"); got != 0 {
		t.Errorf("libraries after goose down = %d, want 0", got)
	}
}

// newSourceMigrationProvider runs the migration files of the source tree on inspection,
// independent of the adapter.
func newSourceMigrationProvider(t *testing.T, inspection *sql.DB) *goose.Provider {
	t.Helper()

	provider, err := goose.NewProvider(goose.DialectSQLite3, inspection, os.DirFS("migrations"),
		goose.WithDisableGlobalRegistry(true))
	if err != nil {
		t.Fatalf("NewProvider error = %v", err)
	}

	return provider
}

// schemaObjects returns every table and index definition, normalised in white space and
// sorted by name.
func schemaObjects(t *testing.T, inspection *sql.DB) []string {
	t.Helper()

	rows, err := inspection.QueryContext(t.Context(),
		"SELECT type || ' ' || name || ': ' || ifnull(sql, '') FROM sqlite_schema ORDER BY type, name")
	if err != nil {
		t.Fatalf("list schema objects: %v", err)
	}

	defer func() { _ = rows.Close() }()

	var objects []string

	for rows.Next() {
		var object string
		if err := rows.Scan(&object); err != nil {
			t.Fatalf("scan schema object: %v", err)
		}

		objects = append(objects, strings.Join(strings.Fields(object), " "))
	}

	if err := rows.Err(); err != nil {
		t.Fatalf("list schema objects: %v", err)
	}

	return objects
}
