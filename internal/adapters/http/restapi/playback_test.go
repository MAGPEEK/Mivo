package restapi_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
	"uuid"

	"github.com/magpeek/mivo/internal/adapters/http/apigen"
	"github.com/magpeek/mivo/internal/adapters/http/restapi"
	appplayback "github.com/magpeek/mivo/internal/app/playback"
	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/media"
	"github.com/magpeek/mivo/internal/domain/playback"
)

// The app service is the production implementation of the port.
var _ restapi.PlaybackService = (*appplayback.Service)(nil)

const (
	testSessionID = "5d4c3b2a-1f0e-4d9c-8b7a-6f5e4d3c2b1a"
	testFileID    = "1c2d3e4f-5a6b-4c7d-8e9f-0a1b2c3d4e5f"
	sessionsPath  = "/api/v1/playback/sessions"
	sessionPath   = sessionsPath + "/" + testSessionID

	// testStreamURL is deliberately not shaped like a stream route: the handler must pass
	// the app's URL through untouched instead of building one.
	testStreamURL = "/opaque/stream?token=a%2Fb&x=1"

	// playbackRequestBody is a minimal well-formed PlaybackRequest.
	playbackRequestBody = `{"fileId":"` + testFileID + `",` +
		`"profile":{"name":"test","directPlay":[],"hls":{"delivery":"none","videoCodecs":[],"audioCodecs":[]}}}`

	// testProfileName must never reach an info log line.
	testProfileName = "secret-profile-name"
)

// fakePlaybackService is an in-memory PlaybackService that records its calls.
type fakePlaybackService struct {
	mutex    sync.Mutex
	session  appplayback.Session
	startErr error
	getErr   error
	stopErr  error
	started  []appplayback.StartRequest
	got      []string
	stopped  []string
}

func (service *fakePlaybackService) Start(_ context.Context, request appplayback.StartRequest) (appplayback.Session, error) {
	service.mutex.Lock()
	defer service.mutex.Unlock()

	service.started = append(service.started, request)
	if service.startErr != nil {
		return appplayback.Session{}, service.startErr
	}

	return service.session, nil
}

func (service *fakePlaybackService) Get(_ context.Context, id string) (appplayback.Session, error) {
	service.mutex.Lock()
	defer service.mutex.Unlock()

	service.got = append(service.got, id)
	if service.getErr != nil {
		return appplayback.Session{}, service.getErr
	}

	return service.session, nil
}

func (service *fakePlaybackService) Stop(_ context.Context, id string) error {
	service.mutex.Lock()
	defer service.mutex.Unlock()

	service.stopped = append(service.stopped, id)

	return service.stopErr
}

func (service *fakePlaybackService) startRequests() []appplayback.StartRequest {
	service.mutex.Lock()
	defer service.mutex.Unlock()

	return append([]appplayback.StartRequest(nil), service.started...)
}

func (service *fakePlaybackService) stoppedIDs() []string {
	service.mutex.Lock()
	defer service.mutex.Unlock()

	return append([]string(nil), service.stopped...)
}

func (service *fakePlaybackService) gotIDs() []string {
	service.mutex.Lock()
	defer service.mutex.Unlock()

	return append([]string(nil), service.got...)
}

func newPlaybackHandler(t *testing.T, service *fakePlaybackService) (http.Handler, *syncBuffer) {
	t.Helper()

	return newTestHandlerWithOptions(t, restapi.Options{Playback: service})
}

// transcodeSession is a session whose plan uses every field of the contract.
func transcodeSession() appplayback.Session {
	created := time.Date(2026, 10, 30, 18, 0, 0, 0, time.UTC)

	return appplayback.Session{
		ID: testSessionID, FileID: testFileID, State: appplayback.StateStarting,
		StreamURL: testStreamURL, StartPosition: 598500 * time.Millisecond,
		Duration: 5423042 * time.Millisecond, CreatedAt: created, LastAccessedAt: created,
		Plan: playback.Plan{
			Mode: playback.ModeTranscode, Delivery: playback.DeliveryHLS,
			SourceContainer: media.ContainerMKV, TargetContainer: media.ContainerMP4,
			Streams: []playback.StreamDecision{
				{
					Kind: media.StreamKindVideo, SourceIndex: 0, Action: playback.ActionTranscode,
					SourceCodec: "hevc", TargetCodec: "h264", TargetWidth: 1920, TargetHeight: 1080,
					Downscaled: true, ScaleRequired: true, Encoder: playback.EncoderLibx264,
				},
				{
					Kind: media.StreamKindAudio, SourceIndex: 2, Action: playback.ActionCopy,
					SourceCodec: "aac", TargetCodec: "aac", TargetChannels: 6,
				},
			},
			Reasons: []playback.Reason{
				{Code: playback.ReasonContainerNotSupported, Scope: playback.ScopeContainer},
				{Code: playback.ReasonVideoCodecNotSupported, Scope: playback.ScopeVideo},
			},
			Warnings: []playback.Warning{{Code: playback.WarningVideoDownscaled, Scope: playback.ScopeVideo}},
		},
	}
}

