package sqlite_test

import (
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/media"
)

// subtitlesVersion is the schema version that adds external_subtitles.
const subtitlesVersion = 18

func externalSubtitle(path string, change func(*media.ExternalSubtitle)) media.ExternalSubtitle {
	entry := media.ExternalSubtitle{
		RelativePath: path, Format: media.SubtitleFormatSubRip, Language: "eng", TextBased: true,
	}
	change(&entry)

	return entry
}

func (fixture *chapterFixture) replaceSubtitles(t *testing.T, fileID string, subtitles ...media.ExternalSubtitle) {
	t.Helper()

	if err := fixture.catalog.ReplaceExternalSubtitles(t.Context(), fileID, subtitles); err != nil {
		t.Fatalf("ReplaceExternalSubtitles(%s) error = %v", fileID, err)
	}
}

func (fixture *chapterFixture) readSubtitles(t *testing.T, libraryID string) map[string][]media.ExternalSubtitle {
	t.Helper()

	got, err := fixture.catalog.ExternalSubtitles(t.Context(), libraryID)
	if err != nil {
		t.Fatalf("ExternalSubtitles(%s) error = %v", libraryID, err)
	}

	return got
}

// The reader returns every field and keeps the order of the list, which is not alphabetical
// here: an unchanged folder must compare equal and never be rewritten.
func TestExternalSubtitlesRoundTripInTheOrderOfTheList(t *testing.T) {
	t.Parallel()

	fixture := newChapterFixture(t)
	fileID, _ := fixture.addPart(t, "Book/a.m4b", 0, time.Second)
	otherID, _ := fixture.addPart(t, "Book/b.m4b", 1, time.Second)

	want := []media.ExternalSubtitle{
		externalSubtitle("Book/z.de.srt", func(entry *media.ExternalSubtitle) { entry.Language = "deu"; entry.Forced = true }),
		externalSubtitle("Book/a.en.ass", func(entry *media.ExternalSubtitle) {
			entry.Format, entry.SDH, entry.Default, entry.Title = media.SubtitleFormatASS, true, true, "commentary"
		}),
		externalSubtitle("Book/m.sup", func(entry *media.ExternalSubtitle) {
			entry.Format, entry.Language, entry.TextBased = media.SubtitleFormatPGS, media.UndeterminedLanguage, false
		}),
	}
	fixture.replaceSubtitles(t, fileID, want...)
	fixture.replaceSubtitles(t, otherID, want[1])

	got := fixture.readSubtitles(t, audiobooksLibraryID)
	assertDeepEqual(t, "subtitles of the first file", got[fileID], want)
	assertDeepEqual(t, "subtitles of the second file", got[otherID], want[1:2])
	assertDeepEqual(t, "other library", fixture.readSubtitles(t, "no-such-library"), map[string][]media.ExternalSubtitle{})
}

func TestReplaceExternalSubtitlesReplacesTheWholeList(t *testing.T) {
	t.Parallel()

	fixture := newChapterFixture(t)
	fileID, _ := fixture.addPart(t, "Book/a.m4b", 0, time.Second)
	keep := externalSubtitle("Book/b.srt", func(*media.ExternalSubtitle) {})
	fixture.replaceSubtitles(t, fileID, externalSubtitle("Book/a.srt", func(*media.ExternalSubtitle) {}), keep)
	fixture.replaceSubtitles(t, fileID, keep)

	assertDeepEqual(t, "after replace", fixture.readSubtitles(t, audiobooksLibraryID)[fileID], []media.ExternalSubtitle{keep})

	fixture.replaceSubtitles(t, fileID)

	if got := fixture.readSubtitles(t, audiobooksLibraryID); len(got) != 0 {
		t.Errorf("subtitles after clearing = %v, want none", got)
	}
}

