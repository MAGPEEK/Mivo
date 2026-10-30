package webui_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"io/fs"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/magpeek/mivo/internal/adapters/http/webui"
)

const (
	indexHTML     = `<!doctype html><html><body><div id="root"></div></body></html>`
	secretContent = "module example.com/secret"
	scriptContent = "console.log('app')"

	wantImmutable  = "public, max-age=31536000, immutable"
	wantRevalidate = "no-cache"
)

// distFiles mimics a Vite build below "dist", next to a file that must stay unreachable:
// the handler receives fs.Sub(..., "dist"), as the embedded variant does.
func distFiles(t *testing.T) fs.FS {
	t.Helper()

	files := fstest.MapFS{
		"go.mod":                        {Data: []byte(secretContent)},
		"dist/index.html":               {Data: []byte(indexHTML)},
		"dist/favicon.svg":              {Data: []byte("<svg/>")},
		"dist/favicon.ico":              {Data: []byte("ico")},
		"dist/logo.png":                 {Data: []byte("png")},
		"dist/robots.txt":               {Data: []byte("User-agent: *")},
		"dist/manifest.json":            {Data: []byte("{}")},
		"dist/site.webmanifest":         {Data: []byte("{}")},
		"dist/data.bin":                 {Data: []byte{0, 1, 2}},
		"dist/.well-known/version.txt":  {Data: []byte("1")},
		"dist/assets/index-a1b2c3.js":   {Data: []byte(scriptContent)},
		"dist/assets/index-a1b2c3.css":  {Data: []byte("body{}")},
		"dist/assets/chunk-d4e5f6.mjs":  {Data: []byte("export {}")},
		"dist/assets/index-a1b2c3.map":  {Data: []byte("{}")},
		"dist/assets/font-0a1b2c.woff2": {Data: []byte("woff2")},
		"dist/assets/LEGACY-9f8e7d.JS":  {Data: []byte("legacy")},
	}

	sub, err := fs.Sub(files, "dist")
	if err != nil {
		t.Fatalf("fs.Sub: %v", err)
	}

	return sub
}

func discardLogger() *slog.Logger {
	return slog.New(slog.DiscardHandler)
}

func newHandler(t *testing.T, files fs.FS) *webui.Handler {
	t.Helper()

	return newHandlerWithLogger(t, files, discardLogger())
}

func newHandlerWithLogger(t *testing.T, files fs.FS, logger *slog.Logger) *webui.Handler {
	t.Helper()

	handler, err := webui.NewHandler(files, logger)
	if err != nil {
		t.Fatalf("NewHandler: %v", err)
	}

	return handler
}

func serve(t *testing.T, handler http.Handler, method, target string) *httptest.ResponseRecorder {
	t.Helper()

	request := httptest.NewRequestWithContext(t.Context(), method, target, nil)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)

	return recorder
}

func TestHandlerServesFilesAndFallsBackToIndex(t *testing.T) {
	t.Parallel()

	handler := newHandler(t, distFiles(t))

	tests := []struct {
		name             string
		target           string
		wantContentType  string
		wantCacheControl string
		wantBody         string
	}{
		{name: "root is index", target: "/", wantContentType: "text/html; charset=utf-8", wantCacheControl: wantRevalidate, wantBody: indexHTML},
		{name: "index by name", target: "/index.html", wantContentType: "text/html; charset=utf-8", wantCacheControl: wantRevalidate, wantBody: indexHTML},
		{name: "hashed script", target: "/assets/index-a1b2c3.js", wantContentType: "text/javascript; charset=utf-8", wantCacheControl: wantImmutable, wantBody: scriptContent},
		{name: "hashed stylesheet", target: "/assets/index-a1b2c3.css", wantContentType: "text/css; charset=utf-8", wantCacheControl: wantImmutable, wantBody: "body{}"},
		{name: "unhashed root file revalidates", target: "/favicon.svg", wantContentType: "image/svg+xml", wantCacheControl: wantRevalidate, wantBody: "<svg/>"},
		{name: "dot directory is served", target: "/.well-known/version.txt", wantContentType: "text/plain; charset=utf-8", wantCacheControl: wantRevalidate, wantBody: "1"},
		{name: "client route falls back", target: "/library/x", wantContentType: "text/html; charset=utf-8", wantCacheControl: wantRevalidate, wantBody: indexHTML},
		{name: "client route with query falls back", target: "/search?q=a.b", wantContentType: "text/html; charset=utf-8", wantCacheControl: wantRevalidate, wantBody: indexHTML},
		{name: "directory is not listed", target: "/assets", wantContentType: "text/html; charset=utf-8", wantCacheControl: wantRevalidate, wantBody: indexHTML},
		{name: "directory with slash is not listed", target: "/assets/", wantContentType: "text/html; charset=utf-8", wantCacheControl: wantRevalidate, wantBody: indexHTML},
		{name: "api lookalike is a client route", target: "/apix/v1", wantContentType: "text/html; charset=utf-8", wantCacheControl: wantRevalidate, wantBody: indexHTML},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			recorder := serve(t, handler, http.MethodGet, test.target)

			assertStatus(t, recorder, http.StatusOK)
			assertHeader(t, recorder, "Content-Type", test.wantContentType)
			assertHeader(t, recorder, "Cache-Control", test.wantCacheControl)

			if body := recorder.Body.String(); body != test.wantBody {
				t.Errorf("body = %q, want %q", body, test.wantBody)
			}
		})
	}
}

