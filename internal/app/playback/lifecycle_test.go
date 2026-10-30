package playback_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	appplayback "github.com/magpeek/mivo/internal/app/playback"
	"github.com/magpeek/mivo/internal/domain"
)

func TestFailureTranscodeFailedMatchesTheAPI(t *testing.T) {
	t.Parallel()

	if appplayback.FailureTranscodeFailed != "transcode_failed" {
		t.Errorf("FailureTranscodeFailed = %q, want the API code transcode_failed", appplayback.FailureTranscodeFailed)
	}
}

// mustGet returns the session or fails the test.
func mustGet(t *testing.T, service *appplayback.Service, id string) appplayback.Session {
	t.Helper()

	session, err := service.Get(t.Context(), id)
	if err != nil {
		t.Fatalf("Get(%s) error = %v", id, err)
	}

	return session
}

// assertState checks the session's state; a failed session must carry the one failure code
// productions report, every other state none.
func assertState(t *testing.T, service *appplayback.Service, id string, want appplayback.State) {
	t.Helper()

	wantFailure := ""
	if want == appplayback.StateFailed {
		wantFailure = appplayback.FailureTranscodeFailed
	}

	session := mustGet(t, service, id)
	if session.State != want || session.FailureCode != wantFailure {
		t.Errorf("session %s state = %s (failure %q), want %s (failure %q)",
			id, session.State, session.FailureCode, want, wantFailure)
	}
}

func TestAnHLSSessionBecomesReadyWithItsFirstSegment(t *testing.T) {
	t.Parallel()

	inBubble(t, func(t *testing.T, env *environment) {
		env.producer.holdSegments = true
		service := env.service(t)
		started := mustStart(t, service, request(remuxFileID))

		synctest.Wait()
		assertState(t, service, started.ID, appplayback.StateStarting)

		env.producer.productionOf(started.ID).writeSegment()
		synctest.Wait()
		assertState(t, service, started.ID, appplayback.StateReady)
	})
}

func TestAnHLSSessionEndsWithACleanExit(t *testing.T) {
	t.Parallel()

	inBubble(t, func(t *testing.T, env *environment) {
		service := env.service(t)
		started := mustStart(t, service, request(transcodeFileID))

		synctest.Wait()
		assertState(t, service, started.ID, appplayback.StateReady)

		env.producer.productionOf(started.ID).exit(nil)
		synctest.Wait()
		assertState(t, service, started.ID, appplayback.StateEnded)

		// An ended session stays playable: the client still loads the produced segments.
		mustAccess(t, service, started.ID).Release()
	})
}

func TestAFailedProductionFailsTheSession(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		firstSegment bool
		exitErr      error
	}{
		{name: "exit error after the first segment", firstSegment: true, exitErr: errBoom},
		{name: "exit error before the first segment", exitErr: errBoom},
		{name: "clean exit without any segment", exitErr: nil},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			inBubble(t, func(t *testing.T, env *environment) {
				env.producer.holdSegments = true
				service := env.service(t)
				started := mustStart(t, service, request(remuxFileID))
				production := env.producer.productionOf(started.ID)

				if test.firstSegment {
					production.writeSegment()
					synctest.Wait()
				}

				production.exit(test.exitErr)
				synctest.Wait()
				assertState(t, service, started.ID, appplayback.StateFailed)

				assertFailureRecordedAtInfo(t, env, started)

				// A failed session stays visible until it is stopped or reaped, so the
				// client can read why it failed.
				if err := service.Stop(t.Context(), started.ID); err != nil {
					t.Errorf("Stop of a failed session error = %v", err)
				}
			})
		})
	}
}

// assertFailureRecordedAtInfo checks the service's log of a failed session: the producer logs
// the failed process at warn, the service only records the state change, so the failure is
// warned about once.
func assertFailureRecordedAtInfo(t *testing.T, env *environment, failed appplayback.Session) {
	t.Helper()

	logs := env.logs.String()
	for _, want := range []string{"level=INFO", failed.ID, failed.FileID, "failure_code=transcode_failed"} {
		if !strings.Contains(logs, want) {
			t.Errorf("logs = %q, want an info line with %q", logs, want)
		}
	}

	if strings.Contains(logs, "level=WARN") {
		t.Errorf("logs = %q, want no second warning for the failure", logs)
	}
}

func TestADirectPlaySessionIsReadyAtOnce(t *testing.T) {
	t.Parallel()

	inBubble(t, func(t *testing.T, env *environment) {
		service := env.service(t)
		started := mustStart(t, service, request(directFileID))

		if started.State != appplayback.StateReady {
			t.Errorf("Start state = %s, want ready", started.State)
		}

		synctest.Wait()
		assertState(t, service, started.ID, appplayback.StateReady)
	})
}

