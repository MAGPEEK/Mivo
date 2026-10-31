package restapi_test

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"
	"uuid"

	"github.com/magpeek/mivo/internal/adapters/http/apigen"
	"github.com/magpeek/mivo/internal/adapters/http/logtext"
	"github.com/magpeek/mivo/internal/adapters/http/restapi"
	applibrary "github.com/magpeek/mivo/internal/app/library"
	appscan "github.com/magpeek/mivo/internal/app/scan"
	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/library"
)

const (
	testLibraryID       = "3f6c1d2a-9b8e-4f70-8a1c-2d3e4f5a6b7c"
	testSecondLibraryID = "4a7d2e3b-0c9f-4a81-9b2d-3e4f5a6b7c8d"
	testItemID          = "8a9b0c1d-2e3f-4a5b-8c6d-7e8f9a0b1c2d"
	unknownID           = "00000000-0000-4000-8000-000000000000"

	librariesPath = "/api/v1/libraries"
	libraryPath   = librariesPath + "/" + testLibraryID
	scanPath      = libraryPath + "/scan"

	// testRoot is the absolute library root; it must never reach a response body.
	testRoot = `D:\Media\Movies`
)

func errLibraryNotFound(id string) error {
	return fmt.Errorf("get library %q: %w", id, domain.ErrNotFound)
}

// testZone gives the test times a non-UTC zone (UTC+1), so the tests see the conversion to
// UTC.
func testZone() *time.Location {
	return time.FixedZone("CET", int(time.Hour/time.Second))
}

func mustUUID(t *testing.T, text string) uuid.UUID {
	t.Helper()

	id, err := uuid.Parse(text)
	if err != nil {
		t.Fatalf("parse uuid %q: %v", text, err)
	}

	return id
}

func testOverviews() []applibrary.Overview {
	return []applibrary.Overview{
		{
			Summary: library.Summary{
				Library:   library.Library{ID: testLibraryID, Name: "Movies", Root: testRoot, Kind: library.KindMovies},
				ItemCount: 42,
			},
			Available: true,
		},
		{
			Summary: library.Summary{
				Library: library.Library{ID: testSecondLibraryID, Name: "Shows", Root: `\\nas\shows`, Kind: library.KindShows},
			},
		},
	}
}

func runningStatus(t *testing.T) appscan.Status {
	t.Helper()

	return appscan.Status{
		State:     appscan.StateRunning,
		StartedAt: time.Date(2026, 10, 30, 19, 4, 5, 123456789, testZone()),
		Counts:    appscan.Counts{Added: 12, Updated: 1, Removed: 2, Unchanged: 29, Failed: 3, Skipped: 4},
	}
}

func failedStatus(t *testing.T) appscan.Status {
	t.Helper()

	return appscan.Status{
		State:      appscan.StateFailed,
		StartedAt:  time.Date(2026, 10, 30, 19, 4, 5, 0, testZone()),
		FinishedAt: time.Date(2026, 10, 30, 19, 4, 31, 999999999, testZone()),
		ErrorCode:  appscan.ErrorCodeRootUnavailable,
	}
}

func TestListLibraries(t *testing.T) {
	t.Parallel()

	libraries := &fakeLibraryService{overviews: testOverviews()}
	scans := &fakeScanService{statuses: map[string]appscan.Status{testLibraryID: runningStatus(t)}}
	handler, _ := newLibraryHandler(t, libraries, scans)

	got := serve(t, handler, http.MethodGet, librariesPath)
	assertJSONResponse(t, got, http.StatusOK)

	list := decodeStrictly[apigen.LibraryList](t, got.body)
	if len(list.Libraries) != 2 {
		t.Fatalf("libraries = %+v, want 2", list.Libraries)
	}

	assertLibrary(t, list.Libraries[0], apigen.Library{
		ID: mustUUID(t, testLibraryID), Name: "Movies", Kind: apigen.LibraryKindMovies, Available: true, ItemCount: 42,
		Scan: apigen.ScanStatus{
			State:     apigen.ScanStateRunning,
			StartedAt: ptr(time.Date(2026, 10, 30, 18, 4, 5, 123000000, time.UTC)),
			Counts:    apigen.ScanCounts{Added: 12, Updated: 1, Removed: 2, Unchanged: 29, Failed: 3, Skipped: 4},
		},
	})
	assertLibrary(t, list.Libraries[1], apigen.Library{
		ID: mustUUID(t, testSecondLibraryID), Name: "Shows", Kind: apigen.LibraryKindShows,
		Scan: apigen.ScanStatus{State: apigen.ScanStateIdle},
	})

	// Timestamps are RFC 3339 UTC with millisecond precision; a never scanned library has
	// no times; the absolute root never leaves the server.
	assertBodyContains(t, got.body, `"startedAt":"2026-10-30T18:04:05.123Z"`, `"available":false`, `"itemCount":0`)
	assertBodyLacks(t, got.body, "finishedAt", "errorCode", "Media", "nas")
}

