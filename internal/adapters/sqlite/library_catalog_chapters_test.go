package sqlite_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/library"
	"github.com/magpeek/mivo/internal/domain/media"

	applibrary "github.com/magpeek/mivo/internal/app/library"
)

// The audiobook library of the chapter fixture and its folder.
const (
	audiobooksLibraryID = "dddddddd-dddd-4ddd-8ddd-dddddddddddd"
	audiobooksRoot      = "/media/audiobooks"
)

// chapterFixture is a placement fixture with an audiobook library.
type chapterFixture struct {
	*placementFixture
	files int
}

func newChapterFixture(t *testing.T) *chapterFixture {
	t.Helper()

	fixture := newPlacementFixture(t)
	execute(t, fixture.inspection, `INSERT INTO libraries (id, name, kind, created_at, updated_at)
		VALUES (?1, 'Audiobooks', 'audiobooks', 0, 0)`, audiobooksLibraryID)
	execute(t, fixture.inspection, `INSERT INTO library_roots (id, library_id, path, path_key, position, created_at)
		VALUES (?1, ?1, ?2, ?2, 0, 0)`, audiobooksLibraryID, audiobooksRoot)

	return &chapterFixture{placementFixture: fixture}
}

// addPart saves one part file of the book "Book" in the first folder and returns its file
// ID and the book's ID.
func (fixture *chapterFixture) addPart(t *testing.T, path string, part int, duration time.Duration) (string, string) {
	t.Helper()

	return fixture.addPartIn(t, audiobooksLibraryID, bookPart{path: path, part: part, duration: duration})
}

// bookPart is one part file of the test book.
type bookPart struct {
	path     string
	part     int
	duration time.Duration
}

// addPartIn is addPart for the library folder rootID.
func (fixture *chapterFixture) addPartIn(t *testing.T, rootID string, part bookPart) (string, string) {
	t.Helper()

	path, duration := part.path, part.duration
	fixture.files++
	fileID := testID('f', fixture.files)
	placement := library.Placement{
		Levels:     []library.Level{{Kind: library.ItemKindAudiobook, MatchKey: "book", Title: "Book"}},
		PartNumber: library.NumberOf(part.part),
	}
	record := placedFile(audiobooksLibraryID, fileID, path, placement)
	record.RootID = rootID
	record.File.Description.Container = media.ContainerMP4
	record.File.Description.FormatName = "mov,mp4,m4a,3gp,3g2,mj2"
	record.File.Description.Duration = duration
	record.File.Description.Streams = []media.Stream{audioStream()}

	chain := fixture.save(t, record)

	return fileID, chain[0]
}

func (fixture *chapterFixture) replace(t *testing.T, fileID string, chapters ...media.Chapter) {
	t.Helper()

	if err := fixture.catalog.ReplaceFileChapters(t.Context(), fileID, chapters); err != nil {
		t.Fatalf("ReplaceFileChapters(%s) error = %v", fileID, err)
	}
}

func (fixture *chapterFixture) read(t *testing.T, itemID string) []applibrary.ItemChapter {
	t.Helper()

	chapters, err := fixture.reader.ItemChapters(t.Context(), itemID)
	if err != nil {
		t.Fatalf("ItemChapters(%s) error = %v", itemID, err)
	}

	return chapters
}

func seconds(count int) time.Duration {
	return time.Duration(count) * time.Second
}

func chapter(title string, start, end time.Duration) media.Chapter {
	return media.Chapter{Title: title, Start: start, End: end}
}

// itemChapter is the expected read-back of a chapter of the file fileID at path.
func itemChapter(fileID, path string, chapter media.Chapter) applibrary.ItemChapter {
	chapter.FileRef = path

	return applibrary.ItemChapter{Chapter: chapter, FileID: fileID}
}

func TestReplaceFileChaptersStoresAndReadsASingleFileBook(t *testing.T) {
	t.Parallel()

	fixture := newChapterFixture(t)
	fileID, bookID := fixture.addPart(t, "Book/book.m4b", 0, seconds(300))
	fixture.replace(t, fileID, chapter("Opening", 0, seconds(100)), chapter("Middle", seconds(100), seconds(300)))

	want := []applibrary.ItemChapter{
		itemChapter(fileID, "Book/book.m4b", chapter("Opening", 0, seconds(100))),
		itemChapter(fileID, "Book/book.m4b", chapter("Middle", seconds(100), seconds(300))),
	}
	assertDeepEqual(t, "chapters", fixture.read(t, bookID), want)
}

func TestReplaceFileChaptersReplacesTheWholeList(t *testing.T) {
	t.Parallel()

	fixture := newChapterFixture(t)
	fileID, bookID := fixture.addPart(t, "Book/book.m4b", 0, seconds(300))
	fixture.replace(t, fileID, chapter("A", 0, seconds(100)), chapter("B", seconds(100), seconds(200)),
		chapter("C", seconds(200), seconds(300)))
	fixture.replace(t, fileID, chapter("Whole", 0, seconds(300)))

	want := []applibrary.ItemChapter{itemChapter(fileID, "Book/book.m4b", chapter("Whole", 0, seconds(300)))}
	assertDeepEqual(t, "chapters after replace", fixture.read(t, bookID), want)

	fixture.replace(t, fileID)

	if got := fixture.read(t, bookID); got == nil || len(got) != 0 {
		t.Errorf("chapters after clearing = %#v, want an empty non-nil list", got)
	}
}

