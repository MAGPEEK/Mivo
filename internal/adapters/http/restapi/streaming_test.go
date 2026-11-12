package restapi_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/magpeek/mivo/internal/adapters/http/apigen"
	"github.com/magpeek/mivo/internal/adapters/http/restapi"
	appplayback "github.com/magpeek/mivo/internal/app/playback"
	"github.com/magpeek/mivo/internal/app/transcoder"
	"github.com/magpeek/mivo/internal/app/trickplay"
	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/playback"
)

// The app services are the production implementations of the ports.
var (
	_ restapi.SessionLister       = (*appplayback.Service)(nil)
	_ restapi.TranscoderService   = (*transcoder.Service)(nil)
	_ restapi.TrickplayService    = (*trickplay.Service)(nil)
	_ restapi.SubtitleTrackLister = appplayback.SubtitleTracks(nil)
)

const (
	testItemTitle   = "Big Buck Bunny"
	sessionListPath = "/api/v1/playback/sessions"
	transcoderPath  = "/api/v1/system/transcoder"
	trickplayPath   = "/api/v1/media-files/" + testFileID + "/trickplay"
)

type fakeSessions struct {
	infos []appplayback.SessionInfo
	err   error
}

func (fake *fakeSessions) List(context.Context) ([]appplayback.SessionInfo, error) {
	return fake.infos, fake.err
}

type fakeSessionItems struct {
	items map[string]restapi.SessionItem
	err   error
}

func (fake *fakeSessionItems) ItemOfFile(_ context.Context, fileID string) (restapi.SessionItem, error) {
	if fake.err != nil {
		return restapi.SessionItem{}, fake.err
	}

	item, found := fake.items[fileID]
	if !found {
		return restapi.SessionItem{}, fmt.Errorf("file %s: %w", fileID, domain.ErrNotFound)
	}

	return item, nil
}

type fakeTranscoder struct {
	mutex   sync.Mutex
	status  transcoder.Status
	testErr error
	// hold blocks Test until it is closed, like a detection that runs for seconds.
	hold    chan struct{}
	started chan struct{}
	tests   int
}

func (fake *fakeTranscoder) Status() transcoder.Status {
	fake.mutex.Lock()
	defer fake.mutex.Unlock()

	return fake.status
}

func (fake *fakeTranscoder) Test(context.Context) (transcoder.Status, error) {
	fake.mutex.Lock()
	fake.tests++
	err := fake.testErr

	if err == nil {
		fake.status.Detecting = true
	}

	fake.mutex.Unlock()

	if err != nil {
		return transcoder.Status{}, err
	}

	if fake.started != nil {
		close(fake.started)
	}

	if fake.hold != nil {
		<-fake.hold
	}

	fake.mutex.Lock()
	defer fake.mutex.Unlock()

	fake.status.Detecting = false

	return fake.status, nil
}

type fakeTrickplay struct {
	mutex       sync.Mutex
	manifest    trickplay.Manifest
	manifestErr error
	requestErr  error
	requested   []string
}

func (fake *fakeTrickplay) Manifest(_ context.Context, _ string) (trickplay.Manifest, error) {
	return fake.manifest, fake.manifestErr
}

func (fake *fakeTrickplay) Request(_ context.Context, fileID string) error {
	fake.mutex.Lock()
	defer fake.mutex.Unlock()

	fake.requested = append(fake.requested, fileID)

	return fake.requestErr
}

func streamingHandler(t *testing.T, services restapi.StreamingServices) (http.Handler, *syncBuffer) {
	t.Helper()

	return newTestHandlerWithOptions(t, restapi.Options{Streaming: services})
}

func do(t *testing.T, handler http.Handler, method, target string) response {
	t.Helper()

	request := httptest.NewRequestWithContext(t.Context(), method, target, strings.NewReader(""))

	return serveRequest(t, handler, request)
}