func TestHandlerSetsContentTypeFromExplicitTable(t *testing.T) {
	t.Parallel()

	handler := newHandler(t, distFiles(t))

	tests := []struct {
		target string
		want   string
	}{
		{target: "/assets/chunk-d4e5f6.mjs", want: "text/javascript; charset=utf-8"},
		{target: "/assets/LEGACY-9f8e7d.JS", want: "text/javascript; charset=utf-8"},
		{target: "/assets/index-a1b2c3.map", want: "application/json"},
		{target: "/assets/font-0a1b2c.woff2", want: "font/woff2"},
		{target: "/manifest.json", want: "application/json"},
		{target: "/site.webmanifest", want: "application/manifest+json"},
		{target: "/logo.png", want: "image/png"},
		{target: "/favicon.ico", want: "image/x-icon"},
		{target: "/robots.txt", want: "text/plain; charset=utf-8"},
		{target: "/data.bin", want: "application/octet-stream"},
	}

	for _, test := range tests {
		t.Run(test.target, func(t *testing.T) {
			t.Parallel()

			recorder := serve(t, handler, http.MethodGet, test.target)

			assertStatus(t, recorder, http.StatusOK)
			assertHeader(t, recorder, "Content-Type", test.want)
		})
	}
}

func TestHandlerAnswersNotFoundWithoutFileContent(t *testing.T) {
	t.Parallel()

	handler := newHandler(t, distFiles(t))

	targets := []string{
		"/missing.js",
		"/assets/missing-000000.js",
		"/library/cover.jpg",
		"/../../go.mod",
		"/../go.mod",
		"/assets/../../go.mod",
		"/%2e%2e/%2e%2e/go.mod",
		"/..%2f..%2fgo.mod",
		"/..%5c..%5cgo.mod",
	}

	for _, target := range targets {
		t.Run(target, func(t *testing.T) {
			t.Parallel()

			recorder := serve(t, handler, http.MethodGet, target)

			assertStatus(t, recorder, http.StatusNotFound)

			body := recorder.Body.String()
			if strings.Contains(body, secretContent) || strings.Contains(body, `id="root"`) {
				t.Errorf("body = %q, want neither file content nor index.html", body)
			}

			if cacheControl := recorder.Header().Get("Cache-Control"); cacheControl != "" {
				t.Errorf("Cache-Control = %q, want none on a 404", cacheControl)
			}
		})
	}
}

func TestHandlerAnswersAPIPathsWithNotFoundProblem(t *testing.T) {
	t.Parallel()

	handler := newHandler(t, distFiles(t))

	targets := []string{
		"/api",
		"/api/",
		"/api/v1/x",
		"/api/library/x",
		"/api/v1/x.js",
		// ServeMux matches the escaped path, so these reach the UI handler in a real server.
		"/api%2Fv1%2Fx",
		"/api%2F",
		"/%2e%2e/api/v1/x",
		"/assets/../api/v1/x",
	}

	for _, target := range targets {
		t.Run(target, func(t *testing.T) {
			t.Parallel()

			recorder := serve(t, handler, http.MethodGet, target)

			assertStatus(t, recorder, http.StatusNotFound)
			assertHeader(t, recorder, "Content-Type", "application/problem+json")

			body := recorder.Body.String()
			if !strings.Contains(body, `"code":"not_found"`) || strings.Contains(body, `id="root"`) {
				t.Errorf("body = %q, want the not_found problem and no index.html", body)
			}
		})
	}
}