// Part 2 has no chapters, yet its duration shifts the chapters of part 3.
func TestItemChaptersAddThePrecedingPartDurations(t *testing.T) {
	t.Parallel()

	fixture := newChapterFixture(t)
	third, bookID := fixture.addPart(t, "Book/03.mp3", 3, seconds(50))
	first, _ := fixture.addPart(t, "Book/01.mp3", 1, seconds(60))
	fixture.addPart(t, "Book/02.mp3", 2, seconds(40))
	fixture.replace(t, first, chapter("One", 0, seconds(30)), chapter("Two", seconds(30), seconds(60)))
	fixture.replace(t, third, chapter("Three", 0, seconds(50)))

	want := []applibrary.ItemChapter{
		itemChapter(first, "Book/01.mp3", chapter("One", 0, seconds(30))),
		itemChapter(first, "Book/01.mp3", chapter("Two", seconds(30), seconds(60))),
		itemChapter(third, "Book/03.mp3", chapter("Three", seconds(100), seconds(150))),
	}
	assertDeepEqual(t, "chapters", fixture.read(t, bookID), want)
}

func TestItemChaptersOfAnItemWithoutFilesAreEmpty(t *testing.T) {
	t.Parallel()

	fixture := newChapterFixture(t)
	chain := fixture.save(t, placedFile(showsLibraryID, testID('f', 1), "Show/S01E01.mkv", episodePlacement("Show", 1, 1)))

	if got := fixture.read(t, chain[0]); got == nil || len(got) != 0 {
		t.Errorf("chapters of a show = %#v, want an empty non-nil list", got)
	}
}

func TestItemChaptersOfAnUnknownItemIsNotFound(t *testing.T) {
	t.Parallel()

	fixture := newChapterFixture(t)

	_, err := fixture.reader.ItemChapters(t.Context(), testID('i', 99))
	if !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("ItemChapters error = %v, want domain.ErrNotFound", err)
	}
}

func TestReplaceFileChaptersOfAnUnknownFileIsNotFound(t *testing.T) {
	t.Parallel()

	fixture := newChapterFixture(t)

	err := fixture.catalog.ReplaceFileChapters(t.Context(), testID('f', 99), []media.Chapter{chapter("A", 0, seconds(1))})
	if !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("ReplaceFileChapters error = %v, want domain.ErrNotFound", err)
	}
}

// An invalid list changes nothing: the stored chapters stay.
func TestReplaceFileChaptersRejectsInvalidChapters(t *testing.T) {
	t.Parallel()

	cases := map[string][]media.Chapter{
		"overlapping":    {chapter("A", 0, seconds(20)), chapter("B", seconds(10), seconds(30))},
		"negative start": {chapter("A", -seconds(1), seconds(1))},
		"empty span":     {chapter("A", seconds(5), seconds(5))},
		"below a millisecond": {
			chapter("A", 0, seconds(1)), chapter("B", seconds(1), seconds(1)+time.Microsecond),
		},
	}

	for name, chapters := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			fixture := newChapterFixture(t)
			fileID, bookID := fixture.addPart(t, "Book/book.m4b", 0, seconds(60))
			fixture.replace(t, fileID, chapter("Kept", 0, seconds(60)))

			err := fixture.catalog.ReplaceFileChapters(t.Context(), fileID, chapters)
			if !errors.Is(err, domain.ErrInvalid) {
				t.Errorf("ReplaceFileChapters error = %v, want domain.ErrInvalid", err)
			}

			want := []applibrary.ItemChapter{itemChapter(fileID, "Book/book.m4b", chapter("Kept", 0, seconds(60)))}
			assertDeepEqual(t, "chapters", fixture.read(t, bookID), want)
		})
	}
}

func TestRemovingAFileCascadesToItsChapters(t *testing.T) {
	t.Parallel()

	fixture := newChapterFixture(t)
	first, bookID := fixture.addPart(t, "Book/01.mp3", 1, seconds(60))
	second, _ := fixture.addPart(t, "Book/02.mp3", 2, seconds(60))
	fixture.replace(t, first, chapter("One", 0, seconds(60)))
	fixture.replace(t, second, chapter("Two", 0, seconds(60)))

	if err := fixture.catalog.RemovePlacedFiles(t.Context(), audiobooksLibraryID, []string{first}); err != nil {
		t.Fatalf("RemovePlacedFiles error = %v", err)
	}

	want := []applibrary.ItemChapter{itemChapter(second, "Book/02.mp3", chapter("Two", 0, seconds(60)))}
	assertDeepEqual(t, "chapters", fixture.read(t, bookID), want)

	if got := countRows(t, fixture.inspection, "SELECT COUNT(*) FROM media_chapters WHERE media_file_id = ?", first); got != 0 {
		t.Errorf("chapters of the removed file = %d, want 0", got)
	}
}

