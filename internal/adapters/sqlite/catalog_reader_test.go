package sqlite_test

import (
	"context"
	"encoding/base64"
	"errors"
	"testing"
	"time"

	"github.com/magpeek/mivo/internal/adapters/sqlite"
	applibrary "github.com/magpeek/mivo/internal/app/library"
	appplayback "github.com/magpeek/mivo/internal/app/playback"
	"github.com/magpeek/mivo/internal/app/scan"
	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/library"
)

// listPerformanceItems and listPerformanceBudget are the acceptance criterion of M2-15: a
// library of 1 000 items lists in under 200 ms on the dev machine.
const (
	listPerformanceItems  = 1_000
	listPerformanceBudget = 200 * time.Millisecond
)

// Paging through three pages returns every item exactly once in (sort title, ID) order,
// including runs of equal sort titles that cross a page boundary, each with its primary
// file and streams.
func TestListItemsPagesThroughALibraryInSortOrder(t *testing.T) {
	t.Parallel()

	fixture := newCatalogFixture(t)
	// Saved out of order; "Same" and "same" share the sort title "same".
	names := []string{"zulu.mkv", "Same.mkv", "alpha.mkv", "same.mp4", "Same.avi", "beta.mkv", "same.webm"}
	records := map[string]scan.ProbedFile{}

	for number, name := range names {
		record := probedRecord(moviesLibraryID, testID('i', len(names)-number), testID('f', number), name)
		records[record.Item.ID] = record
		mustSave(t, fixture.writer, record)
	}

	mustSave(t, fixture.writer, probedRecord(showsLibraryID, testID('i', 99), testID('f', 99), "other.mkv"))

	pages := listAllPages(t, fixture.reader, moviesLibraryID, 3)
	if len(pages) != 3 || len(pages[0].Items) != 3 || len(pages[1].Items) != 3 || len(pages[2].Items) != 1 {
		t.Fatalf("pages = %d with sizes %v, want 3 pages of 3, 3 and 1", len(pages), pageSizes(pages))
	}

	var got []applibrary.ItemSummary
	for _, page := range pages {
		got = append(got, page.Items...)
	}

	// alpha (i5), beta (i2), the four "same" items by item ID (i1 same.webm, i3 Same.avi,
	// i4 same.mp4, i6 Same.mkv), zulu (i7); the items are numbered backwards.
	wantOrder := []string{
		testID('i', 5), testID('i', 2), testID('i', 1), testID('i', 3), testID('i', 4), testID('i', 6), testID('i', 7),
	}

	want := make([]applibrary.ItemSummary, 0, len(wantOrder))
	for _, itemID := range wantOrder {
		record := records[itemID]
		want = append(want, applibrary.ItemSummary{Item: record.Item, PrimaryFile: record.File})
	}

	assertDeepEqual(t, "items of all pages", got, want)
}

// The primary file of an item is its first file by relative path, in the list and in the
// detail.
func TestListItemsReturnsThePrimaryFileOfAnItemWithSeveralFiles(t *testing.T) {
	t.Parallel()

	fixture := newCatalogFixture(t)
	later := probedRecord(moviesLibraryID, testID('i', 1), testID('f', 1), "Movie/b.mkv")
	primary := failedRecord(moviesLibraryID, testID('i', 1), testID('f', 2), "Movie/a.mkv")

	mustSave(t, fixture.writer, later)
	mustSave(t, fixture.writer, primary)

	page, err := fixture.reader.ListItems(t.Context(), moviesLibraryID, library.PageRequest{})
	if err != nil {
		t.Fatalf("ListItems error = %v", err)
	}

	assertDeepEqual(t, "page", page, applibrary.ItemPage{
		Items: []applibrary.ItemSummary{{Item: later.Item, PrimaryFile: primary.File}},
	})
}

func TestListItemsOfAnEmptyLibraryIsAnEmptyLastPage(t *testing.T) {
	t.Parallel()

	fixture := newCatalogFixture(t)

	page, err := fixture.reader.ListItems(t.Context(), showsLibraryID, library.PageRequest{Limit: 1})
	if err != nil || page.Items == nil || len(page.Items) != 0 || page.NextCursor != "" {
		t.Errorf("ListItems = %#v, %v, want an empty, non-nil list without a cursor", page, err)
	}
}

