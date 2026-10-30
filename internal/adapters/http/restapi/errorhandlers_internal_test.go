package restapi

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/magpeek/mivo/internal/adapters/http/apigen"
	"github.com/magpeek/mivo/internal/adapters/http/requestid"
	"github.com/magpeek/mivo/internal/domain"
)

// internalText stands for error text that must never reach a client.
const internalText = `sqlite: open C:\ProgramData\mivo\mivo.db`

// These tests call the error handlers through the option structs Handler passes to the
// generated constructors, so every entry point is covered with exactly the error types the
// generated code produces, and none can fall back to the generated text/plain defaults
// unnoticed. binding_test.go and bodylimit_test.go cover the same paths through requests.

func newInternalTestAPI(t *testing.T) (*API, *bytes.Buffer) {
	t.Helper()

	logs := &bytes.Buffer{}
	logger := slog.New(slog.NewTextHandler(logs, &slog.HandlerOptions{Level: slog.LevelDebug}))

	api, err := NewAPI(Options{System: nilSystemService{}, Logger: logger})
	if err != nil {
		t.Fatalf("NewAPI: %v", err)
	}

	return api, logs
}

type nilSystemService struct{ SystemService }

type errorHandler func(writer http.ResponseWriter, request *http.Request, err error)

// TestMalformedRequestHandlersWriteProblemsWithoutErrorText covers both places where the
// generated code reports a request it could not parse or bind.
func TestMalformedRequestHandlersWriteProblemsWithoutErrorText(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		handler func(api *API) errorHandler
	}{
		{name: "strict request error", handler: func(api *API) errorHandler { return api.strictOptions().RequestErrorHandlerFunc }},
		{name: "parameter binding error", handler: func(api *API) errorHandler {
			return api.routerOptions(http.NewServeMux()).ErrorHandlerFunc
		}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			api, logs := newInternalTestAPI(t)
			recorder := httptest.NewRecorder()
			bindError := &apigen.InvalidParamFormatError{ParamName: "id", Err: errors.New(internalText)}
			test.handler(api)(recorder, requestWithID(t, "req-1"), bindError)

			problem := assertInternalProblem(t, recorder, http.StatusBadRequest, CodeBadRequest)
			if problem.Detail != nil {
				t.Errorf("detail = %q, want none", *problem.Detail)
			}

			if !strings.Contains(logs.String(), "mivo.db") || !strings.Contains(logs.String(), "request_id=req-1") {
				t.Errorf("log %q does not record the cause with the request ID", logs.String())
			}
		})
	}
}

func TestResponseErrorHandlerMapsErrorsWithoutErrorText(t *testing.T) {
	t.Parallel()

	// wantLevel is the only level the cause is logged at; empty means debug.
	tests := []struct {
		name       string
		err        error
		wantStatus int
		wantCode   string
		wantLevel  string
	}{
		{name: "unexpected response type", err: fmt.Errorf("unexpected response type: %s", internalText), wantStatus: 500, wantCode: CodeInternal, wantLevel: "ERROR"},
		{name: "encoding failure", err: &json.UnsupportedValueError{Str: internalText}, wantStatus: 500, wantCode: CodeInternal, wantLevel: "ERROR"},
		{name: "domain not found", err: fmt.Errorf("%s: %w", internalText, domain.ErrNotFound), wantStatus: 404, wantCode: CodeNotFound},
		{name: "domain invalid", err: fmt.Errorf("%s: %w", internalText, domain.ErrInvalid), wantStatus: 400, wantCode: CodeInvalid},
		{name: "domain unsupported", err: fmt.Errorf("%s: %w", internalText, domain.ErrUnsupported), wantStatus: 422, wantCode: CodeUnsupported},
		{name: "domain unavailable", err: fmt.Errorf("%s: %w", internalText, domain.ErrUnavailable), wantStatus: 503, wantCode: CodeUnavailable, wantLevel: "WARN"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			api, logs := newInternalTestAPI(t)
			recorder := httptest.NewRecorder()
			api.strictOptions().ResponseErrorHandlerFunc(recorder, requestWithID(t, "req-2"), test.err)

			assertInternalProblem(t, recorder, test.wantStatus, test.wantCode)

			wantLevel := test.wantLevel
			if wantLevel == "" {
				// Client mistakes are traced at debug level only.
				wantLevel = "DEBUG"
			}

			for _, level := range []string{"ERROR", "WARN", "DEBUG"} {
				logged := strings.Contains(logs.String(), "level="+level) && strings.Contains(logs.String(), "mivo.db")
				if logged != (level == wantLevel) {
					t.Errorf("cause logged at %s = %v, want level %s; log %q", level, logged, wantLevel, logs.String())
				}
			}
		})
	}
}

