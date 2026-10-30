package restapi_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/magpeek/mivo/internal/adapters/http/restapi"
	"github.com/magpeek/mivo/internal/domain"
)

func TestProblemForError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		err        error
		wantStatus int
		wantCode   string
	}{
		{name: "not found", err: domain.ErrNotFound, wantStatus: http.StatusNotFound, wantCode: "not_found"},
		{name: "forbidden", err: domain.ErrForbidden, wantStatus: http.StatusForbidden, wantCode: "forbidden"},
		{name: "conflict", err: domain.ErrConflict, wantStatus: http.StatusConflict, wantCode: "conflict"},
		{name: "invalid", err: domain.ErrInvalid, wantStatus: http.StatusBadRequest, wantCode: "invalid"},
		{name: "unsupported", err: domain.ErrUnsupported, wantStatus: http.StatusUnprocessableEntity, wantCode: "unsupported"},
		{name: "unavailable", err: domain.ErrUnavailable, wantStatus: http.StatusServiceUnavailable, wantCode: "unavailable"},
		{
			name:       "wrapped unsupported",
			err:        fmt.Errorf("start playback of file 9: %w", domain.ErrUnsupported),
			wantStatus: http.StatusUnprocessableEntity,
			wantCode:   "unsupported",
		},
		{
			name:       "not found before unavailable",
			err:        errors.Join(domain.ErrUnavailable, domain.ErrNotFound),
			wantStatus: http.StatusNotFound,
			wantCode:   "not_found",
		},
		{
			name:       "wrapped not found",
			err:        fmt.Errorf("load library 7: %w", domain.ErrNotFound),
			wantStatus: http.StatusNotFound,
			wantCode:   "not_found",
		},
		{
			name:       "joined errors take the first mapping",
			err:        errors.Join(domain.ErrInvalid, domain.ErrNotFound),
			wantStatus: http.StatusNotFound,
			wantCode:   "not_found",
		},
		{name: "unknown error", err: errors.New("disk full"), wantStatus: http.StatusInternalServerError, wantCode: "internal_error"},
		{name: "context canceled", err: context.Canceled, wantStatus: http.StatusInternalServerError, wantCode: "internal_error"},
		{name: "nil error", err: nil, wantStatus: http.StatusInternalServerError, wantCode: "internal_error"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			problem := restapi.ProblemForError(test.err)
			if int(problem.Status) != test.wantStatus || problem.Code != test.wantCode {
				t.Errorf("ProblemForError(%v) = %d %q, want %d %q",
					test.err, problem.Status, problem.Code, test.wantStatus, test.wantCode)
			}

			if problem.Type != "about:blank" || problem.Title != http.StatusText(test.wantStatus) || problem.Detail != nil {
				t.Errorf("problem = %+v, want about:blank, the status title and no detail", problem)
			}
		})
	}
}

// TestPublishedProblemCodes pins the codes of api/openapi.yaml (Problem.code). Clients
// translate them, so a published code never changes; api/spec_test.go checks the same list
// against the contract text.
func TestPublishedProblemCodes(t *testing.T) {
	t.Parallel()

	codes := map[string]string{
		restapi.CodeBadRequest:           "bad_request",
		restapi.CodeInvalid:              "invalid",
		restapi.CodeForbidden:            "forbidden",
		restapi.CodeNotFound:             "not_found",
		restapi.CodeMethodNotAllowed:     "method_not_allowed",
		restapi.CodeConflict:             "conflict",
		restapi.CodePayloadTooLarge:      "payload_too_large",
		restapi.CodeUnsupported:          "unsupported",
		restapi.CodeNoCompatibleFormat:   "no_compatible_format",
		restapi.CodeTranscodeUnavailable: "transcode_unavailable",
		restapi.CodeInternal:             "internal_error",
		restapi.CodeUnavailable:          "unavailable",
		restapi.CodeSessionLimitReached:  "session_limit_reached",
		restapi.CodeStreamNotReady:       "stream_not_ready",
	}

	const publishedCodeCount = 14
	if len(codes) != publishedCodeCount {
		t.Fatalf("%d distinct codes, want %d: two constants share a value", len(codes), publishedCodeCount)
	}

	for got, want := range codes {
		if got != want {
			t.Errorf("code constant = %q, want %q", got, want)
		}
	}
}

