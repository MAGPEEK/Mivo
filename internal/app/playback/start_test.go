package playback_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	appplayback "github.com/magpeek/mivo/internal/app/playback"
	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/playback"
	"github.com/magpeek/mivo/internal/domain/playback/playbacktest"
)

func TestStartDirectPlayNeedsNoFFmpeg(t *testing.T) {
	t.Parallel()

	env := newEnvironment()
	startRequest := request(directFileID)
	startRequest.StartPosition = 30 * time.Second

	session := mustStart(t, env.service(t), startRequest)

	if session.Plan.Mode != playback.ModeDirectPlay || session.State != appplayback.StateReady {
		t.Errorf("mode %s, state %s; want direct_play and ready", session.Plan.Mode, session.State)
	}

	if want := "/stream/" + session.ID + "/file"; session.StreamURL != want {
		t.Errorf("StreamURL = %q, want %q", session.StreamURL, want)
	}

	// The stream is the whole file; the player seeks inside it locally.
	if session.StartPosition != 0 {
		t.Errorf("StartPosition = %s, want 0 for direct play", session.StartPosition)
	}

	if session.FileID != directFileID || session.Duration != fileDuration || session.FailureCode != "" {
		t.Errorf("session = %+v, want file %s with duration %s and no failure", session, directFileID, fileDuration)
	}

	if jobs, created := env.producer.jobs(), env.workspace.createdIDs(); len(jobs) != 0 || len(created) != 0 {
		t.Errorf("producer jobs = %d, workspaces = %v; want none for direct play", len(jobs), created)
	}

	if calls := env.keyframes.recordedCalls(); len(calls) != 0 {
		t.Errorf("keyframe lookups = %v, want none for direct play", calls)
	}
}

func TestStartHLSModesStartTheProducer(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		fileID       string
		relativePath string
		wantMode     playback.Mode
	}{
		{name: "remux", fileID: remuxFileID, relativePath: remuxRelativePath, wantMode: playback.ModeRemux},
		{name: "transcode", fileID: transcodeFileID, relativePath: transcodeRelativePath, wantMode: playback.ModeTranscode},
		{name: "audio-only transcode", fileID: audioTranscodeFileID, relativePath: audioTranscodeRelative, wantMode: playback.ModeTranscode},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			env := newEnvironment()
			session := mustStart(t, env.service(t), request(test.fileID))

			if session.Plan.Mode != test.wantMode || session.State != appplayback.StateStarting {
				t.Errorf("mode %s, state %s; want %s and starting", session.Plan.Mode, session.State, test.wantMode)
			}

			if want := "/stream/" + session.ID + "/hls"; session.StreamURL != want {
				t.Errorf("StreamURL = %q, want %q", session.StreamURL, want)
			}

			jobs := env.producer.jobs()
			if len(jobs) != 1 {
				t.Fatalf("producer jobs = %d, want 1", len(jobs))
			}

			want := appplayback.Job{
				SessionID: session.ID, LibraryRoot: libraryRoot, RelativePath: test.relativePath,
				InputPath: libraryRoot + "/" + test.relativePath, OutputDir: workspaceDir(session.ID),
			}
			got := jobs[0].job
			if got.SessionID != want.SessionID || got.LibraryRoot != want.LibraryRoot ||
				got.RelativePath != want.RelativePath || got.InputPath != want.InputPath ||
				got.OutputDir != want.OutputDir || got.StartPosition != 0 || got.Plan.Mode != test.wantMode {
				t.Errorf("job = %+v, want %+v with mode %s at 0", got, want, test.wantMode)
			}
		})
	}
}