func TestChapterMethodsFailWithACancelledContext(t *testing.T) {
	t.Parallel()

	fixture := newChapterFixture(t)
	fileID, bookID := fixture.addPart(t, "Book/book.m4b", 0, seconds(60))
	ctx := cancelledContext(t)

	if err := fixture.catalog.ReplaceFileChapters(ctx, fileID, nil); !errors.Is(err, context.Canceled) {
		t.Errorf("ReplaceFileChapters error = %v, want context.Canceled", err)
	}

	if _, err := fixture.reader.ItemChapters(ctx, bookID); !errors.Is(err, context.Canceled) {
		t.Errorf("ItemChapters error = %v, want context.Canceled", err)
	}
}

// A part with an unknown duration or chapters past its probed end still shifts the next part
// by its last chapter end, so the book's chapters never overlap.
func TestItemChaptersShiftByTheLongerOfDurationAndLastChapterEnd(t *testing.T) {
	t.Parallel()

	fixture := newChapterFixture(t)
	first, bookID := fixture.addPart(t, "Book/01.mp3", 1, 0)
	second, _ := fixture.addPart(t, "Book/02.mp3", 2, seconds(10))
	third, _ := fixture.addPart(t, "Book/03.mp3", 3, seconds(10))
	fixture.replace(t, first, chapter("One", 0, seconds(20)))
	fixture.replace(t, second, chapter("Two", 0, seconds(12)))
	fixture.replace(t, third, chapter("Three", 0, seconds(10)))

	want := []applibrary.ItemChapter{
		itemChapter(first, "Book/01.mp3", chapter("One", 0, seconds(20))),
		itemChapter(second, "Book/02.mp3", chapter("Two", seconds(20), seconds(32))),
		itemChapter(third, "Book/03.mp3", chapter("Three", seconds(32), seconds(42))),
	}
	assertDeepEqual(t, "chapters", fixture.read(t, bookID), want)
}

// Files with the same part number follow the folder order before the path, as ListItemFiles.
func TestItemChaptersFollowTheFolderOrderBeforeThePath(t *testing.T) {
	t.Parallel()

	const secondRoot = "eeeeeeee-eeee-4eee-8eee-eeeeeeeeeeee"

	fixture := newChapterFixture(t)
	execute(t, fixture.inspection, `INSERT INTO library_roots (id, library_id, path, path_key, position, created_at)
		VALUES (?1, ?2, '/media/more', 'media/more', 1, 0)`, secondRoot, audiobooksLibraryID)

	later, bookID := fixture.addPartIn(t, secondRoot, bookPart{path: "Book/a.mp3", duration: seconds(10)})
	earlier, _ := fixture.addPart(t, "Book/z.mp3", 0, seconds(10))
	fixture.replace(t, later, chapter("Later", 0, seconds(10)))
	fixture.replace(t, earlier, chapter("Earlier", 0, seconds(10)))

	want := []applibrary.ItemChapter{
		itemChapter(earlier, "Book/z.mp3", chapter("Earlier", 0, seconds(10))),
		itemChapter(later, "Book/a.mp3", chapter("Later", seconds(10), seconds(20))),
	}
	assertDeepEqual(t, "chapters", fixture.read(t, bookID), want)
}

// Titles are cut to the API's 500 characters without splitting a multi-byte character.
func TestReplaceFileChaptersCutsLongTitlesPerCharacter(t *testing.T) {
	t.Parallel()

	fixture := newChapterFixture(t)
	fileID, bookID := fixture.addPart(t, "Book/book.m4b", 0, seconds(60))
	fixture.replace(t, fileID, chapter(strings.Repeat("ä", 600), 0, seconds(30)), chapter("Short", seconds(30), seconds(60)))

	want := []applibrary.ItemChapter{
		itemChapter(fileID, "Book/book.m4b", chapter(strings.Repeat("ä", 500), 0, seconds(30))),
		itemChapter(fileID, "Book/book.m4b", chapter("Short", seconds(30), seconds(60))),
	}
	assertDeepEqual(t, "chapters", fixture.read(t, bookID), want)
}

// A file with more than 2000 chapters keeps the first 2000.
func TestReplaceFileChaptersKeepsAtMost2000Chapters(t *testing.T) {
	t.Parallel()

	const stored, given = 2000, 2001

	fixture := newChapterFixture(t)
	fileID, bookID := fixture.addPart(t, "Book/book.m4b", 0, seconds(given))
	chapters := make([]media.Chapter, 0, given)

	for index := range given {
		chapters = append(chapters, chapter(fmt.Sprintf("C%d", index), seconds(index), seconds(index+1)))
	}

	fixture.replace(t, fileID, chapters...)

	got := fixture.read(t, bookID)
	if len(got) != stored || got[stored-1].Chapter.Title != fmt.Sprintf("C%d", stored-1) {
		t.Errorf("stored %d chapters, want the first %d", len(got), stored)
	}
}
