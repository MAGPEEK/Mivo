package restapi_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/magpeek/mivo/internal/adapters/http/logtext"
	"github.com/magpeek/mivo/internal/adapters/http/restapi"
)

// TestRequestLogTruncatesClientText sends a 10 KiB item ID: both the path and the binder's
// error text, which quotes the value, are client-controlled.
func TestRequestLogTruncatesClientText(t *testing.T) {
	t.Parallel()

	handler, logs := newTestHandlerWithOptions(t, restapi.Options{})
	longPath := "/api/v1/items/" + strings.Repeat("x", 10*kibibyte)

	got := serve(t, handler, http.MethodGet, longPath)
	assertProblem(t, got, http.StatusBadRequest, restapi.CodeBadRequest)

	record := logs.String()
	assertLogged(t, record, "rejected malformed request")

	// The bound includes the marker, so no value in the record keeps MaxBytes of the ID.
	if strings.Contains(record, strings.Repeat("x", logtext.MaxBytes)) {
		t.Errorf("record of %d bytes holds more than the bound of the client value", len(record))
	}

	if !strings.Contains(record, "path="+logtext.Truncate(longPath)) {
		t.Errorf("record %q lacks the cut path", record)
	}
}

// TestMalformedRequestIsLoggedWithItsStatus: the record is written after the 400/413
// classification, so it names the status the client got.
func TestMalformedRequestIsLoggedWithItsStatus(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		body       string
		wantStatus string
	}{
		{name: "malformed", body: "fileId=1", wantStatus: "status=400"},
		{name: "oversized non-JSON", body: strings.Repeat("x", restapi.MaxRequestBodyBytes+kibibyte), wantStatus: "status=413"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			handler, logs := newTestHandlerWithOptions(t, restapi.Options{})
			serveRequest(t, handler, newBodyRequest(t, sessionsPath, test.body, false))

			for _, line := range strings.Split(logs.String(), "\n") {
				if strings.Contains(line, `msg="rejected malformed request"`) {
					if !strings.Contains(line, test.wantStatus) {
						t.Errorf("record %q, want %s", line, test.wantStatus)
					}

					return
				}
			}

			t.Errorf("log %q has no rejected malformed request record", logs.String())
		})
	}
}
