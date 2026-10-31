package playback_test

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	appplayback "github.com/magpeek/mivo/internal/app/playback"
	"github.com/magpeek/mivo/internal/domain/media"
	"github.com/magpeek/mivo/internal/domain/playback"
	"github.com/magpeek/mivo/internal/domain/playback/playbacktest"
)

const (
	libraryRoot   = "/library"
	fileDuration  = 60 * time.Second
	maxTranscodes = 2
	// cappedFFmpegSessions mirrors appplayback.MaxFFmpegSessions so a change of the cap is a
	// visible decision, not a silent test adjustment.
	cappedFFmpegSessions = 8
	// cappedSessions mirrors appplayback.MaxSessions, the cap of all sessions.
	cappedSessions = 256
)

// File IDs of the catalogue every test environment starts with.
const (
	directFileID           = "f1000000-0000-4000-8000-000000000001"
	remuxFileID            = "f1000000-0000-4000-8000-000000000002"
	transcodeFileID        = "f1000000-0000-4000-8000-000000000003"
	audioTranscodeFileID   = "f1000000-0000-4000-8000-000000000004"
	audioOnlyFileID        = "f1000000-0000-4000-8000-000000000005"
	probeFailedFileID      = "f1000000-0000-4000-8000-000000000006"
	unknownFileID          = "f1000000-0000-4000-8000-0000000000ff"
	unknownSessionID       = "5d4c3b2a-1f0e-4d9c-8b7a-6f5e4d3c2b1a"
	directRelativePath     = "movies/direct.mp4"
	remuxRelativePath      = "movies/remux.mkv"
	transcodeRelativePath  = "movies/transcode.avi"
	audioTranscodeRelative = "movies/dts.mkv"
	audioOnlyRelativePath  = "music/concert.mkv"
)

var errBoom = errors.New("boom")

func h264Video(index int) media.Stream {
	return media.Stream{
		Index: index, Kind: media.StreamKindVideo, Codec: "h264", Profile: "High",
		PixelFormat: "yuv420p", BitDepth: 8, Width: 1920, Height: 1080, Default: true,
	}
}

func audio(index int, codec string) media.Stream {
	return media.Stream{Index: index, Kind: media.StreamKindAudio, Codec: codec, Channels: 2, Default: true}
}

func catalogFile(id, relativePath string, container media.Container, streams ...media.Stream) appplayback.PlayableFile {
	return appplayback.PlayableFile{
		LibraryRoot: libraryRoot,
		File: media.File{
			ID: id, RelativePath: relativePath, Size: 1 << 20,
			Description: media.Description{Container: container, Duration: fileDuration, Streams: streams},
		},
	}
}

// catalog holds one file per mode the Chromium reference profile ends up with.
func catalog() map[string]appplayback.PlayableFile {
	mpeg4 := media.Stream{
		Index: 0, Kind: media.StreamKindVideo, Codec: "mpeg4", Profile: "Simple Profile",
		PixelFormat: "yuv420p", BitDepth: 8, Width: 720, Height: 404,
	}

	return map[string]appplayback.PlayableFile{
		directFileID:         catalogFile(directFileID, directRelativePath, media.ContainerMP4, h264Video(0), audio(1, "aac")),
		remuxFileID:          catalogFile(remuxFileID, remuxRelativePath, media.ContainerMKV, h264Video(0), audio(1, "aac")),
		transcodeFileID:      catalogFile(transcodeFileID, transcodeRelativePath, media.ContainerAVI, mpeg4, audio(1, "mp3")),
		audioTranscodeFileID: catalogFile(audioTranscodeFileID, audioTranscodeRelative, media.ContainerMKV, h264Video(0), audio(1, "dts")),
		audioOnlyFileID:      catalogFile(audioOnlyFileID, audioOnlyRelativePath, media.ContainerMKV, audio(0, "opus")),
		probeFailedFileID: {
			LibraryRoot: libraryRoot,
			File:        media.File{ID: probeFailedFileID, RelativePath: "broken.mkv", ProbeError: media.ProbeErrorFailed},
		},
	}
}

