package restapi_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/magpeek/mivo/internal/adapters/http/apigen"
	"github.com/magpeek/mivo/internal/adapters/http/restapi"
)

// TestStreamingStubsAnswerNotImplemented: every strict operation of the M4-02b contract is
// routed and answers 501 "not_implemented" until M4-14b serves it.
func TestStreamingStubsAnswerNotImplemented(t *testing.T) {
	t.Parallel()

	const trickplay = "/api/v1/media-files/1c2d3e4f-5a6b-4c7d-8e9f-0a1b2c3d4e5f/trickplay"

	tests := []struct {
		method string
		target string
	}{
		{method: http.MethodGet, target: "/api/v1/playback/sessions"},
		{method: http.MethodGet, target: "/api/v1/system/transcoder"},
		{method: http.MethodPost, target: "/api/v1/system/transcoder/test"},
		{method: http.MethodGet, target: trickplay},
		{method: http.MethodPost, target: trickplay},
	}

	handler, _ := newTestHandlerWithOptions(t, restapi.Options{})

	for _, test := range tests {
		t.Run(test.method+" "+test.target, func(t *testing.T) {
			t.Parallel()

			request := httptest.NewRequestWithContext(t.Context(), test.method, test.target, strings.NewReader(""))

			assertProblem(t, serveRequest(t, handler, request), http.StatusNotImplemented, "not_implemented")
		})
	}
}

// TestPlaybackRequestWireNames pins the request fields M4-13a and M4-14b map: the quality cap
// is maxBitRate (renamed from maxBitrate before a release carried it) and the subtitle
// fallback is allowDegradeSubtitles.
func TestPlaybackRequestWireNames(t *testing.T) {
	t.Parallel()

	body := []byte(`{"maxBitRate":8000000,"allowDegradeSubtitles":true,` +
		`"profile":{"maxAudioSampleRate":48000,"maxAudioBitDepth":16}}`)

	request := decode[apigen.PlaybackRequest](t, body)

	if request.MaxBitRate == nil || *request.MaxBitRate != 8000000 {
		t.Error("maxBitRate was not decoded as 8000000")
	}

	if request.AllowDegradeSubtitles == nil || !*request.AllowDegradeSubtitles {
		t.Error("allowDegradeSubtitles was not decoded")
	}

	profile := request.Profile
	if profile.MaxAudioSampleRate == nil || *profile.MaxAudioSampleRate != 48000 ||
		profile.MaxAudioBitDepth == nil || *profile.MaxAudioBitDepth != 16 {
		t.Error("maxAudioSampleRate and maxAudioBitDepth were not decoded as 48000 Hz and 16 bit")
	}

	encoded, err := json.Marshal(request)
	if err != nil {
		t.Fatalf("marshal the request: %v", err)
	}

	// encoding/json matches keys without regard to case when it decodes, so only the encoded
	// form shows the spelling of the contract.
	if !strings.Contains(string(encoded), `"maxBitRate":8000000`) || strings.Contains(string(encoded), "maxBitrate") {
		t.Errorf("the request does not carry maxBitRate: %s", encoded)
	}
}