func TestHandlerSupportsHead(t *testing.T) {
	t.Parallel()

	handler := newHandler(t, distFiles(t))

	tests := []struct {
		target    string
		wantBytes int
	}{
		{target: "/", wantBytes: len(indexHTML)},
		{target: "/library/x", wantBytes: len(indexHTML)},
		{target: "/assets/index-a1b2c3.js", wantBytes: len(scriptContent)},
	}

	for _, test := range tests {
		t.Run(test.target, func(t *testing.T) {
			t.Parallel()

			recorder := serve(t, handler, http.MethodHead, test.target)

			assertStatus(t, recorder, http.StatusOK)
			assertHeader(t, recorder, "Content-Length", strconv.Itoa(test.wantBytes))

			if recorder.Body.Len() != 0 {
				t.Errorf("HEAD body = %q, want empty", recorder.Body)
			}
		})
	}
}

func TestHandlerRejectsOtherMethods(t *testing.T) {
	t.Parallel()

	handler := newHandler(t, distFiles(t))

	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodOptions} {
		t.Run(method, func(t *testing.T) {
			t.Parallel()

			recorder := serve(t, handler, method, "/")

			assertStatus(t, recorder, http.StatusMethodNotAllowed)
			assertHeader(t, recorder, "Allow", "GET, HEAD")
		})
	}
}

func TestHandlerServesRanges(t *testing.T) {
	t.Parallel()

	handler := newHandler(t, distFiles(t))

	request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/assets/index-a1b2c3.js", nil)
	request.Header.Set("Range", "bytes=0-6")

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)

	assertStatus(t, recorder, http.StatusPartialContent)

	if body := recorder.Body.String(); body != scriptContent[:7] {
		t.Errorf("body = %q, want %q", body, scriptContent[:7])
	}
}

func TestHandlerServesFilesWithoutSeek(t *testing.T) {
	t.Parallel()

	handler := newHandler(t, unseekableFS{files: distFiles(t)})

	recorder := serve(t, handler, http.MethodGet, "/assets/index-a1b2c3.js")

	assertStatus(t, recorder, http.StatusOK)
	assertHeader(t, recorder, "Cache-Control", wantImmutable)

	if body := recorder.Body.String(); body != scriptContent {
		t.Errorf("body = %q, want %q", body, scriptContent)
	}
}

func TestHandlerReportsUnreadableFilesAsServerError(t *testing.T) {
	t.Parallel()

	files := distFiles(t)
	failure := errors.New("disk on fire")

	tests := []struct {
		name            string
		files           fs.FS
		target          string
		wantLoggedCause string
	}{
		{
			name: "open fails", files: faultyFS{files: files, failOpen: "assets/index-a1b2c3.js", err: failure},
			target: "/assets/index-a1b2c3.js", wantLoggedCause: failure.Error(),
		},
		{
			name: "stat fails", files: faultyFS{files: files, failStat: "assets/index-a1b2c3.js", err: failure},
			target: "/assets/index-a1b2c3.js", wantLoggedCause: failure.Error(),
		},
		{
			name: "permission denied is no client route", files: faultyFS{files: files, failOpen: "library/x", err: fs.ErrPermission},
			target: "/library/x", wantLoggedCause: fs.ErrPermission.Error(),
		},
		{
			name: "read fails", files: unseekableFS{files: files, readErr: failure},
			target: "/assets/index-a1b2c3.js", wantLoggedCause: failure.Error(),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			var logOutput bytes.Buffer

			logger := slog.New(slog.NewJSONHandler(&logOutput, nil))

			recorder := serve(t, newHandlerWithLogger(t, test.files, logger), http.MethodGet, test.target)

			assertStatus(t, recorder, http.StatusInternalServerError)

			if strings.Contains(recorder.Body.String(), failure.Error()) {
				t.Errorf("body = %q, want no internal error text", recorder.Body)
			}

			assertLoggedFailure(t, logOutput.Bytes(), test.target, test.wantLoggedCause)
		})
	}
}

