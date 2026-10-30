package restapi_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/magpeek/mivo/internal/adapters/http/apigen"
	"github.com/magpeek/mivo/internal/adapters/http/restapi"
	applibrary "github.com/magpeek/mivo/internal/app/library"
	appscan "github.com/magpeek/mivo/internal/app/scan"
	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/library"
	"github.com/magpeek/mivo/internal/domain/media"
)

const (
	testSecondItemID        = "9b0c1d2e-3f4a-4b5c-9d6e-8f9a0b1c2d3e"
	testCatalogFileID       = "1c2d3e4f-5a6b-4c7d-8e9f-0a1b2c3d4e5f"
	testSecondCatalogFileID = "2d3e4f5a-6b7c-4d8e-9f0a-1b2c3d4e5f6a"

	itemsPath = libraryPath + "/items"
	itemPath  = "/api/v1/items/" + testItemID

	// maxCursorLength is the contract's maxLength of the cursor parameter.
	maxCursorLength = 2048
)

// testDuration is 1:30:23.042123456; the API sends it with millisecond precision.
const testDuration = time.Hour + 30*time.Minute + 23*time.Second + 42123456*time.Nanosecond

func probedFile(t *testing.T) media.File {
	t.Helper()

	return media.File{
		ID: testCatalogFileID, ItemID: testItemID, LibraryID: testLibraryID,
		RelativePath: "Movies/Blender/Big.Buck.Bunny.mkv",
		Size:         276134947,
		ModifiedAt:   time.Date(2026, 10, 1, 11, 30, 0, 500000000, testZone()),
		Description: media.Description{
			Container: media.ContainerMKV, FormatName: "matroska,webm", Duration: testDuration, BitRate: 4073000,
			Streams: []media.Stream{
				{Index: 0, Kind: media.StreamKindVideo, Codec: "mjpeg", Width: 600, Height: 800, AttachedPicture: true},
				{
					Index: 1, Kind: media.StreamKindVideo, Codec: "hevc", Profile: "Main 10", Level: 120, BitDepth: 10,
					Width: 3840, Height: 2160, FrameRate: media.NewFrameRate(24000, 1001), HDR: media.HDRFormatHDR10,
					Default: true, BitRate: 3500000,
				},
				{
					Index: 2, Kind: media.StreamKindAudio, Codec: "eac3", Channels: 6, ChannelLayout: "5.1(side)",
					SampleRate: 48000, Language: "eng",
				},
				{Index: 3, Kind: media.StreamKindAudio, Codec: "aac", Channels: 2, SampleRate: 44100, Default: true},
				{Index: 4, Kind: media.StreamKindSubtitle, Codec: "subrip", Language: "ger", Title: "Forced", Forced: true},
				{Index: 5, Kind: media.StreamKindOther, Codec: "ttf"},
			},
		},
	}
}

func unprobedFile() media.File {
	return media.File{
		ID: testSecondCatalogFileID, ItemID: testSecondItemID, LibraryID: testLibraryID,
		RelativePath: `Broken\odd name.avi`,
		Size:         0,
		ModifiedAt:   time.Date(2026, 10, 2, 0, 0, 0, 0, time.UTC),
		ProbeError:   media.ProbeErrorTimedOut,
	}
}

func testItem(id, title string) library.Item {
	return library.Item{
		ID: id, LibraryID: testLibraryID, Kind: library.ItemKindMovie, Title: title, SortTitle: strings.ToLower(title),
		CreatedAt: time.Date(2026, 10, 30, 0, 0, 0, 0, time.UTC),
	}
}

