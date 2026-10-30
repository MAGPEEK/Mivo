package server

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/magpeek/mivo/internal/adapters/http/apigen"
	"github.com/magpeek/mivo/internal/adapters/http/logtext"
	"github.com/magpeek/mivo/internal/adapters/http/requestid"
	"github.com/magpeek/mivo/internal/adapters/http/restapi"
)

// syncBuffer collects log output written from handler goroutines.
type syncBuffer struct {
	mutex  sync.Mutex
	buffer bytes.Buffer
}

func (buffer *syncBuffer) Write(data []byte) (int, error) {
	buffer.mutex.Lock()
	defer buffer.mutex.Unlock()

	return buffer.buffer.Write(data)
}

func (buffer *syncBuffer) String() string {
	buffer.mutex.Lock()
	defer buffer.mutex.Unlock()

	return buffer.buffer.String()
}

func newTestLogger() (*slog.Logger, *syncBuffer) {
	logs := &syncBuffer{}

	return slog.New(slog.NewTextHandler(logs, nil)), logs
}

// textHandler answers with status and body text.
func textHandler(status int, body string) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(status)
		_, _ = io.WriteString(writer, body)
	})
}

// partialBody is what panickingHandler sends before it panics when asked to.
const partialBody = "partial"

// panickingHandler sets headers of the response it never finishes and panics. With
// sendPartialBody it first sends a 202 and part of the body to the client.
func panickingHandler(sendPartialBody bool) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		writer.Header().Set("Cache-Control", "max-age=3600")
		writer.Header().Set("Content-Length", "1000")

		if sendPartialBody {
			writer.Header().Del("Content-Length")
			writer.WriteHeader(http.StatusAccepted)
			_, _ = io.WriteString(writer, partialBody)
			_ = http.NewResponseController(writer).Flush()
		}

		panic("boom: " + `C:\secret\path`) //nolint:forbidigo // the recover middleware under test needs a panicking handler
	})
}

func abortingHandler() http.Handler {
	return http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic(http.ErrAbortHandler) //nolint:forbidigo // the recover middleware under test must pass a deliberate abort on
	})
}

func serveChain(t *testing.T, options Options, request *http.Request) *httptest.ResponseRecorder {
	t.Helper()

	recorder := httptest.NewRecorder()
	newTestChain(t, options).ServeHTTP(recorder, request)

	return recorder
}

// newTestChain builds the routed middleware chain the way New does.
func newTestChain(t *testing.T, options Options) http.Handler {
	t.Helper()

	settings, err := newChainSettings(options)
	if err != nil {
		t.Fatalf("newChainSettings: %v", err)
	}

	return newHandler(options, settings)
}

// lanPeer is the socket peer of test requests: a LAN address the LAN guard lets through.
// httptest's default, 192.0.2.1, is a public documentation address the guard refuses.
const lanPeer = "192.168.1.20:50000"

func newRequest(t *testing.T, method, target string) *http.Request {
	t.Helper()

	request := httptest.NewRequestWithContext(t.Context(), method, target, nil)
	request.RemoteAddr = lanPeer

	return request
}

