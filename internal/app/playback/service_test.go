package playback_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	appplayback "github.com/magpeek/mivo/internal/app/playback"
	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/playback"
)

func TestMaxFFmpegSessionsFollowsOD4(t *testing.T) {
	t.Parallel()

	if appplayback.MaxFFmpegSessions != cappedFFmpegSessions {
		t.Errorf("MaxFFmpegSessions = %d, want %d (OD-4 A)", appplayback.MaxFFmpegSessions, cappedFFmpegSessions)
	}

	if appplayback.MaxSessions != cappedSessions {
		t.Errorf("MaxSessions = %d, want %d", appplayback.MaxSessions, cappedSessions)
	}
}

func TestNewServiceRejectsMissingDependencies(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		mutate   func(*appplayback.Dependencies)
		wantText string
	}{
		{name: "files", mutate: func(d *appplayback.Dependencies) { d.Files = nil }, wantText: "file source"},
		{name: "paths", mutate: func(d *appplayback.Dependencies) { d.Paths = nil }, wantText: "path resolver"},
		{name: "keyframes", mutate: func(d *appplayback.Dependencies) { d.Keyframes = nil }, wantText: "keyframe locator"},
		{name: "producer", mutate: func(d *appplayback.Dependencies) { d.Producer = nil }, wantText: "stream producer"},
		{name: "workspace", mutate: func(d *appplayback.Dependencies) { d.Workspace = nil }, wantText: "workspace"},
		{name: "urls", mutate: func(d *appplayback.Dependencies) { d.URLs = nil }, wantText: "stream url builder"},
		{name: "ids", mutate: func(d *appplayback.Dependencies) { d.IDs = nil }, wantText: "id generator"},
		{name: "clock", mutate: func(d *appplayback.Dependencies) { d.Clock = nil }, wantText: "clock"},
		{name: "logger", mutate: func(d *appplayback.Dependencies) { d.Logger = nil }, wantText: "logger"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			env := newEnvironment()
			dependencies := env.dependencies()
			test.mutate(&dependencies)

			service, err := appplayback.NewService(dependencies, env.options)
			assertWrappedError(t, err, domain.ErrInvalid, test.wantText)

			if service != nil {
				t.Error("NewService returned a service together with an error")
			}
		})
	}
}

func TestNewServiceReportsEveryMissingDependency(t *testing.T) {
	t.Parallel()

	_, err := appplayback.NewService(appplayback.Dependencies{}, newEnvironment().options)
	if err == nil {
		t.Fatal("NewService(empty) error = nil, want error")
	}

	for _, name := range []string{
		"file source", "path resolver", "keyframe locator", "stream producer", "workspace",
		"stream url builder", "id generator", "clock", "logger",
	} {
		if !strings.Contains(err.Error(), name) {
			t.Errorf("NewService(empty) error = %q, want it to name %q", err, name)
		}
	}
}

func TestNewServiceRejectsInvalidOptions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		mutate   func(*appplayback.Options)
		wantText string
	}{
		{name: "odd max height", mutate: func(o *appplayback.Options) { o.Policy.MaxTranscodeHeight = 1081 }, wantText: "max transcode height"},
		{name: "negative max height", mutate: func(o *appplayback.Options) { o.Policy.MaxTranscodeHeight = -2 }, wantText: "max transcode height"},
		{name: "no transcode sessions", mutate: func(o *appplayback.Options) { o.MaxTranscodeSessions = 0 }, wantText: "re-encoding session limit"},
		{name: "above the ffmpeg cap", mutate: func(o *appplayback.Options) { o.MaxTranscodeSessions = cappedFFmpegSessions + 1 }, wantText: "re-encoding session limit"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			env := newEnvironment()
			test.mutate(&env.options)

			service, err := appplayback.NewService(env.dependencies(), env.options)
			assertWrappedError(t, err, domain.ErrInvalid, test.wantText)

			if service != nil {
				t.Error("NewService returned a service together with an error")
			}
		})
	}
}

func TestNewServiceAcceptsTheLimitBounds(t *testing.T) {
	t.Parallel()

	for _, limit := range []int{1, cappedFFmpegSessions} {
		env := newEnvironment()
		env.options.MaxTranscodeSessions = limit
		env.options.Policy.MaxTranscodeHeight = 0

		if _, err := appplayback.NewService(env.dependencies(), env.options); err != nil {
			t.Errorf("NewService(max transcodes %d, no height cap) error = %v", limit, err)
		}
	}
}