func TestStartSeeksCopiedVideoToAKeyframe(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		fileID       string
		relativePath string
	}{
		{name: "remux", fileID: remuxFileID, relativePath: remuxRelativePath},
		{name: "audio re-encoded, video copied", fileID: audioTranscodeFileID, relativePath: audioTranscodeRelative},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			env := newEnvironment()
			startRequest := request(test.fileID)
			startRequest.StartPosition = 21 * time.Second

			session := mustStart(t, env.service(t), startRequest)

			if session.StartPosition != env.keyframes.keyframe {
				t.Errorf("StartPosition = %s, want the keyframe %s", session.StartPosition, env.keyframes.keyframe)
			}

			if job := env.producer.jobs()[0].job; job.StartPosition != env.keyframes.keyframe {
				t.Errorf("job start = %s, want the keyframe %s", job.StartPosition, env.keyframes.keyframe)
			}

			want := keyframeCall{path: libraryRoot + "/" + test.relativePath, position: 21 * time.Second}
			if calls := env.keyframes.recordedCalls(); len(calls) != 1 || calls[0] != want {
				t.Errorf("keyframe lookups = %v, want [%v]", calls, want)
			}
		})
	}
}

func TestStartSeeksExactlyWithoutCopiedVideo(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		fileID string
	}{
		{name: "video transcoded", fileID: transcodeFileID},
		{name: "audio only", fileID: audioOnlyFileID},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			env := newEnvironment()
			startRequest := request(test.fileID)
			startRequest.StartPosition = 21 * time.Second

			session := mustStart(t, env.service(t), startRequest)

			if session.StartPosition != startRequest.StartPosition {
				t.Errorf("StartPosition = %s, want exactly %s", session.StartPosition, startRequest.StartPosition)
			}

			if job := env.producer.jobs()[0].job; job.StartPosition != startRequest.StartPosition {
				t.Errorf("job start = %s, want %s", job.StartPosition, startRequest.StartPosition)
			}

			if calls := env.keyframes.recordedCalls(); len(calls) != 0 {
				t.Errorf("keyframe lookups = %v, want none", calls)
			}
		})
	}
}

func TestStartAtTheBeginningLooksUpNoKeyframe(t *testing.T) {
	t.Parallel()

	env := newEnvironment()
	session := mustStart(t, env.service(t), request(remuxFileID))

	if session.StartPosition != 0 {
		t.Errorf("StartPosition = %s, want 0", session.StartPosition)
	}

	if calls := env.keyframes.recordedCalls(); len(calls) != 0 {
		t.Errorf("keyframe lookups = %v, want none at position 0", calls)
	}
}

func TestStartFallsBackToTheBeginningWhenTheKeyframeLookupFails(t *testing.T) {
	t.Parallel()

	env := newEnvironment()
	env.keyframes.err = fmt.Errorf("current timestamp is not defined: %w", domain.ErrUnsupported)
	startRequest := request(remuxFileID)
	startRequest.StartPosition = 21 * time.Second

	session := mustStart(t, env.service(t), startRequest)

	if session.StartPosition != 0 {
		t.Errorf("StartPosition = %s, want 0 after a failed keyframe lookup", session.StartPosition)
	}

	if job := env.producer.jobs()[0].job; job.StartPosition != 0 {
		t.Errorf("job start = %s, want 0", job.StartPosition)
	}

	if logs := env.logs.String(); !strings.Contains(logs, "level=WARN") || !strings.Contains(logs, session.ID) {
		t.Errorf("logs = %q, want a warning naming the session", logs)
	}
}

func TestStartGivesUpWhenTheRequestEndsDuringTheKeyframeLookup(t *testing.T) {
	t.Parallel()

	env := newEnvironment()
	ctx, cancel := contextWithCancel(t)
	cancel()

	env.keyframes.err = fmt.Errorf("find keyframe: %w", context.Canceled)
	startRequest := request(remuxFileID)
	startRequest.StartPosition = 21 * time.Second
	service := env.service(t)

	_, err := service.Start(ctx, startRequest)
	assertWrappedError(t, err, context.Canceled, "start playback of file "+remuxFileID)

	if jobs := env.producer.jobs(); len(jobs) != 0 {
		t.Errorf("producer jobs = %d, want none", len(jobs))
	}

	assertSlotsFree(t, env, service)
}

