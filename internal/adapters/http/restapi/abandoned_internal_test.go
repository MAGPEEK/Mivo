package restapi

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/magpeek/mivo/internal/adapters/http/statuswriter"
	"github.com/magpeek/mivo/internal/domain/system"
)

// headerCountingWriter counts status lines, so a test can tell that none or no second one
// was written.
type headerCountingWriter struct {
	*httptest.ResponseRecorder
	writeHeaderCalls int
}

func (writer *headerCountingWriter) WriteHeader(status int) {
	writer.writeHeaderCalls++
	writer.ResponseRecorder.WriteHeader(status)
}

func cancelledRequest(t *testing.T) *http.Request {
	t.Helper()

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	return httptest.NewRequestWithContext(ctx, http.MethodGet, "/api/v1/system/info", nil)
}

// assertAbandoned checks that an abandoned response was logged at debug level only.
func assertAbandoned(t *testing.T, logs string) {
	t.Helper()

	if strings.Contains(logs, "level=ERROR") {
		t.Errorf("log %q reports an abandoned response as an error", logs)
	}

	if !strings.Contains(logs, "level=DEBUG") || !strings.Contains(logs, "api response abandoned") {
		t.Errorf("log %q has no debug record of the abandoned response", logs)
	}
}

func TestResponseErrorHandlerSkipsAbandonedResponses(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		request         func(t *testing.T) *http.Request
		headersSent     bool
		wantWriteHeader int
	}{
		{name: "headers already sent", request: func(t *testing.T) *http.Request { return requestWithID(t, "req-3") }, headersSent: true, wantWriteHeader: 1},
		{name: "client gone", request: cancelledRequest, wantWriteHeader: 0},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			api, logs := newInternalTestAPI(t)
			counter := &headerCountingWriter{ResponseRecorder: httptest.NewRecorder()}
			writer := statuswriter.New(counter)

			if test.headersSent {
				writer.WriteHeader(http.StatusOK)
			}

			api.strictOptions().ResponseErrorHandlerFunc(writer, test.request(t), errors.New("write: connection reset"))

			if counter.writeHeaderCalls != test.wantWriteHeader {
				t.Errorf("WriteHeader calls = %d, want %d", counter.writeHeaderCalls, test.wantWriteHeader)
			}

			if counter.Body.Len() != 0 {
				t.Errorf("body = %q, want no problem", counter.Body)
			}

			assertAbandoned(t, logs.String())
		})
	}
}

// infoFailingService fails Info with the error of its context, as a cancelled query does.
type infoFailingService struct{ SystemService }

func (infoFailingService) Info(ctx context.Context) (system.Info, error) {
	return system.Info{}, ctx.Err()
}

func TestGetSystemInfoForCancelledRequestWritesNothing(t *testing.T) {
	t.Parallel()

	logs := &bytes.Buffer{}
	logger := slog.New(slog.NewTextHandler(logs, &slog.HandlerOptions{Level: slog.LevelDebug}))

	api, err := NewAPI(infoFailingService{}, logger)
	if err != nil {
		t.Fatalf("NewAPI: %v", err)
	}

	counter := &headerCountingWriter{ResponseRecorder: httptest.NewRecorder()}
	api.Handler().ServeHTTP(counter, cancelledRequest(t))

	if counter.writeHeaderCalls != 0 || counter.Body.Len() != 0 {
		t.Errorf("WriteHeader calls = %d, body %q, want nothing written", counter.writeHeaderCalls, counter.Body)
	}

	assertAbandoned(t, logs.String())

	if !strings.Contains(logs.String(), "context canceled") {
		t.Errorf("log %q does not record the cause", logs.String())
	}
}