func TestAStoppedProductionIsNotAFailure(t *testing.T) {
	t.Parallel()

	inBubble(t, func(t *testing.T, env *environment) {
		env.producer.holdSegments = true
		service := env.service(t)
		started := mustStart(t, service, request(remuxFileID))

		if err := service.Stop(t.Context(), started.ID); err != nil {
			t.Fatalf("Stop error = %v", err)
		}

		synctest.Wait()

		if logs := env.logs.String(); strings.Contains(logs, "level=WARN") {
			t.Errorf("logs = %q, want no warning for a stopped session", logs)
		}
	})
}

func TestCloseStopsEverySessionAndRefusesNewOnes(t *testing.T) {
	t.Parallel()

	inBubble(t, func(t *testing.T, env *environment) {
		service := env.service(t)
		sessions := []appplayback.Session{
			mustStart(t, service, request(directFileID)),
			mustStart(t, service, request(remuxFileID)),
			mustStart(t, service, request(transcodeFileID)),
		}

		if err := service.Close(t.Context()); err != nil {
			t.Fatalf("Close error = %v", err)
		}

		assertEveryProductionStoppedOnce(t, env)
		assertProducerContextsCancelled(t, env)

		if removed := env.workspace.removedIDs(); len(removed) != 2 {
			t.Errorf("removed workspaces = %v, want both HLS sessions", removed)
		}

		for _, session := range sessions {
			assertGone(t, service, session.ID)
		}

		_, err := service.Start(t.Context(), request(directFileID))
		assertWrappedError(t, err, appplayback.ErrServiceClosed, "start playback")

		if !errors.Is(err, domain.ErrUnavailable) {
			t.Errorf("Start after Close error = %v, want it to wrap domain.ErrUnavailable", err)
		}

		if err := service.Stop(t.Context(), sessions[1].ID); err != nil {
			t.Errorf("Stop after Close error = %v, want nil", err)
		}
	})
}

func assertEveryProductionStoppedOnce(t *testing.T, env *environment) {
	t.Helper()

	for _, started := range env.producer.jobs() {
		if stops := started.production.stopCount(); stops != 1 {
			t.Errorf("production of %s stops = %d, want 1", started.job.SessionID, stops)
		}
	}
}

func assertProducerContextsCancelled(t *testing.T, env *environment) {
	t.Helper()

	for _, started := range env.producer.jobs() {
		if err := started.ctx.Err(); !errors.Is(err, context.Canceled) {
			t.Errorf("producer context of %s error = %v after Close, want context.Canceled", started.job.SessionID, err)
		}
	}
}

func TestCloseStopsEveryProductionBeforeItCancelsTheProcessContext(t *testing.T) {
	t.Parallel()

	inBubble(t, func(t *testing.T, env *environment) {
		service := env.service(t)
		mustStart(t, service, request(remuxFileID))
		mustStart(t, service, request(transcodeFileID))

		if err := service.Close(t.Context()); err != nil {
			t.Fatalf("Close error = %v", err)
		}

		// ADR 0024: Stop first, so the producer never sees a kill through its context, which
		// it would report as a crash; the context is cancelled only afterwards.
		for _, started := range env.producer.jobs() {
			if started.production.stoppedAfterKill() {
				t.Errorf("production of %s was stopped after its context was cancelled, want Stop first",
					started.job.SessionID)
			}
		}

		assertEveryProductionStoppedOnce(t, env)
		assertProducerContextsCancelled(t, env)
	})
}

func TestAStartRacingCloseReportsTheShutdown(t *testing.T) {
	t.Parallel()

	inBubble(t, func(t *testing.T, env *environment) {
		service := env.service(t)
		results := startHeldAtTheGate(t, env, service, request(remuxFileID))
		closed := closeInBackground(t.Context(), service)
		synctest.Wait()

		// The producer sees the cancelled process context and refuses to start FFmpeg.
		env.producer.err = context.Canceled
		close(env.producer.gate)

		result := <-results
		assertWrappedError(t, result.err, appplayback.ErrServiceClosed, "start stream producer")

		if errors.Is(result.err, context.Canceled) {
			t.Errorf("Start error = %v wraps context.Canceled, want only the shutdown (not a 500)", result.err)
		}

		if err := <-closed; err != nil {
			t.Fatalf("Close error = %v", err)
		}

		if removed := env.workspace.removedIDs(); len(removed) != 1 {
			t.Errorf("removed workspaces = %v, want the starting session's", removed)
		}
	})
}

