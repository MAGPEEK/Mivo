package restapi_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/magpeek/mivo/internal/adapters/http/restapi"
)

const (
	testSessionID = "5d4c3b2a-1f0e-4d9c-8b7a-6f5e4d3c2b1a"
	sessionsPath  = "/api/v1/playback/sessions"

	// playbackRequestBody is a minimal well-formed PlaybackRequest.
	playbackRequestBody = `{"fileId":"1c2d3e4f-5a6b-4c7d-8e9f-0a1b2c3d4e5f",` +
		`"profile":{"name":"test","directPlay":[],"hls":{"delivery":"none","videoCodecs":[],"audioCodecs":[]}}}`
)

// TestPlaybackOperationsAnswerNotImplemented covers the stubs M2-28 replaces.
func TestPlaybackOperationsAnswerNotImplemented(t *testing.T) {
	t.Parallel()

	handler, _ := newTestHandlerWithOptions(t, restapi.Options{})

	tests := []struct {
		name   string
		method string
		target string
		body   string
	}{
		{name: "create session", method: http.MethodPost, target: sessionsPath, body: playbackRequestBody},
		{name: "get session", method: http.MethodGet, target: sessionsPath + "/" + testSessionID},
		{name: "stop session", method: http.MethodDelete, target: sessionsPath + "/" + testSessionID},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			request := httptest.NewRequestWithContext(t.Context(), test.method, test.target, strings.NewReader(test.body))
			got := serveRequest(t, handler, request)
			assertProblem(t, got, http.StatusNotImplemented, notImplementedCode)
		})
	}
}

// TestStreamRoutesAreNotGenerated checks that the stream operations are excluded from the
// strict server (oapi-codegen.yaml): without mounts nothing serves them.
func TestStreamRoutesAreNotGenerated(t *testing.T) {
	t.Parallel()

	handler, _ := newTestHandlerWithOptions(t, restapi.Options{})

	for _, target := range []string{
		sessionsPath + "/" + testSessionID + "/file",
		sessionsPath + "/" + testSessionID + "/hls/index.m3u8",
	} {
		t.Run(target, func(t *testing.T) {
			t.Parallel()

			assertProblem(t, serve(t, handler, http.MethodGet, target), http.StatusNotFound, restapi.CodeNotFound)
		})
	}
}

func TestCreatePlaybackSessionRejectsMalformedBodies(t *testing.T) {
	t.Parallel()

	handler, logs := newTestHandlerWithOptions(t, restapi.Options{})

	for name, body := range map[string]string{
		"not json":       "fileId=1",
		"wrong type":     `{"fileId":42}`,
		"malformed uuid": `{"fileId":"not-a-uuid"}`,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			request := httptest.NewRequestWithContext(t.Context(), http.MethodPost, sessionsPath, strings.NewReader(body))
			got := serveRequest(t, handler, request)
			assertProblem(t, got, http.StatusBadRequest, restapi.CodeBadRequest)
			assertNoInternalText(t, got, body)
		})
	}

	t.Cleanup(func() {
		assertLogged(t, logs.String(), "rejected malformed request")
	})
}
