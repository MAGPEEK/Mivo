package server

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"unicode/utf8"

	"github.com/magpeek/mivo/internal/adapters/http/apigen"
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
	newHandler(options).ServeHTTP(recorder, request)

	return recorder
}

func newRequest(t *testing.T, method, target string) *http.Request {
	t.Helper()

	return httptest.NewRequestWithContext(t.Context(), method, target, nil)
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
		{name: "nil fallback answers 404", options: withoutFallback, target: "/library/7", wantStatus: http.StatusNotFound},
		{name: "nil fallback keeps api", options: withoutFallback, target: "/api/v1/health", wantStatus: http.StatusOK, wantBody: "api"},
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

	testServer := httptest.NewServer(newHandler(options))
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

	if !strings.Contains(log, `msg="http request"`) || !strings.Contains(log, "aborted=true") {
		t.Errorf("access log %q does not record the aborted request", log)
	}
}

func TestAccessLogTruncatesClientText(t *testing.T) {
	t.Parallel()

	logger, logs := newTestLogger()
	longPath := "/api/" + strings.Repeat("p", 2*maxLoggedValueBytes)
	longAgent := strings.Repeat("ä", maxLoggedValueBytes)

	request := newRequest(t, http.MethodGet, longPath)
	request.Header.Set("User-Agent", longAgent)

	serveChain(t, Options{Logger: logger, APIHandler: textHandler(http.StatusOK, "")}, request)

	log := logs.String()
	if strings.Contains(log, longPath) || strings.Contains(log, longAgent) {
		t.Errorf("access log has an uncut client value: %d bytes", len(log))
	}

	for _, want := range []string{"path=" + truncateForLog(longPath), "user_agent=" + truncateForLog(longAgent)} {
		if !strings.Contains(log, want) {
			t.Errorf("access log %q lacks the cut value %q", log, want)
		}
	}
}

func TestTruncateForLog(t *testing.T) {
	t.Parallel()

	fits := strings.Repeat("a", maxLoggedValueBytes)
	ascii := strings.Repeat("a", maxLoggedValueBytes+1)
	// "€" is three bytes, so a cut at a fixed byte offset lands inside a sequence.
	multiByte := strings.Repeat("€", maxLoggedValueBytes)

	tests := []struct {
		name  string
		value string
		want  string
	}{
		{name: "empty", value: "", want: ""},
		{name: "fits exactly", value: fits, want: fits},
		{name: "ascii", value: ascii, want: ascii[:maxLoggedValueBytes-len(truncationMarker)] + truncationMarker},
		{name: "multi-byte", value: multiByte, want: strings.Repeat("€", (maxLoggedValueBytes-len(truncationMarker))/len("€")) + truncationMarker},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			got := truncateForLog(test.value)
			if got != test.want {
				t.Errorf("truncateForLog = %q, want %q", got, test.want)
			}

			if len(got) > maxLoggedValueBytes || !utf8.ValidString(got) {
				t.Errorf("truncateForLog = %d bytes, valid UTF-8 %v; want at most %d valid bytes", len(got), utf8.ValidString(got), maxLoggedValueBytes)
			}
		})
	}
}