func TestCloseWaitsForARunningPurge(t *testing.T) {
	t.Parallel()

	inBubble(t, func(t *testing.T, env *environment) {
		env.workspace.purgeGate = make(chan struct{})
		service := env.service(t)
		purged := make(chan error, 1)

		go func() { purged <- service.PurgeWorkspace(t.Context()) }()

		synctest.Wait()

		closed := closeInBackground(t.Context(), service)
		synctest.Wait()
		assertStillClosing(t, closed)

		close(env.workspace.purgeGate)

		if err := <-purged; err != nil {
			t.Errorf("PurgeWorkspace error = %v", err)
		}

		if err := <-closed; err != nil {
			t.Fatalf("Close error = %v", err)
		}
	})
}

func TestASecondCloseIsANoOp(t *testing.T) {
	t.Parallel()

	inBubble(t, func(t *testing.T, env *environment) {
		service := env.service(t)
		started := mustStart(t, service, request(remuxFileID))

		for attempt := range 2 {
			if err := service.Close(t.Context()); err != nil {
				t.Fatalf("Close #%d error = %v", attempt+1, err)
			}
		}

		if stops := env.producer.productionOf(started.ID).stopCount(); stops != 1 {
			t.Errorf("production stops = %d after two Close calls, want 1", stops)
		}

		if removed := env.workspace.removedIDs(); len(removed) != 1 {
			t.Errorf("removed workspaces = %v after two Close calls, want one removal", removed)
		}
	})
}

// closeInBackground runs Close in a goroutine and returns the channel of its result.
func closeInBackground(ctx context.Context, service *appplayback.Service) <-chan error {
	closed := make(chan error, 1)

	go func() { closed <- service.Close(ctx) }()

	return closed
}

func assertStillClosing(t *testing.T, closed <-chan error) {
	t.Helper()

	select {
	case err := <-closed:
		t.Fatalf("Close returned %v before the sessions were stopped", err)
	default:
	}
}

func TestCloseDuringAStartTearsTheNewProductionDown(t *testing.T) {
	t.Parallel()

	inBubble(t, func(t *testing.T, env *environment) {
		service := env.service(t)
		results := startHeldAtTheGate(t, env, service, request(remuxFileID))

		closed := closeInBackground(t.Context(), service)
		synctest.Wait()
		assertStillClosing(t, closed)

		close(env.producer.gate)

		result := <-results
		assertWrappedError(t, result.err, appplayback.ErrServiceClosed, "ended while it started")

		if err := <-closed; err != nil {
			t.Fatalf("Close error = %v", err)
		}

		started := env.producer.jobs()
		if len(started) != 1 || started[0].production.stopCount() != 1 {
			t.Fatalf("productions = %d, want one, stopped once", len(started))
		}

		if removed := env.workspace.removedIDs(); len(removed) != 1 || removed[0] != started[0].job.SessionID {
			t.Errorf("removed workspaces = %v, want the starting session's", removed)
		}
	})
}

func TestCloseWaitsForSlowStopsRunningInParallel(t *testing.T) {
	t.Parallel()

	inBubble(t, func(t *testing.T, env *environment) {
		env.producer.stopGate = make(chan struct{})
		service := env.service(t)
		first := mustStart(t, service, request(remuxFileID))
		second := mustStart(t, service, request(transcodeFileID))

		closed := closeInBackground(t.Context(), service)
		synctest.Wait()
		assertStillClosing(t, closed)

		// Both stops are in progress at the same time: Close stops sessions in parallel.
		for _, id := range []string{first.ID, second.ID} {
			if stops := env.producer.productionOf(id).stopCount(); stops != 1 {
				t.Errorf("production of %s stops in progress = %d, want 1", id, stops)
			}
		}

		if removed := env.workspace.removedIDs(); len(removed) != 0 {
			t.Errorf("removed workspaces = %v while FFmpeg still runs, want none", removed)
		}

		close(env.producer.stopGate)

		if err := <-closed; err != nil {
			t.Fatalf("Close error = %v", err)
		}

		if removed := env.workspace.removedIDs(); len(removed) != 2 {
			t.Errorf("removed workspaces = %v, want both", removed)
		}
	})
}

func TestCloseGivesUpWhenItsContextEnds(t *testing.T) {
	t.Parallel()

	const closeBudget = 5 * time.Second

	inBubble(t, func(t *testing.T, env *environment) {
		env.producer.stopGate = make(chan struct{})
		service := env.service(t)
		mustStart(t, service, request(remuxFileID))

		ctx, cancel := context.WithTimeout(t.Context(), closeBudget)
		defer cancel()

		// The bubble's fake clock runs the timeout out at once while Stop hangs.
		err := service.Close(ctx)
		assertWrappedError(t, err, context.DeadlineExceeded, "close playback service")

		// The hanging FFmpeg is killed through its cancelled context as a last resort.
		assertProducerContextsCancelled(t, env)

		// Let the hanging stop finish, so no goroutine outlives the bubble.
		close(env.producer.stopGate)
		synctest.Wait()

		if removed := env.workspace.removedIDs(); len(removed) != 1 {
			t.Errorf("removed workspaces = %v after the stop finished, want the session's", removed)
		}
	})
}