// directPlaySession is a direct-play session as the app returns it: nil reasons and
// warnings (M2-09) and an unknown duration.
func directPlaySession() appplayback.Session {
	return appplayback.Session{
		ID: testSessionID, FileID: testFileID, State: appplayback.StateReady, StreamURL: testStreamURL,
		Plan: playback.Plan{
			Mode: playback.ModeDirectPlay, Delivery: playback.DeliveryFile,
			SourceContainer: media.ContainerMP4, TargetContainer: media.ContainerMP4,
			Streams: []playback.StreamDecision{{
				Kind: media.StreamKindVideo, Action: playback.ActionCopy, SourceCodec: "h264", TargetCodec: "h264",
			}},
		},
	}
}

// playbackPointer returns a pointer to value, for the optional fields of the contract.
func playbackPointer[T any](value T) *T {
	return &value
}

func playbackUUID(t *testing.T, text string) uuid.UUID {
	t.Helper()

	id, err := uuid.Parse(text)
	if err != nil {
		t.Fatalf("parse uuid %q: %v", text, err)
	}

	return id
}

// wantTranscodeSession is transcodeSession in the contract's shape.
func wantTranscodeSession(t *testing.T) apigen.PlaybackSession {
	t.Helper()

	return apigen.PlaybackSession{
		ID: playbackUUID(t, testSessionID), FileID: playbackUUID(t, testFileID),
		State: apigen.PlaybackSessionStateStarting, Mode: apigen.PlaybackModeTranscode,
		Delivery: apigen.PlaybackDeliveryHls, StreamURL: testStreamURL,
		StartPositionSeconds: 598.5, DurationSeconds: playbackPointer(5423.042),
		SourceContainer: "mkv", TargetContainer: "mp4",
		Streams: []apigen.StreamDecision{
			{
				Kind: apigen.MediaStreamKindVideo, SourceIndex: 0, Action: apigen.StreamActionTranscode,
				SourceCodec: "hevc", TargetCodec: "h264", TargetWidth: playbackPointer[int32](1920),
				TargetHeight: playbackPointer[int32](1080), Encoder: playbackPointer("libx264"),
			},
			{
				Kind: apigen.MediaStreamKindAudio, SourceIndex: 2, Action: apigen.StreamActionCopy,
				SourceCodec: "aac", TargetCodec: "aac", TargetChannels: playbackPointer[int32](6),
			},
		},
		Reasons: []apigen.PlaybackReason{
			{Code: "container_not_supported", Scope: apigen.PlaybackReasonScopeContainer},
			{Code: "video_codec_not_supported", Scope: apigen.PlaybackReasonScopeVideo},
		},
		Warnings: []apigen.PlaybackReason{{Code: "video_downscaled", Scope: apigen.PlaybackReasonScopeVideo}},
	}
}

func postSession(t *testing.T, handler http.Handler, body string) response {
	t.Helper()

	request := httptest.NewRequestWithContext(t.Context(), http.MethodPost, sessionsPath, strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")

	return serveRequest(t, handler, request)
}

func TestCreatePlaybackSessionAnswersCreated(t *testing.T) {
	t.Parallel()

	service := &fakePlaybackService{session: transcodeSession()}
	handler, _ := newPlaybackHandler(t, service)

	got := postSession(t, handler, playbackRequestBody)

	if got.status != http.StatusCreated {
		t.Fatalf("status = %d, want %d (body %s)", got.status, http.StatusCreated, got.body)
	}

	if contentType := got.header.Get("Content-Type"); contentType != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", contentType)
	}

	if location := got.header.Get("Location"); location != sessionPath {
		t.Errorf("Location = %q, want %q", location, sessionPath)
	}

	assertSessionBody(t, got.body, wantTranscodeSession(t))
}

func TestGetPlaybackSessionAnswersTheSession(t *testing.T) {
	t.Parallel()

	service := &fakePlaybackService{session: transcodeSession()}
	handler, _ := newPlaybackHandler(t, service)

	got := serve(t, handler, http.MethodGet, sessionPath)

	if got.status != http.StatusOK {
		t.Fatalf("status = %d, want %d (body %s)", got.status, http.StatusOK, got.body)
	}

	assertSessionBody(t, got.body, wantTranscodeSession(t))

	if ids := service.gotIDs(); len(ids) != 1 || ids[0] != testSessionID {
		t.Errorf("Get calls = %v, want [%s]", ids, testSessionID)
	}
}