func TestSessionListMapsTheOperatorView(t *testing.T) {
	t.Parallel()

	started := time.Date(2026, 11, 11, 19, 30, 0, 0, time.UTC)
	service := &fakeSessions{infos: []appplayback.SessionInfo{
		{
			ID: testSessionID, FileID: testFileID, State: appplayback.StateReady, Mode: playback.ModeTranscode,
			Encoder: "h264_vaapi", Hardware: true, OutputBitRate: 8_000_000, Speed: 2.4, Paused: true,
			ClientName: "chromium-desktop", StartedAt: started,
		},
		{
			ID: "6e5d4c3b-2a1f-4e0d-9c8b-7a6f5e4d3c2b", FileID: testFileID, State: appplayback.StateReady,
			Mode: playback.ModeDirectPlay, ClientName: "tv", StartedAt: started,
		},
	}}
	items := &fakeSessionItems{items: map[string]restapi.SessionItem{testFileID: {ID: testItemID, Title: testItemTitle}}}
	handler, _ := streamingHandler(t, restapi.StreamingServices{Sessions: service, SessionItems: items})

	got := serve(t, handler, http.MethodGet, sessionListPath)
	if got.status != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body %s)", got.status, got.body)
	}

	list := decode[apigen.PlaybackSessionList](t, got.body)
	if len(list.Sessions) != 2 {
		t.Fatalf("sessions = %d, want 2", len(list.Sessions))
	}

	first := list.Sessions[0]
	if first.ID.String() != testSessionID || first.ItemID.String() != testItemID || first.Title != testItemTitle ||
		first.Mode != apigen.PlaybackModeTranscode || first.State != apigen.PlaybackSessionStateReady ||
		first.ClientName != "chromium-desktop" || !first.Hardware || !first.StartedAt.Equal(started) {
		t.Errorf("first = %+v", first)
	}

	if first.VideoEncoder == nil || *first.VideoEncoder != "h264_vaapi" || first.OutputBitRate == nil ||
		*first.OutputBitRate != 8_000_000 || first.TranscodeSpeed == nil || *first.TranscodeSpeed != 2.4 ||
		first.Paused == nil || !*first.Paused {
		t.Errorf("first optional fields = %+v", first)
	}

	direct := rawSession(t, serve(t, handler, http.MethodGet, sessionListPath).body)
	if !strings.Contains(string(direct["sessions"]), `"id":"6e5d4c3b`) {
		t.Errorf("sessions = %s", direct["sessions"])
	}

	second := list.Sessions[1]
	if second.Paused != nil || second.VideoEncoder != nil || second.TranscodeSpeed != nil || second.OutputBitRate != nil {
		t.Errorf("direct play carries transcoding fields: %+v", second)
	}
}

func TestSessionListIsEmptyArrayAndNeverCarriesPaths(t *testing.T) {
	t.Parallel()

	handler, _ := streamingHandler(t, restapi.StreamingServices{Sessions: &fakeSessions{}, SessionItems: &fakeSessionItems{}})

	got := serve(t, handler, http.MethodGet, sessionListPath)
	if got.status != http.StatusOK || strings.TrimSpace(string(got.body)) != `{"sessions":[]}` {
		t.Errorf("empty list = %d %s", got.status, got.body)
	}

	items := &fakeSessionItems{items: map[string]restapi.SessionItem{testFileID: {ID: testItemID, Title: testItemTitle}}}
	infos := []appplayback.SessionInfo{{ID: testSessionID, FileID: testFileID, State: appplayback.StateReady, Mode: playback.ModeRemux}}
	handler, _ = streamingHandler(t, restapi.StreamingServices{Sessions: &fakeSessions{infos: infos}, SessionItems: items})

	body := string(serve(t, handler, http.MethodGet, sessionListPath).body)
	for _, forbidden := range []string{"streamUrl", "fileName", "/media", "\\\\", "path", testFileID} {
		if strings.Contains(body, forbidden) {
			t.Errorf("body %s contains %q", body, forbidden)
		}
	}
}