func TestCloseLeavesNoGoroutineBehind(t *testing.T) {
	t.Parallel()

	// The bubble fails the test when any goroutine of the service (reaper, production
	// watchers, teardowns) is still blocked after Close and the cleanups returned.
	inBubble(t, func(t *testing.T, env *environment) {
		env.producer.holdSegments = true
		service := env.service(t)

		mustStart(t, service, request(directFileID))
		mustStart(t, service, request(remuxFileID))

		ready := mustStart(t, service, request(transcodeFileID))
		env.producer.productionOf(ready.ID).writeSegment()

		env.passTime(appplayback.IdleCheckInterval)

		if err := service.Close(t.Context()); err != nil {
			t.Fatalf("Close error = %v", err)
		}
	})
}

func TestPurgeWorkspaceRemovesLeftoversOnce(t *testing.T) {
	t.Parallel()

	env := newEnvironment()
	env.workspace.purgeRemoved = 3
	service := env.service(t)

	if err := service.PurgeWorkspace(t.Context()); err != nil {
		t.Fatalf("PurgeWorkspace error = %v", err)
	}

	if purges := env.workspace.purgeCount(); purges != 1 {
		t.Errorf("workspace purges = %d, want 1", purges)
	}

	if logs := env.logs.String(); !strings.Contains(logs, "level=INFO") || !strings.Contains(logs, "removed=3") {
		t.Errorf("logs = %q, want an info line with the number of removed directories", logs)
	}

	// Sessions start normally after the purge.
	mustStart(t, service, request(remuxFileID))
}

func TestPurgeWorkspaceReportsAFailedPurge(t *testing.T) {
	t.Parallel()

	env := newEnvironment()
	env.workspace.purgeRemoved = 1
	env.workspace.purgeErr = errors.New("directory locked")

	err := env.service(t).PurgeWorkspace(t.Context())
	assertWrappedError(t, err, env.workspace.purgeErr, "purge playback workspace (1 removed)")

	if logs := env.logs.String(); !strings.Contains(logs, "removed=1") {
		t.Errorf("logs = %q, want the number of removed directories", logs)
	}
}

func TestPurgeWorkspaceRefusesOnceSessionsExist(t *testing.T) {
	t.Parallel()

	env := newEnvironment()
	service := env.service(t)
	started := mustStart(t, service, request(remuxFileID))

	// A purge now would delete the directory FFmpeg writes into.
	err := service.PurgeWorkspace(t.Context())
	assertWrappedError(t, err, domain.ErrConflict, "before the first session")

	if purges := env.workspace.purgeCount(); purges != 0 {
		t.Errorf("workspace purges = %d, want none", purges)
	}

	// Still refused after the session is gone: another one may start at any time.
	if err := service.Stop(t.Context(), started.ID); err != nil {
		t.Fatalf("Stop error = %v", err)
	}

	if err := service.PurgeWorkspace(t.Context()); !errors.Is(err, domain.ErrConflict) {
		t.Errorf("PurgeWorkspace after the first session error = %v, want ErrConflict", err)
	}
}

func TestPurgeWorkspaceAfterCloseIsRefused(t *testing.T) {
	t.Parallel()

	env := newEnvironment()
	service := env.service(t)

	if err := service.Close(t.Context()); err != nil {
		t.Fatalf("Close error = %v", err)
	}

	err := service.PurgeWorkspace(t.Context())
	assertWrappedError(t, err, appplayback.ErrServiceClosed, "purge playback workspace")
}

func TestNoSessionStartsWhileTheWorkspaceIsPurged(t *testing.T) {
	t.Parallel()

	inBubble(t, func(t *testing.T, env *environment) {
		env.workspace.purgeGate = make(chan struct{})
		service := env.service(t)
		purged := make(chan error, 1)

		go func() { purged <- service.PurgeWorkspace(t.Context()) }()

		synctest.Wait()

		_, err := service.Start(t.Context(), request(directFileID))
		assertWrappedError(t, err, domain.ErrUnavailable, "purged")

		if err := service.PurgeWorkspace(t.Context()); !errors.Is(err, domain.ErrConflict) {
			t.Errorf("concurrent PurgeWorkspace error = %v, want ErrConflict", err)
		}

		close(env.workspace.purgeGate)

		if err := <-purged; err != nil {
			t.Fatalf("PurgeWorkspace error = %v", err)
		}

		mustStart(t, service, request(directFileID))
	})
}
