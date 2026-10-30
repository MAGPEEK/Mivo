package restapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/magpeek/mivo/internal/adapters/http/apigen"
	"github.com/magpeek/mivo/internal/adapters/http/restapi"
	"github.com/magpeek/mivo/internal/domain/system"
)

const testServerID = "0b8c6d4e-3f2a-4c1b-9d8e-7f6a5b4c3d2e"

// fakeSystemService is an in-memory SystemService.
type fakeSystemService struct {
	health  system.Health
	info    system.Info
	infoErr error
}

func (service fakeSystemService) Health(context.Context) system.Health {
	return service.health
}

func (service fakeSystemService) Info(context.Context) (system.Info, error) {
	return service.info, service.infoErr
}

// syncBuffer collects log output; slog handlers may write from several goroutines.
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

	return slog.New(slog.NewTextHandler(logs, &slog.HandlerOptions{Level: slog.LevelDebug})), logs
}

func newTestHandler(t *testing.T, service restapi.SystemService) (http.Handler, *syncBuffer) {
	t.Helper()

	return newTestHandlerWithOptions(t, restapi.Options{System: service})
}

// newTestHandlerWithOptions fills in the logger and, if missing, a healthy system service.
func newTestHandlerWithOptions(t *testing.T, options restapi.Options) (http.Handler, *syncBuffer) {
	t.Helper()

	logger, logs := newTestLogger()
	options.Logger = logger

	if options.System == nil {
		options.System = fakeSystemService{health: healthyHealth()}
	}

	handler, err := restapi.NewHandler(options)
	if err != nil {
		t.Fatalf("NewHandler: %v", err)
	}

	return handler, logs
}

func mustServerID(t *testing.T, text string) system.ServerID {
	t.Helper()

	id, err := system.ParseServerID(text)
	if err != nil {
		t.Fatalf("ParseServerID(%q): %v", text, err)
	}

	return id
}

// response is a recorded response with its body read once.
type response struct {
	status int
	header http.Header
	body   []byte
}

func serve(t *testing.T, handler http.Handler, method, target string) response {
	t.Helper()

	return serveRequest(t, handler, httptest.NewRequestWithContext(t.Context(), method, target, nil))
}

func serveRequest(t *testing.T, handler http.Handler, request *http.Request) response {
	t.Helper()

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)

	result := recorder.Result()
	defer func() {
		if err := result.Body.Close(); err != nil {
			t.Errorf("close body: %v", err)
		}
	}()

	body, err := io.ReadAll(result.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}

	return response{status: result.StatusCode, header: result.Header, body: body}
}

func decode[T any](t *testing.T, body []byte) T {
	t.Helper()

	var value T
	if err := json.Unmarshal(body, &value); err != nil {
		t.Fatalf("decode %s: %v", body, err)
	}

	return value
}

// assertProblem checks the problem content type, status and code of response.
func assertProblem(t *testing.T, got response, wantStatus int, wantCode string) apigen.Problem {
	t.Helper()

	if got.status != wantStatus {
		t.Errorf("status = %d, want %d", got.status, wantStatus)
	}

	if contentType := got.header.Get("Content-Type"); contentType != restapi.ProblemMediaType {
		t.Errorf("Content-Type = %q, want %q", contentType, restapi.ProblemMediaType)
	}

	problem := decode[apigen.Problem](t, got.body)
	if problem.Code != wantCode || int(problem.Status) != wantStatus {
		t.Errorf("problem = %+v, want code %q and status %d", problem, wantCode, wantStatus)
	}

	if problem.Type != "about:blank" || problem.Title != http.StatusText(wantStatus) {
		t.Errorf("problem type/title = %q/%q, want about:blank/%q", problem.Type, problem.Title, http.StatusText(wantStatus))
	}

	return problem
}

func healthyHealth() system.Health {
	return system.NewHealth(system.PassedCheck(system.HealthCheckDatabase))
}

// assertLogged checks that logs contain message.
func assertLogged(t *testing.T, logs, message string) {
	t.Helper()

	if !strings.Contains(logs, message) {
		t.Errorf("log %q does not contain %q", logs, message)
	}
}

// assertNoInternalText checks that a problem body repeats neither the client's input nor
// the parser's error text, which may quote internals.
func assertNoInternalText(t *testing.T, got response, input string) {
	t.Helper()

	for _, leak := range []string{input, "json", "invalid character", "uuid", "unmarshal", "parameter"} {
		if strings.Contains(string(got.body), leak) {
			t.Errorf("body %s contains %q", got.body, leak)
		}
	}
}