func wantProbedFile(t *testing.T) apigen.MediaFile {
	t.Helper()

	return apigen.MediaFile{
		ID: mustUUID(t, testCatalogFileID), FileName: "Big.Buck.Bunny.mkv", SizeBytes: 276134947,
		ModifiedAt: time.Date(2026, 10, 1, 10, 30, 0, 500000000, time.UTC), Available: true,
		Container: ptr("mkv"), DurationSeconds: ptr(5423.042), BitRate: ptr(int64(4073000)),
		Streams: []apigen.MediaStream{
			{Index: 0, Kind: apigen.MediaStreamKindVideo, Codec: ptr("mjpeg"), Width: ptr(int32(600)), Height: ptr(int32(800)), AttachedPicture: true},
			{
				Index: 1, Kind: apigen.MediaStreamKindVideo, Codec: ptr("hevc"), Profile: ptr("Main 10"), Level: ptr(int32(120)),
				BitDepth: ptr(int32(10)), Width: ptr(int32(3840)), Height: ptr(int32(2160)), FrameRate: ptr(23.976),
				Hdr: ptr(apigen.HdrFormatHdr10), Default: true, BitRate: ptr(int64(3500000)),
			},
			{
				Index: 2, Kind: apigen.MediaStreamKindAudio, Codec: ptr("eac3"), Channels: ptr(int32(6)),
				ChannelLayout: ptr("5.1(side)"), SampleRate: ptr(int32(48000)), Language: ptr("eng"),
			},
			{Index: 3, Kind: apigen.MediaStreamKindAudio, Codec: ptr("aac"), Channels: ptr(int32(2)), SampleRate: ptr(int32(44100)), Default: true},
			{Index: 4, Kind: apigen.MediaStreamKindSubtitle, Codec: ptr("subrip"), Language: ptr("ger"), Title: ptr("Forced"), Forced: true},
			{Index: 5, Kind: apigen.MediaStreamKindOther, Codec: ptr("ttf")},
		},
	}
}

func wantUnprobedFile(t *testing.T) apigen.MediaFile {
	t.Helper()

	return apigen.MediaFile{
		ID: mustUUID(t, testSecondCatalogFileID), FileName: "odd name.avi",
		ModifiedAt: time.Date(2026, 10, 2, 0, 0, 0, 0, time.UTC),
		ErrorCode:  ptr("probe_timed_out"), Streams: []apigen.MediaStream{},
	}
}

// assertSameJSON compares got and want by their JSON encoding, which is what clients see
// and which prints readably when they differ.
func assertSameJSON(t *testing.T, got, want any) {
	t.Helper()

	gotJSON, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("encode got: %v", err)
	}

	wantJSON, err := json.Marshal(want)
	if err != nil {
		t.Fatalf("encode want: %v", err)
	}

	if string(gotJSON) != string(wantJSON) {
		t.Errorf("got  %s\nwant %s", gotJSON, wantJSON)
	}
}

func TestGetItem(t *testing.T) {
	t.Parallel()

	libraries := &fakeLibraryService{detail: applibrary.ItemDetail{
		Item:  testItem(testItemID, "Big Buck Bunny"),
		Files: []media.File{probedFile(t), unprobedFile()},
	}}
	handler, _ := newLibraryHandler(t, libraries, &fakeScanService{})

	got := serve(t, handler, http.MethodGet, itemPath)
	assertJSONResponse(t, got, http.StatusOK)

	assertSameJSON(t, decodeStrictly[apigen.ItemDetail](t, got.body), apigen.ItemDetail{
		ID: mustUUID(t, testItemID), LibraryID: mustUUID(t, testLibraryID), Title: "Big Buck Bunny",
		Files: []apigen.MediaFile{wantProbedFile(t), wantUnprobedFile(t)},
	})

	assertBodyContains(t, got.body, `"durationSeconds":5423.042,`, `"modifiedAt":"2026-10-01T10:30:00.5Z"`, `"streams":[]`)
	assertBodyLacks(t, got.body, "Movies/", "Blender", "Broken")

	if requested := libraries.requestedItems; len(requested) != 1 || requested[0] != testItemID {
		t.Errorf("requested items = %v, want [%s]", requested, testItemID)
	}
}

