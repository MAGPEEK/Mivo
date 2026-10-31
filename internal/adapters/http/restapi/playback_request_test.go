package restapi_test

import (
	"encoding/json"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"
	"uuid"

	"github.com/magpeek/mivo/internal/adapters/http/apigen"
	"github.com/magpeek/mivo/internal/adapters/http/restapi"
	appplayback "github.com/magpeek/mivo/internal/app/playback"
	"github.com/magpeek/mivo/internal/domain/media"
	"github.com/magpeek/mivo/internal/domain/playback"
)

const replacedSessionID = "0f1e2d3c-4b5a-4968-8776-655443322110"

// fullPlaybackRequest uses every field of PlaybackRequest.
func fullPlaybackRequest(t *testing.T) apigen.PlaybackRequest {
	t.Helper()

	return apigen.PlaybackRequest{
		FileID:               playbackUUID(t, testFileID),
		StartPositionSeconds: playbackPointer(600.25),
		AudioStreamIndex:     playbackPointer[int32](1),
		ReplacesSessionID:    playbackPointer(playbackUUID(t, replacedSessionID)),
		Profile: apigen.ClientProfile{
			Name: testProfileName,
			DirectPlay: []apigen.DirectPlayProfile{
				{Container: "mp4", VideoCodecs: []string{"h264", "av1"}, AudioCodecs: []string{"aac"}},
				{Container: "webm", VideoCodecs: []string{"vp9"}, AudioCodecs: []string{"opus"}},
			},
			Hls: apigen.HlsProfile{Delivery: apigen.HlsDeliveryMse, VideoCodecs: []string{"h264"}, AudioCodecs: []string{"aac", "mp3"}},
			CodecLimits: &[]apigen.CodecLimit{
				{Codec: "h264", MaxBitDepth: playbackPointer[int32](8), Profiles: &[]string{"Constrained Baseline", "High"}},
				{Codec: "hevc"},
			},
		},
	}
}

// wantFullStartRequest is fullPlaybackRequest as the app receives it.
func wantFullStartRequest() appplayback.StartRequest {
	audioIndex := 1

	return appplayback.StartRequest{
		FileID:            testFileID,
		StartPosition:     600*time.Second + 250*time.Millisecond,
		AudioStreamIndex:  &audioIndex,
		ReplacesSessionID: replacedSessionID,
		Profile: playback.ClientProfile{
			Name: testProfileName,
			DirectPlay: []playback.DirectPlayProfile{
				{Container: media.DirectPlayMP4, VideoCodecs: []string{"h264", "av1"}, AudioCodecs: []string{"aac"}},
				{Container: media.DirectPlayWebM, VideoCodecs: []string{"vp9"}, AudioCodecs: []string{"opus"}},
			},
			HLS: playback.HLSProfile{Delivery: playback.HLSDeliveryMSE, VideoCodecs: []string{"h264"}, AudioCodecs: []string{"aac", "mp3"}},
			CodecLimits: []playback.CodecLimit{
				{Codec: "h264", MaxBitDepth: 8, Profiles: []string{"Constrained Baseline", "High"}},
				{Codec: "hevc"},
			},
		},
	}
}

func marshalPlaybackRequest(t *testing.T, request apigen.PlaybackRequest) string {
	t.Helper()

	body, err := json.Marshal(request)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}

	return string(body)
}

func TestCreatePlaybackSessionPassesTheRequestToTheService(t *testing.T) {
	t.Parallel()

	service := &fakePlaybackService{session: transcodeSession()}
	handler, _ := newPlaybackHandler(t, service)

	got := postSession(t, handler, marshalPlaybackRequest(t, fullPlaybackRequest(t)))
	if got.status != http.StatusCreated {
		t.Fatalf("status = %d, want %d (body %s)", got.status, http.StatusCreated, got.body)
	}

	started := service.startRequests()
	if len(started) != 1 {
		t.Fatalf("Start calls = %d, want 1", len(started))
	}

	if want := wantFullStartRequest(); !reflect.DeepEqual(started[0], want) {
		t.Errorf("start request =\n%+v\nwant\n%+v", started[0], want)
	}
}

func TestCreatePlaybackSessionDefaultsOptionalFields(t *testing.T) {
	t.Parallel()

	service := &fakePlaybackService{session: transcodeSession()}
	handler, _ := newPlaybackHandler(t, service)

	got := postSession(t, handler, playbackRequestBody)
	if got.status != http.StatusCreated {
		t.Fatalf("status = %d, want %d (body %s)", got.status, http.StatusCreated, got.body)
	}

	started := service.startRequests()
	if len(started) != 1 {
		t.Fatalf("Start calls = %d, want 1", len(started))
	}

	request := started[0]
	if request.StartPosition != 0 || request.AudioStreamIndex != nil || request.ReplacesSessionID != "" ||
		request.Profile.CodecLimits != nil {
		t.Errorf("start request = %+v, want no start position, audio index, replaced session or codec limits", request)
	}
}