func TestSessionListKeepsASessionWhoseItemIsGone(t *testing.T) {
	t.Parallel()

	infos := []appplayback.SessionInfo{{ID: testSessionID, FileID: testFileID, State: appplayback.StateReady, Mode: playback.ModeRemux}}
	handler, _ := streamingHandler(t, restapi.StreamingServices{Sessions: &fakeSessions{infos: infos}, SessionItems: &fakeSessionItems{}})

	got := serve(t, handler, http.MethodGet, sessionListPath)
	list := decode[apigen.PlaybackSessionList](t, got.body)

	if got.status != http.StatusOK || len(list.Sessions) != 1 || list.Sessions[0].Title != "" {
		t.Errorf("got %d %s, want the session with an empty title", got.status, got.body)
	}
}

func TestSessionListErrors(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		sessions   *fakeSessions
		items      *fakeSessionItems
		wantStatus int
		wantCode   string
	}{
		"unauthenticated": {&fakeSessions{err: fmt.Errorf("viewer: %w", domain.ErrUnauthenticated)}, &fakeSessionItems{}, http.StatusUnauthorized, "unauthenticated"},
		"list failure":    {&fakeSessions{err: errors.New("boom /srv/media")}, &fakeSessionItems{}, http.StatusInternalServerError, "internal_error"},
		"item lookup": {
			&fakeSessions{infos: []appplayback.SessionInfo{{ID: testSessionID, FileID: testFileID, Mode: playback.ModeRemux}}},
			&fakeSessionItems{err: errors.New("database is locked")}, http.StatusInternalServerError, "internal_error",
		},
		"bad session id": {
			&fakeSessions{infos: []appplayback.SessionInfo{{ID: "nope", FileID: testFileID, Mode: playback.ModeRemux}}},
			&fakeSessionItems{items: map[string]restapi.SessionItem{testFileID: {ID: testItemID}}}, http.StatusInternalServerError, "internal_error",
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			handler, _ := streamingHandler(t, restapi.StreamingServices{Sessions: test.sessions, SessionItems: test.items})

			got := serve(t, handler, http.MethodGet, sessionListPath)
			assertProblem(t, got, test.wantStatus, test.wantCode)

			if strings.Contains(string(got.body), "/srv/media") {
				t.Errorf("body leaks the error text: %s", got.body)
			}
		})
	}
}

func TestStreamingOperationsAnswerNotImplementedWithoutServices(t *testing.T) {
	t.Parallel()

	handler, _ := newTestHandlerWithOptions(t, restapi.Options{})

	for _, test := range []struct{ method, target string }{
		{http.MethodGet, sessionListPath},
		{http.MethodGet, transcoderPath},
		{http.MethodPost, transcoderPath + "/test"},
		{http.MethodGet, trickplayPath},
		{http.MethodPost, trickplayPath},
	} {
		assertProblem(t, do(t, handler, test.method, test.target), http.StatusNotImplemented, "not_implemented")
	}
}

func TestTranscoderStatusMapsDevicesAndHidesPaths(t *testing.T) {
	t.Parallel()

	tested := time.Date(2026, 11, 11, 19, 30, 0, 0, time.UTC)
	fake := &fakeTranscoder{status: transcoder.Status{
		Setting: playback.HardwareAccelerationAuto, ThrottleSupported: true, LastTestedAt: tested,
		Devices: []transcoder.Device{
			{Encoder: playback.HardwareEncoder{API: playback.HardwareAPIVAAPI, Name: "h264_vaapi", Codec: "h264", Device: "/dev/dri/renderD128", Usable: true}},
			{
				Encoder: playback.HardwareEncoder{API: playback.HardwareAPINVENC, Name: "hevc_nvenc", Codec: "hevc", Degraded: true, Usable: true},
				Message: "A session failed with this encoder.",
			},
			{
				Encoder: playback.HardwareEncoder{API: playback.HardwareAPIQSV, Name: "h264_qsv", Codec: "h264", Device: "/dev/dri/renderD129"},
				Message: "open /dev/dri/renderD129: permission denied",
			},
		},
	}}
	handler, _ := streamingHandler(t, restapi.StreamingServices{Transcoder: fake})

	got := serve(t, handler, http.MethodGet, transcoderPath)
	if got.status != http.StatusOK {
		t.Fatalf("status = %d (body %s)", got.status, got.body)
	}

	status := decode[apigen.TranscoderStatus](t, got.body)
	if status.Hwaccel != apigen.HardwareAccelerationSettingAuto || !status.ThrottleSupported ||
		status.LastTestedAt == nil || !status.LastTestedAt.Equal(tested) || len(status.Devices) != 3 {
		t.Fatalf("status = %+v", status)
	}

	first, second, third := status.Devices[0], status.Devices[1], status.Devices[2]
	if first.API != apigen.HardwareAccelerationAPIVaapi || first.Encoder != "h264_vaapi" || first.Codec != "h264" ||
		!first.Usable || first.Degraded || first.Message != nil {
		t.Errorf("first = %+v", first)
	}

	if !second.Degraded || second.Message == nil || *second.Message != "A session failed with this encoder." {
		t.Errorf("second = %+v", second)
	}

	if third.Usable || third.Message != nil {
		t.Errorf("a message with a path must be dropped: %+v", third)
	}

	if strings.Contains(string(got.body), "/dev/") {
		t.Errorf("body leaks a device path: %s", got.body)
	}
}

