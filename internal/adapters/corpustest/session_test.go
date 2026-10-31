package corpustest_test

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/magpeek/mivo/internal/adapters/corpustest"
	appplayback "github.com/magpeek/mivo/internal/app/playback"
	"github.com/magpeek/mivo/internal/domain/playback"
)

// sessionTimeout bounds one session from start to the end of its FFmpeg run (at most 30 s
// of 320x240, re-encoded, under the race detector).
const sessionTimeout = 2 * time.Minute

// sessionRun is one session that is played to the end and checked.
type sessionRun struct {
	group   sessionGroup
	video   cataloguedCase
	profile playback.ClientProfile
	want    corpustest.Expectation
	// start is the requested position and wantStart the start the session must report;
	// both are 0 for a run from the beginning.
	start     time.Duration
	wantStart time.Duration
}

// playAndVerify starts the session, checks its plan and start and then its output: the
// original file for direct play, the finished HLS stream otherwise.
func playAndVerify(t *testing.T, run sessionRun) {
	t.Helper()

	session := startSession(t, run.group.service, appplayback.StartRequest{
		FileID: run.video.fileID, Profile: run.profile, StartPosition: run.start,
	})

	assertPlan(t, session.Plan, run.want)
	assertStart(t, session, run)

	if session.Plan.Delivery == playback.DeliveryFile {
		assertDirectPlaySource(t, run, session.ID)
		t.Logf("expected %s, observed %s; output: the original file", run.want.Mode, session.Plan.Mode)

		return
	}

	waitForEnd(t, run.group.service, session.ID)

	output := run.group.graph.probeOutput(t, run.group.service, session.ID)
	assertOutput(t, run, session.StartPosition, output)
	t.Logf("expected %s, observed %s; output: video %s, audio %s, %.2f s from %s",
		run.want.Mode, session.Plan.Mode, output.videoCodec, output.audioCodec,
		output.duration.Seconds(), session.StartPosition)
}

// startSession starts a session and stops it when the subtest ends, which kills FFmpeg and
// removes the session directory.
func startSession(t *testing.T, service *appplayback.Service, request appplayback.StartRequest) appplayback.Session {
	t.Helper()

	session, err := service.Start(t.Context(), request)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}

	t.Cleanup(func() { stopSession(t, service, session.ID) })

	return session
}

func stopSession(t *testing.T, service *appplayback.Service, id string) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.WithoutCancel(t.Context()), shutdownTimeout)
	defer cancel()

	if err := service.Stop(ctx, id); err != nil {
		t.Errorf("Stop: %v", err)
	}
}

// assertPlan compares mode, stream actions, reasons and warnings with the expectation. A
// wrong mode or action ends the subtest: its output checks would only repeat the mismatch.
func assertPlan(t *testing.T, plan playback.Plan, want corpustest.Expectation) {
	t.Helper()

	video, _ := plan.Video()
	audio, _ := plan.Audio()

	if plan.Mode != want.Mode || video.Action != want.Video || audio.Action != want.Audio {
		t.Fatalf("plan = %s (video %q, audio %q, reasons %v), want %s (video %q, audio %q, reasons %v)",
			plan.Mode, video.Action, audio.Action, plan.Reasons, want.Mode, want.Video, want.Audio, want.Reasons)
	}

	if !slices.Equal(plan.Reasons, want.Reasons) {
		t.Errorf("reasons = %v, want %v", plan.Reasons, want.Reasons)
	}

	if !slices.Equal(plan.Warnings, want.Warnings) {
		t.Errorf("warnings = %v, want %v", plan.Warnings, want.Warnings)
	}
}

