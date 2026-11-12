package stream_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/magpeek/mivo/internal/adapters/http/restapi"
	"github.com/magpeek/mivo/internal/adapters/http/stream"
	appaccess "github.com/magpeek/mivo/internal/app/access"
	appsubtitles "github.com/magpeek/mivo/internal/app/subtitles"
	"github.com/magpeek/mivo/internal/domain"
)

const (
	subtitleFileID = "f2000000-0000-4000-8000-000000000001"
	subtitleBody   = "WEBVTT\n\n00:00.000 --> 00:01.000\nHello\n"
)

var errDisk = errors.New("disk exploded")

// fakeSubtitles is an in-memory Subtitles: like the real service it needs a viewer.
type fakeSubtitles struct {
	mutex   sync.Mutex
	err     error
	block   bool
	modTime time.Time
	calls   []string
}

type closingReader struct{ *bytes.Reader }

func (closingReader) Close() error { return nil }

func (fake *fakeSubtitles) WebVTT(ctx context.Context, fileID, trackID string) (appsubtitles.WebVTT, error) {
	fake.mutex.Lock()
	fake.calls = append(fake.calls, fileID+"|"+trackID)
	err, block, modTime := fake.err, fake.block, fake.modTime
	fake.mutex.Unlock()

	if _, viewerErr := appaccess.RestrictionFrom(ctx); viewerErr != nil {
		return appsubtitles.WebVTT{}, fmt.Errorf("subtitle: %w", viewerErr)
	}

	if block {
		<-ctx.Done()

		return appsubtitles.WebVTT{}, fmt.Errorf("wait for conversion: %w", ctx.Err())
	}

	if err != nil {
		return appsubtitles.WebVTT{}, err
	}

	return appsubtitles.WebVTT{
		Content: closingReader{bytes.NewReader([]byte(subtitleBody))}, Size: int64(len(subtitleBody)), ModTime: modTime,
	}, nil
}

func (fake *fakeSubtitles) callCount() int {
	fake.mutex.Lock()
	defer fake.mutex.Unlock()

	return len(fake.calls)
}

// fakeSheets is an in-memory Trickplay: like the real service it needs a viewer.
type fakeSheets struct {
	mutex sync.Mutex
	path  string
	err   error
	calls []string
}

func (fake *fakeSheets) Sheet(ctx context.Context, fileID string, sheet int) (string, error) {
	fake.mutex.Lock()
	defer fake.mutex.Unlock()

	fake.calls = append(fake.calls, fmt.Sprintf("%s|%d", fileID, sheet))

	if _, err := appaccess.RestrictionFrom(ctx); err != nil {
		return "", fmt.Errorf("sheet: %w", err)
	}

	return fake.path, fake.err
}

func subtitleURL(trackFile string) string {
	return "/api/v1/media-files/" + subtitleFileID + "/subtitles/" + trackFile
}

func sheetURL(sheetFile string) string {
	return "/api/v1/media-files/" + subtitleFileID + "/trickplay/" + sheetFile
}

func TestSubtitleIsServedWithCachingHeaders(t *testing.T) {
	t.Parallel()

	env := newEnvironment(t)
	env.subtitles.modTime = time.Date(2026, 11, 1, 10, 0, 0, 0, time.UTC)

	recorder := env.get(t, subtitleURL("s:3.vtt"))
	if recorder.Code != http.StatusOK || recorder.Body.String() != subtitleBody {
		t.Fatalf("GET = %d %q, want 200 with the WebVTT body", recorder.Code, recorder.Body.String())
	}

	assertHeader(t, recorder.Header(), "Content-Type", "text/vtt; charset=utf-8")
	assertHeader(t, recorder.Header(), "Cache-Control", "private, max-age=3600")

	etag := recorder.Header().Get("ETag")
	if etag == "" || etag[0] != '"' {
		t.Fatalf("ETag = %q, want a quoted validator", etag)
	}

	if got := env.subtitles.calls; len(got) != 1 || got[0] != subtitleFileID+"|s:3" {
		t.Errorf("service calls = %v, want one for the canonical file and track id", got)
	}

	conditional := newRequest(t, http.MethodGet, subtitleURL("s:3.vtt"))
	conditional.Header.Set("If-None-Match", etag)

	if again := env.serve(conditional); again.Code != http.StatusNotModified || again.Body.Len() != 0 {
		t.Errorf("conditional GET = %d with %d bytes, want 304 without body", again.Code, again.Body.Len())
	}
}

func TestSubtitleSupportsRangeAndHead(t *testing.T) {
	t.Parallel()

	env := newEnvironment(t)

	ranged := newRequest(t, http.MethodGet, subtitleURL("x:ab12.vtt"))
	ranged.Header.Set("Range", "bytes=0-5")

	partial := env.serve(ranged)
	if partial.Code != http.StatusPartialContent || partial.Body.String() != subtitleBody[:6] {
		t.Errorf("Range GET = %d %q, want 206 %q", partial.Code, partial.Body.String(), subtitleBody[:6])
	}

	head := env.serve(newRequest(t, http.MethodHead, subtitleURL("x:ab12.vtt")))
	if head.Code != http.StatusOK || head.Body.Len() != 0 {
		t.Errorf("HEAD = %d with %d bytes, want 200 without body", head.Code, head.Body.Len())
	}

	assertHeader(t, head.Header(), "Content-Length", fmt.Sprint(len(subtitleBody)))
}