func TestWriteProblem(t *testing.T) {
	t.Parallel()

	recorder := httptest.NewRecorder()
	recorder.Header().Set("Content-Length", "999")
	recorder.Header().Set("Content-Type", "text/plain")

	restapi.WriteProblem(recorder, restapi.NewProblem(http.StatusConflict, restapi.CodeConflict))

	got := response{status: recorder.Code, header: recorder.Header(), body: recorder.Body.Bytes()}
	assertProblem(t, got, http.StatusConflict, restapi.CodeConflict)

	if length := recorder.Header().Get("Content-Length"); length != "" {
		t.Errorf("Content-Length = %q, want it removed", length)
	}
}

func TestRouting(t *testing.T) {
	t.Parallel()

	service := fakeSystemService{info: testInfo(t, "", ""), health: healthyHealth()}
	handler, _ := newTestHandler(t, service)

	tests := []struct {
		name       string
		method     string
		target     string
		wantStatus int
	}{
		{name: "health is routed", method: http.MethodGet, target: healthPath, wantStatus: http.StatusOK},
		{name: "system info is routed", method: http.MethodGet, target: infoPath, wantStatus: http.StatusOK},
		{name: "head on a get route", method: http.MethodHead, target: healthPath, wantStatus: http.StatusOK},
		{name: "path cleaning redirect passes through", method: http.MethodGet, target: "/api/v1//health", wantStatus: http.StatusTemporaryRedirect},
		// Unmatched, so the mux's own answer goes through the problem writer, which must
		// leave statuses other than 404 and 405 alone.
		{name: "unmatched path cleaning redirect passes through", method: http.MethodGet, target: "/api/v1//nope", wantStatus: http.StatusTemporaryRedirect},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			got := serve(t, handler, test.method, test.target)
			if got.status != test.wantStatus {
				t.Errorf("%s %s status = %d, want %d", test.method, test.target, got.status, test.wantStatus)
			}
		})
	}
}

func TestRoutingProblems(t *testing.T) {
	t.Parallel()

	handler, _ := newTestHandler(t, fakeSystemService{health: healthyHealth()})

	tests := []struct {
		name       string
		method     string
		target     string
		wantStatus int
		wantCode   string
		wantAllow  string
	}{
		{name: "unknown route", method: http.MethodGet, target: "/api/v1/nope", wantStatus: http.StatusNotFound, wantCode: "not_found"},
		{name: "unknown version", method: http.MethodGet, target: "/api/v2/health", wantStatus: http.StatusNotFound, wantCode: "not_found"},
		{name: "api root", method: http.MethodGet, target: "/api/", wantStatus: http.StatusNotFound, wantCode: "not_found"},
		{name: "trailing slash", method: http.MethodGet, target: healthPath + "/", wantStatus: http.StatusNotFound, wantCode: "not_found"},
		{
			name: "post on health", method: http.MethodPost, target: healthPath,
			wantStatus: http.StatusMethodNotAllowed, wantCode: "method_not_allowed", wantAllow: "GET, HEAD",
		},
		{
			name: "delete on system info", method: http.MethodDelete, target: infoPath,
			wantStatus: http.StatusMethodNotAllowed, wantCode: "method_not_allowed", wantAllow: "GET, HEAD",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			got := serve(t, handler, test.method, test.target)
			assertProblem(t, got, test.wantStatus, test.wantCode)

			if allow := got.header.Get("Allow"); allow != test.wantAllow {
				t.Errorf("Allow = %q, want %q", allow, test.wantAllow)
			}
		})
	}
}