func TestListLibrariesWithoutLibrariesAnswersAnEmptyArray(t *testing.T) {
	t.Parallel()

	handler, _ := newLibraryHandler(t, &fakeLibraryService{}, &fakeScanService{})

	got := serve(t, handler, http.MethodGet, librariesPath)
	assertJSONResponse(t, got, http.StatusOK)

	if body := strings.TrimSpace(string(got.body)); body != `{"libraries":[]}` {
		t.Errorf("body = %s, want an empty libraries array", body)
	}
}

func TestGetLibrary(t *testing.T) {
	t.Parallel()

	libraries := &fakeLibraryService{overviews: testOverviews()}
	scans := &fakeScanService{statuses: map[string]appscan.Status{testLibraryID: failedStatus(t)}}
	handler, _ := newLibraryHandler(t, libraries, scans)

	got := serve(t, handler, http.MethodGet, libraryPath)
	assertJSONResponse(t, got, http.StatusOK)

	assertLibrary(t, decodeStrictly[apigen.Library](t, got.body), apigen.Library{
		ID: mustUUID(t, testLibraryID), Name: "Movies", Kind: apigen.LibraryKindMovies, Available: true, ItemCount: 42,
		Scan: apigen.ScanStatus{
			State:      apigen.ScanStateFailed,
			StartedAt:  ptr(time.Date(2026, 10, 30, 18, 4, 5, 0, time.UTC)),
			FinishedAt: ptr(time.Date(2026, 10, 30, 18, 4, 31, 999000000, time.UTC)),
			ErrorCode:  ptr("root_unavailable"),
		},
	})
	assertBodyContains(t, got.body, `"startedAt":"2026-10-30T18:04:05Z"`, `"finishedAt":"2026-10-30T18:04:31.999Z"`)
}

func TestGetLibraryUnknownIDAnswersNotFound(t *testing.T) {
	t.Parallel()

	handler, logs := newLibraryHandler(t, &fakeLibraryService{overviews: testOverviews()}, &fakeScanService{})

	got := serve(t, handler, http.MethodGet, librariesPath+"/"+unknownID)
	assertProblem(t, got, http.StatusNotFound, restapi.CodeNotFound)
	assertLogged(t, logs.String(), "path="+librariesPath+"/"+unknownID)
}

// TestLibraryReadFailuresAreInternal pins the orchestrator rule: listLibraries, getLibrary
// and getItem take no client value besides path IDs, so every failure other than
// ErrNotFound is a 500 without the error text, whatever sentinel it wraps.
func TestLibraryReadFailuresAreInternal(t *testing.T) {
	t.Parallel()

	causes := []struct {
		name string
		err  error
	}{
		{name: "plain error", err: errors.New("disk I/O error at " + testRoot)},
		{name: "wraps invalid", err: fmt.Errorf("stored kind in %s: %w", testRoot, domain.ErrInvalid)},
		{name: "wraps conflict", err: fmt.Errorf("read %s: %w", testRoot, domain.ErrConflict)},
		{name: "wraps forbidden", err: fmt.Errorf("open %s: %w", testRoot, domain.ErrForbidden)},
		{name: "wraps unavailable", err: fmt.Errorf("open %s: %w", testRoot, domain.ErrUnavailable)},
		{name: "wraps unsupported", err: fmt.Errorf("read %s: %w", testRoot, domain.ErrUnsupported)},
	}

	for _, cause := range causes {
		for _, target := range []string{librariesPath, libraryPath, itemPath} {
			t.Run(cause.name+" "+target, func(t *testing.T) {
				t.Parallel()

				libraries := &fakeLibraryService{listErr: cause.err, getErr: cause.err, itemErr: cause.err}
				handler, logs := newLibraryHandler(t, libraries, &fakeScanService{})

				got := serve(t, handler, http.MethodGet, target)
				assertProblem(t, got, http.StatusInternalServerError, restapi.CodeInternal)
				assertInternalFailureLogged(t, got, logs.String(), target)
			})
		}
	}
}

