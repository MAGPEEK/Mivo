package restapi_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/magpeek/mivo/internal/adapters/http/clientorigin"
)

// The session records whether its client is in the local network, for the activity view
// (M5-16). A request that never passed the origin resolver counts as remote.
func TestCreatePlaybackSessionPassesTheClientClassOn(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		local      bool
		classified bool
		wantLocal  bool
	}{
		{name: "local client", local: true, classified: true, wantLocal: true},
		{name: "remote client", local: false, classified: true, wantLocal: false},
		{name: "unclassified request is remote", classified: false, wantLocal: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			service := &fakePlaybackService{session: transcodeSession()}
			handler, _ := newPlaybackHandler(t, service)

			request := httptest.NewRequestWithContext(t.Context(), http.MethodPost, sessionsPath, strings.NewReader(playbackRequestBody))
			request.Header.Set("Content-Type", "application/json")

			if test.classified {
				request = request.WithContext(clientorigin.WithLocal(request.Context(), test.local))
			}

			if got := serveRequest(t, handler, request); got.status != http.StatusCreated {
				t.Fatalf("status = %d, want 201 (body %s)", got.status, got.body)
			}

			started := service.startRequests()
			if len(started) != 1 || started[0].Local != test.wantLocal {
				t.Errorf("Start requests = %+v, want one with Local = %v", started, test.wantLocal)
			}
		})
	}
}