func TestRoutesSplitAPIAndFallback(t *testing.T) {
	t.Parallel()

	logger, _ := newTestLogger()
	withFallback := Options{Logger: logger, APIHandler: textHandler(http.StatusOK, "api"), Fallback: textHandler(http.StatusOK, "web")}
	withoutFallback := Options{Logger: logger, APIHandler: textHandler(http.StatusOK, "api")}

	tests := []struct {
		name       string
		options    Options
		target     string
		wantStatus int
		wantBody   string
	}{
		{name: "api route", options: withFallback, target: "/api/v1/health", wantStatus: http.StatusOK, wantBody: "api"},
		{name: "unknown api route stays in api", options: withFallback, target: "/api/nope", wantStatus: http.StatusOK, wantBody: "api"},
		{name: "root goes to fallback", options: withFallback, target: "/", wantStatus: http.StatusOK, wantBody: "web"},
		{name: "web route goes to fallback", options: withFallback, target: "/library/7", wantStatus: http.StatusOK, wantBody: "web"},
		// ServeMux redirects a subtree root without its slash; the API then answers 404.
		{name: "api without slash redirects into api", options: withFallback, target: "/api", wantStatus: http.StatusTemporaryRedirect},
		{name: "prefix lookalike is not api", options: withFallback, target: "/apix", wantStatus: http.StatusOK, wantBody: "web"},
		// ServeMux matches the escaped path, so these miss the /api/ pattern; the guard in
		// front of the fallback still sends them to the API.
		{name: "encoded slashes stay in api", options: withFallback, target: "/api%2Fv1%2Fx", wantStatus: http.StatusOK, wantBody: "api"},
		{name: "encoded dot segments stay in api", options: withFallback, target: "/%2e%2e/api/v1/x", wantStatus: http.StatusOK, wantBody: "api"},
		{name: "encoded slash lookalike is not api", options: withFallback, target: "/apix%2Fv1", wantStatus: http.StatusOK, wantBody: "web"},
		{name: "nil fallback answers 404", options: withoutFallback, target: "/library/7", wantStatus: http.StatusNotFound},
		{name: "nil fallback keeps api", options: withoutFallback, target: "/api/v1/health", wantStatus: http.StatusOK, wantBody: "api"},
		// The guard also runs in front of the default 404 handler.
		{name: "nil fallback keeps encoded api paths in api", options: withoutFallback, target: "/api%2Fv1%2Fx", wantStatus: http.StatusOK, wantBody: "api"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			recorder := serveChain(t, test.options, newRequest(t, http.MethodGet, test.target))
			if recorder.Code != test.wantStatus {
				t.Errorf("status = %d, want %d", recorder.Code, test.wantStatus)
			}

			if test.wantBody != "" && recorder.Body.String() != test.wantBody {
				t.Errorf("body = %q, want %q", recorder.Body, test.wantBody)
			}
		})
	}
}

func TestSecurityHeadersOnEveryResponse(t *testing.T) {
	t.Parallel()

	logger, _ := newTestLogger()

	tests := []struct {
		name    string
		options Options
		target  string
	}{
		{name: "api", options: Options{Logger: logger, APIHandler: textHandler(http.StatusOK, "")}, target: "/api/v1/health"},
		{name: "not found", options: Options{Logger: logger, APIHandler: textHandler(http.StatusOK, "")}, target: "/x"},
		{name: "panic", options: Options{Logger: logger, APIHandler: panickingHandler(false)}, target: "/api/v1/health"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			header := serveChain(t, test.options, newRequest(t, http.MethodGet, test.target)).Header()
			if got := header.Get("X-Content-Type-Options"); got != "nosniff" {
				t.Errorf("X-Content-Type-Options = %q, want nosniff", got)
			}

			if got := header.Get("Referrer-Policy"); got != "no-referrer" {
				t.Errorf("Referrer-Policy = %q, want no-referrer", got)
			}
		})
	}
}

func TestRequestIDIsEchoedAndLogged(t *testing.T) {
	t.Parallel()

	logger, logs := newTestLogger()
	options := Options{Logger: logger, APIHandler: textHandler(http.StatusOK, "")}

	request := newRequest(t, http.MethodGet, "/api/v1/health")
	request.Header.Set(requestid.HeaderName, "client-id-42")

	recorder := serveChain(t, options, request)
	if got := recorder.Header().Get(requestid.HeaderName); got != "client-id-42" {
		t.Errorf("X-Request-ID = %q, want client-id-42", got)
	}

	if !strings.Contains(logs.String(), "request_id=client-id-42") {
		t.Errorf("access log %q has no request ID", logs.String())
	}

	generated := serveChain(t, options, newRequest(t, http.MethodGet, "/api/v1/health"))
	if generated.Header().Get(requestid.HeaderName) == "" {
		t.Error("no X-Request-ID generated for a request without one")
	}
}

func TestAccessLog(t *testing.T) {
	t.Parallel()

	logger, logs := newTestLogger()
	options := Options{Logger: logger, APIHandler: textHandler(http.StatusTeapot, "hello")}

	request := newRequest(t, http.MethodPost, "/api/v1/things?token=query-secret")
	request.Header.Set("User-Agent", "mivo-test/1.0")
	request.Header.Set("Authorization", "Bearer header-secret")
	request.Header.Set("Cookie", "session=cookie-secret")
	request.Header.Set(requestid.HeaderName, "access-1")

	serveChain(t, options, request)

	line := logs.String()
	for _, want := range []string{
		`msg="http request"`, "method=POST", "path=/api/v1/things", "status=418", "duration=",
		"bytes=5", "request_id=access-1", "user_agent=mivo-test/1.0",
	} {
		if !strings.Contains(line, want) {
			t.Errorf("access log %q lacks %q", line, want)
		}
	}

	for _, secret := range []string{"query-secret", "header-secret", "cookie-secret"} {
		if strings.Contains(line, secret) {
			t.Errorf("access log %q leaks %q", line, secret)
		}
	}
}

