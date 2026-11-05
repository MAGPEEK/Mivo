package sqlite_test

import (
	"slices"
	"strings"
	"testing"
)

// onlineMetadataVersion is the schema version of online metadata (00005).
const onlineMetadataVersion = 5

// Migrations 00005 and 00006 from a populated 00004 catalog keep every row; 00006 adds the
// album index and nothing else, and goose down removes exactly that index again.
func TestMigrateFrom00004To00006KeepsRowsAndAddsTheAlbumIndex(t *testing.T) {
	t.Parallel()

	inspection, _ := seedVersion4(t)
	before := dumpVersion4Rows(t, inspection)
	provider := newSourceMigrationProvider(t, inspection)

	if _, err := provider.UpTo(t.Context(), onlineMetadataVersion); err != nil {
		t.Fatalf("UpTo(%d) error = %v", onlineMetadataVersion, err)
	}

	version5 := schemaObjects(t, inspection)

	if _, err := provider.Up(t.Context()); err != nil {
		t.Fatalf("Up error = %v", err)
	}

	if after := dumpVersion4Rows(t, inspection); !slices.Equal(after, before) {
		t.Errorf("rows after 00006 =\n%s\nwant\n%s", strings.Join(after, "\n"), strings.Join(before, "\n"))
	}

	indexes := queryStrings(t, inspection, "SELECT name FROM sqlite_schema WHERE name = 'items_albums'")
	assertDeepEqual(t, "album index", indexes, []string{"items_albums"})

	if _, err := provider.DownTo(t.Context(), onlineMetadataVersion); err != nil {
		t.Fatalf("DownTo(%d) error = %v", onlineMetadataVersion, err)
	}

	if got := schemaObjects(t, inspection); !slices.Equal(got, version5) {
		t.Errorf("schema after goose down =\n%s\nwant the 00005 schema\n%s", strings.Join(got, "\n"), strings.Join(version5, "\n"))
	}
}