func TestSubtitleRejectsOddIdentifiersBeforeAnyLookup(t *testing.T) {
	t.Parallel()

	tests := map[string]string{
		"traversal":        "..%2F..%2Fetc%2Fpasswd.vtt",
		"parent":           "...vtt",
		"missing suffix":   "s:3",
		"wrong suffix":     "s:3.txt",
		"negative index":   "s:-1.vtt",
		"index too long":   "s:1234567890.vtt",
		"uppercase hash":   "x:AB.vtt",
		"hash too long":    "x:" + string(bytes.Repeat([]byte("a"), 65)) + ".vtt",
		"unknown prefix":   "y:1.vtt",
		"empty id":         ".vtt",
		"colon only":       "s:.vtt",
		"nested":           "s:3%2Fx.vtt",
		"double extension": "s:3.vtt.vtt",
	}

	for name, trackFile := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			env := newEnvironment(t)
			assertProblem(t, env.get(t, subtitleURL(trackFile)), http.StatusNotFound, "not_found")

			if calls := env.subtitles.callCount(); calls != 0 {
				t.Errorf("service called %d times for %q, want none", calls, trackFile)
			}
		})
	}
}

func TestSubtitleRejectsAMalformedFileID(t *testing.T) {
	t.Parallel()

	env := newEnvironment(t)

	recorder := env.get(t, "/api/v1/media-files/not-a-uuid/subtitles/s:3.vtt")
	assertProblem(t, recorder, http.StatusNotFound, "not_found")

	if env.subtitles.callCount() != 0 {
		t.Error("service was called for a malformed file id")
	}
}

func TestSubtitleFailuresAreMappedInTheAdapter(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		err        error
		wantStatus int
		wantCode   string
	}{
		"unknown or hidden":  {fmt.Errorf("file: %w", domain.ErrNotFound), http.StatusNotFound, "not_found"},
		"picture subtitle":   {fmt.Errorf("track: %w", domain.ErrInvalid), http.StatusUnprocessableEntity, "unsupported"},
		"path left the root": {fmt.Errorf("source: %w", domain.ErrForbidden), http.StatusUnprocessableEntity, "unsupported"},
		"root unreachable":   {fmt.Errorf("root: %w", domain.ErrUnavailable), http.StatusServiceUnavailable, "unavailable"},
		"converter failed":   {fmt.Errorf("convert: %w", errDisk), http.StatusInternalServerError, "internal_error"},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			env := newEnvironment(t)
			env.subtitles.err = test.err

			recorder := env.get(t, subtitleURL("s:1.vtt"))
			assertProblem(t, recorder, test.wantStatus, test.wantCode)

			if body := recorder.Body.String(); bytes.Contains([]byte(body), []byte("disk exploded")) {
				t.Errorf("body %q leaks the error text", body)
			}
		})
	}
}

func TestSubtitleWithoutAViewerIsUnauthenticated(t *testing.T) {
	t.Parallel()

	env := newEnvironment(t)
	api := mustAPIWithoutViewer(t, env.handlers.Mounts())

	for _, target := range []string{subtitleURL("s:1.vtt"), sheetURL("0.jpg")} {
		recorder := httptest.NewRecorder()
		api.ServeHTTP(recorder, newRequest(t, http.MethodGet, target))
		assertProblem(t, recorder, http.StatusUnauthorized, "unauthenticated")
	}
}

func mustAPIWithoutViewer(t *testing.T, mounts []restapi.Mount) http.Handler {
	t.Helper()

	logger := slog.New(slog.DiscardHandler)

	api, err := restapi.NewHandler(restapi.Options{
		System: healthySystem{}, Libraries: unusedLibraries{}, Items: unusedItems{},
		Scan: unusedScans{}, Playback: unusedPlayback{}, Manager: unusedManager{}, Folders: unusedFolders{},
		Logger: logger, Mounts: mounts,
	})
	if err != nil {
		t.Fatalf("restapi.NewHandler error = %v", err)
	}

	return api
}

func TestSubtitleConversionQueuedTooLongAnswers503WithRetryAfter(t *testing.T) {
	t.Parallel()

	env := newEnvironment(t)
	env.subtitles.block = true

	stream.SetConversionTimeout(env.handlers, 20*time.Millisecond)

	recorder := env.get(t, subtitleURL("s:2.vtt"))
	assertProblem(t, recorder, http.StatusServiceUnavailable, "unavailable")
	assertHeader(t, recorder.Header(), "Retry-After", "5")
}