func TestAccessLogDefaultsToOK(t *testing.T) {
	t.Parallel()

	logger, logs := newTestLogger()
	silent := http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})

	serveChain(t, Options{Logger: logger, APIHandler: silent}, newRequest(t, http.MethodGet, "/api/v1/health"))

	if !strings.Contains(logs.String(), "status=200") {
		t.Errorf("access log %q, want status=200 for a handler that wrote nothing", logs.String())
	}
}

func TestRecoverWritesProblemAndLogsStack(t *testing.T) {
	t.Parallel()

	logger, logs := newTestLogger()
	options := Options{Logger: logger, APIHandler: panickingHandler(false)}

	recorder := serveChain(t, options, newRequest(t, http.MethodGet, "/api/v1/health"))

	if recorder.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", recorder.Code)
	}

	if contentType := recorder.Header().Get("Content-Type"); contentType != restapi.ProblemMediaType {
		t.Errorf("Content-Type = %q, want %q", contentType, restapi.ProblemMediaType)
	}

	var problem apigen.Problem
	if err := json.Unmarshal(recorder.Body.Bytes(), &problem); err != nil || problem.Code != restapi.CodeInternal {
		t.Errorf("body %s, decode error %v, want internal_error problem", recorder.Body, err)
	}

	if strings.Contains(recorder.Body.String(), "secret") {
		t.Errorf("body %s exposes the panic value", recorder.Body)
	}

	for _, dropped := range []string{"Cache-Control", "Content-Length"} {
		if value := recorder.Header().Get(dropped); value != "" {
			t.Errorf("%s = %q survived from the panicking handler", dropped, value)
		}
	}

	for _, kept := range []string{"X-Content-Type-Options", "Referrer-Policy", requestid.HeaderName} {
		if recorder.Header().Get(kept) == "" {
			t.Errorf("%s was dropped from the problem response", kept)
		}
	}

	log := logs.String()
	for _, want := range []string{"level=ERROR", "http handler panicked", "boom", "stack=", "panickingHandler", "status=500"} {
		if !strings.Contains(log, want) {
			t.Errorf("log %q lacks %q", log, want)
		}
	}
}

// serveOverNetwork serves the middleware chain over a real connection, because only
// net/http itself shows how an aborted response reaches a client.
func serveOverNetwork(t *testing.T, options Options) (*http.Response, error) {
	t.Helper()

	testServer := httptest.NewServer(newTestChain(t, options))
	t.Cleanup(testServer.Close)

	request, err := http.NewRequestWithContext(t.Context(), http.MethodGet, testServer.URL+"/api/v1/health", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}

	return testServer.Client().Do(request)
}

func TestRecoverAbortsResponseAfterHeadersWereSent(t *testing.T) {
	t.Parallel()

	logger, logs := newTestLogger()

	response, err := serveOverNetwork(t, Options{Logger: logger, APIHandler: panickingHandler(true)})
	if err != nil {
		t.Fatalf("the status line was flushed, want a response before the abort: %v", err)
	}

	body, readErr := io.ReadAll(response.Body)
	if closeErr := response.Body.Close(); closeErr != nil {
		t.Logf("close body: %v", closeErr)
	}

	if response.StatusCode != http.StatusAccepted {
		t.Errorf("status = %d, want the already sent 202", response.StatusCode)
	}

	// A cleanly finished chunked body would present the truncated response as complete.
	if !errors.Is(readErr, io.ErrUnexpectedEOF) {
		t.Errorf("read body = %q, error %v, want io.ErrUnexpectedEOF", body, readErr)
	}

	log := logs.String()
	for _, want := range []string{"level=ERROR", "http handler panicked", "status=202", "aborted=true"} {
		if !strings.Contains(log, want) {
			t.Errorf("log %q lacks %q", log, want)
		}
	}
}