func TestReplaceExternalSubtitlesRejectsBadInputAndChangesNothing(t *testing.T) {
	t.Parallel()

	fixture := newChapterFixture(t)
	fileID, _ := fixture.addPart(t, "Book/a.m4b", 0, time.Second)
	stored := externalSubtitle("Book/a.srt", func(*media.ExternalSubtitle) {})
	fixture.replaceSubtitles(t, fileID, stored)

	duplicate := externalSubtitle("Book/b.srt", func(*media.ExternalSubtitle) {})
	err := fixture.catalog.ReplaceExternalSubtitles(t.Context(), fileID, []media.ExternalSubtitle{duplicate, duplicate})

	if !errors.Is(err, domain.ErrInvalid) {
		t.Errorf("duplicate path error = %v, want domain.ErrInvalid", err)
	}

	assertDeepEqual(t, "after rollback", fixture.readSubtitles(t, audiobooksLibraryID)[fileID], []media.ExternalSubtitle{stored})

	err = fixture.catalog.ReplaceExternalSubtitles(t.Context(), "no-such-file", []media.ExternalSubtitle{stored})
	if !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("unknown file error = %v, want domain.ErrNotFound", err)
	}
}

func TestExternalSubtitlesCascadeWithFileAndLibrary(t *testing.T) {
	t.Parallel()

	fixture := newChapterFixture(t)
	first, _ := fixture.addPart(t, "Book/a.m4b", 0, time.Second)
	second, _ := fixture.addPart(t, "Book/b.m4b", 1, time.Second)
	entry := externalSubtitle("Book/a.srt", func(*media.ExternalSubtitle) {})
	fixture.replaceSubtitles(t, first, entry)
	fixture.replaceSubtitles(t, second, entry)

	execute(t, fixture.inspection, `DELETE FROM media_files WHERE id = ?`, first)
	assertDeepEqual(t, "after file delete", queryStrings(t, fixture.inspection,
		"SELECT media_file_id FROM external_subtitles"), []string{second})

	execute(t, fixture.inspection, `DELETE FROM libraries WHERE id = ?`, audiobooksLibraryID)

	if left := queryStrings(t, fixture.inspection, "SELECT media_file_id FROM external_subtitles"); len(left) != 0 {
		t.Errorf("subtitles after library delete = %v, want none", left)
	}
}

// Migrating a populated 00008 catalog up to 00018 keeps every row and adds external_subtitles;
// goose down restores the 00017 schema.
func TestMigrateTo00018AddsTheSubtitleTableAndKeepsRows(t *testing.T) {
	t.Parallel()

	inspection, _ := seedVersion4(t)
	provider := newSourceMigrationProvider(t, inspection)

	if _, err := provider.UpTo(t.Context(), subtitlesVersion-1); err != nil {
		t.Fatalf("UpTo(%d) error = %v", subtitlesVersion-1, err)
	}

	before := dumpVersion4Rows(t, inspection)
	version17 := schemaObjects(t, inspection)

	if _, err := provider.UpTo(t.Context(), subtitlesVersion); err != nil {
		t.Fatalf("UpTo(%d) error = %v", subtitlesVersion, err)
	}

	if after := dumpVersion4Rows(t, inspection); !slices.Equal(after, before) {
		t.Errorf("rows after 00018 =\n%s\nwant\n%s", strings.Join(after, "\n"), strings.Join(before, "\n"))
	}

	assertDeepEqual(t, "subtitle table", queryStrings(t, inspection,
		"SELECT name FROM sqlite_schema WHERE name = 'external_subtitles'"), []string{"external_subtitles"})

	if _, err := provider.DownTo(t.Context(), subtitlesVersion-1); err != nil {
		t.Fatalf("DownTo(%d) error = %v", subtitlesVersion-1, err)
	}

	if got := schemaObjects(t, inspection); !slices.Equal(got, version17) {
		t.Errorf("schema after goose down =\n%s\nwant the 00017 schema\n%s", strings.Join(got, "\n"), strings.Join(version17, "\n"))
	}
}