func TestStartFailures(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		configure func(*environment, *appplayback.StartRequest)
		wantErr   error
		wantText  string
	}{
		{
			name:      "unknown file",
			configure: func(_ *environment, r *appplayback.StartRequest) { r.FileID = unknownFileID },
			wantErr:   domain.ErrNotFound, wantText: "load file",
		},
		{
			name:      "file source fails",
			configure: func(env *environment, _ *appplayback.StartRequest) { env.files.err = errBoom },
			wantErr:   errBoom, wantText: "load file",
		},
		{
			name:      "probe failed",
			configure: func(_ *environment, r *appplayback.StartRequest) { r.FileID = probeFailedFileID },
			wantErr:   domain.ErrUnsupported, wantText: "probe_failed",
		},
		{
			name: "no compatible format",
			configure: func(_ *environment, r *appplayback.StartRequest) {
				r.Profile.HLS = playback.HLSProfile{Delivery: playback.HLSDeliveryNone}
			},
			wantErr: playback.ErrNoCompatibleFormat, wantText: "decide playback",
		},
		{
			name: "transcode unavailable",
			configure: func(env *environment, r *appplayback.StartRequest) {
				r.FileID = transcodeFileID
				env.options.Capabilities = playbacktest.NoEncoders()
			},
			wantErr: playback.ErrTranscodeUnavailable, wantText: "decide playback",
		},
		{
			name:      "start beyond the end",
			configure: func(_ *environment, r *appplayback.StartRequest) { r.StartPosition = fileDuration },
			wantErr:   domain.ErrInvalid, wantText: "start position",
		},
		{
			name: "unknown audio stream",
			configure: func(_ *environment, r *appplayback.StartRequest) {
				index := 7
				r.AudioStreamIndex = &index
			},
			wantErr: domain.ErrInvalid, wantText: "audio stream",
		},
		{
			name: "path resolution fails",
			configure: func(env *environment, _ *appplayback.StartRequest) {
				env.resolver.err = fmt.Errorf("gone: %w", domain.ErrNotFound)
			},
			wantErr: domain.ErrNotFound, wantText: "resolve",
		},
		{
			name: "direct-play file deleted",
			configure: func(env *environment, r *appplayback.StartRequest) {
				r.FileID = directFileID
				env.resolver.err = fmt.Errorf("gone: %w", domain.ErrNotFound)
			},
			wantErr: domain.ErrNotFound, wantText: "resolve " + directRelativePath,
		},
		{
			name: "direct-play file escapes the library",
			configure: func(env *environment, r *appplayback.StartRequest) {
				r.FileID = directFileID
				env.resolver.err = fmt.Errorf("outside the root: %w", domain.ErrForbidden)
			},
			wantErr: appplayback.ErrFileOutsideLibrary, wantText: "resolve " + directRelativePath,
		},
		{
			name:      "id generator fails",
			configure: func(env *environment, _ *appplayback.StartRequest) { env.ids.err = errBoom },
			wantErr:   errBoom, wantText: "session id",
		},
		{
			name:      "workspace creation fails",
			configure: func(env *environment, _ *appplayback.StartRequest) { env.workspace.createErr = errBoom },
			wantErr:   errBoom, wantText: "create workspace",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			env := newEnvironment()
			startRequest := request(remuxFileID)
			test.configure(env, &startRequest)
			service := env.service(t)

			session, err := service.Start(t.Context(), startRequest)
			assertWrappedError(t, err, test.wantErr, test.wantText)

			if session.ID != "" {
				t.Errorf("Start = %+v on error, want the zero session", session)
			}

			if jobs := env.producer.jobs(); len(jobs) != 0 {
				t.Errorf("producer jobs = %d after a failed start, want none", len(jobs))
			}

			assertSlotsFree(t, env, service)
		})
	}
}

func TestStartRejectsAnInvalidProfileBeforeLoadingTheFile(t *testing.T) {
	t.Parallel()

	env := newEnvironment()
	startRequest := request(remuxFileID)
	startRequest.Profile.Name = "evil\nname"

	_, err := env.service(t).Start(t.Context(), startRequest)
	assertWrappedError(t, err, domain.ErrInvalid, "client profile")

	if loads := env.files.loadCount(); loads != 0 {
		t.Errorf("file loads = %d, want 0 for an invalid profile", loads)
	}
}