func TestRecoverPassesDeliberateAbortWithoutErrorLog(t *testing.T) {
	t.Parallel()

	logger, logs := newTestLogger()

	response, err := serveOverNetwork(t, Options{Logger: logger, APIHandler: abortingHandler()})
	if err == nil {
		_ = response.Body.Close()

		t.Fatalf("status = %d, want a dropped connection", response.StatusCode)
	}

	log := logs.String()
	if strings.Contains(log, "level=ERROR") || strings.Contains(log, "panicked") {
		t.Errorf("log %q reports a deliberate abort as a panic", log)
	}

	// No status line reached the client, so the log must not claim a 200.
	if !strings.Contains(log, `msg="http request"`) || !strings.Contains(log, "status=0 aborted=true") {
		t.Errorf("access log %q does not record the aborted request with status 0", log)
	}
}

// TestLogRecordsTruncateClientText sends a 10 KiB method, path, User-Agent and panic value
// to a panicking handler, so both the panic record and the access record carry them.
func TestLogRecordsTruncateClientText(t *testing.T) {
	t.Parallel()

	logger, logs := newTestLogger()
	longMethod := strings.Repeat("M", tenKibibytes)
	longPath := "/api/" + strings.Repeat("p", tenKibibytes)
	longAgent := strings.Repeat("ä", tenKibibytes/len("ä"))
	longPanic := strings.Repeat("v", tenKibibytes) + " cause"

	request := newRequest(t, http.MethodGet, longPath)
	request.Method = longMethod
	request.Header.Set("User-Agent", longAgent)

	panicking := http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic(longPanic) //nolint:forbidigo // the recover middleware under test needs a panicking handler
	})
	serveChain(t, Options{Logger: logger, APIHandler: panicking}, request)

	records := strings.Split(strings.TrimSpace(logs.String()), "\n")
	if len(records) != 2 {
		t.Fatalf("log %q, want the panic record and the access record", logs.String())
	}

	// The bound includes the marker, so no record holds MaxBytes bytes of a client value.
	uncut := []string{"M", "p", "ä"}
	for _, record := range records {
		for _, letter := range uncut {
			if strings.Contains(record, strings.Repeat(letter, logtext.MaxBytes/len(letter))) {
				t.Errorf("record of %d bytes holds an uncut %q value", len(record), letter)
			}
		}

		for _, want := range []string{"method=" + logtext.Truncate(longMethod), "path=" + logtext.Truncate(longPath)} {
			if !strings.Contains(record, want) {
				t.Errorf("record lacks the cut value %.40q...", want)
			}
		}
	}

	if strings.Contains(records[0], strings.Repeat("v", maxLoggedPanicBytes)) || !strings.Contains(records[0], "cause") {
		t.Errorf("panic record of %d bytes, want the panic value cut to %d with its end kept", len(records[0]), maxLoggedPanicBytes)
	}

	if !strings.Contains(records[1], "user_agent="+logtext.Truncate(longAgent)) {
		t.Errorf("access record %q lacks the cut User-Agent", records[1])
	}
}

// tenKibibytes is the size of the hostile client values in the truncation tests.
const tenKibibytes = 10 << 10

// streamPatterns are the quiet patterns M2-32 passes for the stream routes.
func streamPatterns() []string {
	return []string{
		"GET /api/v1/playback/sessions/{sessionId}/file",
		"GET /api/v1/playback/sessions/{sessionId}/hls/{fileName}",
	}
}