// assertSessionBody compares the decoded body with want; JSON numbers survive the round
// trip exactly for the values used here.
func assertSessionBody(t *testing.T, body []byte, want apigen.PlaybackSession) {
	t.Helper()

	got := decode[apigen.PlaybackSession](t, body)

	gotJSON, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("marshal got: %v", err)
	}

	wantJSON, err := json.Marshal(want)
	if err != nil {
		t.Fatalf("marshal want: %v", err)
	}

	if string(gotJSON) != string(wantJSON) {
		t.Errorf("session =\n%s\nwant\n%s", gotJSON, wantJSON)
	}
}

func TestGetPlaybackSessionErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		err        error
		wantStatus int
		wantCode   string
	}{
		{
			name: "unknown or stopped session", err: fmt.Errorf("playback session %s: %w", testSessionID, domain.ErrNotFound),
			wantStatus: http.StatusNotFound, wantCode: restapi.CodeNotFound,
		},
		{name: "service failure", err: errors.New("registry broken"), wantStatus: http.StatusInternalServerError, wantCode: restapi.CodeInternal},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			handler, _ := newPlaybackHandler(t, &fakePlaybackService{getErr: test.err})
			assertProblem(t, serve(t, handler, http.MethodGet, sessionPath), test.wantStatus, test.wantCode)
		})
	}
}

func TestStopPlaybackSessionAnswersNoContent(t *testing.T) {
	t.Parallel()

	// The fake stops every ID without an error, like the app for unknown sessions: DELETE is
	// idempotent.
	service := &fakePlaybackService{}
	handler, logs := newPlaybackHandler(t, service)

	for range 2 {
		got := serve(t, handler, http.MethodDelete, sessionPath)
		if got.status != http.StatusNoContent || len(got.body) != 0 {
			t.Errorf("status = %d with body %q, want %d without a body", got.status, got.body, http.StatusNoContent)
		}
	}

	if stopped := service.stoppedIDs(); len(stopped) != 2 || stopped[0] != testSessionID {
		t.Errorf("Stop calls = %v, want the session twice", stopped)
	}

	assertLogged(t, logs.String(), `level=INFO msg="playback session stopped" session=`+testSessionID)
}

func TestStopPlaybackSessionMapsServiceErrors(t *testing.T) {
	t.Parallel()

	// Stop succeeds today; the error path is there for the owner check of milestone 7.
	service := &fakePlaybackService{stopErr: fmt.Errorf("stop session: %w", domain.ErrForbidden)}
	handler, logs := newPlaybackHandler(t, service)

	assertProblem(t, serve(t, handler, http.MethodDelete, sessionPath), http.StatusForbidden, restapi.CodeForbidden)

	if strings.Contains(logs.String(), "playback session stopped") {
		t.Errorf("a failed stop was logged as stopped: %s", logs.String())
	}
}

// TestPlaybackOperationsAnswerNotImplemented covers a nil service, which keeps answering
// 501 until M2-32 makes the service mandatory.
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
		{name: "get session", method: http.MethodGet, target: sessionPath},
		{name: "stop session", method: http.MethodDelete, target: sessionPath},
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

	handler, _ := newPlaybackHandler(t, &fakePlaybackService{session: transcodeSession()})

	for _, target := range []string{sessionPath + "/file", sessionPath + "/hls/index.m3u8"} {
		t.Run(target, func(t *testing.T) {
			t.Parallel()

			assertProblem(t, serve(t, handler, http.MethodGet, target), http.StatusNotFound, restapi.CodeNotFound)
		})
	}
}

func TestCreatePlaybackSessionRejectsMalformedBodies(t *testing.T) {
	t.Parallel()

	service := &fakePlaybackService{session: transcodeSession()}
	handler, logs := newPlaybackHandler(t, service)

	for name, body := range map[string]string{
		"not json":       "fileId=1",
		"wrong type":     `{"fileId":42}`,
		"malformed uuid": `{"fileId":"not-a-uuid"}`,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got := postSession(t, handler, body)
			assertProblem(t, got, http.StatusBadRequest, restapi.CodeBadRequest)
			assertNoInternalText(t, got, body)
		})
	}

	t.Cleanup(func() {
		assertLogged(t, logs.String(), "rejected malformed request")

		if started := service.startRequests(); len(started) != 0 {
			t.Errorf("Start was called for a malformed body: %+v", started)
		}
	})
}