func TestStartCleansTheWorkspaceWhenTheProducerFails(t *testing.T) {
	t.Parallel()

	env := newEnvironment()
	env.producer.err = fmt.Errorf("ffmpeg missing: %w", domain.ErrUnavailable)
	service := env.service(t)

	_, err := service.Start(t.Context(), request(remuxFileID))
	assertWrappedError(t, err, domain.ErrUnavailable, "start stream producer")

	created, removed := env.workspace.createdIDs(), env.workspace.removedIDs()
	if len(created) != 1 || len(removed) != 1 || created[0] != removed[0] {
		t.Errorf("created %v, removed %v; want the created workspace removed", created, removed)
	}

	if _, err := service.Get(t.Context(), sessionID(1)); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("Get of the failed session error = %v, want ErrNotFound", err)
	}

	env.producer.err = nil
	assertSlotsFree(t, env, service)
}

func TestStartLogsAFailedCleanupButReportsTheProducerError(t *testing.T) {
	t.Parallel()

	env := newEnvironment()
	env.producer.err = errBoom
	env.workspace.removeErr = errors.New("directory locked")

	_, err := env.service(t).Start(t.Context(), request(remuxFileID))
	assertWrappedError(t, err, errBoom, "start stream producer")

	if logs := env.logs.String(); !strings.Contains(logs, "directory locked") {
		t.Errorf("logs = %q, want the failed cleanup", logs)
	}
}

func TestStartRunsTheProductionUnderTheServiceContext(t *testing.T) {
	t.Parallel()

	env := newEnvironment()
	service := env.service(t)
	ctx, cancel := contextWithCancel(t)

	if _, err := service.Start(ctx, request(remuxFileID)); err != nil {
		t.Fatalf("Start error = %v", err)
	}

	cancel()

	started := env.producer.jobs()[0]
	if err := started.ctx.Err(); err != nil {
		t.Errorf("production context error = %v after the request ended, want nil", err)
	}

	if stops := started.production.stopCount(); stops != 0 {
		t.Errorf("production stops = %d after the request ended, want 0", stops)
	}

	select {
	case <-started.production.Done():
		t.Error("production Done is closed after the request ended, want it running")
	default:
	}
}

func TestStartReportsADuplicateSessionIDAsAnInternalError(t *testing.T) {
	t.Parallel()

	env := newEnvironment()
	env.ids.repeat = true
	env.ids.next = 1
	service := env.service(t)
	first := mustStart(t, service, request(remuxFileID))

	_, err := service.Start(t.Context(), request(remuxFileID))
	if err == nil || !strings.Contains(err.Error(), first.ID+" is already in use") {
		t.Fatalf("Start with a repeated ID error = %v, want it to name the ID in use", err)
	}

	// A generator collision is a server bug, never a client conflict (409).
	for _, sentinel := range []error{
		domain.ErrConflict, domain.ErrNotFound, domain.ErrInvalid, domain.ErrUnsupported,
		domain.ErrUnavailable, domain.ErrForbidden,
	} {
		if errors.Is(err, sentinel) {
			t.Errorf("error = %v wraps %v, want an internal error", err, sentinel)
		}
	}

	if jobs := env.producer.jobs(); len(jobs) != 1 {
		t.Errorf("producer jobs = %d, want only the first session", len(jobs))
	}
}

// TestStartReportsALibraryEscapeAsUnsupported: forbidden means a missing permission (403,
// also for clients outside the LAN), so a file that leaves its library is unsupported and
// logged at warn, because the HTTP adapter logs client errors at debug only.
func TestStartReportsALibraryEscapeAsUnsupported(t *testing.T) {
	t.Parallel()

	env := newEnvironment()
	env.resolver.err = fmt.Errorf("evaluated path leaves the library root: %w", domain.ErrForbidden)

	_, err := env.service(t).Start(t.Context(), request(directFileID))

	if !errors.Is(err, domain.ErrUnsupported) || errors.Is(err, domain.ErrForbidden) {
		t.Errorf("error = %v, want it to wrap ErrUnsupported and not ErrForbidden", err)
	}

	if !strings.Contains(err.Error(), "evaluated path leaves the library root") {
		t.Errorf("error = %q, want the resolver's message kept", err)
	}

	logs := env.logs.String()
	for _, want := range []string{"level=WARN", "leaves its library folder", "file=" + directFileID, "path=" + directRelativePath} {
		if !strings.Contains(logs, want) {
			t.Errorf("log %q does not contain %q", logs, want)
		}
	}
}