func TestAccessLogLevelForQuietPatterns(t *testing.T) {
	t.Parallel()

	const (
		session = "/api/v1/playback/sessions/0b8c6d4e-3f2a-4c1b-9d8e-7f6a5b4c3d2e"
		segment = session + "/hls/segment-1.m4s"
	)

	tests := []struct {
		name      string
		method    string
		target    string
		handler   http.Handler
		wantLevel slog.Level
	}{
		{name: "segment ok", target: segment, handler: textHandler(http.StatusOK, "data"), wantLevel: slog.LevelDebug},
		{name: "file partial content", target: session + "/file", handler: textHandler(http.StatusPartialContent, "data"), wantLevel: slog.LevelDebug},
		{name: "file head", method: http.MethodHead, target: session + "/file", handler: textHandler(http.StatusOK, ""), wantLevel: slog.LevelDebug},
		{name: "segment not modified", target: segment, handler: textHandler(http.StatusNotModified, ""), wantLevel: slog.LevelDebug},
		{name: "segment nothing written", target: segment, handler: http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}), wantLevel: slog.LevelDebug},
		{name: "segment bad request", target: segment, handler: textHandler(http.StatusBadRequest, ""), wantLevel: slog.LevelInfo},
		{name: "segment not found", target: segment, handler: textHandler(http.StatusNotFound, ""), wantLevel: slog.LevelInfo},
		{name: "segment not ready", target: segment, handler: textHandler(http.StatusServiceUnavailable, ""), wantLevel: slog.LevelInfo},
		{name: "segment panic", target: segment, handler: panickingHandler(false), wantLevel: slog.LevelInfo},
		{name: "segment aborted", target: segment, handler: abortingHandler(), wantLevel: slog.LevelInfo},
		{name: "segment with another method", method: http.MethodPost, target: segment, handler: textHandler(http.StatusOK, ""), wantLevel: slog.LevelInfo},
		{name: "nested below hls", target: session + "/hls/a/b", handler: textHandler(http.StatusOK, ""), wantLevel: slog.LevelInfo},
		{name: "get session", target: session, handler: textHandler(http.StatusOK, ""), wantLevel: slog.LevelInfo},
		{name: "stop session", method: http.MethodDelete, target: session, handler: textHandler(http.StatusNoContent, ""), wantLevel: slog.LevelInfo},
		{name: "other api path", target: "/api/v1/health", handler: textHandler(http.StatusOK, ""), wantLevel: slog.LevelInfo},
		{name: "web path", target: "/library/1", handler: textHandler(http.StatusOK, ""), wantLevel: slog.LevelInfo},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			logs := &syncBuffer{}
			logger := slog.New(slog.NewTextHandler(logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
			options := Options{Logger: logger, APIHandler: test.handler, Fallback: test.handler, QuietPatterns: streamPatterns()}

			method := test.method
			if method == "" {
				method = http.MethodGet
			}

			serveAndRecoverAbort(t, options, newRequest(t, method, test.target))

			want := "level=" + test.wantLevel.String() + ` msg="http request"`
			if !strings.Contains(logs.String(), want) {
				t.Errorf("log %q, want an access record with %q", logs.String(), want)
			}
		})
	}
}

// serveAndRecoverAbort serves request through the chain and swallows the abort panic that
// net/http would otherwise turn into a dropped connection.
func serveAndRecoverAbort(t *testing.T, options Options, request *http.Request) {
	t.Helper()

	defer func() {
		if recovered := recover(); recovered != nil && recovered != any(http.ErrAbortHandler) {
			t.Errorf("panic %v escaped the chain, want only http.ErrAbortHandler", recovered)
		}
	}()

	serveChain(t, options, request)
}

func TestNewQuietRoutesRejectsBadPatterns(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		patterns []string
	}{
		{name: "empty", patterns: []string{""}},
		{name: "unclosed wildcard", patterns: []string{"GET /api/v1/{id"}},
		{name: "duplicate", patterns: []string{"GET /api/v1/x", "GET /api/v1/x"}},
		{name: "conflicting wildcards", patterns: []string{"GET /api/v1/{a}/file", "GET /api/v1/{b}/file"}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			routes, err := newQuietRoutes(test.patterns)
			if err == nil || routes.mux != nil {
				t.Errorf("newQuietRoutes(%q) = %v, %v, want an error", test.patterns, routes, err)
			}
		})
	}
}

func TestQuietRoutesWithoutPatternsMatchNothing(t *testing.T) {
	t.Parallel()

	routes, err := newQuietRoutes(nil)
	if err != nil {
		t.Fatalf("newQuietRoutes(nil): %v", err)
	}

	if routes.match(newRequest(t, http.MethodGet, "/api/v1/playback/sessions/1/file")) {
		t.Error("match without patterns = true, want false")
	}
}

// TestRecoverRepanicsPlainAbortSentinel: net/http only stays silent for the sentinel itself
// (it compares with ==); a wrapped abort would be logged with a stack.
func TestRecoverRepanicsPlainAbortSentinel(t *testing.T) {
	t.Parallel()

	wrappedAbort := http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic(fmt.Errorf("stop streaming: %w", http.ErrAbortHandler)) //nolint:forbidigo // the recover middleware under test must normalise a wrapped abort
	})
	logger, logs := newTestLogger()

	defer func() {
		if recovered := recover(); recovered != any(http.ErrAbortHandler) {
			t.Errorf("re-panicked %v, want the plain http.ErrAbortHandler", recovered)
		}

		if strings.Contains(logs.String(), "panicked") {
			t.Errorf("log %q reports a deliberate abort as a panic", logs.String())
		}
	}()

	recoverPanics(wrappedAbort, logger).ServeHTTP(httptest.NewRecorder(), newRequest(t, http.MethodGet, "/api/v1/x"))
}
