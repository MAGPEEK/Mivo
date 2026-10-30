package playback

import (
	"errors"
	"strings"
	"testing"
	"time"
)

// activatedAt returns the activation time the internal tests hand to the registry.
func activatedAt() time.Time { return time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC) }

func TestRegistryActivationRestartsTheIdleTime(t *testing.T) {
	t.Parallel()

	sessions := newRegistry(sessionLimits{maxReencoding: 1, maxFFmpeg: 1, maxSessions: 1})
	created := activatedAt().Add(-time.Minute)

	entry := &sessionEntry{session: Session{ID: "slow", CreatedAt: created, LastAccessedAt: created}}
	if err := sessions.reserve(entry); err != nil {
		t.Fatalf("reserve error = %v", err)
	}

	session, _, found := sessions.activate("slow", activation{activatedAt: activatedAt()})
	if !found || !session.LastAccessedAt.Equal(activatedAt()) || !session.CreatedAt.Equal(created) {
		t.Errorf("activate = %+v, %t; want last access %s and creation %s", session, found, activatedAt(), created)
	}
}

func TestRegistryEndAccessNeverGoesNegative(t *testing.T) {
	t.Parallel()

	sessions := newRegistry(sessionLimits{maxReencoding: 1, maxFFmpeg: 1, maxSessions: 1})
	entry := &sessionEntry{session: Session{ID: "direct"}}

	if err := sessions.reserve(entry); err != nil {
		t.Fatalf("reserve error = %v", err)
	}

	sessions.activate("direct", activation{activatedAt: activatedAt()})
	sessions.endAccess(entry, activatedAt())

	if _, _, found := sessions.touch("direct", activatedAt()); !found {
		t.Fatal("touch reported the session missing")
	}

	if idle := sessions.removeIdle(activatedAt()); len(idle) != 0 {
		t.Errorf("removeIdle with an open access = %d sessions, want none (stray release went negative)", len(idle))
	}
}

func TestRegistryActivateReportsAVanishedSession(t *testing.T) {
	t.Parallel()

	sessions := newRegistry(sessionLimits{maxReencoding: 1, maxFFmpeg: 1, maxSessions: 1})

	if _, _, found := sessions.activate("missing", activation{activatedAt: activatedAt()}); found {
		t.Error("activate of an unknown session reported found")
	}

	entry := &sessionEntry{session: Session{ID: "active"}}
	if err := sessions.reserve(entry); err != nil {
		t.Fatalf("reserve error = %v", err)
	}

	if _, _, found := sessions.activate("active", activation{activatedAt: activatedAt()}); !found {
		t.Fatal("first activate reported the session missing")
	}

	if _, _, found := sessions.activate("active", activation{activatedAt: activatedAt()}); found {
		t.Error("second activate of an active session reported found, want false")
	}
}

func TestStateTransitionsFollowTheProduction(t *testing.T) {
	t.Parallel()

	states := []State{StateStarting, StateReady, StateEnded, StateFailed}
	allowed := map[[2]State]bool{
		{StateStarting, StateReady}:  true,
		{StateStarting, StateEnded}:  true,
		{StateStarting, StateFailed}: true,
		{StateReady, StateEnded}:     true,
		{StateReady, StateFailed}:    true,
	}

	for _, from := range states {
		for _, to := range states {
			if got, want := canTransition(from, to), allowed[[2]State{from, to}]; got != want {
				t.Errorf("canTransition(%s, %s) = %t, want %t", from, to, got, want)
			}
		}
	}
}

func TestRegistryTransitionIgnoresUnknownAndFinishedSessions(t *testing.T) {
	t.Parallel()

	sessions := newRegistry(sessionLimits{maxReencoding: 1, maxFFmpeg: 1, maxSessions: 1})

	if _, changed := sessions.transition("missing", StateReady, ""); changed {
		t.Error("transition of an unknown session reported a change")
	}

	if err := sessions.reserve(&sessionEntry{session: Session{ID: "hls", State: StateStarting}}); err != nil {
		t.Fatalf("reserve error = %v", err)
	}

	if _, changed := sessions.transition("hls", StateReady, ""); changed {
		t.Error("transition of a pending session reported a change")
	}

	sessions.activate("hls", activation{activatedAt: activatedAt()})

	if failed, changed := sessions.transition("hls", StateFailed, "transcode_failed"); !changed || failed.State != StateFailed {
		t.Fatal("transition starting -> failed reported no change")
	}

	if _, changed := sessions.transition("hls", StateEnded, ""); changed {
		t.Error("transition failed -> ended reported a change, want final states kept")
	}

	if session, _ := sessions.get("hls"); session.State != StateFailed || session.FailureCode != "transcode_failed" {
		t.Errorf("session = %s / %q, want failed / transcode_failed", session.State, session.FailureCode)
	}
}

func TestRegistryADirectPlayReplacementKeepsTheOldFFmpegSlotCounted(t *testing.T) {
	t.Parallel()

	const ffmpegCap = 2

	sessions := newRegistry(sessionLimits{maxReencoding: 1, maxFFmpeg: ffmpegCap, maxSessions: 10})

	for _, id := range []string{"remux-1", "remux-2"} {
		if err := sessions.reserve(&sessionEntry{session: Session{ID: id}, runsFFmpeg: true}); err != nil {
			t.Fatalf("reserve %s error = %v", id, err)
		}

		sessions.activate(id, activation{activatedAt: activatedAt()})
	}

	// A seek turns remux-1 into direct play: the replacement runs no FFmpeg, but remux-1's
	// FFmpeg keeps running until the replacement is activated.
	direct := &sessionEntry{session: Session{ID: "direct"}, replaces: "remux-1"}
	if err := sessions.reserve(direct); err != nil {
		t.Fatalf("reserve of the direct-play replacement error = %v", err)
	}

	err := sessions.reserve(&sessionEntry{session: Session{ID: "remux-3"}, runsFFmpeg: true})
	if !errors.Is(err, ErrSessionLimitReached) || !strings.Contains(err.Error(), "FFmpeg") {
		t.Errorf("reserve of a third remux during the swap error = %v, want the FFmpeg limit", err)
	}
}

func TestRegistryReleaseOfAnUnknownSessionIsHarmless(t *testing.T) {
	t.Parallel()

	sessions := newRegistry(sessionLimits{maxReencoding: 1, maxFFmpeg: 1, maxSessions: 1})
	sessions.release("missing")

	if err := sessions.reserve(&sessionEntry{session: Session{ID: "next"}}); err != nil {
		t.Errorf("reserve after releasing an unknown session error = %v", err)
	}
}
