package restapi_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/magpeek/mivo/internal/adapters/http/apigen"
	"github.com/magpeek/mivo/internal/adapters/http/restapi"
)

// TestWatchStubsAnswerNotImplemented: every watch operation of the M7-03 contract is routed
// and answers 501 "not_implemented" until M7-23 serves it.
func TestWatchStubsAnswerNotImplemented(t *testing.T) {
	t.Parallel()

	const item = "/api/v1/items/8a9b0c1d-2e3f-4a5b-8c6d-7e8f9a0b1c2d"

	tests := []struct {
		method string
		target string
		body   string
	}{
		{method: http.MethodPut, target: item + "/progress", body: `{"positionSeconds":2530.5,"playbackSessionId":"5d4c3b2a-1f0e-4d9c-8b7a-6f5e4d3c2b1a"}`},
		{method: http.MethodPut, target: item + "/watched"},
		{method: http.MethodDelete, target: item + "/watched"},
		{method: http.MethodDelete, target: item + "/continue-watching"},
	}

	handler, _ := newTestHandlerWithOptions(t, restapi.Options{})

	for _, test := range tests {
		t.Run(test.method+" "+test.target, func(t *testing.T) {
			t.Parallel()

			request := httptest.NewRequestWithContext(t.Context(), test.method, test.target, strings.NewReader(test.body))
			if test.body != "" {
				request.Header.Set("Content-Type", "application/json")
			}

			assertProblem(t, serveRequest(t, handler, request), http.StatusNotImplemented, "not_implemented")
		})
	}
}

// TestProgressReportOutsideTheDoubleRangeIsABadRequest pins the contract for M7-23: JSON has
// no NaN or Infinity, and a number beyond the double range fails to bind with 400
// "bad_request" before any handler sees it.
func TestProgressReportOutsideTheDoubleRangeIsABadRequest(t *testing.T) {
	t.Parallel()

	handler, _ := newTestHandlerWithOptions(t, restapi.Options{})

	for _, body := range []string{`{"positionSeconds":1e400}`, `{"positionSeconds":NaN}`, `{"positionSeconds":"Infinity"}`} {
		t.Run(body, func(t *testing.T) {
			t.Parallel()

			request := httptest.NewRequestWithContext(t.Context(), http.MethodPut,
				"/api/v1/items/8a9b0c1d-2e3f-4a5b-8c6d-7e8f9a0b1c2d/progress", strings.NewReader(body))
			request.Header.Set("Content-Type", "application/json")

			assertProblem(t, serveRequest(t, handler, request), http.StatusBadRequest, "bad_request")
		})
	}
}

// TestViewerStateOmitsUnknownFields pins the generated type M7-23 fills: lastPlayedAt and
// unwatchedCount are optional, the rest is always present.
func TestViewerStateOmitsUnknownFields(t *testing.T) {
	t.Parallel()

	state := decode[apigen.ViewerState](t, []byte(`{"watched":false,"positionSeconds":0,"playCount":0}`))
	if state.LastPlayedAt != nil || state.UnwatchedCount != nil {
		t.Errorf("state = %+v, want no lastPlayedAt and no unwatchedCount", state)
	}
}