// TestListLibrariesNotFoundIsInternal: listLibraries has no ID a client could get wrong, so
// even ErrNotFound from the catalog is a server fault (500), not a 404 for the collection.
func TestListLibrariesNotFoundIsInternal(t *testing.T) {
	t.Parallel()

	cause := fmt.Errorf("library table of %s: %w", testRoot, domain.ErrNotFound)
	handler, logs := newLibraryHandler(t, &fakeLibraryService{listErr: cause}, &fakeScanService{})

	got := serve(t, handler, http.MethodGet, librariesPath)
	assertProblem(t, got, http.StatusInternalServerError, restapi.CodeInternal)
	assertInternalFailureLogged(t, got, logs.String(), librariesPath)
}

// assertInternalFailureLogged checks that a 500 hides the cause from the client and logs it
// at error level together with the request path.
func assertInternalFailureLogged(t *testing.T, got response, logs, path string) {
	t.Helper()

	assertBodyLacks(t, got.body, "Media", "D:")

	for _, want := range []string{"level=ERROR", "Media", "path=" + path} {
		if !strings.Contains(logs, want) {
			t.Errorf("log %q does not contain %q", logs, want)
		}
	}
}

// TestCorruptStoredLibrariesAreInternal covers stored values the contract cannot express:
// they are a server fault (500), never a bad request.
func TestCorruptStoredLibrariesAreInternal(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		mutate func(*applibrary.Overview)
		status appscan.Status
	}{
		{name: "unknown kind", mutate: func(overview *applibrary.Overview) { overview.Library.Kind = "music" }},
		{name: "malformed id", mutate: func(overview *applibrary.Overview) { overview.Library.ID = "not-a-uuid" }},
		{name: "unknown scan state", mutate: func(*applibrary.Overview) {}, status: appscan.Status{State: "paused"}},
	}

	for _, test := range tests {
		for _, target := range []string{librariesPath, libraryPath} {
			t.Run(test.name+" "+target, func(t *testing.T) {
				t.Parallel()

				overviews := testOverviews()
				test.mutate(&overviews[0])

				// found answers getLibrary even when the mutation broke the stored ID.
				libraries := &fakeLibraryService{overviews: overviews, found: &overviews[0]}
				scans := &fakeScanService{statuses: map[string]appscan.Status{}}
				if test.status.State != "" {
					scans.statuses[testLibraryID] = test.status
				}

				handler, logs := newLibraryHandler(t, libraries, scans)

				got := serve(t, handler, http.MethodGet, target)
				assertProblem(t, got, http.StatusInternalServerError, restapi.CodeInternal)
				assertLogged(t, logs.String(), "level=ERROR")
			})
		}
	}
}

func TestStartLibraryScan(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		status appscan.Status
		want   apigen.ScanStatus
	}{
		{
			name:   "new scan",
			status: appscan.Status{State: appscan.StateRunning, StartedAt: time.Date(2026, 10, 30, 19, 4, 5, 0, testZone())},
			want:   apigen.ScanStatus{State: apigen.ScanStateRunning, StartedAt: ptr(time.Date(2026, 10, 30, 18, 4, 5, 0, time.UTC))},
		},
		{
			name:   "scan already running",
			status: runningStatus(t),
			want: apigen.ScanStatus{
				State:     apigen.ScanStateRunning,
				StartedAt: ptr(time.Date(2026, 10, 30, 18, 4, 5, 123000000, time.UTC)),
				Counts:    apigen.ScanCounts{Added: 12, Updated: 1, Removed: 2, Unchanged: 29, Failed: 3, Skipped: 4},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			scans := &fakeScanService{startStatus: test.status}
			handler, _ := newLibraryHandler(t, &fakeLibraryService{}, scans)

			got := serve(t, handler, http.MethodPost, scanPath)
			assertJSONResponse(t, got, http.StatusAccepted)
			assertScanStatus(t, decodeStrictly[apigen.ScanStatus](t, got.body), test.want)
			assertBodyLacks(t, got.body, "finishedAt", "errorCode")
			assertBodyContains(t, got.body, `"counts":{`)

			if started := scans.startedIDs(); len(started) != 1 || started[0] != testLibraryID {
				t.Errorf("started = %v, want [%s]", started, testLibraryID)
			}
		})
	}
}

func TestStartLibraryScanErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		err        error
		wantStatus int
		wantCode   string
	}{
		{name: "unknown library", err: errLibraryNotFound(testLibraryID), wantStatus: http.StatusNotFound, wantCode: restapi.CodeNotFound},
		{
			name:       "server shutting down",
			err:        fmt.Errorf("start scan: scan service is closed: %w", domain.ErrUnavailable),
			wantStatus: http.StatusServiceUnavailable, wantCode: restapi.CodeUnavailable,
		},
		{
			name:       "corrupt stored library",
			err:        fmt.Errorf("start scan: stored kind: %w", domain.ErrInvalid),
			wantStatus: http.StatusInternalServerError, wantCode: restapi.CodeInternal,
		},
		{name: "plain error", err: errors.New("database is locked"), wantStatus: http.StatusInternalServerError, wantCode: restapi.CodeInternal},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			handler, logs := newLibraryHandler(t, &fakeLibraryService{}, &fakeScanService{startErr: test.err})

			got := serve(t, handler, http.MethodPost, scanPath)
			assertProblem(t, got, test.wantStatus, test.wantCode)
			assertLogged(t, logs.String(), "path="+scanPath)
		})
	}
}

func TestStartLibraryScanRejectsUnknownState(t *testing.T) {
	t.Parallel()

	handler, logs := newLibraryHandler(t, &fakeLibraryService{}, &fakeScanService{startStatus: appscan.Status{State: "paused"}})

	got := serve(t, handler, http.MethodPost, scanPath)
	assertProblem(t, got, http.StatusInternalServerError, restapi.CodeInternal)
	assertLogged(t, logs.String(), "paused")
}

func assertLibrary(t *testing.T, got, want apigen.Library) {
	t.Helper()

	if got.ID != want.ID || got.Name != want.Name || got.Kind != want.Kind ||
		got.Available != want.Available || got.ItemCount != want.ItemCount {
		t.Errorf("library = %+v, want %+v", got, want)
	}

	assertScanStatus(t, got.Scan, want.Scan)
}

func assertScanStatus(t *testing.T, got, want apigen.ScanStatus) {
	t.Helper()

	if got.State != want.State || got.Counts != want.Counts {
		t.Errorf("scan = %+v, want %+v", got, want)
	}

	assertSameTime(t, "startedAt", got.StartedAt, want.StartedAt)
	assertSameTime(t, "finishedAt", got.FinishedAt, want.FinishedAt)

	if (got.ErrorCode == nil) != (want.ErrorCode == nil) || (got.ErrorCode != nil && *got.ErrorCode != *want.ErrorCode) {
		t.Errorf("errorCode = %v, want %v", got.ErrorCode, want.ErrorCode)
	}
}

func assertSameTime(t *testing.T, name string, got, want *time.Time) {
	t.Helper()

	if (got == nil) != (want == nil) {
		t.Errorf("%s = %v, want %v", name, got, want)

		return
	}

	if got != nil && (!got.Equal(*want) || got.Location() != time.UTC) {
		t.Errorf("%s = %v, want %v in UTC", name, got, want)
	}
}

// TestLibraryLogsCutLongClientText checks that a failure caused by a long client value is
// logged cut to the logtext bound (M2-08), not with the whole value.
func TestLibraryLogsCutLongClientText(t *testing.T) {
	t.Parallel()

	cursor := strings.Repeat("c", 2000)
	libraries := &fakeLibraryService{pageErr: fmt.Errorf("decode cursor %q: %w", cursor, domain.ErrInvalid)}
	handler, logs := newLibraryHandler(t, libraries, &fakeScanService{})

	got := serve(t, handler, http.MethodGet, itemsPath+"?cursor="+cursor)
	assertProblem(t, got, http.StatusBadRequest, restapi.CodeInvalid)

	if strings.Contains(logs.String(), cursor) {
		t.Errorf("log contains the whole %d byte cursor", len(cursor))
	}

	assertLogged(t, logs.String(), logtext.TruncationMarker)
	assertLogged(t, logs.String(), "path="+itemsPath)
}
