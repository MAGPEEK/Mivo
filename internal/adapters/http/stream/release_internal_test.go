package stream

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/playback"
)

const (
	releaseSessionID = "5d4c3b2a-1f0e-4d9c-8b7a-6f5e4d3c2b1a"
	releaseFileName  = "movie.mp4"
	releaseFileSize  = 4096
	// releaseCopyLimit is where the broken connection of the mid-copy case gives up.
	releaseCopyLimit = 100
	// releaseReadyTimeout shortens the readiness wait of the not-ready case.
	releaseReadyTimeout = 20 * time.Millisecond
)

var (
	errReleaseRegistry = errors.New("registry failed")
	errReleaseExit     = errors.New("ffmpeg exited with status 1")
	errReleaseBroken   = errors.New("connection broke")
)

// countingGrants hands out grants for one session and counts how often each is released.
type countingGrants struct {
	mutex    sync.Mutex
	template grant
	err      error
	granted  int
	released []int
}

func (grants *countingGrants) access(_ context.Context, sessionID string) (grant, error) {
	grants.mutex.Lock()
	defer grants.mutex.Unlock()

	if grants.err != nil {
		return grant{}, grants.err
	}

	index := grants.granted
	grants.granted++
	grants.released = append(grants.released, 0)

	granted := grants.template
	granted.sessionID = sessionID
	granted.release = func() {
		grants.mutex.Lock()
		defer grants.mutex.Unlock()

		grants.released[index]++
	}

	return granted, nil
}

// assertEachReleasedOnce checks that wantGranted grants were handed out and each was
// released exactly once.
func (grants *countingGrants) assertEachReleasedOnce(t *testing.T, wantGranted int) {
	t.Helper()

	grants.mutex.Lock()
	defer grants.mutex.Unlock()

	if grants.granted != wantGranted {
		t.Fatalf("granted %d accesses, want %d", grants.granted, wantGranted)
	}

	for index, count := range grants.released {
		if count != 1 {
			t.Errorf("grant %d released %d times, want once", index, count)
		}
	}
}

// directoryFiles opens files of one directory through os.Root, for both file ports.
type directoryFiles struct {
	directory string
}

func (files directoryFiles) Open(_, relativePath string) (*os.File, error) {
	return files.open(relativePath)
}

func (files directoryFiles) OpenFile(_, name string) (*os.File, error) {
	return files.open(name)
}

func (files directoryFiles) open(name string) (*os.File, error) {
	file, err := os.OpenInRoot(files.directory, name)
	if errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("open %s: %w: %w", name, domain.ErrNotFound, err)
	}

	return file, err
}

// brokenWriter breaks the connection after releaseCopyLimit body bytes; when cancel is set
// it cancels the request first, as net/http does when the client's connection fails.
type brokenWriter struct {
	*httptest.ResponseRecorder
	cancel context.CancelFunc
}

func (writer *brokenWriter) ReadFrom(source io.Reader) (int64, error) {
	copied, err := io.Copy(writer.ResponseRecorder, io.LimitReader(source, releaseCopyLimit))
	if err == nil {
		if writer.cancel != nil {
			writer.cancel()
		}

		err = errReleaseBroken
	}

	return copied, err
}

type releaseCase struct {
	route       func(*Handlers) http.HandlerFunc
	fileName    string
	template    grant
	accessErr   error
	writeFiles  []string
	brokenWrite bool
	cancelOnEnd bool
	wantGranted int
	wantStatus  int
}

func fileRoute(handlers *Handlers) http.HandlerFunc { return handlers.serveFile }
func hlsRoute(handlers *Handlers) http.HandlerFunc  { return handlers.serveHLS }

func readyAtOnce(context.Context) error { return nil }
func failedProduction(context.Context) error {
	return errReleaseExit
}

func neverReady(ctx context.Context) error {
	<-ctx.Done()

	return ctx.Err()
}

func directGrant() grant {
	return grant{
		delivery: playback.DeliveryFile, relativePath: releaseFileName, contentType: "video/mp4",
		waitReady: readyAtOnce,
	}
}

func hlsGrant(waitReady func(context.Context) error) grant {
	return grant{delivery: playback.DeliveryHLS, waitReady: waitReady}
}

