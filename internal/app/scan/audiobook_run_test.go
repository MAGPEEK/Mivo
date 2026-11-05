package scan_test

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/magpeek/mivo/internal/app/scan"
	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/library"
	"github.com/magpeek/mivo/internal/domain/media"
	"github.com/magpeek/mivo/internal/domain/metadata"
)

// bookAt is an audio file of the audiobooks library whose probe reports a one-minute
// audio file (audioDescription).
func (h *harness) bookAt(relativePath string, size int64) library.FileState {
	h.prober.script(absolutePath(booksRoot, relativePath), probeOutcome{description: audioDescription()})

	return audioAt(relativePath, size)
}

// chaptersOf returns the stored chapters of the file at path.
func (h *harness) chaptersOf(chapters *memoryChapters, path string) []media.Chapter {
	id, _ := h.catalogV2.fileOf(path)

	return chapters.of(id)
}

func chapterTitles(chapters []media.Chapter) []string {
	titles := make([]string, 0, len(chapters))
	for _, chapter := range chapters {
		titles = append(titles, chapter.Title)
	}

	return titles
}

// embeddedOf returns the embedded record the last refresh request carried for the book of path.
func (h *harness) embeddedOf(path string) (metadata.Record, bool) {
	h.refresher.mutex.Lock()
	defer h.refresher.mutex.Unlock()

	record, found := h.refresher.last.Embedded[h.catalogV2.itemOf(path)]

	return record, found
}

func TestAudiobookScanKeepsTheChaptersOfAnM4B(t *testing.T) {
	t.Parallel()

	h, tools, chapters := newAudiobookHarness(t)
	h.start(t)

	tools.script("Shelf/Dune.m4b", bookFile{
		tags:     media.AudioTags{Album: "Dune", AlbumArtist: "Frank Herbert", Narrator: "Simon Vance", Year: 1965, Genre: "SF"},
		chapters: []media.Chapter{chapter("Book One", 0, 20), chapter("Book Two", 20, 40), chapter("Book Three", 40, 60)},
	})
	tools.script("Shelf/Other.m4b", bookFile{tags: media.AudioTags{Artist: "Anon"}})

	status := h.scanBooks(t, h.bookAt("Shelf/Dune.m4b", 1), h.bookAt("Shelf/Other.m4b", 2))

	assertState(t, status, scan.StateIdle, scan.ErrorCodeNone)
	assertCounts(t, status, scan.Counts{Added: 2})
	assertKeys(t, "books", h.catalogV2.matchKeys(library.ItemKindAudiobook), "file:shelf/dune.m4b", "file:shelf/other.m4b")
	assertKeys(t, "Dune chapters", chapterTitles(h.chaptersOf(chapters, "Shelf/Dune.m4b")), "Book One", "Book Two", "Book Three")

	// A file without chapters gets one chapter over its probed length.
	other := h.chaptersOf(chapters, "Shelf/Other.m4b")
	if len(other) != 1 || other[0].Title != "Other" || other[0].End != time.Minute {
		t.Errorf("Other chapters = %+v, want one chapter Other of one minute", other)
	}

	record, found := h.embeddedOf("Shelf/Dune.m4b")
	wantPeople := []metadata.Credit{
		{Name: "Frank Herbert", Role: metadata.RoleAuthor}, {Name: "Simon Vance", Role: metadata.RoleNarrator, Order: 1},
	}

	if !found || !slices.Equal(record.People, wantPeople) || record.Year != 1965 || !slices.Equal(record.Genres, []string{"SF"}) {
		t.Errorf("embedded record = %+v (found %t), want author, narrator, year and genre", record, found)
	}
}