// A page that ends exactly with the last item has no cursor.
func TestListItemsWithALimitOfExactlyTheItemCountHasNoNextPage(t *testing.T) {
	t.Parallel()

	fixture := newCatalogFixture(t)
	mustSave(t, fixture.writer, probedRecord(moviesLibraryID, testID('i', 1), testID('f', 1), "a.mkv"))
	mustSave(t, fixture.writer, probedRecord(moviesLibraryID, testID('i', 2), testID('f', 2), "b.mkv"))

	page, err := fixture.reader.ListItems(t.Context(), moviesLibraryID, library.PageRequest{Limit: 2})
	if err != nil || len(page.Items) != 2 || page.NextCursor != "" {
		t.Errorf("ListItems = %d items, cursor %q, %v, want 2 items and no cursor", len(page.Items), page.NextCursor, err)
	}
}

func TestListItemsRejectsInvalidRequests(t *testing.T) {
	t.Parallel()

	fixture := newCatalogFixture(t)
	mustSave(t, fixture.writer, probedRecord(moviesLibraryID, testID('i', 1), testID('f', 1), "a.mkv"))
	mustSave(t, fixture.writer, probedRecord(moviesLibraryID, testID('i', 2), testID('f', 2), "b.mkv"))

	first, err := fixture.reader.ListItems(t.Context(), moviesLibraryID, library.PageRequest{Limit: 1})
	if err != nil || first.NextCursor == "" {
		t.Fatalf("first page = %+v, %v, want a cursor", first, err)
	}

	encode := base64.RawURLEncoding.EncodeToString

	tests := []struct {
		name      string
		libraryID string
		page      library.PageRequest
		wantErr   error
	}{
		{name: "limit above the maximum", page: library.PageRequest{Limit: library.MaxPageLimit + 1}, wantErr: domain.ErrInvalid},
		{name: "negative limit", page: library.PageRequest{Limit: -1}, wantErr: domain.ErrInvalid},
		{name: "cursor with characters outside base64url", page: library.PageRequest{Cursor: "a+b/c"}, wantErr: domain.ErrInvalid},
		{name: "padded cursor", page: library.PageRequest{Cursor: first.NextCursor + "="}, wantErr: domain.ErrInvalid},
		{name: "cursor without a separator", page: library.PageRequest{Cursor: encode([]byte("item-1"))}, wantErr: domain.ErrInvalid},
		{name: "cursor without an item id", page: library.PageRequest{Cursor: encode([]byte("\x00title"))}, wantErr: domain.ErrInvalid},
		{name: "unknown library", libraryID: unknownID, wantErr: domain.ErrNotFound},
		{name: "unknown library with a cursor", libraryID: unknownID, page: library.PageRequest{Cursor: first.NextCursor}, wantErr: domain.ErrNotFound},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			libraryID := test.libraryID
			if libraryID == "" {
				libraryID = moviesLibraryID
			}

			if _, err := fixture.reader.ListItems(t.Context(), libraryID, test.page); !errors.Is(err, test.wantErr) {
				t.Errorf("ListItems error = %v, want %v", err, test.wantErr)
			}
		})
	}
}

// A well-formed cursor is a position, not a reference: the next page continues after it
// even when the item it names was removed in between (a rescan while a client pages).
func TestListItemsContinuesAfterACursorWhoseItemWasRemoved(t *testing.T) {
	t.Parallel()

	fixture := newCatalogFixture(t)
	records := []scan.ProbedFile{
		probedRecord(moviesLibraryID, testID('i', 1), testID('f', 1), "a.mkv"),
		probedRecord(moviesLibraryID, testID('i', 2), testID('f', 2), "b.mkv"),
		probedRecord(moviesLibraryID, testID('i', 3), testID('f', 3), "c.mkv"),
	}

	for _, record := range records {
		mustSave(t, fixture.writer, record)
	}

	first, err := fixture.reader.ListItems(t.Context(), moviesLibraryID, library.PageRequest{Limit: 2})
	if err != nil {
		t.Fatalf("first page error = %v", err)
	}

	if err := fixture.writer.RemoveFiles(t.Context(), moviesLibraryID, []string{records[1].File.ID}); err != nil {
		t.Fatalf("RemoveFiles error = %v", err)
	}

	second, err := fixture.reader.ListItems(t.Context(), moviesLibraryID, library.PageRequest{Limit: 2, Cursor: first.NextCursor})
	if err != nil {
		t.Fatalf("second page error = %v", err)
	}

	assertDeepEqual(t, "second page", second, applibrary.ItemPage{
		Items: []applibrary.ItemSummary{{Item: records[2].Item, PrimaryFile: records[2].File}},
	})
}