// assertLoggedFailure checks that exactly one error record names the request and the cause.
func assertLoggedFailure(t *testing.T, output []byte, wantPath, wantCause string) {
	t.Helper()

	lines := bytes.Split(bytes.TrimSpace(output), []byte("\n"))
	if len(lines) != 1 {
		t.Fatalf("log = %q, want exactly one record", output)
	}

	var record struct {
		Level string `json:"level"`
		Msg   string `json:"msg"`
		Path  string `json:"path"`
		Error string `json:"error"`
	}

	if err := json.Unmarshal(lines[0], &record); err != nil {
		t.Fatalf("decode log record %q: %v", lines[0], err)
	}

	if record.Level != slog.LevelError.String() || record.Msg != "web ui request failed" || record.Path != wantPath {
		t.Errorf("log record = %+v, want an error about %s", record, wantPath)
	}

	if !strings.Contains(record.Error, wantCause) {
		t.Errorf("logged error = %q, want it to contain %q", record.Error, wantCause)
	}
}

func TestNewHandlerRequiresIndexFile(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		files fs.FS
	}{
		{name: "nil file system", files: nil},
		{name: "missing index", files: fstest.MapFS{"assets/app.js": {Data: []byte("x")}}},
		{name: "index is a directory", files: fstest.MapFS{"index.html/app.js": {Data: []byte("x")}}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			handler, err := webui.NewHandler(test.files, discardLogger())
			if err == nil || handler != nil {
				t.Fatalf("NewHandler = (%v, %v), want an error", handler, err)
			}

			if !strings.Contains(err.Error(), "index.html") {
				t.Errorf("error = %q, want it to name index.html", err)
			}
		})
	}
}

func TestNewHandlerRequiresLogger(t *testing.T) {
	t.Parallel()

	handler, err := webui.NewHandler(distFiles(t), nil)
	if err == nil || handler != nil {
		t.Fatalf("NewHandler = (%v, %v), want an error", handler, err)
	}

	if !strings.Contains(err.Error(), "logger") {
		t.Errorf("error = %q, want it to name the logger", err)
	}
}

func assertStatus(t *testing.T, recorder *httptest.ResponseRecorder, want int) {
	t.Helper()

	if recorder.Code != want {
		t.Fatalf("status = %d, want %d (body %q)", recorder.Code, want, recorder.Body)
	}
}

func assertHeader(t *testing.T, recorder *httptest.ResponseRecorder, name, want string) {
	t.Helper()

	if got := recorder.Header().Get(name); got != want {
		t.Errorf("%s = %q, want %q", name, got, want)
	}
}

// faultyFS fails opening failOpen, or the Stat of the opened failStat, with err and passes
// everything else to files.
type faultyFS struct {
	files    fs.FS
	failOpen string
	failStat string
	err      error
}

func (faulty faultyFS) Open(name string) (fs.File, error) {
	if name == faulty.failOpen {
		return nil, &fs.PathError{Op: "open", Path: name, Err: faulty.err}
	}

	file, err := faulty.files.Open(name)
	if err != nil || name != faulty.failStat {
		return file, err
	}

	return statFailingFile{File: file, err: faulty.err}, nil
}

// statFailingFile is an open file whose Stat fails.
type statFailingFile struct {
	fs.File
	err error
}

func (file statFailingFile) Stat() (fs.FileInfo, error) { return nil, file.err }

// unseekableFS hides io.Seeker from the files it opens and optionally fails every read, like
// a file system that streams its content.
type unseekableFS struct {
	files   fs.FS
	readErr error
}

func (unseekable unseekableFS) Open(name string) (fs.File, error) {
	file, err := unseekable.files.Open(name)
	if err != nil {
		return nil, err
	}

	return unseekableFile{file: file, readErr: unseekable.readErr}, nil
}

type unseekableFile struct {
	file    fs.File
	readErr error
}

func (file unseekableFile) Stat() (fs.FileInfo, error) { return file.file.Stat() }

func (file unseekableFile) Close() error { return file.file.Close() }

func (file unseekableFile) Read(buffer []byte) (int, error) {
	if file.readErr != nil {
		return 0, file.readErr
	}

	// Passed through unwrapped: readers must return io.EOF itself, not a wrapped copy.
	return file.file.Read(buffer)
}