func TestTranscoderStatusOfAFreshServer(t *testing.T) {
	t.Parallel()

	fake := &fakeTranscoder{status: transcoder.Status{Setting: playback.HardwareAccelerationOff}}
	handler, _ := streamingHandler(t, restapi.StreamingServices{Transcoder: fake})

	fields := rawSession(t, serve(t, handler, http.MethodGet, transcoderPath).body)

	if string(fields["devices"]) != "[]" || string(fields["hwaccel"]) != `"off"` {
		t.Errorf("devices/hwaccel = %s/%s", fields["devices"], fields["hwaccel"])
	}

	if _, present := fields["lastTestedAt"]; present {
		t.Error("lastTestedAt must be omitted before the first run")
	}

	fake.status.Setting = ""
	if hwaccel := rawSession(t, serve(t, handler, http.MethodGet, transcoderPath).body)["hwaccel"]; string(hwaccel) != `"auto"` {
		t.Errorf("an unset setting is auto, got %s", hwaccel)
	}
}

func TestTestTranscoderStartsABackgroundRun(t *testing.T) {
	t.Parallel()

	fake := &fakeTranscoder{hold: make(chan struct{}), started: make(chan struct{})}
	handler, _ := streamingHandler(t, restapi.StreamingServices{Transcoder: fake})

	got := do(t, handler, http.MethodPost, transcoderPath+"/test")
	if got.status != http.StatusAccepted {
		t.Fatalf("status = %d, want 202 (body %s)", got.status, got.body)
	}

	select {
	case <-fake.started:
	case <-time.After(5 * time.Second):
		t.Fatal("the test run never started")
	}

	close(fake.hold)

	fake.mutex.Lock()
	defer fake.mutex.Unlock()

	if fake.tests != 1 {
		t.Errorf("tests = %d, want 1", fake.tests)
	}
}

func TestTestTranscoderMapsRefusals(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		err        error
		wantStatus int
		wantCode   string
	}{
		"busy":        {transcoder.ErrBusy, http.StatusConflict, "conflict"},
		"off":         {transcoder.ErrDisabled, http.StatusBadRequest, "invalid"},
		"shutting":    {fmt.Errorf("closed: %w", domain.ErrUnavailable), http.StatusServiceUnavailable, "unavailable"},
		"detect fail": {errors.New("test hardware acceleration: exec /usr/bin/ffmpeg: boom"), http.StatusInternalServerError, "internal_error"},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			handler, _ := streamingHandler(t, restapi.StreamingServices{Transcoder: &fakeTranscoder{testErr: test.err}})

			got := do(t, handler, http.MethodPost, transcoderPath+"/test")
			assertProblem(t, got, test.wantStatus, test.wantCode)

			if strings.Contains(string(got.body), "ffmpeg") {
				t.Errorf("body leaks the error text: %s", got.body)
			}
		})
	}
}