func TestStartRejectsSessionsAboveTheTranscodeLimit(t *testing.T) {
	t.Parallel()

	env := newEnvironment()
	service := env.service(t)

	for range maxTranscodes {
		mustStart(t, service, request(transcodeFileID))
	}

	_, err := service.Start(t.Context(), request(audioTranscodeFileID))
	assertWrappedError(t, err, appplayback.ErrSessionLimitReached, "re-encoding")

	if !errors.Is(err, domain.ErrUnavailable) {
		t.Errorf("error = %v, want it to wrap domain.ErrUnavailable", err)
	}

	// Remux and direct play do not re-encode and still start.
	mustStart(t, service, request(remuxFileID))
	mustStart(t, service, request(directFileID))
}

func TestStartRejectsFFmpegSessionsAboveTheSafetyCap(t *testing.T) {
	t.Parallel()

	env := newEnvironment()
	service := env.service(t)

	for range maxTranscodes {
		mustStart(t, service, request(transcodeFileID))
	}

	for range cappedFFmpegSessions - maxTranscodes {
		mustStart(t, service, request(remuxFileID))
	}

	_, err := service.Start(t.Context(), request(remuxFileID))
	assertWrappedError(t, err, appplayback.ErrSessionLimitReached, "FFmpeg")

	// Direct play runs no FFmpeg and is never limited.
	mustStart(t, service, request(directFileID))
}

func TestStartRejectsSessionsAboveTheOverallCap(t *testing.T) {
	t.Parallel()

	env := newEnvironment()
	service := env.service(t)

	var last appplayback.Session
	for range cappedSessions {
		last = mustStart(t, service, request(directFileID))
	}

	_, err := service.Start(t.Context(), request(directFileID))
	assertWrappedError(t, err, appplayback.ErrSessionLimitReached, "sessions running")

	// Seeking at the cap still works: the replacement takes over the old session's slot.
	seek := request(directFileID)
	seek.ReplacesSessionID = last.ID
	mustStart(t, service, seek)

	if _, err := service.Get(t.Context(), last.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("Get of the replaced session error = %v, want ErrNotFound", err)
	}
}

func TestStopFreesTheSlot(t *testing.T) {
	t.Parallel()

	env := newEnvironment()
	env.options.MaxTranscodeSessions = 1
	service := env.service(t)
	first := mustStart(t, service, request(transcodeFileID))

	if _, err := service.Start(t.Context(), request(transcodeFileID)); !errors.Is(err, appplayback.ErrSessionLimitReached) {
		t.Fatalf("second transcode error = %v, want ErrSessionLimitReached", err)
	}

	if err := service.Stop(t.Context(), first.ID); err != nil {
		t.Fatalf("Stop error = %v", err)
	}

	mustStart(t, service, request(transcodeFileID))
}

func TestStartReplacesASessionBeforeCheckingTheLimit(t *testing.T) {
	t.Parallel()

	env := newEnvironment()
	env.options.MaxTranscodeSessions = 1
	service := env.service(t)
	first := mustStart(t, service, request(transcodeFileID))

	seek := request(transcodeFileID)
	seek.StartPosition = 40 * time.Second
	seek.ReplacesSessionID = first.ID

	second := mustStart(t, service, seek)

	if second.ID == first.ID || second.StartPosition != seek.StartPosition {
		t.Errorf("replacement = %+v, want a new session at %s", second, seek.StartPosition)
	}

	if stops := env.producer.productionOf(first.ID).stopCount(); stops != 1 {
		t.Errorf("replaced production stops = %d, want 1", stops)
	}

	if _, err := service.Get(t.Context(), first.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("Get of the replaced session error = %v, want ErrNotFound", err)
	}
}

