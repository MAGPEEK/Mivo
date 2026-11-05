package sqlite_test

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// photosVersion is the schema version that adds photo_info.
const photosVersion = 9

const insertPhotoInfo = `INSERT INTO photo_info (item_id, library_id, parent_id, name_key, taken_at, taken_at_source, make, model, lens, exposure,
	f_number, iso, focal_mm, orientation, lat, lon, width, height, thumbnail_state) VALUES `

// photoInfoValues returns the values of a valid photo_info row of item-1 below season-1 with
// one column literal replaced.
func photoInfoValues(column, literal string) string {
	columns := []string{
		"item_id", "library_id", "parent_id", "name_key", "taken_at", "taken_at_source", "make", "model", "lens", "exposure", "f_number",
		"iso", "focal_mm", "orientation", "lat", "lon", "width", "height", "thumbnail_state",
	}
	values := []string{
		"'item-1'", "'lib-1'", "'season-1'", "'a.jpg'", "0", "'exif'", "''", "''", "''", "0.01", "2.8",
		"100", "35", "1", "48.1", "11.5", "4000", "3000", "'ready'",
	}

	if index := slices.Index(columns, column); index >= 0 {
		values[index] = literal
	}

	return "(" + strings.Join(values, ", ") + ")"
}

func photoConstraintCases() []catalogConstraintCase {
	invalid := map[string][2]string{
		"missing item":            {"item_id", "'item-9'"},
		"missing parent":          {"parent_id", "'item-9'"},
		"empty name key":          {"name_key", "''"},
		"missing capture time":    {"taken_at", "NULL"},
		"text capture time":       {"taken_at", "'today'"},
		"negative exposure":       {"exposure", "-1"},
		"negative f-number":       {"f_number", "-0.5"},
		"negative ISO":            {"iso", "-1"},
		"negative focal length":   {"focal_mm", "-1"},
		"orientation 9":           {"orientation", "9"},
		"negative orientation":    {"orientation", "-1"},
		"latitude out of range":   {"lat", "90.5"},
		"longitude out of range":  {"lon", "-181"},
		"latitude without lon":    {"lon", "NULL"},
		"negative width":          {"width", "-1"},
		"negative height":         {"height", "-1"},
		"unknown thumbnail state": {"thumbnail_state", "'pending'"},
		"missing make":            {"make", "NULL"},
		"missing library":         {"library_id", "'lib-9'"},
		"unknown time source":     {"taken_at_source", "'xmp'"},
	}

	cases := make([]catalogConstraintCase, 0, len(invalid))
	for name, change := range invalid {
		cases = append(cases, catalogConstraintCase{name: name, statement: insertPhotoInfo + photoInfoValues(change[0], change[1])})
	}

	return cases
}

func TestPhotoInfoConstraintsRejectInvalidRows(t *testing.T) {
	t.Parallel()

	for _, test := range photoConstraintCases() {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			inspection := openConstraintFixture(t)

			if _, err := inspection.ExecContext(t.Context(), test.statement); err == nil {
				t.Errorf("statement accepted, want a constraint error:\n%s", test.statement)
			}
		})
	}
}

// A top-level photo without GPS and with zero EXIF values is a valid row.
func TestPhotoInfoAcceptsAPhotoWithoutExif(t *testing.T) {
	t.Parallel()

	inspection := openConstraintFixture(t)
	execute(t, inspection, insertPhotoInfo+`('item-1', 'lib-1', NULL, 'a.jpg', 0, 'file', '', '', '', 0, 0, 0, 0, 0, NULL, NULL, 0, 0, 'unsupported')`)
}

// Deleting the album deletes the photo rows below it, deleting the item deletes its own row.
func TestDeletingItemsCascadesToPhotoInfo(t *testing.T) {
	t.Parallel()

	inspection := openConstraintFixture(t)
	execute(t, inspection, insertPhotoInfo+photoInfoValues("", ""))
	execute(t, inspection, newItem(func(*itemRow) {}))
	execute(t, inspection, insertPhotoInfo+`('item-2', 'lib-1', NULL, 'b.jpg', 0, 'exif', '', '', '', 0, 0, 0, 0, 0, NULL, NULL, 0, 0, 'ready')`)
	execute(t, inspection, `DELETE FROM items WHERE id = 'season-1'`)

	got := queryStrings(t, inspection, "SELECT item_id FROM photo_info ORDER BY item_id")
	assertDeepEqual(t, "photo rows after deleting the album", got, []string{"item-2"})

	execute(t, inspection, `DELETE FROM items WHERE id = 'item-2'`)

	if got := queryStrings(t, inspection, "SELECT item_id FROM photo_info"); len(got) != 0 {
		t.Errorf("photo rows after deleting the photo = %v, want none", got)
	}
}

