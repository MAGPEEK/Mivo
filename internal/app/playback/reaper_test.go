package playback_test

import (
	"errors"
	"strings"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	appplayback "github.com/magpeek/mivo/internal/app/playback"
	"github.com/magpeek/mivo/internal/domain"
)

// idleTimeout mirrors appplayback.IdleTimeout (ADR 0024: 2 minutes), so a change is a
// visible decision.
const idleTimeout = 2 * time.Minute

func TestIdleTimingFollowsADR0024(t *testing.T) {
	t.Parallel()

	if appplayback.IdleTimeout != idleTimeout {
		t.Errorf("IdleTimeout = %s, want %s (ADR 0024)", appplayback.IdleTimeout, idleTimeout)
	}

	// The tests advance time in check intervals; the timeout must be a whole number of them
	// for the boundary tests below to hit it exactly.
	if appplayback.IdleCheckInterval <= 0 || idleTimeout%appplayback.IdleCheckInterval != 0 {
		t.Errorf("IdleCheckInterval = %s, want a positive divisor of %s", appplayback.IdleCheckInterval, idleTimeout)
	}
}

// assertGone checks that the session was stopped.
func assertGone(t *testing.T, service *appplayback.Service, id string) {
	t.Helper()

	if _, err := service.Get(t.Context(), id); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("Get(%s) error = %v, want ErrNotFound (session stopped)", id, err)
	}
}

func TestTheReaperStopsIdleSessions(t *testing.T) {
	t.Parallel()

	inBubble(t, func(t *testing.T, env *environment) {
		service := env.service(t)
		direct := mustStart(t, service, request(directFileID))
		remux := mustStart(t, service, request(remuxFileID))

		env.passTime(idleTimeout - appplayback.IdleCheckInterval)
		mustGet(t, service, direct.ID)
		mustGet(t, service, remux.ID)

		env.passTime(appplayback.IdleCheckInterval)
		assertGone(t, service, direct.ID)
		assertGone(t, service, remux.ID)

		if stops := env.producer.productionOf(remux.ID).stopCount(); stops != 1 {
			t.Errorf("idle production stops = %d, want 1", stops)
		}

		if removed := env.workspace.removedIDs(); len(removed) != 1 || removed[0] != remux.ID {
			t.Errorf("removed workspaces = %v, want [%s]", removed, remux.ID)
		}

		logs := env.logs.String()
		if !strings.Contains(logs, "idle") || !strings.Contains(logs, direct.ID) || !strings.Contains(logs, remux.ID) {
			t.Errorf("logs = %q, want an idle stop line per session", logs)
		}
	})
}

func TestAccessKeepsASessionAlive(t *testing.T) {
	t.Parallel()

	inBubble(t, func(t *testing.T, env *environment) {
		service := env.service(t)
		started := mustStart(t, service, request(remuxFileID))
		almostIdle := idleTimeout - appplayback.IdleCheckInterval

		env.passTime(almostIdle)

		// A short request, such as a playlist reload, ends at once.
		mustAccess(t, service, started.ID).Release()

		// Older than the timeout since creation, but not since the last access.
		env.passTime(almostIdle)
		mustGet(t, service, started.ID)

		// Get does not count as an access.
		env.passTime(appplayback.IdleCheckInterval)
		assertGone(t, service, started.ID)
	})
}

func TestTheReaperKeepsAStartingSession(t *testing.T) {
	t.Parallel()

	inBubble(t, func(t *testing.T, env *environment) {
		service := env.service(t)
		results := startHeldAtTheGate(t, env, service, request(remuxFileID))

		env.passTime(2 * idleTimeout)
		close(env.producer.gate)

		result := <-results
		if result.err != nil {
			t.Fatalf("Start held past the idle timeout error = %v", result.err)
		}

		if stops := env.producer.productionOf(result.session.ID).stopCount(); stops != 0 {
			t.Errorf("production stops = %d, want the started session running", stops)
		}

		// The idle time counts from activation: the client learns the session ID only then.
		env.passTime(idleTimeout - appplayback.IdleCheckInterval)
		mustGet(t, service, result.session.ID)
	})
}

func TestTheReaperStopsAnEndedSession(t *testing.T) {
	t.Parallel()

	inBubble(t, func(t *testing.T, env *environment) {
		service := env.service(t)
		started := mustStart(t, service, request(remuxFileID))
		env.producer.productionOf(started.ID).exit(nil)
		synctest.Wait()
		assertState(t, service, started.ID, appplayback.StateEnded)

		env.passTime(idleTimeout)
		assertGone(t, service, started.ID)

		if removed := env.workspace.removedIDs(); len(removed) != 1 {
			t.Errorf("removed workspaces = %v, want the ended session's", removed)
		}
	})
}

// mustAccess opens a stream access of the session or fails the test.
func mustAccess(t *testing.T, service *appplayback.Service, id string) appplayback.StreamAccess {
	t.Helper()

	access, err := service.Access(t.Context(), id)
	if err != nil {
		t.Fatalf("Access(%s) error = %v", id, err)
	}

	return access
}

