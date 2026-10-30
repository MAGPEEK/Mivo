package stream_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/magpeek/mivo/internal/adapters/http/restapi"
	"github.com/magpeek/mivo/internal/adapters/http/stream"
	appplayback "github.com/magpeek/mivo/internal/app/playback"
)

const mp4ContentType = "video/mp4"

func TestDirectFileServesRanges(t *testing.T) {
	t.Parallel()

	env := newEnvironment(t)
	session := env.start(t, directFileID)
	size := len(env.content)

	tests := map[string]struct {
		rangeHeader      string
		wantStatus       int
		wantBody         []byte
		wantContentRange string
	}{
		"whole file": {wantStatus: http.StatusOK, wantBody: env.content},
		"range": {
			rangeHeader: "bytes=100-199", wantStatus: http.StatusPartialContent,
			wantBody: env.content[100:200], wantContentRange: "bytes 100-199/" + strconv.Itoa(size),
		},
		"open range": {
			rangeHeader: "bytes=4000-", wantStatus: http.StatusPartialContent,
			wantBody: env.content[4000:], wantContentRange: "bytes 4000-4095/" + strconv.Itoa(size),
		},
		"suffix range": {
			rangeHeader: "bytes=-10", wantStatus: http.StatusPartialContent,
			wantBody: env.content[size-10:], wantContentRange: "bytes 4086-4095/" + strconv.Itoa(size),
		},
		"unsatisfiable range": {
			rangeHeader: "bytes=5000-6000", wantStatus: http.StatusRequestedRangeNotSatisfiable,
			wantContentRange: "bytes */" + strconv.Itoa(size),
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			request := newRequest(t, http.MethodGet, session.StreamURL)
			if test.rangeHeader != "" {
				request.Header.Set("Range", test.rangeHeader)
			}

			recorder := env.serve(request)
			if recorder.Code != test.wantStatus {
				t.Fatalf("status = %d, want %d", recorder.Code, test.wantStatus)
			}

			assertHeader(t, recorder.Header(), "Content-Range", test.wantContentRange)

			if test.wantBody != nil {
				assertDirectHeaders(t, recorder.Header(), len(test.wantBody))

				if !bytes.Equal(recorder.Body.Bytes(), test.wantBody) {
					t.Errorf("body has %d bytes, want the %d bytes of the range", recorder.Body.Len(), len(test.wantBody))
				}
			}
		})
	}
}

func assertDirectHeaders(t *testing.T, header http.Header, wantLength int) {
	t.Helper()

	assertHeader(t, header, "Content-Type", mp4ContentType)
	assertHeader(t, header, "Accept-Ranges", "bytes")
	assertHeader(t, header, "Content-Length", strconv.Itoa(wantLength))
	assertHeader(t, header, "ETag", "")

	if header.Get("Last-Modified") == "" {
		t.Error("Last-Modified is missing")
	}
}

func TestDirectFileAnswersHead(t *testing.T) {
	t.Parallel()

	env := newEnvironment(t)
	session := env.start(t, directFileID)

	recorder := env.serve(newRequest(t, http.MethodHead, session.StreamURL))
	if recorder.Code != http.StatusOK || recorder.Body.Len() != 0 {
		t.Fatalf("HEAD = %d with %d body bytes, want 200 without a body", recorder.Code, recorder.Body.Len())
	}

	assertDirectHeaders(t, recorder.Header(), len(env.content))
}

func TestDirectFileHonoursIfRange(t *testing.T) {
	t.Parallel()

	env := newEnvironment(t)
	session := env.start(t, directFileID)

	modified := env.serve(newRequest(t, http.MethodHead, session.StreamURL)).Header().Get("Last-Modified")
	stale := time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC).Format(http.TimeFormat)

	tests := map[string]struct {
		ifRange    string
		wantStatus int
	}{
		"unchanged file serves the range": {ifRange: modified, wantStatus: http.StatusPartialContent},
		"changed file serves it whole":    {ifRange: stale, wantStatus: http.StatusOK},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			request := newRequest(t, http.MethodGet, session.StreamURL)
			request.Header.Set("Range", "bytes=0-9")
			request.Header.Set("If-Range", test.ifRange)

			if recorder := env.serve(request); recorder.Code != test.wantStatus {
				t.Errorf("status = %d, want %d", recorder.Code, test.wantStatus)
			}
		})
	}
}

// readerFromWriter is a response writer with io.ReaderFrom, like net/http's own, that
// records whether the body went through ReadFrom (the sendfile path).
type readerFromWriter struct {
	*httptest.ResponseRecorder
	readFromUsed bool
	// limit, when positive, makes ReadFrom stop after limit bytes with failure, as a
	// connection that broke mid-body; onFailure runs before it returns.
	limit     int64
	onFailure func()
}