func TestTrickplayManifestMapsTheLayout(t *testing.T) {
	t.Parallel()

	fake := &fakeTrickplay{manifest: trickplay.Manifest{
		TileWidth: 160, TileHeight: 90, Columns: 10, Rows: 10, Interval: 10 * time.Second, SheetCount: 3,
	}}
	handler, _ := streamingHandler(t, restapi.StreamingServices{Trickplay: fake})

	got := serve(t, handler, http.MethodGet, trickplayPath)
	if got.status != http.StatusOK {
		t.Fatalf("status = %d (body %s)", got.status, got.body)
	}

	manifest := decode[apigen.TrickplayManifest](t, got.body)
	wantURLs := []string{
		"/api/v1/media-files/" + testFileID + "/trickplay/0.jpg",
		"/api/v1/media-files/" + testFileID + "/trickplay/1.jpg",
		"/api/v1/media-files/" + testFileID + "/trickplay/2.jpg",
	}

	if manifest.TileWidth != 160 || manifest.TileHeight != 90 || manifest.Columns != 10 || manifest.Rows != 10 ||
		manifest.IntervalSeconds != 10 || manifest.SheetCount != 3 || strings.Join(manifest.SheetUrls, ",") != strings.Join(wantURLs, ",") {
		t.Errorf("manifest = %+v", manifest)
	}
}

func TestTrickplayErrors(t *testing.T) {
	t.Parallel()

	notFound := fmt.Errorf("trickplay: %w", domain.ErrNotFound)
	tests := map[string]struct {
		method     string
		fake       *fakeTrickplay
		wantStatus int
		wantCode   string
	}{
		"manifest unknown or hidden":  {http.MethodGet, &fakeTrickplay{manifestErr: notFound}, http.StatusNotFound, "not_found"},
		"manifest no profile":         {http.MethodGet, &fakeTrickplay{manifestErr: fmt.Errorf("x: %w", domain.ErrForbidden)}, http.StatusForbidden, "forbidden"},
		"manifest unauthenticated":    {http.MethodGet, &fakeTrickplay{manifestErr: fmt.Errorf("x: %w", domain.ErrUnauthenticated)}, http.StatusUnauthorized, "unauthenticated"},
		"manifest store broken":       {http.MethodGet, &fakeTrickplay{manifestErr: errors.New("read /cache/trickplay/x.json")}, http.StatusInternalServerError, "internal_error"},
		"request unknown or hidden":   {http.MethodPost, &fakeTrickplay{requestErr: notFound}, http.StatusNotFound, "not_found"},
		"request cannot have preview": {http.MethodPost, &fakeTrickplay{requestErr: fmt.Errorf("no video: %w", domain.ErrUnsupported)}, http.StatusUnprocessableEntity, "unsupported"},
		"request shutting down":       {http.MethodPost, &fakeTrickplay{requestErr: fmt.Errorf("closed: %w", domain.ErrUnavailable)}, http.StatusServiceUnavailable, "unavailable"},
		"request broken":              {http.MethodPost, &fakeTrickplay{requestErr: errors.New("queue /cache boom")}, http.StatusInternalServerError, "internal_error"},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			handler, _ := streamingHandler(t, restapi.StreamingServices{Trickplay: test.fake})

			got := do(t, handler, test.method, trickplayPath)
			assertProblem(t, got, test.wantStatus, test.wantCode)

			if strings.Contains(string(got.body), "/cache") {
				t.Errorf("body leaks the error text: %s", got.body)
			}
		})
	}
}

func TestRequestTrickplayAnswersAcceptedAndPassesTheFileID(t *testing.T) {
	t.Parallel()

	fake := &fakeTrickplay{}
	handler, _ := streamingHandler(t, restapi.StreamingServices{Trickplay: fake})

	got := do(t, handler, http.MethodPost, trickplayPath)
	if got.status != http.StatusAccepted || len(got.body) != 0 {
		t.Errorf("got %d %q, want an empty 202", got.status, got.body)
	}

	if len(fake.requested) != 1 || fake.requested[0] != testFileID {
		t.Errorf("requested = %v, want [%s]", fake.requested, testFileID)
	}
}