func TestGetReturnsTheSession(t *testing.T) {
	t.Parallel()

	env := newEnvironment()
	service := env.service(t)
	started := mustStart(t, service, request(remuxFileID))

	got, err := service.Get(t.Context(), started.ID)
	if err != nil {
		t.Fatalf("Get error = %v", err)
	}

	if got.ID != started.ID || got.Plan.Mode != playback.ModeRemux || got.StreamURL != started.StreamURL {
		t.Errorf("Get = %+v, want the started session %+v", got, started)
	}
}

func TestGetReturnsACopy(t *testing.T) {
	t.Parallel()

	env := newEnvironment()
	service := env.service(t)
	started := mustStart(t, service, request(transcodeFileID))

	started.Plan.Streams[0].TargetCodec = "changed"
	started.Plan.Reasons[0].Code = "changed"

	got, err := service.Get(t.Context(), started.ID)
	if err != nil {
		t.Fatalf("Get error = %v", err)
	}

	if got.Plan.Streams[0].TargetCodec == "changed" || got.Plan.Reasons[0].Code == "changed" {
		t.Errorf("Get = %+v, changing a returned session changed the stored one", got.Plan)
	}
}

func TestGetAndAccessReportUnknownSessions(t *testing.T) {
	t.Parallel()

	service := newEnvironment().service(t)

	_, err := service.Get(t.Context(), unknownSessionID)
	assertWrappedError(t, err, domain.ErrNotFound, unknownSessionID)

	_, err = service.Access(t.Context(), unknownSessionID)
	assertWrappedError(t, err, domain.ErrNotFound, unknownSessionID)
}

func TestStopEndsTheSession(t *testing.T) {
	t.Parallel()

	env := newEnvironment()
	service := env.service(t)
	started := mustStart(t, service, request(remuxFileID))

	if err := service.Stop(t.Context(), started.ID); err != nil {
		t.Fatalf("Stop error = %v", err)
	}

	if stops := env.producer.productionOf(started.ID).stopCount(); stops != 1 {
		t.Errorf("production stops = %d, want 1", stops)
	}

	if removed := env.workspace.removedIDs(); len(removed) != 1 || removed[0] != started.ID {
		t.Errorf("removed workspaces = %v, want [%s]", removed, started.ID)
	}

	if _, err := service.Get(t.Context(), started.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("Get after Stop error = %v, want ErrNotFound", err)
	}
}

func TestStopIsIdempotent(t *testing.T) {
	t.Parallel()

	env := newEnvironment()
	service := env.service(t)
	started := mustStart(t, service, request(remuxFileID))

	for attempt := range 3 {
		if err := service.Stop(t.Context(), started.ID); err != nil {
			t.Fatalf("Stop #%d error = %v", attempt+1, err)
		}
	}

	if err := service.Stop(t.Context(), unknownSessionID); err != nil {
		t.Errorf("Stop(unknown) error = %v, want nil", err)
	}

	if stops := env.producer.productionOf(started.ID).stopCount(); stops != 1 {
		t.Errorf("production stops = %d, want 1", stops)
	}

	if removed := env.workspace.removedIDs(); len(removed) != 1 {
		t.Errorf("removed workspaces = %v, want one removal", removed)
	}
}

func TestStopOfADirectPlaySessionTouchesNoWorkspace(t *testing.T) {
	t.Parallel()

	env := newEnvironment()
	service := env.service(t)
	started := mustStart(t, service, request(directFileID))

	if err := service.Stop(t.Context(), started.ID); err != nil {
		t.Fatalf("Stop error = %v", err)
	}

	if removed := env.workspace.removedIDs(); len(removed) != 0 {
		t.Errorf("removed workspaces = %v, want none for direct play", removed)
	}
}

func TestStopLogsALeftoverWorkspaceAndSucceeds(t *testing.T) {
	t.Parallel()

	env := newEnvironment()
	env.workspace.removeErr = errors.New("directory locked")
	service := env.service(t)
	started := mustStart(t, service, request(remuxFileID))

	if err := service.Stop(t.Context(), started.ID); err != nil {
		t.Fatalf("Stop error = %v, want nil: the session is gone, only a directory is left", err)
	}

	logs := env.logs.String()
	if !strings.Contains(logs, "level=WARN") || !strings.Contains(logs, "directory locked") ||
		!strings.Contains(logs, started.ID) {
		t.Errorf("logs = %q, want a warning naming the session and the leftover", logs)
	}

	if _, err := service.Get(t.Context(), started.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("Get after a failed removal error = %v, want ErrNotFound (the session is gone)", err)
	}

	if err := service.Stop(t.Context(), started.ID); err != nil {
		t.Errorf("second Stop error = %v, want nil", err)
	}
}