// TestRequestErrorHandlerAnswersCutOffBodies covers the body limit: the generated code
// wraps the reader's *http.MaxBytesError in its decode error.
func TestRequestErrorHandlerAnswersCutOffBodies(t *testing.T) {
	t.Parallel()

	api, _ := newInternalTestAPI(t)
	recorder := httptest.NewRecorder()
	cutOff := fmt.Errorf("can't decode JSON body: %w", &http.MaxBytesError{Limit: MaxRequestBodyBytes})
	api.strictOptions().RequestErrorHandlerFunc(recorder, requestWithID(t, "req-4"), cutOff)

	assertInternalProblem(t, recorder, http.StatusRequestEntityTooLarge, CodePayloadTooLarge)
}

func requestWithID(t *testing.T, id string) *http.Request {
	t.Helper()

	request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/v1/health", nil)

	return request.WithContext(requestid.NewContext(request.Context(), id))
}

func assertInternalProblem(t *testing.T, recorder *httptest.ResponseRecorder, wantStatus int, wantCode string) apigen.Problem {
	t.Helper()

	if recorder.Code != wantStatus {
		t.Errorf("status = %d, want %d", recorder.Code, wantStatus)
	}

	if contentType := recorder.Header().Get("Content-Type"); contentType != ProblemMediaType {
		t.Errorf("Content-Type = %q, want %q", contentType, ProblemMediaType)
	}

	if strings.Contains(recorder.Body.String(), "mivo.db") {
		t.Errorf("body %s exposes the internal error text", recorder.Body)
	}

	var problem apigen.Problem
	if err := json.Unmarshal(recorder.Body.Bytes(), &problem); err != nil {
		t.Fatalf("decode %s: %v", recorder.Body, err)
	}

	if problem.Code != wantCode {
		t.Errorf("code = %q, want %q", problem.Code, wantCode)
	}

	return problem
}

func TestParseBuildTime(t *testing.T) {
	t.Parallel()

	commitTime := time.Date(2026, 10, 29, 12, 30, 0, 0, time.UTC)

	tests := []struct {
		name    string
		text    string
		want    *time.Time
		wantErr bool
	}{
		{name: "unknown", text: "", want: nil},
		{name: "utc", text: "2026-10-29T12:30:00Z", want: &commitTime},
		{name: "offset", text: "2026-10-29T13:30:00+01:00", want: &commitTime},
		{name: "date only", text: "2026-10-29", wantErr: true},
		{name: "not a time", text: "yesterday", wantErr: true},
		{name: "missing zone", text: "2026-10-29T12:30:00", wantErr: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			got, err := parseBuildTime(test.text)
			if (err != nil) != test.wantErr {
				t.Fatalf("parseBuildTime(%q) error = %v, wantErr %v", test.text, err, test.wantErr)
			}

			if (got == nil) != (test.want == nil) || (got != nil && !got.Equal(*test.want)) {
				t.Errorf("parseBuildTime(%q) = %v, want %v", test.text, got, test.want)
			}
		})
	}
}

func TestRoutingProblemWriterUnwraps(t *testing.T) {
	t.Parallel()

	recorder := httptest.NewRecorder()
	writer := &routingProblemWriter{ResponseWriter: recorder}

	if writer.Unwrap() != recorder {
		t.Error("Unwrap() does not return the underlying writer")
	}
}