func TestSubtitleModeIsReportedBySessions(t *testing.T) {
	t.Parallel()

	session := transcodeSession()
	session.SubtitleMode = playback.SubtitleBurnIn
	session.SubtitleTrackID = "s:3"
	session.SourceBitRate = 4_073_000
	session.OutputBitRate = 8_000_000

	handler, _ := newPlaybackHandler(t, &fakePlaybackService{session: session})

	fields := rawSession(t, serve(t, handler, http.MethodGet, sessionPath).body)

	if string(fields["subtitle"]) != `{"mode":"burn_in","trackId":"s:3"}` ||
		string(fields["sourceBitRate"]) != "4073000" || string(fields["outputBitRate"]) != "8000000" {
		t.Errorf("subtitle/source/output = %s/%s/%s", fields["subtitle"], fields["sourceBitRate"], fields["outputBitRate"])
	}

	session.SubtitleMode = playback.SubtitleOff
	session.SubtitleTrackID = ""
	handler, _ = newPlaybackHandler(t, &fakePlaybackService{session: session})

	if subtitle := rawSession(t, serve(t, handler, http.MethodGet, sessionPath).body)["subtitle"]; string(subtitle) != `{"mode":"off"}` {
		t.Errorf("subtitle = %s, want {\"mode\":\"off\"}", subtitle)
	}

	handler, _ = newPlaybackHandler(t, &fakePlaybackService{session: directPlaySession()})

	if _, present := rawSession(t, serve(t, handler, http.MethodGet, sessionPath).body)["subtitle"]; present {
		t.Error("a session without a subtitle decision must omit subtitle")
	}
}

func TestPlaybackRequestMapsSubtitleAndBitRate(t *testing.T) {
	t.Parallel()

	service := &fakePlaybackService{session: transcodeSession()}
	handler, _ := newPlaybackHandler(t, service)

	body := `{"fileId":"` + testFileID + `","subtitleTrackId":"s:3","maxBitRate":8000000,"allowDegradeSubtitles":true,` +
		`"profile":{"name":"test","directPlay":[],"hls":{"delivery":"none","videoCodecs":[],"audioCodecs":[]}}}`
	if got := postSession(t, handler, body); got.status != http.StatusCreated {
		t.Fatalf("status = %d (body %s)", got.status, got.body)
	}

	request := service.startRequests()[0]
	if request.SubtitleTrackID == nil || *request.SubtitleTrackID != "s:3" || request.MaxBitrate != 8_000_000 || !request.AllowDegradeSubtitles {
		t.Errorf("request = %+v", request)
	}

	service = &fakePlaybackService{session: transcodeSession()}
	handler, _ = newPlaybackHandler(t, service)
	empty := strings.Replace(body, `"subtitleTrackId":"s:3"`, `"subtitleTrackId":""`, 1)
	postSession(t, handler, empty)

	if track := service.startRequests()[0].SubtitleTrackID; track == nil || *track != "" {
		t.Errorf("an empty subtitleTrackId switches subtitles off, got %v", track)
	}

	service = &fakePlaybackService{session: transcodeSession()}
	handler, _ = newPlaybackHandler(t, service)
	postSession(t, handler, playbackRequestBody)

	if request := service.startRequests()[0]; request.SubtitleTrackID != nil || request.MaxBitrate != 0 || request.AllowDegradeSubtitles {
		t.Errorf("omitted fields must stay zero: %+v", request)
	}
}

func TestPlaybackRequestRejectsANegativeBitRate(t *testing.T) {
	t.Parallel()

	handler, _ := newPlaybackHandler(t, &fakePlaybackService{session: transcodeSession()})

	body := strings.Replace(playbackRequestBody, `"profile"`, `"maxBitRate":-1,"profile"`, 1)
	assertProblem(t, postSession(t, handler, body), http.StatusBadRequest, "invalid")
}