func TestAnOpenAccessKeepsASessionAlive(t *testing.T) {
	t.Parallel()

	const longRangeRequest = 10 * time.Minute

	inBubble(t, func(t *testing.T, env *environment) {
		service := env.service(t)
		started := mustStart(t, service, request(directFileID))

		// A browser streams the whole MP4 over one long Range request.
		access := mustAccess(t, service, started.ID)
		env.passTime(longRangeRequest)
		mustGet(t, service, started.ID)

		// The idle clock restarts when the last request ends.
		access.Release()
		env.passTime(idleTimeout - appplayback.IdleCheckInterval)
		mustGet(t, service, started.ID)

		env.passTime(appplayback.IdleCheckInterval)
		assertGone(t, service, started.ID)
	})
}

func TestReleasingAnAccessTwiceCountsOnce(t *testing.T) {
	t.Parallel()

	inBubble(t, func(t *testing.T, env *environment) {
		service := env.service(t)
		started := mustStart(t, service, request(remuxFileID))
		first := mustAccess(t, service, started.ID)
		second := mustAccess(t, service, started.ID)

		// Copies share the release: a deferred Release plus an explicit one stay one release.
		copied := first
		first.Release()
		copied.Release()
		first.Release()

		env.passTime(2 * idleTimeout)
		mustGet(t, service, started.ID)

		second.Release()
		env.passTime(idleTimeout)
		assertGone(t, service, started.ID)
	})
}

func TestConcurrentReleaseOfCopiesCountsOnce(t *testing.T) {
	t.Parallel()

	const releasingGoroutines = 8

	inBubble(t, func(t *testing.T, env *environment) {
		service := env.service(t)
		started := mustStart(t, service, request(remuxFileID))
		first := mustAccess(t, service, started.ID)
		second := mustAccess(t, service, started.ID)

		var group sync.WaitGroup
		for range releasingGoroutines {
			copied := first
			group.Go(copied.Release)
		}

		group.Wait()

		// Had the copies released more than once, the second request would no longer hold
		// the session.
		env.passTime(2 * idleTimeout)
		mustGet(t, service, started.ID)

		second.Release()
		env.passTime(idleTimeout)
		assertGone(t, service, started.ID)
	})
}

func TestReleaseRacingTheReaperAndStopTearsDownOnce(t *testing.T) {
	t.Parallel()

	inBubble(t, func(t *testing.T, env *environment) {
		service := env.service(t)
		started := mustStart(t, service, request(remuxFileID))
		access := mustAccess(t, service, started.ID)

		// Idle long enough that the next reaper check stops it once the request ends.
		env.passTime(idleTimeout)
		mustGet(t, service, started.ID)

		var group sync.WaitGroup
		group.Go(access.Release)
		group.Go(func() { env.clock.advance(appplayback.IdleCheckInterval) })
		group.Go(func() {
			if err := service.Stop(t.Context(), started.ID); err != nil {
				t.Errorf("Stop error = %v", err)
			}
		})
		group.Wait()
		synctest.Wait()

		assertGone(t, service, started.ID)

		if stops := env.producer.productionOf(started.ID).stopCount(); stops != 1 {
			t.Errorf("production stops = %d, want 1", stops)
		}

		if removed := env.workspace.removedIDs(); len(removed) != 1 || removed[0] != started.ID {
			t.Errorf("removed workspaces = %v, want [%s]", removed, started.ID)
		}
	})
}

func TestAReplacementStopsTheOldSessionDespiteAnOpenAccess(t *testing.T) {
	t.Parallel()

	env := newEnvironment()
	service := env.service(t)
	old := mustStart(t, service, request(remuxFileID))
	access := mustAccess(t, service, old.ID)

	defer access.Release()

	seek := request(remuxFileID)
	seek.ReplacesSessionID = old.ID
	mustStart(t, service, seek)

	if stops := env.producer.productionOf(old.ID).stopCount(); stops != 1 {
		t.Errorf("old production stops = %d with an open access, want 1", stops)
	}

	assertGone(t, service, old.ID)
}

func TestReleaseAfterTheSessionEndedIsHarmless(t *testing.T) {
	t.Parallel()

	inBubble(t, func(t *testing.T, env *environment) {
		service := env.service(t)
		stopped := mustStart(t, service, request(remuxFileID))
		closed := mustStart(t, service, request(directFileID))
		stoppedAccess := mustAccess(t, service, stopped.ID)
		closedAccess := mustAccess(t, service, closed.ID)

		// Stop tears a session down even while a request still reads from it.
		if err := service.Stop(t.Context(), stopped.ID); err != nil {
			t.Fatalf("Stop error = %v", err)
		}

		if stops := env.producer.productionOf(stopped.ID).stopCount(); stops != 1 {
			t.Errorf("production stops = %d with an open access, want 1", stops)
		}

		assertGone(t, service, stopped.ID)
		stoppedAccess.Release()

		if err := service.Close(t.Context()); err != nil {
			t.Fatalf("Close with an open access error = %v", err)
		}

		closedAccess.Release()
		appplayback.StreamAccess{}.Release()
	})
}