func TestStopRemovesTheWorkspaceEvenWhenTheRequestIsCancelled(t *testing.T) {
	t.Parallel()

	env := newEnvironment()
	service := env.service(t)
	started := mustStart(t, service, request(remuxFileID))

	ctx, cancel := contextWithCancel(t)
	cancel()

	if err := service.Stop(ctx, started.ID); err != nil {
		t.Fatalf("Stop with a cancelled request error = %v", err)
	}

	if removed := env.workspace.removedIDs(); len(removed) != 1 {
		t.Errorf("removed workspaces = %v, want the session's directory", removed)
	}
}

func TestAccessOfADirectPlaySession(t *testing.T) {
	t.Parallel()

	env := newEnvironment()
	service := env.service(t)
	started := mustStart(t, service, request(directFileID))

	access, err := service.Access(t.Context(), started.ID)
	if err != nil {
		t.Fatalf("Access error = %v", err)
	}

	want := appplayback.StreamAccess{
		Delivery: playback.DeliveryFile, LibraryRoot: libraryRoot,
		RelativePath: directRelativePath, ContentType: "video/mp4",
	}
	if access.Delivery != want.Delivery || access.LibraryRoot != want.LibraryRoot ||
		access.RelativePath != want.RelativePath || access.ContentType != want.ContentType ||
		access.Directory != "" {
		t.Errorf("Access = %+v, want %+v", access, want)
	}

	if err := access.WaitReady(t.Context()); err != nil {
		t.Errorf("WaitReady of direct play error = %v, want nil", err)
	}
}

func TestAccessOfAnHLSSessionWaitsForTheProduction(t *testing.T) {
	t.Parallel()

	env := newEnvironment()
	service := env.service(t)
	started := mustStart(t, service, request(remuxFileID))

	access, err := service.Access(t.Context(), started.ID)
	if err != nil {
		t.Fatalf("Access error = %v", err)
	}

	if access.Delivery != playback.DeliveryHLS || access.Directory != workspaceDir(started.ID) ||
		access.LibraryRoot != "" || access.RelativePath != "" || access.ContentType != "" {
		t.Errorf("Access = %+v, want HLS with the workspace %s only", access, workspaceDir(started.ID))
	}

	if err := access.WaitReady(t.Context()); err != nil {
		t.Errorf("WaitReady error = %v, want nil", err)
	}

	env.producer.productionOf(started.ID).setReadyErr(errBoom)

	assertWrappedError(t, access.WaitReady(t.Context()), errBoom, "wait for stream of session "+started.ID)
}

func TestWaitReadyAfterStopReportsAStoppedProduction(t *testing.T) {
	t.Parallel()

	env := newEnvironment()
	service := env.service(t)
	started := mustStart(t, service, request(remuxFileID))

	access, err := service.Access(t.Context(), started.ID)
	if err != nil {
		t.Fatalf("Access error = %v", err)
	}

	if err := service.Stop(t.Context(), started.ID); err != nil {
		t.Fatalf("Stop error = %v", err)
	}

	err = access.WaitReady(t.Context())
	assertWrappedError(t, err, appplayback.ErrProductionStopped, started.ID)

	if !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("WaitReady error = %v, want it to wrap domain.ErrNotFound (session gone)", err)
	}
}

func TestAccessRecordsTheAccessTime(t *testing.T) {
	t.Parallel()

	env := newEnvironment()
	service := env.service(t)
	created := env.clock.Now()
	started := mustStart(t, service, request(remuxFileID))

	if !started.CreatedAt.Equal(created) || !started.LastAccessedAt.Equal(created) {
		t.Errorf("new session times = %s / %s, want both %s", started.CreatedAt, started.LastAccessedAt, created)
	}

	env.clock.advance(90 * time.Second)

	if _, err := service.Access(t.Context(), started.ID); err != nil {
		t.Fatalf("Access error = %v", err)
	}

	env.clock.advance(time.Second)

	got, err := service.Get(t.Context(), started.ID)
	if err != nil {
		t.Fatalf("Get error = %v", err)
	}

	if want := created.Add(90 * time.Second); !got.LastAccessedAt.Equal(want) || !got.CreatedAt.Equal(created) {
		t.Errorf("after Access: created %s, last access %s; want %s and %s (Get does not count)",
			got.CreatedAt, got.LastAccessedAt, created, want)
	}
}
