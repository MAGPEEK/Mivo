package sqlite_test

import (
	"testing"
	"time"

	"github.com/magpeek/mivo/internal/domain/media"
)

// The per-file reader returns the list in the order the scan stored it, and nothing for a
// file without subtitles, an unknown one included.
func TestExternalSubtitlesOfFileReturnsOneFilesListInOrder(t *testing.T) {
	t.Parallel()

	fixture := newChapterFixture(t)
	fileID, _ := fixture.addPart(t, "Book/a.m4b", 0, time.Second)
	otherID, _ := fixture.addPart(t, "Book/b.m4b", 1, time.Second)

	want := []media.ExternalSubtitle{
		externalSubtitle("Book/z.de.srt", func(entry *media.ExternalSubtitle) { entry.Language = "deu"; entry.Forced = true }),
		externalSubtitle("Book/a.en.ass", func(entry *media.ExternalSubtitle) {
			entry.Format, entry.SDH, entry.Default, entry.Title = media.SubtitleFormatASS, true, true, "commentary"
		}),
	}
	fixture.replaceSubtitles(t, fileID, want...)
	fixture.replaceSubtitles(t, otherID, want[1])

	got, err := fixture.reader.ExternalSubtitlesOfFile(t.Context(), fileID)
	if err != nil {
		t.Fatalf("ExternalSubtitlesOfFile error = %v", err)
	}

	assertDeepEqual(t, "subtitles of the file", got, want)

	none, err := fixture.reader.ExternalSubtitlesOfFile(t.Context(), "no-such-file")
	if err != nil || len(none) != 0 {
		t.Errorf("ExternalSubtitlesOfFile(unknown) = %v, %v, want none", none, err)
	}
}

func TestFileKnownReportsWhetherTheFileExists(t *testing.T) {
	t.Parallel()

	fixture := newChapterFixture(t)
	fileID, _ := fixture.addPart(t, "Book/a.m4b", 0, time.Second)

	for id, want := range map[string]bool{fileID: true, "no-such-file": false} {
		got, err := fixture.reader.FileKnown(t.Context(), id)
		if err != nil || got != want {
			t.Errorf("FileKnown(%q) = %t, %v, want %t", id, got, err, want)
		}
	}
}