func TestAudiobookScanMakesAFolderOfPartsOneBook(t *testing.T) {
	t.Parallel()

	h, tools, chapters := newAudiobookHarness(t)
	h.start(t)

	tools.script("Herbert/Dune/a.mp3", bookFile{tags: media.AudioTags{Track: 3, Title: "Finale"}})
	tools.script("Herbert/Dune/b.mp3", bookFile{tags: media.AudioTags{Track: 1}})
	tools.script("Herbert/Dune/c.mp3", bookFile{tags: media.AudioTags{Track: 2, Artist: "Frank Herbert"}})

	status := h.scanBooks(t, h.bookAt("Herbert/Dune/a.mp3", 1), h.bookAt("Herbert/Dune/b.mp3", 2), h.bookAt("Herbert/Dune/c.mp3", 3))

	assertCounts(t, status, scan.Counts{Added: 3})
	assertKeys(t, "books", h.catalogV2.matchKeys(library.ItemKindAudiobook), "folder:herbert/dune/dune")

	wantParts := map[string]string{"Herbert/Dune/b.mp3": "1", "Herbert/Dune/c.mp3": "2", "Herbert/Dune/a.mp3": "3"}
	for path, part := range wantParts {
		if _, key := h.catalogV2.fileOf(path); !containsText(key, "part "+part) {
			t.Errorf("placement of %s = %q, want part %s", path, key, part)
		}
	}

	assertKeys(t, "chapters of a", chapterTitles(h.chaptersOf(chapters, "Herbert/Dune/a.mp3")), "Finale")
	assertKeys(t, "chapters of b", chapterTitles(h.chaptersOf(chapters, "Herbert/Dune/b.mp3")), "b")

	if record, _ := h.embeddedOf("Herbert/Dune/a.mp3"); len(record.People) != 1 || record.People[0].Name != "Frank Herbert" {
		t.Errorf("people = %+v, want the author of the first tagged part", record.People)
	}
}

func TestAudiobookScanSplitsAMixedFolder(t *testing.T) {
	t.Parallel()

	h, tools, chapters := newAudiobookHarness(t)
	h.start(t)

	tools.script("Mixed/1.mp3", bookFile{tags: media.AudioTags{Album: "Alpha"}})
	tools.script("Mixed/2.mp3", bookFile{
		tags: media.AudioTags{Album: "Alpha"}, chapters: []media.Chapter{chapter("Part A", 0, 5), chapter("Part B", 5, 10)},
	})
	tools.script("Mixed/3.mp3", bookFile{})
	tools.script("Mixed/4.mp3", bookFile{tags: media.AudioTags{Album: "Beta"}})
	tools.script("Mixed/Gamma.m4b", bookFile{tags: media.AudioTags{Album: "Alpha"}})
	tools.script("Mixed/5.mp3", bookFile{tags: media.AudioTags{Album: "Alpha"}, chapterErr: errors.New("broken chapters")})

	status := h.scanBooks(t, h.bookAt("Mixed/1.mp3", 1), h.bookAt("Mixed/2.mp3", 2), h.bookAt("Mixed/3.mp3", 3),
		h.bookAt("Mixed/4.mp3", 4), h.bookAt("Mixed/Gamma.m4b", 5), h.bookAt("Mixed/5.mp3", 6))

	assertCounts(t, status, scan.Counts{Added: 6, MetadataWarnings: 1})
	assertKeys(t, "books", h.catalogV2.matchKeys(library.ItemKindAudiobook), "file:mixed/gamma.m4b", "folder:mixed/alpha", "folder:mixed/beta")
	assertKeys(t, "files per book", intTexts(h.catalogV2.filesPerItem()), "1", "1", "4")
	assertKeys(t, "chapters of 2", chapterTitles(h.chaptersOf(chapters, "Mixed/2.mp3")), "Part A", "Part B")
	assertKeys(t, "chapters of 5", chapterTitles(h.chaptersOf(chapters, "Mixed/5.mp3")), "5")
}

