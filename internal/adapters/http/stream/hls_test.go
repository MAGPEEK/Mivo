package stream_test

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/magpeek/mivo/internal/adapters/http/restapi"
	"github.com/magpeek/mivo/internal/adapters/http/stream"
)

const (
	playlistBody = "#EXTM3U\n#EXT-X-VERSION:7\n#EXT-X-MAP:URI=\"init.mp4\"\n#EXTINF:6.0,\nseg-00000.m4s\n"
	initBody     = "init segment"
	segmentBody  = "media segment"
	// shortReadyTimeout replaces the 20 s readiness wait in tests that time out.
	shortReadyTimeout = 50 * time.Millisecond
	// promptAnswer is how soon a cancelled request must be answered: far below the 20 s
	// readiness bound, far above scheduling noise on a loaded CI machine.
	promptAnswer = 2 * time.Second
)

func TestHLSServesItsFilesWithTypesAndCaching(t *testing.T) {
	t.Parallel()

	env := newEnvironment(t)
	session, production := env.startHLS(t)
	writeOutput(t, production, "index.m3u8", []byte(playlistBody))
	writeOutput(t, production, "init.mp4", []byte(initBody))
	writeOutput(t, production, "seg-00000.m4s", []byte(segmentBody))
	writeOutput(t, production, "seg-99999.m4s", []byte(segmentBody))
	production.markReady()

	tests := map[string]struct {
		wantBody, wantType, wantCaching string
		wantLastModified                bool
	}{
		"index.m3u8":    {wantBody: playlistBody, wantType: "application/vnd.apple.mpegurl", wantCaching: "no-store"},
		"init.mp4":      {wantBody: initBody, wantType: "video/mp4", wantCaching: "private, max-age=3600", wantLastModified: true},
		"seg-00000.m4s": {wantBody: segmentBody, wantType: "video/iso.segment", wantCaching: "private, max-age=3600", wantLastModified: true},
		"seg-99999.m4s": {wantBody: segmentBody, wantType: "video/iso.segment", wantCaching: "private, max-age=3600", wantLastModified: true},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			recorder := env.get(t, hlsURL(session.ID, name))
			if recorder.Code != http.StatusOK || recorder.Body.String() != test.wantBody {
				t.Fatalf("GET %s = %d %q, want 200 %q", name, recorder.Code, recorder.Body.String(), test.wantBody)
			}

			assertHeader(t, recorder.Header(), "Content-Type", test.wantType)
			assertHeader(t, recorder.Header(), "Cache-Control", test.wantCaching)
			assertHeader(t, recorder.Header(), "ETag", "")

			if hasLastModified := recorder.Header().Get("Last-Modified") != ""; hasLastModified != test.wantLastModified {
				t.Errorf("Last-Modified present = %t, want %t", hasLastModified, test.wantLastModified)
			}
		})
	}
}

// TestHLSRefusesEveryOtherName requests names next to the three allowed shapes, through the
// REST API mux. None of them may reach the file system; the decoy files would be served if
// they did.
func TestHLSRefusesEveryOtherName(t *testing.T) {
	t.Parallel()

	env := newEnvironment(t)
	session, production := env.startHLS(t)
	production.markReady()

	sessionDirectory := production.job.OutputDir
	for _, decoy := range []string{"INDEX.M3U8", "seg-1.m4s", "x", "index.m3u8.tmp", "seg-00000.m4s.tmp", "init.MP4"} {
		writeOutput(t, production, decoy, []byte("decoy"))
	}

	writeFile(t, filepath.Join(filepath.Dir(sessionDirectory), "x"), []byte("outside"))

	names := []string{
		"INDEX.M3U8", "Index.m3u8", "init.MP4", "seg-1.m4s", "seg-000001.m4s", "seg-0000a.m4s",
		"seg-%EF%BC%91%EF%BC%92%EF%BC%93%EF%BC%94%EF%BC%95.m4s", "seg-00000.m4s.tmp", "index.m3u8.tmp",
		"seg-00000.mp4", "x", "%2e%2e%2fx", "..%2fx", "..%5cx", "index.m3u8%00", "index.m3u8%20", "%20index.m3u8",
	}

	for _, name := range names {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			assertProblem(t, env.get(t, hlsURL(session.ID, name)), http.StatusNotFound, restapi.CodeNotFound)
		})
	}
}

