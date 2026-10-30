package restapi

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/magpeek/mivo/internal/adapters/http/logtext"
	"github.com/magpeek/mivo/internal/domain"
)

// tenKibibytes is the size of the hostile client values in these tests.
const tenKibibytes = 10 << 10

// TestErrorHandlersCutClientText calls every error log of the API with a 10 KiB method and
// an error that quotes 10 KiB of client input before its cause.
func TestErrorHandlersCutClientText(t *testing.T) {
	t.Parallel()

	quoting := fmt.Errorf("item %s: %w", strings.Repeat("q", tenKibibytes), domain.ErrNotFound)

	tests := []struct {
		name    string
		request func(t *testing.T) *http.Request
		handle  func(api *API) errorHandler
		message string
	}{
		{
			name: "malformed request", request: longMethodRequest, message: "rejected malformed request",
			handle: func(api *API) errorHandler { return api.writeRequestError },
		},
		{
			name: "client mistake", request: longMethodRequest, message: "api request rejected",
			handle: func(api *API) errorHandler { return api.writeResponseError },
		},
		{
			name: "abandoned response", message: "api response abandoned",
			request: func(t *testing.T) *http.Request {
				t.Helper()

				ctx, cancel := context.WithCancel(longMethodRequest(t).Context())
				cancel()

				return longMethodRequest(t).WithContext(ctx)
			},
			handle: func(api *API) errorHandler { return api.writeResponseError },
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			api, logs := newInternalTestAPI(t)
			test.handle(api)(httptest.NewRecorder(), test.request(t), quoting)

			record := logs.String()
			if !strings.Contains(record, test.message) || !strings.Contains(record, domain.ErrNotFound.Error()) {
				t.Errorf("record %q lacks %q or the cause", record, test.message)
			}

			for _, uncut := range []string{"M", "q"} {
				if strings.Contains(record, strings.Repeat(uncut, logtext.MaxBytes)) {
					t.Errorf("record of %d bytes holds an uncut %q value", len(record), uncut)
				}
			}
		})
	}
}

// longMethodRequest is a request with a 10 KiB method, a valid HTTP token.
func longMethodRequest(t *testing.T) *http.Request {
	t.Helper()

	request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/v1/items/x", nil)
	request.Method = strings.Repeat("M", tenKibibytes)

	return request
}

// TestRemainingBodyDrainIsBounded: the drain stops after the limit even for a body the
// limit middleware did not wrap.
func TestRemainingBodyDrainIsBounded(t *testing.T) {
	t.Parallel()

	body := &endlessBody{}
	request := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/api/v1/x", nil)
	request.Body = io.NopCloser(body)

	if remainingBodyTooLarge(request) {
		t.Error("remainingBodyTooLarge = true for a body without the limit error")
	}

	if body.read > MaxRequestBodyBytes+1 {
		t.Errorf("drained %d bytes, want at most %d", body.read, MaxRequestBodyBytes+1)
	}
}

// endlessBody never ends and counts what was read from it.
type endlessBody struct {
	read int
}

func (body *endlessBody) Read(buffer []byte) (int, error) {
	body.read += len(buffer)

	return len(buffer), nil
}
