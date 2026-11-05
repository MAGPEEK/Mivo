package sqlite_test

import (
	"slices"
	"strings"
	"testing"
)

// musicVersion is the schema version of music libraries (00006); the chapters test migrates
// up from it across 00007 (match refs) to 00008.
const musicVersion = 6

// chaptersVersion is the schema version that adds media_chapters.
const chaptersVersion = 8

const insertChapter = `INSERT INTO media_chapters (media_file_id, ordinal, title, start_ms, end_ms) VALUES `

func chapterConstraintCases() []catalogConstraintCase {
	return []catalogConstraintCase{
		{name: "chapter of a missing file", statement: insertChapter + `('file-9', 0, 'A', 0, 1)`},
		{name: "duplicate ordinal in one file", statement: insertChapter + `('file-1', 0, 'A', 0, 1), ('file-1', 0, 'B', 1, 2)`},
		{name: "negative ordinal", statement: insertChapter + `('file-1', -1, 'A', 0, 1)`},
		{name: "negative start", statement: insertChapter + `('file-1', 0, 'A', -1, 1)`},
		{name: "end not after start", statement: insertChapter + `('file-1', 0, 'A', 5, 5)`},
		{name: "missing title", statement: insertChapter + `('file-1', 0, NULL, 0, 1)`},
		{name: "text offset", statement: insertChapter + `('file-1', 0, 'A', 'zero', 1)`},
	}
}

func TestChapterConstraintsRejectInvalidRows(t *testing.T) {
	t.Parallel()

	for _, test := range chapterConstraintCases() {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			inspection := openConstraintFixture(t)

			if _, err := inspection.ExecContext(t.Context(), test.statement); err == nil {
				t.Errorf("statement accepted, want a constraint error:\n%s", test.statement)
			}
		})
	}
}

// Deleting a file deletes its chapters; the chapters of another file stay.
func TestDeletingAFileCascadesToItsChapters(t *testing.T) {
	t.Parallel()

	inspection := openConstraintFixture(t)
	execute(t, inspection, newFile(func(*fileRow) {}))
	execute(t, inspection, insertChapter+`('file-1', 0, 'A', 0, 1), ('file-1', 1, '', 1, 2), ('file-2', 0, 'B', 0, 1)`)
	execute(t, inspection, `DELETE FROM media_files WHERE id = 'file-1'`)

	got := queryStrings(t, inspection, "SELECT media_file_id || ':' || title FROM media_chapters")
	assertDeepEqual(t, "chapters", got, []string{"file-2:B"})
}

// Migrating a populated 00006 catalog up through 00007 to 00008 keeps every row and adds
// media_chapters; goose down to 00006 restores the 00006 schema.
func TestMigrateFrom00006AddsTheChapterTableAndKeepsRows(t *testing.T) {
	t.Parallel()

	inspection, _ := seedVersion4(t)
	provider := newSourceMigrationProvider(t, inspection)

	if _, err := provider.UpTo(t.Context(), musicVersion); err != nil {
		t.Fatalf("UpTo(%d) error = %v", musicVersion, err)
	}

	before := dumpVersion4Rows(t, inspection)
	version6 := schemaObjects(t, inspection)

	if _, err := provider.UpTo(t.Context(), chaptersVersion); err != nil {
		t.Fatalf("UpTo(%d) error = %v", chaptersVersion, err)
	}

	if after := dumpVersion4Rows(t, inspection); !slices.Equal(after, before) {
		t.Errorf("rows after 00008 =\n%s\nwant\n%s", strings.Join(after, "\n"), strings.Join(before, "\n"))
	}

	tables := queryStrings(t, inspection, "SELECT name FROM sqlite_schema WHERE name = 'media_chapters'")
	assertDeepEqual(t, "chapter table", tables, []string{"media_chapters"})

	if _, err := provider.DownTo(t.Context(), musicVersion); err != nil {
		t.Fatalf("DownTo(%d) error = %v", musicVersion, err)
	}

	if got := schemaObjects(t, inspection); !slices.Equal(got, version6) {
		t.Errorf("schema after goose down =\n%s\nwant the 00006 schema\n%s", strings.Join(got, "\n"), strings.Join(version6, "\n"))
	}
}