// TestHLSSegmentNumbersAreASCIIDigitsOnly pins the digit check: each name has the
// segment shape with five bytes between prefix and suffix, and a file of that name exists, so
// only the ASCII-digit check stands between the request and the file. Arabic-Indic digits
// (two bytes each) pass unicode.IsDigit and the length check.
func TestHLSSegmentNumbersAreASCIIDigitsOnly(t *testing.T) {
	t.Parallel()

	env := newEnvironment(t)
	session, production := env.startHLS(t)
	production.markReady()

	names := []string{
		"seg-0000a.m4s",
		"seg-000-1.m4s",
		"seg-٠123.m4s", // ARABIC-INDIC DIGIT ZERO + "123"
		"seg-٠١2.m4s",  // two Arabic-Indic digits + "2"
		"seg-1۰۱.m4s",  // "1" + EXTENDED ARABIC-INDIC DIGITS ZERO and ONE
	}

	for _, name := range names {
		writeOutput(t, production, name, []byte(segmentBody))

		t.Run(name, func(t *testing.T) {
			t.Parallel()

			target := hlsURL(session.ID, url.PathEscape(name))
			assertProblem(t, env.get(t, target), http.StatusNotFound, restapi.CodeNotFound)
		})
	}
}

// TestHLSRefusesTraversalNamesBeforeTheFileSystem calls the handler with path values the mux
// would clean or split, as a future router might pass them.
func TestHLSRefusesTraversalNamesBeforeTheFileSystem(t *testing.T) {
	t.Parallel()

	env := newEnvironment(t)
	session, production := env.startHLS(t)
	production.markReady()
	writeFile(t, filepath.Join(filepath.Dir(production.job.OutputDir), "x"), []byte("outside"))

	hlsMount := env.handlers.Mounts()[1]

	for _, name := range []string{"../x", "%2e%2e/x", `..\x`, "/x", "./index.m3u8", "sub/index.m3u8", ""} {
		request := newRequest(t, http.MethodGet, hlsURL(session.ID, "index.m3u8"))
		request.SetPathValue("sessionId", session.ID)
		request.SetPathValue("fileName", name)

		recorder := httptest.NewRecorder()
		hlsMount.Handler.ServeHTTP(recorder, request)
		assertProblem(t, recorder, http.StatusNotFound, restapi.CodeNotFound)
	}
}

func TestHLSFileNotWrittenYetAnswersNotFound(t *testing.T) {
	t.Parallel()

	env := newEnvironment(t)
	session, production := env.startHLS(t)
	writeOutput(t, production, "index.m3u8", []byte(playlistBody))
	production.markReady()

	assertProblem(t, env.get(t, hlsURL(session.ID, "seg-00001.m4s")), http.StatusNotFound, restapi.CodeNotFound)
}

func TestHLSDirectoryNamedLikeASegmentAnswersNotFound(t *testing.T) {
	t.Parallel()

	env := newEnvironment(t)
	session, production := env.startHLS(t)
	production.markReady()

	if err := os.Mkdir(filepath.Join(production.job.OutputDir, "seg-00000.m4s"), 0o750); err != nil {
		t.Fatalf("create directory: %v", err)
	}

	assertProblem(t, env.get(t, hlsURL(session.ID, "seg-00000.m4s")), http.StatusNotFound, restapi.CodeNotFound)
}

// brokenOutput is a SessionFiles port whose storage fails.
type brokenOutput struct{}

var errOutputStorage = errors.New("transcode disk failed")

func (brokenOutput) OpenFile(string, string) (*os.File, error) {
	return nil, errOutputStorage
}

func TestHLSOutputThatCannotBeOpenedAnswersInternalError(t *testing.T) {
	t.Parallel()

	env := newEnvironment(t)
	session, production := env.startHLS(t)
	production.markReady()

	logger := slog.New(slog.NewTextHandler(env.logs, nil))
	handlers := mustHandlers(t, stream.Options{
		Sessions: env.service, Library: mustLibraryFS(t, logger), Output: brokenOutput{}, Logger: logger,
	})
	recorder := httptest.NewRecorder()
	mustAPI(t, logger, handlers.Mounts()).ServeHTTP(recorder, newRequest(t, http.MethodGet, hlsURL(session.ID, "init.mp4")))

	assertProblem(t, recorder, http.StatusInternalServerError, restapi.CodeInternal)

	if logs := env.logs.String(); !strings.Contains(logs, errOutputStorage.Error()) || !strings.Contains(logs, "session="+session.ID) {
		t.Errorf("logs lack the failure with the session:\n%s", logs)
	}
}

func TestPlaylistWaitsForTheFirstSegment(t *testing.T) {
	t.Parallel()

	env := newEnvironment(t)
	session, production := env.startHLS(t)
	responses := make(chan *httptest.ResponseRecorder, 1)

	go func() {
		responses <- env.serve(newRequest(t, http.MethodGet, session.StreamURL))
	}()

	<-production.waiting
	writeOutput(t, production, "index.m3u8", []byte(playlistBody))
	production.markReady()

	if recorder := <-responses; recorder.Code != http.StatusOK || recorder.Body.String() != playlistBody {
		t.Errorf("playlist = %d %q, want 200 %q", recorder.Code, recorder.Body.String(), playlistBody)
	}
}