// photo_info copies items.parent_id and items.library_id on the promise that no statement
// moves an item; this guards the promise for every query file.
func TestNoQueryReparentsAnItem(t *testing.T) {
	t.Parallel()

	entries, err := os.ReadDir("queries")
	if err != nil {
		t.Fatalf("read queries: %v", err)
	}

	for _, entry := range entries {
		content, err := os.ReadFile(filepath.Join("queries", entry.Name()))
		if err != nil {
			t.Fatalf("read %s: %v", entry.Name(), err)
		}

		for _, statement := range strings.Split(string(content), ";") {
			update := strings.ToLower(strings.Join(strings.Fields(statement), " "))
			if _, set, found := strings.Cut(update, "update items set "); found &&
				(strings.Contains(set, "parent_id =") || strings.Contains(set, "library_id =")) {
				t.Errorf("%s moves items:\n%s", entry.Name(), strings.TrimSpace(statement))
			}
		}
	}
}

// Migration 00009 goes down on its own to 00008 and up again.
func TestMigrate00009DownToChapters(t *testing.T) {
	t.Parallel()

	inspection, _ := seedVersion4(t)
	provider := newSourceMigrationProvider(t, inspection)

	if _, err := provider.UpTo(t.Context(), chaptersVersion); err != nil {
		t.Fatalf("UpTo(%d) error = %v", chaptersVersion, err)
	}

	version8 := schemaObjects(t, inspection)

	for _, step := range []func() error{
		func() error { _, err := provider.UpTo(t.Context(), photosVersion); return err },
		func() error { _, err := provider.DownTo(t.Context(), chaptersVersion); return err },
	} {
		if err := step(); err != nil {
			t.Fatalf("migrate: %v", err)
		}
	}

	if got := schemaObjects(t, inspection); !slices.Equal(got, version8) {
		t.Errorf("schema after 00009 down =\n%s\nwant the 00008 schema\n%s", strings.Join(got, "\n"), strings.Join(version8, "\n"))
	}
}

// Migrating a populated 00007 catalog up through 00008 to 00009 keeps every row and adds
// photo_info with its index; goose down to 00007 restores the 00007 schema.
func TestMigrateFrom00007AddsThePhotoTableAndKeepsRows(t *testing.T) {
	t.Parallel()

	const matchRefVersion = 7

	inspection, _ := seedVersion4(t)
	provider := newSourceMigrationProvider(t, inspection)

	if _, err := provider.UpTo(t.Context(), matchRefVersion); err != nil {
		t.Fatalf("UpTo(%d) error = %v", matchRefVersion, err)
	}

	before := dumpVersion4Rows(t, inspection)
	version7 := schemaObjects(t, inspection)

	if _, err := provider.UpTo(t.Context(), photosVersion); err != nil {
		t.Fatalf("UpTo(%d) error = %v", photosVersion, err)
	}

	if after := dumpVersion4Rows(t, inspection); !slices.Equal(after, before) {
		t.Errorf("rows after 00009 =\n%s\nwant\n%s", strings.Join(after, "\n"), strings.Join(before, "\n"))
	}

	objects := queryStrings(t, inspection,
		"SELECT name FROM sqlite_schema WHERE tbl_name = 'photo_info' AND name NOT LIKE 'sqlite_%' ORDER BY name")
	assertDeepEqual(t, "photo schema objects", objects, []string{"photo_info", "photo_info_album", "photo_info_library", "photo_info_primary_file"})

	if _, err := provider.DownTo(t.Context(), matchRefVersion); err != nil {
		t.Fatalf("DownTo(%d) error = %v", matchRefVersion, err)
	}

	if got := schemaObjects(t, inspection); !slices.Equal(got, version7) {
		t.Errorf("schema after goose down =\n%s\nwant the 00007 schema\n%s", strings.Join(got, "\n"), strings.Join(version7, "\n"))
	}
}