func releaseCases() map[string]releaseCase {
	return map[string]releaseCase{
		"direct file served": {
			route: fileRoute, template: directGrant(), writeFiles: []string{releaseFileName},
			wantGranted: 1, wantStatus: http.StatusOK,
		},
		"direct file, client gone mid-copy": {
			route: fileRoute, template: directGrant(), writeFiles: []string{releaseFileName},
			brokenWrite: true, cancelOnEnd: true, wantGranted: 1, wantStatus: http.StatusOK,
		},
		"direct file, connection broke mid-copy": {
			route: fileRoute, template: directGrant(), writeFiles: []string{releaseFileName},
			brokenWrite: true, wantGranted: 1, wantStatus: http.StatusOK,
		},
		"direct file missing": {
			route: fileRoute, template: directGrant(), wantGranted: 1, wantStatus: http.StatusNotFound,
		},
		"direct route for an hls session": {
			route: fileRoute, template: hlsGrant(readyAtOnce), wantGranted: 1, wantStatus: http.StatusNotFound,
		},
		"access refused": {
			route: fileRoute, accessErr: errReleaseRegistry, wantGranted: 0, wantStatus: http.StatusInternalServerError,
		},
		"hls segment served": {
			route: hlsRoute, fileName: "seg-00000.m4s", template: hlsGrant(readyAtOnce),
			writeFiles: []string{"seg-00000.m4s"}, wantGranted: 1, wantStatus: http.StatusOK,
		},
		"hls segment missing": {
			route: hlsRoute, fileName: "seg-00001.m4s", template: hlsGrant(readyAtOnce),
			wantGranted: 1, wantStatus: http.StatusNotFound,
		},
		"hls playlist not ready": {
			route: hlsRoute, fileName: "index.m3u8", template: hlsGrant(neverReady),
			wantGranted: 1, wantStatus: http.StatusServiceUnavailable,
		},
		"hls production failed": {
			route: hlsRoute, fileName: "index.m3u8", template: hlsGrant(failedProduction),
			wantGranted: 1, wantStatus: http.StatusInternalServerError,
		},
		"hls route for a direct session": {
			route: hlsRoute, fileName: "index.m3u8", template: directGrant(),
			wantGranted: 1, wantStatus: http.StatusNotFound,
		},
		"hls name never served": {
			route: hlsRoute, fileName: "secret.txt", template: hlsGrant(readyAtOnce),
			wantGranted: 0, wantStatus: http.StatusNotFound,
		},
	}
}

// TestEveryGrantIsReleasedOnce checks that each successful Access is paired with exactly one
// Release, whichever way the request ends: served, failed after the access, or broken off
// mid-copy.
func TestEveryGrantIsReleasedOnce(t *testing.T) {
	t.Parallel()

	for name, test := range releaseCases() {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			directory := t.TempDir()
			for _, file := range test.writeFiles {
				writeReleaseFile(t, filepath.Join(directory, file))
			}

			grants := &countingGrants{template: test.template, err: test.accessErr}
			files := directoryFiles{directory: directory}
			handlers := &Handlers{
				access: grants.access, library: files, output: files,
				logger: slog.New(slog.DiscardHandler), readyTimeout: releaseReadyTimeout,
			}

			recorder := serveReleaseCase(t, handlers, test)
			if recorder.Code != test.wantStatus {
				t.Errorf("status = %d, want %d", recorder.Code, test.wantStatus)
			}

			grants.assertEachReleasedOnce(t, test.wantGranted)
		})
	}
}

func serveReleaseCase(t *testing.T, handlers *Handlers, test releaseCase) *httptest.ResponseRecorder {
	t.Helper()

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	request := httptest.NewRequestWithContext(ctx, http.MethodGet, "/stream", nil)
	request.SetPathValue(sessionIDParameter, releaseSessionID)
	request.SetPathValue(fileNameParameter, test.fileName)

	recorder := httptest.NewRecorder()

	var writer http.ResponseWriter = recorder

	if test.brokenWrite {
		broken := &brokenWriter{ResponseRecorder: recorder}
		if test.cancelOnEnd {
			broken.cancel = cancel
		}

		writer = broken
	}

	test.route(handlers)(writer, request)

	return recorder
}

func writeReleaseFile(t *testing.T, path string) {
	t.Helper()

	if err := os.WriteFile(path, make([]byte, releaseFileSize), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
