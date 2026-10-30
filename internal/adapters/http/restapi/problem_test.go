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
