package requestid_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"uuid"

	"github.com/magpeek/mivo/internal/adapters/http/requestid"
)

// serve runs the middleware with incoming as X-Request-ID (none when empty) and returns the
// ID the handler saw in its context and the echoed response header.
func serve(t *testing.T, incoming string) (fromContext, echoed string) {
	t.Helper()

	handler := requestid.Middleware(http.HandlerFunc(func(_ http.ResponseWriter, request *http.Request) {
		fromContext = requestid.FromContext(request.Context())
	}))

	request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil)
	if incoming != "" {
		request.Header.Set(requestid.HeaderName, incoming)
	}

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)

	return fromContext, recorder.Header().Get(requestid.HeaderName)
}

func TestMiddlewareReusesAcceptableID(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		id   string
	}{
		{name: "uuid", id: "0b8c6d4e-3f2a-4c1b-9d8e-7f6a5b4c3d2e"},
		{name: "proxy trace id", id: "1-5759e988_bd862e3f.0:a"},
		{name: "maximum length", id: strings.Repeat("a", 128)},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			fromContext, echoed := serve(t, test.id)
			if fromContext != test.id || echoed != test.id {
				t.Errorf("context = %q, header = %q, want both %q", fromContext, echoed, test.id)
			}
		})
	}
}

func TestMiddlewareGeneratesIDForMissingOrUnacceptableHeader(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		id   string
	}{
		{name: "missing", id: ""},
		{name: "too long", id: strings.Repeat("a", 129)},
		{name: "space", id: "abc def"},
		{name: "log injection", id: "abc\" level=ERROR msg=\"forged"},
		{name: "non ascii", id: "abcä"},
		{name: "equals sign", id: "Root=abc"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			fromContext, echoed := serve(t, test.id)
			if fromContext != echoed {
				t.Errorf("context = %q, header = %q, want the same ID", fromContext, echoed)
			}

			if _, err := uuid.Parse(echoed); err != nil {
				t.Errorf("generated ID %q is not a UUID: %v", echoed, err)
			}
		})
	}
}

func TestMiddlewareGeneratesDistinctIDs(t *testing.T) {
	t.Parallel()

	first, _ := serve(t, "")
	second, _ := serve(t, "")

	if first == second {
		t.Errorf("two requests got the same generated ID %q", first)
	}
}

func TestFromContextWithoutID(t *testing.T) {
	t.Parallel()

	if id := requestid.FromContext(t.Context()); id != "" {
		t.Errorf("FromContext = %q, want empty", id)
	}
}

func TestNewContextRoundTrip(t *testing.T) {
	t.Parallel()

	ctx := requestid.NewContext(t.Context(), "abc")
	if id := requestid.FromContext(ctx); id != "abc" {
		t.Errorf("FromContext = %q, want abc", id)
	}
}
