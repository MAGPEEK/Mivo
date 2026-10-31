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

	if idle := sessions.removeIdle(activatedAt(), 0); len(idle) != 0 {
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

// sessionKind is what a test session occupies besides its overall slot.
type sessionKind int

const (
	kindDirect sessionKind = iota
	kindRemux
	kindTranscode
)

// newKindEntry returns a starting session record of kind.
func newKindEntry(id string, kind sessionKind) *sessionEntry {
	return &sessionEntry{
		session:    Session{ID: id, State: StateStarting},
		reencodes:  kind == kindTranscode,
		runsFFmpeg: kind != kindDirect,
	}
}

// visibleSession describes a session addSessionInState adds.
type visibleSession struct {
	kind  sessionKind
	state State
}

// addSessionInState reserves and activates a session described by wanted and moves it to its
// state.
func addSessionInState(t *testing.T, sessions *registry, id string, wanted visibleSession) {
	t.Helper()

	if err := sessions.reserve(newKindEntry(id, wanted.kind)); err != nil {
		t.Fatalf("reserve %s error = %v", id, err)
	}

	sessions.activate(id, activation{activatedAt: activatedAt()})

	if wanted.state == StateStarting {
		return
	}

	if _, changed := sessions.transition(id, wanted.state, ""); !changed {
		t.Fatalf("transition %s to %s reported no change", id, wanted.state)
	}
}

func TestRegistryFinishedSessionsCountOnlyTowardTheOverallCap(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		limits        sessionLimits
		existingKind  sessionKind
		existingState State
		newKind       sessionKind
		wantRefused   bool
	}{
		{"ended remux admits a remux", sessionLimits{1, 1, 10}, kindRemux, StateEnded, kindRemux, false},
		{"failed remux admits a remux", sessionLimits{1, 1, 10}, kindRemux, StateFailed, kindRemux, false},
		{"failed transcode admits a transcode", sessionLimits{1, 8, 10}, kindTranscode, StateFailed, kindTranscode, false},
		{"ended transcode admits a transcode", sessionLimits{1, 8, 10}, kindTranscode, StateEnded, kindTranscode, false},
		{"ended transcode frees the FFmpeg cap", sessionLimits{1, 1, 10}, kindTranscode, StateEnded, kindRemux, false},
		{"starting remux refuses a remux", sessionLimits{1, 1, 10}, kindRemux, StateStarting, kindRemux, true},
		{"ready remux refuses a remux", sessionLimits{1, 1, 10}, kindRemux, StateReady, kindRemux, true},
		{"starting transcode refuses a transcode", sessionLimits{1, 8, 10}, kindTranscode, StateStarting, kindTranscode, true},
		{"ready transcode refuses a transcode", sessionLimits{1, 8, 10}, kindTranscode, StateReady, kindTranscode, true},
		{"ended remux still fills the overall cap", sessionLimits{1, 8, 1}, kindRemux, StateEnded, kindDirect, true},
		{"failed transcode still fills the overall cap", sessionLimits{1, 8, 1}, kindTranscode, StateFailed, kindRemux, true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			sessions := newRegistry(test.limits)
			addSessionInState(t, sessions, "existing", visibleSession{test.existingKind, test.existingState})

			err := sessions.reserve(newKindEntry("new", test.newKind))

			refused := errors.Is(err, ErrSessionLimitReached)
			if refused != test.wantRefused || (err != nil && !refused) {
				t.Errorf("reserve error = %v, want refused %t", err, test.wantRefused)
			}
		})
	}
}

func TestRegistryReplacingAFinishedSessionFollowsTheSameRules(t *testing.T) {
	t.Parallel()

	// One re-encoding slot, two FFmpeg slots: a seek turns an ended transcode into a remux.
	sessions := newRegistry(sessionLimits{maxReencoding: 1, maxFFmpeg: 2, maxSessions: 3})
	addSessionInState(t, sessions, "old", visibleSession{kindTranscode, StateEnded})

	remux := newKindEntry("remux", kindRemux)
	remux.replaces = "old"

	if err := sessions.reserve(remux); err != nil {
		t.Fatalf("reserve of the replacement error = %v", err)
	}

	// The ended transcode re-encodes nothing any more, so another viewer gets the slot.
	if err := sessions.reserve(newKindEntry("transcode", kindTranscode)); err != nil {
		t.Fatalf("reserve of a transcode during the swap error = %v", err)
	}

	// The FFmpeg cap is full now: the pending remux and the pending transcode.
	if err := sessions.reserve(newKindEntry("third", kindRemux)); !errors.Is(err, ErrSessionLimitReached) {
		t.Errorf("reserve of a third FFmpeg session error = %v, want ErrSessionLimitReached", err)
	}

	_, replaced, found := sessions.activate("remux", activation{activatedAt: activatedAt()})
	if !found || replaced == nil || replaced.session.ID != "old" {
		t.Errorf("activate of the replacement = %v, %t; want the ended session handed over", replaced, found)
	}

	if _, found := sessions.get("old"); found {
		t.Error("the replaced session is still visible after the swap")
	}
}

func TestRegistryAReplacementOfAnEndedSessionTakesOverTheOverallSlot(t *testing.T) {
	t.Parallel()

	sessions := newRegistry(sessionLimits{maxReencoding: 1, maxFFmpeg: 1, maxSessions: 1})
	addSessionInState(t, sessions, "old", visibleSession{kindRemux, StateEnded})

	seek := newKindEntry("seek", kindRemux)
	seek.replaces = "old"

	if err := sessions.reserve(seek); err != nil {
		t.Errorf("reserve of a replacement at the overall cap error = %v", err)
	}
}

func TestRegistryRemoveIdleReportsTheIdleTime(t *testing.T) {
	t.Parallel()

	const timeout = 2 * time.Minute

	sessions := newRegistry(sessionLimits{maxReencoding: 1, maxFFmpeg: 1, maxSessions: 2})
	addSessionInState(t, sessions, "idle", visibleSession{kindDirect, StateReady})
	addSessionInState(t, sessions, "busy", visibleSession{kindDirect, StateReady})

	if _, _, found := sessions.touch("busy", activatedAt()); !found {
		t.Fatal("touch reported the session missing")
	}

	wantIdle := timeout + time.Second
	idle := sessions.removeIdle(activatedAt().Add(wantIdle), timeout)

	if len(idle) != 1 || idle[0].entry.session.ID != "idle" || idle[0].idleFor != wantIdle {
		t.Fatalf("removeIdle = %+v, want only the idle session, idle for %s", idle, wantIdle)
	}
}

func TestRegistryOpenRequestsFollowsTheRemovedRecord(t *testing.T) {
	t.Parallel()

	sessions := newRegistry(sessionLimits{maxReencoding: 1, maxFFmpeg: 1, maxSessions: 1})
	addSessionInState(t, sessions, "direct", visibleSession{kindDirect, StateReady})

	_, entry, _ := sessions.touch("direct", activatedAt())
	sessions.touch("direct", activatedAt())

	removed, found := sessions.remove("direct")
	if !found {
		t.Fatal("remove reported the session missing")
	}

	defer sessions.untrack()

	if open := sessions.openRequests(removed); open != 2 {
		t.Errorf("openRequests after two touches = %d, want 2", open)
	}

	// A request of the removed session still ends on its record.
	sessions.endAccess(entry, activatedAt())

	if open := sessions.openRequests(removed); open != 1 {
		t.Errorf("openRequests after one endAccess = %d, want 1", open)
	}
}