var errConnectionBroke = errors.New("connection broke")

func (writer *readerFromWriter) ReadFrom(source io.Reader) (int64, error) {
	writer.readFromUsed = true

	if writer.limit <= 0 {
		return io.Copy(writer.ResponseRecorder, source)
	}

	copied, err := io.Copy(writer.ResponseRecorder, io.LimitReader(source, writer.limit))
	if err == nil {
		if writer.onFailure != nil {
			writer.onFailure()
		}

		err = errConnectionBroke
	}

	return copied, err
}

// TestDirectFileKeepsTheReaderFromPath checks that the writer chain of restapi and the
// handler still hands the file to io.ReaderFrom, so net/http can use sendfile (M2-08).
func TestDirectFileKeepsTheReaderFromPath(t *testing.T) {
	t.Parallel()

	env := newEnvironment(t)
	session := env.start(t, directFileID)
	writer := &readerFromWriter{ResponseRecorder: httptest.NewRecorder()}

	env.api.ServeHTTP(writer, newRequest(t, http.MethodGet, session.StreamURL))

	if !writer.readFromUsed {
		t.Error("the body did not go through ReadFrom")
	}

	if !bytes.Equal(writer.Body.Bytes(), env.content) {
		t.Errorf("body has %d bytes, want %d", writer.Body.Len(), len(env.content))
	}
}

func TestDirectFileLogsABodyThatBrokeOff(t *testing.T) {
	t.Parallel()

	env := newEnvironment(t)
	session := env.start(t, directFileID)
	writer := &readerFromWriter{ResponseRecorder: httptest.NewRecorder(), limit: 100}

	env.api.ServeHTTP(writer, newRequest(t, http.MethodGet, session.StreamURL))

	logs := env.logs.String()
	if !strings.Contains(logs, "level=WARN") || !strings.Contains(logs, "ended before its declared length") ||
		!strings.Contains(logs, "session="+session.ID) || !strings.Contains(logs, "written=100") {
		t.Errorf("logs lack the warning about the short body:\n%s", logs)
	}
}

func TestDirectFileIgnoresAClientThatWentAway(t *testing.T) {
	t.Parallel()

	env := newEnvironment(t)
	session := env.start(t, directFileID)

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	// net/http cancels the request context when the client's connection fails.
	writer := &readerFromWriter{ResponseRecorder: httptest.NewRecorder(), limit: 100, onFailure: cancel}
	request := httptest.NewRequestWithContext(ctx, http.MethodGet, session.StreamURL, nil)

	env.api.ServeHTTP(writer, request)

	if logs := env.logs.String(); strings.Contains(logs, "ended before its declared length") {
		t.Errorf("a client that went away was logged as a failure:\n%s", logs)
	}
}

func TestDirectFileMissingOnDiskAnswersNotFound(t *testing.T) {
	t.Parallel()

	env := newEnvironment(t)
	session := env.start(t, directFileID)

	if err := os.Remove(filepath.Join(env.libraryRoot, filepath.FromSlash(directRelativePath))); err != nil {
		t.Fatalf("remove file: %v", err)
	}

	assertProblem(t, env.get(t, session.StreamURL), http.StatusNotFound, restapi.CodeNotFound)

	if logs := env.logs.String(); !strings.Contains(logs, "level=WARN") || !strings.Contains(logs, "session="+session.ID) {
		t.Errorf("logs lack the warning with the session:\n%s", logs)
	}
}

// failingSessions is a Sessions port whose Access fails without a domain error.
type failingSessions struct{}

var errRegistry = errors.New("registry exploded")

func (failingSessions) Access(context.Context, string) (appplayback.StreamAccess, error) {
	return appplayback.StreamAccess{}, errRegistry
}

func TestUnexpectedAccessFailureAnswersInternalError(t *testing.T) {
	t.Parallel()

	logs := &syncBuffer{}
	logger := slog.New(slog.NewTextHandler(logs, nil))
	handlers := mustHandlers(t, stream.Options{
		Sessions: failingSessions{}, Library: mustLibraryFS(t, logger), Output: mustWorkspace(t), Logger: logger,
	})
	api := mustAPI(t, logger, handlers.Mounts())

	for _, target := range []string{fileURL(unknownSessionID), hlsURL(unknownSessionID, "init.mp4")} {
		recorder := httptest.NewRecorder()
		api.ServeHTTP(recorder, newRequest(t, http.MethodGet, target))
		assertProblem(t, recorder, http.StatusInternalServerError, restapi.CodeInternal)
	}

	if text := logs.String(); !strings.Contains(text, errRegistry.Error()) || !strings.Contains(text, "session="+unknownSessionID) {
		t.Errorf("logs lack the cause with the session:\n%s", text)
	}
}