func TestAudiobookRescanReadsOnlyTheChangedFolder(t *testing.T) {
	t.Parallel()

	h, tools, chapters := newAudiobookHarness(t)
	h.start(t)

	for _, path := range []string{"A/1.mp3", "A/2.mp3", "A/3.mp3"} {
		tools.script(path, bookFile{tags: media.AudioTags{Album: "A", AlbumArtist: "Author A"}})
	}

	files := []library.FileState{h.bookAt("A/1.mp3", 1), h.bookAt("A/2.mp3", 2), h.bookAt("A/3.mp3", 3), h.bookAt("B/1.mp3", 4)}
	h.scanBooks(t, files...)
	tools.takeReads()

	saves, writes := h.catalogV2.saves, chapters.writeCount()

	status := h.scanBooks(t, files...)
	assertCounts(t, status, scan.Counts{Unchanged: 4})

	if tagReads, _ := tools.takeReads(); len(tagReads) != 0 || h.catalogV2.saves != saves || chapters.writeCount() != writes {
		t.Errorf("unchanged rescan read %v, saved %d files, wrote %d chapters; want nothing", tagReads,
			h.catalogV2.saves-saves, chapters.writeCount()-writes)
	}

	changed := slices.Clone(files)
	changed[2] = h.bookAt("A/3.mp3", 30)
	status = h.scanBooks(t, changed...)

	assertCounts(t, status, scan.Counts{Updated: 1, Unchanged: 3})

	tagReads, chapterReads := tools.takeReads()
	assertKeys(t, "tag reads", tagReads, "A/1.mp3", "A/2.mp3", "A/3.mp3")
	assertKeys(t, "chapter reads", chapterReads, "A/3.mp3")

	if record, found := h.embeddedOf("A/1.mp3"); !found || len(record.People) != 1 {
		t.Errorf("embedded record of the changed book = %+v (found %t), want its author", record, found)
	}

	// Removing the first part renumbers the others: they move, the book keeps its ID.
	book := h.catalogV2.itemOf("A/2.mp3")
	status = h.scanBooks(t, changed[1], changed[2], changed[3])

	assertCounts(t, status, scan.Counts{Updated: 2, Unchanged: 1, Removed: 1})

	if _, key := h.catalogV2.fileOf("A/2.mp3"); !containsText(key, "part 1") || h.catalogV2.itemOf("A/2.mp3") != book {
		t.Errorf("A/2.mp3 = %q in %s, want part 1 of book %s", key, h.catalogV2.itemOf("A/2.mp3"), book)
	}
}

func TestAudiobookSidecarChangeReadsTheBookForItsPeople(t *testing.T) {
	t.Parallel()

	h, tools, _ := newAudiobookHarness(t)
	h.start(t)

	tools.script("A/1.mp3", bookFile{tags: media.AudioTags{Album: "A", Artist: "Author A"}})

	files := []library.FileState{h.bookAt("A/1.mp3", 1), h.bookAt("A/2.mp3", 2), h.bookAt("B/1.mp3", 3)}
	h.scanBooks(t, files...)
	tools.takeReads()

	cover := fileAt("A/cover.jpg", 9)
	cover.Class = library.FileClassImage

	status := h.scanBooks(t, append(slices.Clone(files), cover)...)

	assertCounts(t, status, scan.Counts{Unchanged: 3})

	tagReads, chapterReads := tools.takeReads()
	assertKeys(t, "tag reads", tagReads, "A/1.mp3", "A/2.mp3")
	assertKeys(t, "chapter reads", chapterReads)

	if record, found := h.embeddedOf("A/1.mp3"); !found || len(record.People) != 1 || len(h.refresher.last.Files) != 2 {
		t.Errorf("refresh = %d files, embedded %+v (found %t); want the book's two parts and its author",
			len(h.refresher.last.Files), record, found)
	}
}

func TestAudiobookChapterWriteFailureIsRetriedByTheNextScan(t *testing.T) {
	t.Parallel()

	h, tools, chapters := newAudiobookHarness(t)
	h.start(t)

	tools.script("Dune.m4b", bookFile{chapters: []media.Chapter{chapter("One", 0, 1)}})
	files := []library.FileState{h.bookAt("Dune.m4b", 1)}

	chapters.failNext(1)
	assertState(t, h.scanBooks(t, files...), scan.StateFailed, scan.ErrorCodeCatalogFailed)

	status := h.scanBooks(t, files...)

	assertState(t, status, scan.StateIdle, scan.ErrorCodeNone)
	assertCounts(t, status, scan.Counts{Updated: 1})
	assertKeys(t, "chapters", chapterTitles(h.chaptersOf(chapters, "Dune.m4b")), "One")

	assertCounts(t, h.scanBooks(t, files...), scan.Counts{Unchanged: 1})
}