func TestCreatePlaybackSessionRejectsInvalidRequests(t *testing.T) {
	t.Parallel()

	const tooManyCodecs = 33

	tests := []struct {
		name   string
		change func(request *apigen.PlaybackRequest)
	}{
		{name: "missing file id", change: func(request *apigen.PlaybackRequest) { request.FileID = uuid.Nil() }},
		{name: "negative start position", change: func(request *apigen.PlaybackRequest) { request.StartPositionSeconds = playbackPointer(-0.5) }},
		{name: "start position beyond the duration range", change: func(request *apigen.PlaybackRequest) { request.StartPositionSeconds = playbackPointer(1e300) }},
		{name: "negative audio stream index", change: func(request *apigen.PlaybackRequest) { request.AudioStreamIndex = playbackPointer[int32](-1) }},
		{name: "too many hls video codecs", change: func(request *apigen.PlaybackRequest) {
			request.Profile.Hls.VideoCodecs = strings.Fields(strings.Repeat("h264 ", tooManyCodecs))
		}},
		{name: "upper-case codec", change: func(request *apigen.PlaybackRequest) { request.Profile.Hls.AudioCodecs = []string{"AAC"} }},
		{name: "codec with a space", change: func(request *apigen.PlaybackRequest) { request.Profile.DirectPlay[0].VideoCodecs = []string{"h 264"} }},
		{name: "container with a slash", change: func(request *apigen.PlaybackRequest) { request.Profile.DirectPlay[0].Container = "mp4/x" }},
		{name: "unknown hls delivery", change: func(request *apigen.PlaybackRequest) { request.Profile.Hls.Delivery = "smoke-signals" }},
		{name: "profile name too long", change: func(request *apigen.PlaybackRequest) { request.Profile.Name = strings.Repeat("n", 65) }},
		{name: "profile name with a newline", change: func(request *apigen.PlaybackRequest) { request.Profile.Name = "forged\nlog line" }},
		{name: "zero max bit depth", change: func(request *apigen.PlaybackRequest) {
			(*request.Profile.CodecLimits)[0].MaxBitDepth = playbackPointer[int32](0)
		}},
		{name: "max bit depth above 16", change: func(request *apigen.PlaybackRequest) {
			(*request.Profile.CodecLimits)[0].MaxBitDepth = playbackPointer[int32](17)
		}},
		{name: "too many codec limits", change: func(request *apigen.PlaybackRequest) {
			limits := make([]apigen.CodecLimit, tooManyCodecs)
			for index := range limits {
				limits[index] = apigen.CodecLimit{Codec: "h264"}
			}

			request.Profile.CodecLimits = &limits
		}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			service := &fakePlaybackService{session: transcodeSession()}
			handler, _ := newPlaybackHandler(t, service)

			request := fullPlaybackRequest(t)
			test.change(&request)

			assertProblem(t, postSession(t, handler, marshalPlaybackRequest(t, request)), http.StatusBadRequest, restapi.CodeInvalid)

			if started := service.startRequests(); len(started) != 0 {
				t.Errorf("Start was called for an invalid request: %+v", started)
			}
		})
	}
}

// TestCreatePlaybackSessionToleratesMissingProfileArraysOnly: the decoder does not enforce
// "required". Missing arrays inside the profile read as empty lists (they can only take
// formats away), but a missing profile has no name and no delivery and is rejected.
func TestCreatePlaybackSessionToleratesMissingProfileArraysOnly(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		body       string
		wantStatus int
	}{
		{name: "profile missing", body: `{"fileId":"` + testFileID + `"}`, wantStatus: http.StatusBadRequest},
		{
			name:       "profile arrays missing",
			body:       `{"fileId":"` + testFileID + `","profile":{"name":"bare","hls":{"delivery":"none"}}}`,
			wantStatus: http.StatusCreated,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			service := &fakePlaybackService{session: transcodeSession()}
			handler, _ := newPlaybackHandler(t, service)

			got := postSession(t, handler, test.body)
			if got.status != test.wantStatus {
				t.Fatalf("status = %d, want %d (body %s)", got.status, test.wantStatus, got.body)
			}

			if test.wantStatus == http.StatusBadRequest {
				assertProblem(t, got, http.StatusBadRequest, restapi.CodeInvalid)

				return
			}

			started := service.startRequests()
			if len(started) != 1 || len(started[0].Profile.DirectPlay) != 0 || len(started[0].Profile.HLS.VideoCodecs) != 0 {
				t.Errorf("Start requests = %+v, want one with empty lists", started)
			}
		})
	}
}