func TestConversionTimeoutIsTwentySeconds(t *testing.T) {
	t.Parallel()

	if stream.ConversionTimeout != 20*time.Second {
		t.Errorf("ConversionTimeout = %s, want 20s (task M4-14a)", stream.ConversionTimeout)
	}
}

func TestTrickplaySheetIsServed(t *testing.T) {
	t.Parallel()

	env := newEnvironment(t)
	jpeg := append([]byte{0xff, 0xd8, 0xff}, bytes.Repeat([]byte{7}, 200)...)
	env.sheets.path = filepath.Join(t.TempDir(), "sheet-001.jpg")
	writeFile(t, env.sheets.path, jpeg)

	recorder := env.get(t, sheetURL("12.jpg"))
	if recorder.Code != http.StatusOK || !bytes.Equal(recorder.Body.Bytes(), jpeg) {
		t.Fatalf("GET = %d with %d bytes, want 200 with the sheet", recorder.Code, recorder.Body.Len())
	}

	assertHeader(t, recorder.Header(), "Content-Type", "image/jpeg")
	assertHeader(t, recorder.Header(), "Cache-Control", "private, max-age=86400")

	if got := env.sheets.calls; len(got) != 1 || got[0] != subtitleFileID+"|12" {
		t.Errorf("service calls = %v, want sheet 12 of the file", got)
	}

	ranged := newRequest(t, http.MethodGet, sheetURL("0.jpg"))
	ranged.Header.Set("Range", "bytes=0-2")

	if partial := env.serve(ranged); partial.Code != http.StatusPartialContent || !bytes.Equal(partial.Body.Bytes(), jpeg[:3]) {
		t.Errorf("Range GET = %d %v, want 206 with 3 bytes", partial.Code, partial.Body.Bytes())
	}

	if head := env.serve(newRequest(t, http.MethodHead, sheetURL("0.jpg"))); head.Code != http.StatusOK || head.Body.Len() != 0 {
		t.Errorf("HEAD = %d with %d bytes, want 200 without body", head.Code, head.Body.Len())
	}
}

func TestTrickplayRejectsOddSheetNamesBeforeAnyLookup(t *testing.T) {
	t.Parallel()

	for _, sheetFile := range []string{
		"00.jpg", "001.jpg", "-1.jpg", "1.5.jpg", "a.jpg", "10000.jpg", "0", "0.png", ".jpg", "..%2Fx.jpg", "0.jpg.jpg",
	} {
		t.Run(sheetFile, func(t *testing.T) {
			t.Parallel()

			env := newEnvironment(t)
			assertProblem(t, env.get(t, sheetURL(sheetFile)), http.StatusNotFound, "not_found")

			if len(env.sheets.calls) != 0 {
				t.Errorf("service called for %q, want no lookup", sheetFile)
			}
		})
	}
}

func TestTrickplayFailuresAreMappedInTheAdapter(t *testing.T) {
	t.Parallel()

	missing := filepath.Join(t.TempDir(), "gone.jpg")

	tests := map[string]struct {
		path       string
		err        error
		wantStatus int
		wantCode   string
	}{
		"unknown or hidden":    {err: fmt.Errorf("file: %w", domain.ErrNotFound), wantStatus: http.StatusNotFound, wantCode: "not_found"},
		"sheet not on disk":    {path: missing, wantStatus: http.StatusNotFound, wantCode: "not_found"},
		"sheet is a directory": {path: t.TempDir(), wantStatus: http.StatusNotFound, wantCode: "not_found"},
		"store unavailable":    {err: fmt.Errorf("store: %w", domain.ErrUnavailable), wantStatus: http.StatusServiceUnavailable, wantCode: "unavailable"},
		"store failed":         {err: fmt.Errorf("store: %w", errDisk), wantStatus: http.StatusInternalServerError, wantCode: "internal_error"},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			env := newEnvironment(t)
			env.sheets.path, env.sheets.err = test.path, test.err

			assertProblem(t, env.get(t, sheetURL("0.jpg")), test.wantStatus, test.wantCode)
		})
	}
}

func TestTrickplayRejectsAMalformedFileID(t *testing.T) {
	t.Parallel()

	env := newEnvironment(t)
	assertProblem(t, env.get(t, "/api/v1/media-files/x/trickplay/0.jpg"), http.StatusNotFound, "not_found")

	if len(env.sheets.calls) != 0 {
		t.Error("service was called for a malformed file id")
	}
}

func TestOptionalRoutesAreMountedOnlyWithTheirService(t *testing.T) {
	t.Parallel()

	env := newEnvironment(t)
	handlers := mustHandlers(t, stream.Options{
		Sessions: env.service, Library: mustLibraryFS(t, slog.New(slog.DiscardHandler)),
		Output: mustWorkspace(t), Logger: slog.New(slog.DiscardHandler),
	})

	if mounts := handlers.Mounts(); len(mounts) != 2 {
		t.Errorf("mounts without subtitles and trickplay = %d, want the 2 session routes", len(mounts))
	}
}