// The timing is measured without t.Parallel, so other tests do not compete for the CPU. The
// race detector and coverage instrumentation slow the pure Go driver down several times;
// the budget is only enforced without them, and the time is logged either way.
func TestListItemsOfAThousandItemsIsFast(t *testing.T) {
	fixture := newCatalogFixture(t)
	insertManyItems(t, fixture.inspection, moviesLibraryID, listPerformanceItems)

	start := time.Now()
	pages := listAllPages(t, fixture.reader, moviesLibraryID, library.MaxPageLimit)
	elapsed := time.Since(start)

	t.Logf("listed %d items with %d streams each in %d pages in %v (race detector %t, coverage %q)",
		listPerformanceItems, manyItemStreams, len(pages), elapsed, raceDetectorEnabled, testing.CoverMode())

	var listed, streams int

	for _, page := range pages {
		for _, summary := range page.Items {
			listed++
			streams += len(summary.PrimaryFile.Description.Streams)
		}
	}

	if listed != listPerformanceItems || streams != listPerformanceItems*manyItemStreams {
		t.Fatalf("listed %d items with %d streams in pages %v, want %d items with %d streams each",
			listed, streams, pageSizes(pages), listPerformanceItems, manyItemStreams)
	}

	if elapsed > listPerformanceBudget && !raceDetectorEnabled && testing.CoverMode() == "" {
		t.Errorf("listing took %v, want under %v", elapsed, listPerformanceBudget)
	}
}

func TestGetItemOfAnUnknownIDReturnsNotFound(t *testing.T) {
	t.Parallel()

	fixture := newCatalogFixture(t)

	if _, err := fixture.reader.GetItem(t.Context(), unknownID); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("GetItem error = %v, want domain.ErrNotFound", err)
	}
}

func TestPlayableFileReturnsTheFileWithStreamsAndTheLibraryRoot(t *testing.T) {
	t.Parallel()

	fixture := newCatalogFixture(t)
	record := probedRecord(showsLibraryID, testID('i', 1), testID('f', 1), "Show/Season 1/S01E01.mkv")
	mustSave(t, fixture.writer, record)

	playable, err := fixture.reader.PlayableFile(t.Context(), record.File.ID)
	if err != nil {
		t.Fatalf("PlayableFile error = %v", err)
	}

	assertDeepEqual(t, "PlayableFile", playable, appplayback.PlayableFile{File: record.File, LibraryRoot: showsRoot})
}

// A file whose probe failed is returned as well; whether it plays is the caller's decision.
func TestPlayableFileReturnsAFileWhoseProbeFailed(t *testing.T) {
	t.Parallel()

	fixture := newCatalogFixture(t)
	record := failedRecord(moviesLibraryID, testID('i', 1), testID('f', 1), "broken.avi")
	mustSave(t, fixture.writer, record)

	playable, err := fixture.reader.PlayableFile(t.Context(), record.File.ID)
	if err != nil {
		t.Fatalf("PlayableFile error = %v", err)
	}

	assertDeepEqual(t, "PlayableFile", playable, appplayback.PlayableFile{File: record.File, LibraryRoot: moviesRoot})
}

func TestPlayableFileOfAnUnknownIDReturnsNotFound(t *testing.T) {
	t.Parallel()

	fixture := newCatalogFixture(t)

	if _, err := fixture.reader.PlayableFile(t.Context(), unknownID); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("PlayableFile error = %v, want domain.ErrNotFound", err)
	}
}