func TestPlaylistNotReadyInTimeAnswersRetryLater(t *testing.T) {
	t.Parallel()

	env := newEnvironment(t)
	stream.SetReadyTimeout(env.handlers, shortReadyTimeout)
	session, _ := env.startHLS(t)

	started := time.Now()
	recorder := env.get(t, session.StreamURL)

	assertProblem(t, recorder, http.StatusServiceUnavailable, restapi.CodeStreamNotReady)
	assertHeader(t, recorder.Header(), "Retry-After", "2")

	if waited := time.Since(started); waited < shortReadyTimeout {
		t.Errorf("answered after %s, want a wait of %s", waited, shortReadyTimeout)
	}
}

// TestPlaylistWaitEndsWithTheRequest keeps the full readiness bound and requires an answer
// long before it, so a wait that ignores the request context fails instead of merely taking
// 20 seconds.
func TestPlaylistWaitEndsWithTheRequest(t *testing.T) {
	t.Parallel()

	env := newEnvironment(t)
	stream.SetReadyTimeout(env.handlers, stream.ReadyTimeout)
	session, production := env.startHLS(t)

	ctx, cancel := context.WithCancel(t.Context())
	responses := make(chan *httptest.ResponseRecorder, 1)

	go func() {
		responses <- env.serve(httptest.NewRequestWithContext(ctx, http.MethodGet, session.StreamURL, nil))
	}()

	<-production.waiting
	cancel()

	select {
	case recorder := <-responses:
		assertProblem(t, recorder, http.StatusServiceUnavailable, restapi.CodeStreamNotReady)
	case <-time.After(promptAnswer):
		t.Fatalf("no answer %s after the request was cancelled; the wait ignores the request context", promptAnswer)
	}
}

var errFFmpegExit = errors.New("ffmpeg exited with status 1")

func TestFailedProductionAnswersInternalError(t *testing.T) {
	t.Parallel()

	env := newEnvironment(t)
	session, production := env.startHLS(t)
	writeOutput(t, production, "index.m3u8", []byte(playlistBody))
	writeOutput(t, production, "seg-00000.m4s", []byte(segmentBody))
	production.fail(errFFmpegExit)

	for _, name := range []string{"index.m3u8", "seg-00000.m4s"} {
		assertProblem(t, env.get(t, hlsURL(session.ID, name)), http.StatusInternalServerError, restapi.CodeInternal)
	}

	// The producer logs the cause once (M2-24); the handler only ties the request to the
	// session and must not repeat the cause for every retry.
	logs := env.logs.String()
	if !strings.Contains(logs, "level=DEBUG") || !strings.Contains(logs, "production failed") ||
		!strings.Contains(logs, "session="+session.ID) {
		t.Errorf("logs lack the refused request with the session:\n%s", logs)
	}

	if strings.Contains(logs, "level=ERROR") || strings.Count(logs, errFFmpegExit.Error()) > 1 {
		t.Errorf("the handler repeated the production failure:\n%s", logs)
	}
}

// TestProductionStoppedWhileWaitingAnswersNotFound covers a stop that races with a playlist
// request: access was granted, then the production reports ErrProductionStopped.
func TestProductionStoppedWhileWaitingAnswersNotFound(t *testing.T) {
	t.Parallel()

	env := newEnvironment(t)
	session, production := env.startHLS(t)
	responses := make(chan *httptest.ResponseRecorder, 1)

	go func() {
		responses <- env.serve(newRequest(t, http.MethodGet, session.StreamURL))
	}()

	<-production.waiting
	production.Stop()

	assertProblem(t, <-responses, http.StatusNotFound, restapi.CodeNotFound)

	if logs := env.logs.String(); strings.Contains(logs, "level=ERROR") {
		t.Errorf("a stopped production was logged as an error:\n%s", logs)
	}
}

func TestFinishedProductionIsStillServed(t *testing.T) {
	t.Parallel()

	env := newEnvironment(t)
	session, production := env.startHLS(t)
	writeOutput(t, production, "index.m3u8", []byte(playlistBody+"#EXT-X-ENDLIST\n"))
	production.markReady()
	production.fail(nil)

	if recorder := env.get(t, session.StreamURL); recorder.Code != http.StatusOK {
		t.Errorf("playlist of a finished production = %d, want 200", recorder.Code)
	}
}