func TestAudiobookUnreadableUnchangedFileKeepsItsPlacement(t *testing.T) {
	t.Parallel()

	h, tools, _ := newAudiobookHarness(t)
	h.start(t)

	tools.script("A/1.mp3", bookFile{tags: media.AudioTags{Album: "Saga"}})
	tools.script("A/2.mp3", bookFile{tags: media.AudioTags{Album: "Saga"}})
	h.scanBooks(t, h.bookAt("A/1.mp3", 1), h.bookAt("A/2.mp3", 2))

	_, before := h.catalogV2.fileOf("A/1.mp3")

	tools.script("A/1.mp3", bookFile{tagErr: errors.New("read audio tags: " + domain.ErrUnsupported.Error())})
	status := h.scanBooks(t, h.bookAt("A/1.mp3", 1), h.bookAt("A/2.mp3", 20))

	assertCounts(t, status, scan.Counts{Updated: 1, Unchanged: 1})

	if _, after := h.catalogV2.fileOf("A/1.mp3"); after != before {
		t.Errorf("placement = %q, want the stored %q", after, before)
	}

	// Known limit (BACKLOG, m3-14e notes, M3-14g): the readable sibling is regrouped alone and
	// becomes part 1 too, so both parts share a part number until the file reads again.
	if _, sibling := h.catalogV2.fileOf("A/2.mp3"); !containsText(before, "part 1") || !containsText(sibling, "part 1") {
		t.Errorf("part keys = %q and %q, want both part 1", before, sibling)
	}

	// The refresh still sees both files of the book; the kept one sorts after the numbered part.
	var refreshed []string
	for _, file := range h.refresher.last.Files {
		refreshed = append(refreshed, file.State.RelativePath)
	}

	assertKeys(t, "refresh files", refreshed, "A/2.mp3", "A/1.mp3")
}

func TestAudiobookEarlyEndKeepsThePeopleOfTouchedBooks(t *testing.T) {
	t.Parallel()

	h, tools, chapters := newAudiobookHarness(t)
	h.start(t)

	tools.script("A/1.mp3", bookFile{tags: media.AudioTags{Album: "A", Artist: "Author A"}})
	tools.script("B/1.mp3", bookFile{tags: media.AudioTags{Album: "B", Artist: "Author B"}})

	// The first book is saved, the second fails on its chapters and ends the scan.
	chapters.failAfter(1, 1)
	status := h.scanBooks(t, h.bookAt("A/1.mp3", 1), h.bookAt("B/1.mp3", 2))

	assertState(t, status, scan.StateFailed, scan.ErrorCodeCatalogFailed)

	book := h.catalogV2.itemOf("A/1.mp3")
	if record, found := h.embeddedOf("A/1.mp3"); !found || len(record.People) != 1 || !slices.Contains(h.refresher.last.ItemIDs, book) {
		t.Errorf("grace refresh items %v, embedded %+v (found %t); want book A with its author", h.refresher.last.ItemIDs, record, found)
	}
}

func TestAudiobookScanFailures(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		setup func(h *harness, tools *audiobookTools)
		want  scan.ErrorCode
	}{
		{
			name: "tag tool cannot run",
			setup: func(_ *harness, tools *audiobookTools) {
				tools.script("A/1.mp3", bookFile{tagErr: domain.ErrUnavailable})
			},
			want: scan.ErrorCodeProberUnavailable,
		},
		{
			name: "chapter tool cannot run",
			setup: func(_ *harness, tools *audiobookTools) {
				tools.script("A/1.mp3", bookFile{chapterErr: domain.ErrUnavailable})
			},
			want: scan.ErrorCodeProberUnavailable,
		},
		{
			name:  "ports missing",
			setup: func(h *harness, _ *audiobookTools) { h.audiobooks = scan.AudiobookPorts{} },
			want:  scan.ErrorCodeFailed,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			h, tools, _ := newAudiobookHarness(t)
			test.setup(h, tools)
			h.start(t)

			assertState(t, h.scanBooks(t, h.bookAt("A/1.mp3", 1)), scan.StateFailed, test.want)
		})
	}
}