// TestGetItemSendsFileNamesOnly pins the path rule: whatever separators the stored relative
// path holds, the client gets the bare file name.
func TestGetItemSendsFileNamesOnly(t *testing.T) {
	t.Parallel()

	tests := []struct {
		relativePath string
		want         string
	}{
		{relativePath: "movie.mkv", want: "movie.mkv"},
		{relativePath: "a/b/c/movie.mkv", want: "movie.mkv"},
		{relativePath: `a/b\movie.mkv`, want: "movie.mkv"},
		{relativePath: `a\b\movie.mkv`, want: "movie.mkv"},
		{relativePath: "日本/映画 ファイル.mp4", want: "映画 ファイル.mp4"},
	}

	for _, test := range tests {
		t.Run(test.relativePath, func(t *testing.T) {
			t.Parallel()

			file := unprobedFile()
			file.RelativePath = test.relativePath

			libraries := &fakeLibraryService{detail: applibrary.ItemDetail{Item: testItem(testItemID, "x"), Files: []media.File{file}}}
			handler, _ := newLibraryHandler(t, libraries, &fakeScanService{})

			got := serve(t, handler, http.MethodGet, itemPath)
			assertJSONResponse(t, got, http.StatusOK)

			name := decodeStrictly[apigen.ItemDetail](t, got.body).Files[0].FileName
			if name != test.want || strings.ContainsAny(name, `/\`) {
				t.Errorf("fileName = %q, want %q without any separator", name, test.want)
			}
		})
	}
}

func TestGetItemErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		detail     applibrary.ItemDetail
		err        error
		wantStatus int
		wantCode   string
	}{
		{
			name:       "unknown item",
			err:        fmt.Errorf("get item %q: %w", testItemID, domain.ErrNotFound),
			wantStatus: http.StatusNotFound, wantCode: restapi.CodeNotFound,
		},
		{
			name:       "malformed stored item id",
			detail:     applibrary.ItemDetail{Item: testItem("item-1", "x"), Files: []media.File{unprobedFile()}},
			wantStatus: http.StatusInternalServerError, wantCode: restapi.CodeInternal,
		},
		{
			name: "malformed stored file id",
			detail: applibrary.ItemDetail{Item: testItem(testItemID, "x"), Files: []media.File{func() media.File {
				file := unprobedFile()
				file.ID = "file-1"

				return file
			}()}},
			wantStatus: http.StatusInternalServerError, wantCode: restapi.CodeInternal,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			handler, logs := newLibraryHandler(t, &fakeLibraryService{detail: test.detail, itemErr: test.err}, &fakeScanService{})

			got := serve(t, handler, http.MethodGet, itemPath)
			assertProblem(t, got, test.wantStatus, test.wantCode)
			assertLogged(t, logs.String(), "path="+itemPath)
		})
	}
}

func TestListLibraryItems(t *testing.T) {
	t.Parallel()

	libraries := &fakeLibraryService{page: applibrary.ItemPage{
		Items: []applibrary.ItemSummary{
			{Item: testItem(testItemID, "Big Buck Bunny"), PrimaryFile: probedFile(t)},
			{Item: testItem(testSecondItemID, "Odd Name"), PrimaryFile: unprobedFile()},
		},
		NextCursor: "bmV4dA",
	}}
	handler, _ := newLibraryHandler(t, libraries, &fakeScanService{})

	got := serve(t, handler, http.MethodGet, itemsPath)
	assertJSONResponse(t, got, http.StatusOK)

	assertSameJSON(t, decodeStrictly[apigen.ItemPage](t, got.body), apigen.ItemPage{
		Items: []apigen.ItemSummary{
			{
				ID: mustUUID(t, testItemID), LibraryID: mustUUID(t, testLibraryID), Title: "Big Buck Bunny",
				Media: &apigen.MediaSummary{
					Available: true, Container: ptr("mkv"), DurationSeconds: ptr(5423.042), VideoCodec: ptr("hevc"),
					AudioCodec: ptr("aac"), Width: ptr(int32(3840)), Height: ptr(int32(2160)), BitDepth: ptr(int32(10)),
					Hdr: ptr(apigen.HdrFormatHdr10),
				},
			},
			{
				ID: mustUUID(t, testSecondItemID), LibraryID: mustUUID(t, testLibraryID), Title: "Odd Name",
				Media: &apigen.MediaSummary{Available: false},
			},
		},
		NextCursor: ptr("bmV4dA"),
	})
	assertBodyContains(t, got.body, `"media":{"available":false}`)

	if pages := libraries.pages(); len(pages) != 1 || pages[0] != (library.PageRequest{}) {
		t.Errorf("page requests = %+v, want one default request", pages)
	}
}

func TestListLibraryItemsSummaryOfFilesWithoutContainerOrAudio(t *testing.T) {
	t.Parallel()

	file := probedFile(t)
	file.Description = media.Description{
		Container: media.ContainerUnknown,
		Streams:   []media.Stream{{Index: 0, Kind: media.StreamKindVideo, Codec: "mpeg4"}},
	}

	libraries := &fakeLibraryService{page: applibrary.ItemPage{Items: []applibrary.ItemSummary{{Item: testItem(testItemID, "x"), PrimaryFile: file}}}}
	handler, _ := newLibraryHandler(t, libraries, &fakeScanService{})

	got := serve(t, handler, http.MethodGet, itemsPath)
	assertJSONResponse(t, got, http.StatusOK)
	assertBodyContains(t, got.body, `"media":{"available":true,"videoCodec":"mpeg4"}`)
	assertBodyLacks(t, got.body, "nextCursor", "unknown")
}

func TestListLibraryItemsLastAndEmptyPages(t *testing.T) {
	t.Parallel()

	handler, _ := newLibraryHandler(t, &fakeLibraryService{}, &fakeScanService{})

	got := serve(t, handler, http.MethodGet, itemsPath+"?cursor=bGFzdA")
	assertJSONResponse(t, got, http.StatusOK)

	if body := strings.TrimSpace(string(got.body)); body != `{"items":[]}` {
		t.Errorf("body = %s, want an empty items array without nextCursor", body)
	}
}

func TestListLibraryItemsPassesPaging(t *testing.T) {
	t.Parallel()

	longestCursor := strings.Repeat("a", maxCursorLength)

	tests := []struct {
		query string
		want  library.PageRequest
	}{
		{query: "?limit=1", want: library.PageRequest{Limit: 1}},
		{query: "?limit=500", want: library.PageRequest{Limit: library.MaxPageLimit}},
		{query: "?limit=25&cursor=abc", want: library.PageRequest{Limit: 25, Cursor: "abc"}},
		{query: "?cursor=" + longestCursor, want: library.PageRequest{Cursor: longestCursor}},
	}

	for _, test := range tests {
		t.Run(test.query[:min(len(test.query), 32)], func(t *testing.T) {
			t.Parallel()

			libraries := &fakeLibraryService{}
			handler, _ := newLibraryHandler(t, libraries, &fakeScanService{})

			got := serve(t, handler, http.MethodGet, itemsPath+test.query)
			assertJSONResponse(t, got, http.StatusOK)

			if pages := libraries.pages(); len(pages) != 1 || pages[0] != test.want {
				t.Errorf("page requests = %+v, want %+v", pages, test.want)
			}
		})
	}
}

// TestListLibraryItemsRejectsInvalidPaging covers the parameters the handler validates
// before the service runs: the contract bounds of limit and cursor.
func TestListLibraryItemsRejectsInvalidPaging(t *testing.T) {
	t.Parallel()

	for _, query := range []string{
		"?limit=0",
		"?limit=-1",
		"?limit=501",
		"?limit=" + strconv.Itoa(1<<31-1),
		"?cursor=" + strings.Repeat("a", maxCursorLength+1),
	} {
		t.Run(query[:min(len(query), 32)], func(t *testing.T) {
			t.Parallel()

			libraries := &fakeLibraryService{}
			handler, _ := newLibraryHandler(t, libraries, &fakeScanService{})

			got := serve(t, handler, http.MethodGet, itemsPath+query)
			assertProblem(t, got, http.StatusBadRequest, restapi.CodeInvalid)

			if pages := libraries.pages(); len(pages) != 0 {
				t.Errorf("service called with %+v, want no call", pages)
			}
		})
	}
}

// TestListLibraryItemsCursorRoundTrip covers the 2026-10-30 decision: cursors of very long
// file names exceed 512 characters and must survive the way out and back unchanged.
func TestListLibraryItemsCursorRoundTrip(t *testing.T) {
	t.Parallel()

	const cursorLength = 1500

	cursor := strings.Repeat("5pel-_", cursorLength/len("5pel-_"))
	libraries := &fakeLibraryService{page: applibrary.ItemPage{
		Items:      []applibrary.ItemSummary{{Item: testItem(testItemID, "x"), PrimaryFile: unprobedFile()}},
		NextCursor: cursor,
	}}
	handler, _ := newLibraryHandler(t, libraries, &fakeScanService{})

	first := serve(t, handler, http.MethodGet, itemsPath)
	assertJSONResponse(t, first, http.StatusOK)

	next := decodeStrictly[apigen.ItemPage](t, first.body).NextCursor
	if next == nil || len(*next) != cursorLength || *next != cursor {
		t.Fatalf("nextCursor = %v, want the %d character cursor", next, cursorLength)
	}

	second := serve(t, handler, http.MethodGet, itemsPath+"?cursor="+url.QueryEscape(*next))
	assertJSONResponse(t, second, http.StatusOK)

	if pages := libraries.pages(); len(pages) != 2 || pages[1].Cursor != cursor {
		t.Errorf("second page request = %+v, want the cursor unchanged", pages)
	}
}

func TestListLibraryItemsErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		page       applibrary.ItemPage
		err        error
		wantStatus int
		wantCode   string
	}{
		{name: "bad cursor", err: fmt.Errorf("decode cursor: %w", domain.ErrInvalid), wantStatus: http.StatusBadRequest, wantCode: restapi.CodeInvalid},
		{name: "unknown library", err: errLibraryNotFound(testLibraryID), wantStatus: http.StatusNotFound, wantCode: restapi.CodeNotFound},
		{name: "stored data", err: errors.New("unreadable stored library data"), wantStatus: http.StatusInternalServerError, wantCode: restapi.CodeInternal},
		{name: "wraps unavailable", err: fmt.Errorf("read: %w", domain.ErrUnavailable), wantStatus: http.StatusInternalServerError, wantCode: restapi.CodeInternal},
		{name: "wraps forbidden", err: fmt.Errorf("read: %w", domain.ErrForbidden), wantStatus: http.StatusInternalServerError, wantCode: restapi.CodeInternal},
		{
			name:       "malformed stored library id",
			page:       applibrary.ItemPage{Items: []applibrary.ItemSummary{{Item: library.Item{ID: testItemID, LibraryID: "lib"}, PrimaryFile: unprobedFile()}}},
			wantStatus: http.StatusInternalServerError, wantCode: restapi.CodeInternal,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			handler, logs := newLibraryHandler(t, &fakeLibraryService{page: test.page, pageErr: test.err}, &fakeScanService{})

			got := serve(t, handler, http.MethodGet, itemsPath)
			assertProblem(t, got, test.wantStatus, test.wantCode)
			assertLogged(t, logs.String(), "path="+itemsPath)
		})
	}
}

// TestItemOperationsNeedNoScanService documents that the item endpoints only use the
// library service, so the scan status of a library cannot break them.
func TestItemOperationsNeedNoScanService(t *testing.T) {
	t.Parallel()

	libraries := &fakeLibraryService{detail: applibrary.ItemDetail{Item: testItem(testItemID, "x"), Files: []media.File{unprobedFile()}}}
	handler, _ := newTestHandlerWithOptions(t, restapi.Options{Library: libraries, Scan: &fakeScanService{
		statuses: map[string]appscan.Status{testLibraryID: {State: "paused"}},
	}})

	for _, target := range []string{itemsPath, itemPath} {
		assertJSONResponse(t, serve(t, handler, http.MethodGet, target), http.StatusOK)
	}
}