// Stored values the domain does not accept are reported, not handed to clients, and the
// error does not wrap domain.ErrInvalid: the fault is in the server's data, not in the
// client's request. The cases bypass the CHECK constraints the way an edited database or a
// newer version would.
func TestReadingCorruptCatalogRowsIsAServerError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		query string
	}{
		{name: "unknown item kind", query: "UPDATE items SET kind = 'show'"},
		{name: "unknown container", query: "UPDATE media_files SET container = 'zip'"},
		{name: "unknown stream kind", query: "UPDATE media_streams SET kind = 'data' WHERE stream_index = 1"},
		{name: "unknown hdr format", query: "UPDATE media_streams SET hdr = 'dolby_vision' WHERE stream_index = 0"},
		{name: "malformed frame rate", query: "UPDATE media_streams SET frame_rate = '1/0' WHERE stream_index = 0"},
		{name: "negative width", query: "UPDATE media_streams SET width = -1 WHERE stream_index = 0"},
		{name: "streams next to a probe error", query: "UPDATE media_files SET probe_error = 'probe_failed'"},
		{name: "absolute relative path", query: "UPDATE media_files SET relative_path = '/etc/passwd'"},
		{name: "relative path leaving the root", query: "UPDATE media_files SET relative_path = '../a.mkv'"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			fixture := newCatalogFixture(t)
			record := probedRecord(moviesLibraryID, testID('i', 1), testID('f', 1), "a.mkv")
			mustSave(t, fixture.writer, record)

			storeUnchecked(t, fixture.inspection, test.query)

			_, listErr := fixture.reader.ListItems(t.Context(), moviesLibraryID, library.PageRequest{})
			_, getErr := fixture.reader.GetItem(t.Context(), record.Item.ID)
			_, playableErr := fixture.reader.PlayableFile(t.Context(), record.File.ID)

			errs := map[string]error{"ListItems": listErr, "GetItem": getErr, "PlayableFile": playableErr}
			for operation, err := range errs {
				if test.name == "unknown item kind" && operation == "PlayableFile" {
					// PlayableFile does not read the item.
					continue
				}

				assertUndecodableRowError(t, operation, err)
			}
		})
	}
}

func TestCatalogReaderReportsDatabaseErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		ctx     func(t *testing.T) context.Context
		closeDB bool
		wantErr error
	}{
		{name: "cancelled context", ctx: cancelledContext, wantErr: context.Canceled},
		{name: "closed database", ctx: func(t *testing.T) context.Context { return t.Context() }, closeDB: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			fixture := newCatalogFixture(t)
			record := probedRecord(moviesLibraryID, testID('i', 1), testID('f', 1), "a.mkv")
			mustSave(t, fixture.writer, record)

			if test.closeDB {
				closeTestDB(t, fixture.db)
			}

			_, listErr := fixture.reader.ListItems(test.ctx(t), moviesLibraryID, library.PageRequest{})
			_, getErr := fixture.reader.GetItem(test.ctx(t), record.Item.ID)
			_, playableErr := fixture.reader.PlayableFile(test.ctx(t), record.File.ID)

			errs := map[string]error{"ListItems": listErr, "GetItem": getErr, "PlayableFile": playableErr}
			for operation, err := range errs {
				assertDatabaseError(t, operation, err, test.wantErr)
			}
		})
	}
}

// listAllPages follows NextCursor until the last page and returns every page.
func listAllPages(t *testing.T, reader *sqlite.CatalogReader, libraryID string, limit int) []applibrary.ItemPage {
	t.Helper()

	var (
		pages  []applibrary.ItemPage
		cursor string
	)

	for {
		page, err := reader.ListItems(t.Context(), libraryID, library.PageRequest{Limit: limit, Cursor: cursor})
		if err != nil {
			t.Fatalf("ListItems page %d error = %v", len(pages)+1, err)
		}

		pages = append(pages, page)

		if page.NextCursor == "" {
			return pages
		}

		cursor = page.NextCursor
	}
}

func pageSizes(pages []applibrary.ItemPage) []int {
	sizes := make([]int, 0, len(pages))
	for _, page := range pages {
		sizes = append(sizes, len(page.Items))
	}

	return sizes
}