func TestAudiobookUnprobeableFileIsStoredWithoutChapters(t *testing.T) {
	t.Parallel()

	h, _, chapters := newAudiobookHarness(t)
	h.start(t)

	h.prober.script(absolutePath(booksRoot, "A/1.mp3"), probeOutcome{err: errors.New("invalid data")})

	status := h.scanBooks(t, audioAt("A/1.mp3", 1))

	assertCounts(t, status, scan.Counts{Failed: 1})

	if id, _ := h.catalogV2.fileOf("A/1.mp3"); id == "" || len(chapters.of(id)) != 0 {
		t.Errorf("file %q with chapters %v, want it stored without chapters", id, chapters.of(id))
	}
}

func TestAudiobookScanCancelledWhileReadingRemovesNothing(t *testing.T) {
	t.Parallel()

	h, _, _ := newAudiobookHarness(t)
	h.start(t)
	h.scanBooks(t, h.bookAt("A/1.mp3", 1), h.bookAt("A/2.mp3", 2))

	h.prober.entered = make(chan struct{}, 1)
	h.prober.gate = make(chan struct{})
	h.walker.set(booksRoot, walkResult{files: []library.FileState{h.bookAt("A/3.mp3", 3)}})

	if _, err := h.service.Start(context.Background(), booksID); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	receive(t, h.prober.entered, "the probe to start")

	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	if err := h.service.Cancel(ctx, booksID); err != nil {
		t.Fatalf("Cancel() error = %v", err)
	}

	assertState(t, h.service.Status(booksID), scan.StateFailed, scan.ErrorCodeCanceled)

	if got := h.catalogV2.storedFiles(); len(got) != 2 {
		t.Errorf("stored files = %v, want both kept: nothing is applied before every read ended", got)
	}
}

func TestAudiobookEarlyEndKeepsBooksAnEarlierScanLeftPending(t *testing.T) {
	t.Parallel()

	h, tools, chapters := newAudiobookHarness(t)
	h.start(t)

	tools.script("A/1.mp3", bookFile{tags: media.AudioTags{Album: "A", Artist: "Author A"}})

	h.refresher.err = errors.New("refresh failed")
	assertState(t, h.scanBooks(t, h.bookAt("A/1.mp3", 1)), scan.StateFailed, scan.ErrorCodeFailed)

	h.refresher.mutex.Lock()
	h.refresher.err = nil
	h.refresher.mutex.Unlock()

	pendingBook := h.catalogV2.itemOf("A/1.mp3")

	// The next scan ends early on the grace path: it read nothing for the pending book.
	chapters.failNext(1)
	assertState(t, h.scanBooks(t, h.bookAt("A/1.mp3", 1), h.bookAt("B/1.mp3", 2)), scan.StateFailed, scan.ErrorCodeCatalogFailed)

	if slices.Contains(h.refresher.last.ItemIDs, pendingBook) {
		t.Errorf("grace refresh items = %v, want the pending book %s left pending", h.refresher.last.ItemIDs, pendingBook)
	}

	tools.takeReads()
	assertState(t, h.scanBooks(t, h.bookAt("A/1.mp3", 1), h.bookAt("B/1.mp3", 2)), scan.StateIdle, scan.ErrorCodeNone)

	tagReads, _ := tools.takeReads()
	assertKeys(t, "tag reads", tagReads, "A/1.mp3", "B/1.mp3")

	if record, found := h.embeddedOf("A/1.mp3"); !found || !slices.Contains(h.refresher.last.ItemIDs, pendingBook) || len(record.People) != 1 {
		t.Errorf("refresh items %v, embedded %+v; want the pending book with its author", h.refresher.last.ItemIDs, record)
	}
}
