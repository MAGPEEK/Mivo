package playback

import (
	"testing"
)

func TestRegistryActivateReportsAVanishedSession(t *testing.T) {
	t.Parallel()

	sessions := newRegistry(sessionLimits{maxReencoding: 1, maxFFmpeg: 1, maxSessions: 1})

	if _, _, found := sessions.activate("missing", activation{}); found {
		t.Error("activate of an unknown session reported found")
	}

	entry := &sessionEntry{session: Session{ID: "active"}}
	if err := sessions.reserve(entry); err != nil {
		t.Fatalf("reserve error = %v", err)
	}

	if _, _, found := sessions.activate("active", activation{}); !found {
		t.Fatal("first activate reported the session missing")
	}

	if _, _, found := sessions.activate("active", activation{}); found {
		t.Error("second activate of an active session reported found, want false")
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