// assertStart checks the reported start: exact for a re-encoded video or a run from the
// beginning; a copied video starts at a keyframe whose position depends on the format start
// time FFmpeg versions report differently (−0.021 s in 7.1, 0 in 9.0 for the long MKV).
func assertStart(t *testing.T, session appplayback.Session, run sessionRun) {
	t.Helper()

	tolerance := time.Duration(0)
	if run.start > 0 && run.want.Video == playback.ActionCopy {
		tolerance = corpustest.KeyframeTolerance
	}

	if (session.StartPosition - run.wantStart).Abs() > tolerance {
		t.Errorf("start position = %s for a request at %s, want %s (±%s)",
			session.StartPosition, run.start, run.wantStart, tolerance)
	}
}

// assertDirectPlaySource checks that the stream of a direct-play session is the original
// corpus file, opened through the library root.
func assertDirectPlaySource(t *testing.T, run sessionRun, id string) {
	t.Helper()

	access, err := run.group.service.Access(t.Context(), id)
	if err != nil {
		t.Fatalf("Access: %v", err)
	}

	defer access.Release()

	if access.Delivery != playback.DeliveryFile || access.RelativePath != run.video.clip.File || access.ContentType == "" {
		t.Fatalf("stream access = %s %q (%q), want the file %q with a content type",
			access.Delivery, access.RelativePath, access.ContentType, run.video.clip.File)
	}

	resolved, err := run.group.graph.files.Resolve(access.LibraryRoot, access.RelativePath)
	if err != nil {
		t.Fatalf("resolve the stream source: %v", err)
	}

	original := filepath.Join(run.group.graph.corpusRoot, filepath.FromSlash(run.video.clip.File))
	if !sameFile(t, resolved, original) {
		t.Errorf("the stream source %s is not the corpus file %s", resolved, original)
	}
}

func sameFile(t *testing.T, first, second string) bool {
	t.Helper()

	firstInfo, firstErr := os.Stat(first)
	secondInfo, secondErr := os.Stat(second)

	if firstErr != nil || secondErr != nil {
		t.Fatalf("stat %s, %s: %v, %v", first, second, firstErr, secondErr)
	}

	return os.SameFile(firstInfo, secondInfo)
}

// waitForEnd waits until FFmpeg has finished the session: ended, or failed with its code.
func waitForEnd(t *testing.T, service *appplayback.Service, id string) {
	t.Helper()

	deadline := time.Now().Add(sessionTimeout)

	for {
		session, err := service.Get(t.Context(), id)
		if err != nil {
			t.Fatalf("Get: %v", err)
		}

		switch {
		case session.State == appplayback.StateEnded:
			return
		case session.State == appplayback.StateFailed:
			t.Fatalf("the session failed with %q", session.FailureCode)
		case time.Now().After(deadline):
			t.Fatalf("the session did not end within %s; state %s", sessionTimeout, session.State)
		}

		time.Sleep(pollInterval)
	}
}

// decisionRun is one session that is only started to check its plan or its failure.
type decisionRun struct {
	group   sessionGroup
	video   cataloguedCase
	profile playback.ClientProfile
	// want is the plan of a session that starts; wantFailure, when set, is the failure code
	// the start must report instead.
	want        corpustest.Expectation
	wantFailure playback.FailureCode
}

// decideOnly starts the session and checks the plan, or the failure code of a start that
// must fail. A started session is stopped at once.
func decideOnly(t *testing.T, run decisionRun) {
	t.Helper()

	request := appplayback.StartRequest{FileID: run.video.fileID, Profile: run.profile}

	if run.wantFailure == "" {
		session := startSession(t, run.group.service, request)
		assertPlan(t, session.Plan, run.want)

		return
	}

	session, err := run.group.service.Start(t.Context(), request)
	if err == nil {
		stopSession(t, run.group.service, session.ID)
		t.Fatalf("Start = %s plan, want the failure %s", session.Plan.Mode, run.wantFailure)
	}

	if code, found := playback.FailureCodeOf(err); !found || code != run.wantFailure {
		t.Errorf("Start error = %v (code %q), want the failure %s", err, code, run.wantFailure)
	}
}