// environment bundles the fakes behind one Service.
type environment struct {
	files     *memoryFiles
	resolver  *joiningResolver
	keyframes *scriptedKeyframes
	producer  *fakeProducer
	workspace *memoryWorkspace
	ids       *sequenceIDs
	clock     *manualClock
	logs      *lockedBuffer     // text output at the default level (info)
	records   *capturingHandler // every record, debug included
	options   appplayback.Options
}

func newEnvironment() *environment {
	return &environment{
		files:     &memoryFiles{files: catalog()},
		resolver:  &joiningResolver{},
		keyframes: &scriptedKeyframes{keyframe: 20 * time.Second},
		producer:  &fakeProducer{},
		workspace: &memoryWorkspace{},
		ids:       &sequenceIDs{},
		clock:     &manualClock{now: time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)},
		logs:      &lockedBuffer{},
		records:   &capturingHandler{},
		options: appplayback.Options{
			Capabilities:         playbacktest.FullCapabilities(),
			Policy:               playback.Policy{MaxTranscodeHeight: 1080},
			MaxTranscodeSessions: maxTranscodes,
		},
	}
}

func (env *environment) dependencies() appplayback.Dependencies {
	return appplayback.Dependencies{
		Files:     env.files,
		Paths:     env.resolver,
		Keyframes: env.keyframes,
		Producer:  env.producer,
		Workspace: env.workspace,
		URLs:      pathURLs{},
		IDs:       env.ids,
		Clock:     env.clock.Now,
		After:     env.clock.After,
		Logger:    slog.New(slog.NewMultiHandler(slog.NewTextHandler(env.logs, nil), env.records)),
	}
}

// service returns a new Service that is closed when the test ends, so its reaper and
// production watchers never outlive the test.
func (env *environment) service(t *testing.T) *appplayback.Service {
	t.Helper()

	service, err := appplayback.NewService(env.dependencies(), env.options)
	if err != nil {
		t.Fatalf("NewService error = %v", err)
	}

	closeAtCleanup(t, service)

	return service
}

func closeAtCleanup(t *testing.T, service *appplayback.Service) {
	t.Helper()

	t.Cleanup(func() {
		// t.Context() is already cancelled when cleanups run.
		if err := service.Close(context.Background()); err != nil {
			t.Errorf("Close at cleanup error = %v", err)
		}
	})
}

// inBubble runs test in a synctest bubble with a fresh environment: synctest.Wait then waits
// until the service's goroutines are idle, and the bubble fails the test when a goroutine
// is still blocked after the cleanups ran (a goroutine leak).
func inBubble(t *testing.T, test func(t *testing.T, env *environment)) {
	t.Helper()

	synctest.Test(t, func(t *testing.T) {
		test(t, newEnvironment())
	})
}

// passTime advances the clock by total in steps of the reaper interval and lets the
// service's goroutines settle after each step, so the reaper checks once per step.
func (env *environment) passTime(total time.Duration) {
	synctest.Wait()

	for passed := time.Duration(0); passed < total; passed += appplayback.IdleCheckInterval {
		env.clock.advance(min(appplayback.IdleCheckInterval, total-passed))
		synctest.Wait()
	}
}

// request asks to play fileID with the Chromium reference profile.
func request(fileID string) appplayback.StartRequest {
	return appplayback.StartRequest{FileID: fileID, Profile: playbacktest.ChromiumDesktop()}
}

func mustStart(t *testing.T, service *appplayback.Service, startRequest appplayback.StartRequest) appplayback.Session {
	t.Helper()

	session, err := service.Start(t.Context(), startRequest)
	if err != nil {
		t.Fatalf("Start(%s) error = %v", startRequest.FileID, err)
	}

	return session
}

// contextWithCancel returns a request context the test cancels itself.
func contextWithCancel(t *testing.T) (context.Context, context.CancelFunc) {
	t.Helper()

	return context.WithCancel(t.Context())
}

func assertWrappedError(t *testing.T, err, wantErr error, wantText string) {
	t.Helper()

	if !errors.Is(err, wantErr) {
		t.Fatalf("error = %v, want it to wrap %v", err, wantErr)
	}

	if !strings.Contains(err.Error(), wantText) {
		t.Errorf("error = %q, want context %q", err, wantText)
	}
}