func TestStartIgnoresAnUnknownReplacedSession(t *testing.T) {
	t.Parallel()

	env := newEnvironment()
	startRequest := request(remuxFileID)
	startRequest.ReplacesSessionID = unknownSessionID

	mustStart(t, env.service(t), startRequest)
}

func TestAFailedReplacementKeepsTheOldSession(t *testing.T) {
	t.Parallel()

	env := newEnvironment()
	env.options.MaxTranscodeSessions = 1
	service := env.service(t)
	first := mustStart(t, service, request(transcodeFileID))

	seek := request(transcodeFileID)
	seek.ReplacesSessionID = first.ID
	env.producer.err = errBoom

	if _, err := service.Start(t.Context(), seek); !errors.Is(err, errBoom) {
		t.Fatalf("failing replacement error = %v, want errBoom", err)
	}

	if _, err := service.Get(t.Context(), first.ID); err != nil {
		t.Errorf("Get of the old session error = %v, want it still running", err)
	}

	if stops := env.producer.productionOf(first.ID).stopCount(); stops != 0 {
		t.Errorf("old production stops = %d, want 0 after a failed replacement", stops)
	}

	// The failed attempt released its claim on the old session: a retry replaces it.
	env.producer.err = nil
	mustStart(t, service, seek)

	if stops := env.producer.productionOf(first.ID).stopCount(); stops != 1 {
		t.Errorf("old production stops = %d after the retry, want 1", stops)
	}
}

func TestStartKeepsTheReplacedSessionWhenTheDecisionFails(t *testing.T) {
	t.Parallel()

	env := newEnvironment()
	service := env.service(t)
	first := mustStart(t, service, request(remuxFileID))

	bad := request(remuxFileID)
	bad.StartPosition = fileDuration
	bad.ReplacesSessionID = first.ID

	if _, err := service.Start(t.Context(), bad); !errors.Is(err, domain.ErrInvalid) {
		t.Fatalf("Start error = %v, want ErrInvalid", err)
	}

	if _, err := service.Get(t.Context(), first.ID); err != nil {
		t.Errorf("Get of the session to replace error = %v, want it still running", err)
	}
}

func TestStartContinuesWhenTheReplacedWorkspaceCannotBeRemoved(t *testing.T) {
	t.Parallel()

	env := newEnvironment()
	service := env.service(t)
	first := mustStart(t, service, request(remuxFileID))
	env.workspace.removeErr = errors.New("directory locked")

	seek := request(remuxFileID)
	seek.ReplacesSessionID = first.ID

	mustStart(t, service, seek)

	if logs := env.logs.String(); !strings.Contains(logs, "directory locked") || !strings.Contains(logs, first.ID) {
		t.Errorf("logs = %q, want a warning about the replaced session", logs)
	}
}

// assertSlotsFree proves that a failed start left no reservation behind: the service still
// accepts as many re-encoding sessions as the limit allows (when it can transcode at all) and
// fills the rest of the FFmpeg cap with remux sessions. The scripted failures are cleared
// first; Start already returned, so no service goroutine reads the fakes concurrently.
func assertSlotsFree(t *testing.T, env *environment, service *appplayback.Service) {
	t.Helper()

	env.files.err = nil
	env.resolver.err = nil
	env.workspace.createErr = nil
	env.ids.err = nil
	env.keyframes.err = nil

	transcodes := 0
	if env.options.Capabilities.H264Encoder != "" {
		transcodes = env.options.MaxTranscodeSessions
	}

	for range transcodes {
		if _, err := service.Start(t.Context(), request(transcodeFileID)); err != nil {
			t.Fatalf("transcode after the failure error = %v, want a free slot", err)
		}
	}

	for range cappedFFmpegSessions - transcodes {
		if _, err := service.Start(t.Context(), request(remuxFileID)); err != nil {
			t.Fatalf("remux after the failure error = %v, want a free slot", err)
		}
	}
}
