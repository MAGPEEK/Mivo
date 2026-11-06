package auth

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/account"
)

func signedIn(t *testing.T, h harness, password string) SignInResult {
	t.Helper()

	result, err := h.service.SignIn(context.Background(), signInRequest("Alice", password))
	if err != nil {
		t.Fatalf("SignIn: %v", err)
	}

	return result
}

func expectUnauthenticated(t *testing.T, h harness, token string) {
	t.Helper()

	resolution, err := h.service.Resolve(context.Background(), token, lanPeer())
	if !errors.Is(err, domain.ErrUnauthenticated) {
		t.Fatalf("Resolve = %+v, %v; want domain.ErrUnauthenticated", resolution, err)
	}

	if errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("Resolve error %v also wraps ErrNotFound; HTTP could map it to 404", err)
	}
}

func TestResolveReturnsTheViewerAndCachesIt(t *testing.T) {
	t.Parallel()

	h, primary := withAlice(t)
	result := signedIn(t, h, alicePassword)

	first, err := h.service.Resolve(context.Background(), result.Token, lanPeer())
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	viewer := first.Viewer
	if viewer.AuthSessionID != result.Session.ID || viewer.AccountID != "alice" ||
		viewer.ProfileID != primary.ID || viewer.Admin || !viewer.Libraries.IsAll() {
		t.Fatalf("viewer = %+v", viewer)
	}

	if first.Account.ID != "alice" || !first.ProfileSelected || first.Profile.ID != primary.ID {
		t.Fatalf("resolution = %+v", first)
	}

	lookups := h.store.lookups()
	h.clock.Advance(CacheTTL - time.Second)

	if _, err := h.service.Resolve(context.Background(), result.Token, lanPeer()); err != nil {
		t.Fatalf("cached Resolve: %v", err)
	}

	if h.store.lookups() != lookups {
		t.Fatal("Resolve within the cache TTL read the store")
	}

	h.clock.Advance(time.Second)

	if _, err := h.service.Resolve(context.Background(), result.Token, lanPeer()); err != nil {
		t.Fatalf("Resolve after the TTL: %v", err)
	}

	if h.store.lookups() != lookups+1 {
		t.Fatal("Resolve after the cache TTL did not read the store")
	}
}

func TestResolveRefusesUnknownTokens(t *testing.T) {
	t.Parallel()

	h, _ := withAlice(t)

	expectUnauthenticated(t, h, "")
	expectUnauthenticated(t, h, "never-issued")
}

func TestResolveRefusesAGoneAccount(t *testing.T) {
	t.Parallel()

	h, _ := withAlice(t)
	result := signedIn(t, h, alicePassword)
	h.store.mutex.Lock()
	delete(h.store.credentials, "alice")
	h.store.mutex.Unlock()

	expectUnauthenticated(t, h, result.Token)
}

func TestResolveExpiresIdleAndOldSessions(t *testing.T) {
	t.Parallel()

	t.Run("idle", func(t *testing.T) {
		t.Parallel()

		h, _ := withAlice(t)
		result := signedIn(t, h, alicePassword)

		h.clock.Advance(IdleTimeout)
		expectUnauthenticated(t, h, result.Token)
	})

	t.Run("sliding idle timeout", func(t *testing.T) {
		t.Parallel()

		h, _ := withAlice(t)
		result := signedIn(t, h, alicePassword)

		h.clock.Advance(IdleTimeout - time.Hour)

		if _, err := h.service.Resolve(context.Background(), result.Token, lanPeer()); err != nil {
			t.Fatalf("Resolve before the idle timeout: %v", err)
		}

		h.clock.Advance(IdleTimeout - time.Hour)

		if _, err := h.service.Resolve(context.Background(), result.Token, lanPeer()); err != nil {
			t.Fatalf("Resolve after the touch slid the timeout: %v", err)
		}
	})

	t.Run("absolute", func(t *testing.T) {
		t.Parallel()

		h, _ := withAlice(t)
		result := signedIn(t, h, alicePassword)

		// Used every half idle timeout, so only the absolute lifetime can end it.
		step := IdleTimeout / 2
		elapsed := time.Duration(0)

		for ; elapsed+step < AbsoluteLifetime; elapsed += step {
			h.clock.Advance(step)

			if _, err := h.service.Resolve(context.Background(), result.Token, lanPeer()); err != nil {
				t.Fatalf("Resolve at %s: %v", elapsed+step, err)
			}
		}

		h.clock.Advance(AbsoluteLifetime - elapsed)
		expectUnauthenticated(t, h, result.Token)
	})
}

func TestResolveExpiresACachedSessionAtItsAbsoluteEnd(t *testing.T) {
	t.Parallel()

	h, _ := withAlice(t)
	result := signedIn(t, h, alicePassword)

	// A session close to its absolute end, served from the cache when the end passes.
	h.store.mutex.Lock()
	session := h.store.sessions[result.Session.TokenHash]
	session.CreatedAt = h.clock.Now().Add(-AbsoluteLifetime + time.Second)
	h.store.sessions[result.Session.TokenHash] = session
	h.store.mutex.Unlock()

	if _, err := h.service.Resolve(context.Background(), result.Token, lanPeer()); err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	h.clock.Advance(time.Second)
	expectUnauthenticated(t, h, result.Token)
}

func TestResolveTouchesAtMostEveryInterval(t *testing.T) {
	t.Parallel()

	h, _ := withAlice(t)
	result := signedIn(t, h, alicePassword)
	ctx := context.Background()

	for range 3 {
		h.clock.Advance(CacheTTL)

		if _, err := h.service.Resolve(ctx, result.Token, lanPeer()); err != nil {
			t.Fatalf("Resolve: %v", err)
		}
	}

	if len(h.store.touches) != 0 {
		t.Fatalf("touched %d times within %s", len(h.store.touches), TouchInterval)
	}

	h.clock.Advance(TouchInterval)

	if _, err := h.service.Resolve(ctx, result.Token, lanPeer()); err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	if len(h.store.touches) != 1 || !h.store.touches[0].At.Equal(h.clock.Now()) {
		t.Fatalf("touches = %+v, want one at now", h.store.touches)
	}

	remote := Peer{Address: otherPeer().Address, Class: AddressRemote}
	h.clock.Advance(CacheTTL)

	resolution, err := h.service.Resolve(ctx, result.Token, remote)
	if err != nil {
		t.Fatalf("Resolve from remote: %v", err)
	}

	if len(h.store.touches) != 2 || resolution.Session.AddressClass != AddressRemote {
		t.Fatalf("an address class change was not recorded: %+v", h.store.touches)
	}
}

func TestResolveSurvivesAFailedTouch(t *testing.T) {
	t.Parallel()

	h, _ := withAlice(t)
	result := signedIn(t, h, alicePassword)
	h.store.failTouch = errStoreDown
	h.clock.Advance(TouchInterval)

	if _, err := h.service.Resolve(context.Background(), result.Token, lanPeer()); err != nil {
		t.Fatalf("Resolve with a failing touch: %v", err)
	}
}

func TestResolveCacheIsEvicted(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		evict func(t *testing.T, h harness, result SignInResult)
	}{
		{"revoked device", func(_ *testing.T, h harness, result SignInResult) {
			_ = h.store.DeleteSession(context.Background(), result.Session.ID)
			h.service.EvictSession(result.Session.ID)
		}},
		{"revoked account sessions", func(t *testing.T, h harness, _ SignInResult) {
			if err := h.service.RevokeAccountSessions(context.Background(), "alice"); err != nil {
				t.Fatalf("RevokeAccountSessions: %v", err)
			}
		}},
		{"disabled account", func(_ *testing.T, h harness, _ SignInResult) {
			h.store.setDisabled("alice", true)
			h.service.EvictAccount("alice")
		}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			h, _ := withAlice(t)
			result := signedIn(t, h, alicePassword)

			if _, err := h.service.Resolve(context.Background(), result.Token, lanPeer()); err != nil {
				t.Fatalf("Resolve: %v", err)
			}

			test.evict(t, h, result)
			expectUnauthenticated(t, h, result.Token)
		})
	}
}

func TestResolveRefusesInputsOfAnotherAccount(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		session Session
		inputs  ViewerInputs
	}{
		{"account mismatch", Session{AccountID: "a"}, ViewerInputs{Account: account.Account{ID: "b"}}},
		{"disabled", Session{AccountID: "a"}, ViewerInputs{Account: account.Account{ID: "a", Disabled: true}}},
		{
			"profile mismatch",
			Session{AccountID: "a", ProfileID: "p"},
			ViewerInputs{Account: account.Account{ID: "a"}, Profile: account.Profile{ID: "q", AccountID: "a"}},
		},
		{
			"profile of another account",
			Session{AccountID: "a", ProfileID: "p"},
			ViewerInputs{Account: account.Account{ID: "a"}, Profile: account.Profile{ID: "p", AccountID: "b"}},
		},
	}

	for _, test := range tests {
		if err := checkInputs(test.session, test.inputs); !errors.Is(err, domain.ErrUnauthenticated) {
			t.Errorf("%s: checkInputs = %v, want domain.ErrUnauthenticated", test.name, err)
		}
	}

	if err := checkInputs(Session{AccountID: "a"}, ViewerInputs{Account: account.Account{ID: "a"}}); err != nil {
		t.Errorf("matching inputs: %v", err)
	}
}

func TestResolveWithoutProfileSeesNothing(t *testing.T) {
	t.Parallel()

	h, _ := withAlice(t)
	h.store.addProfile(account.Profile{ID: "kid", AccountID: "alice", Name: "Kid", AvatarColor: account.AvatarColorBlue})
	result := signedIn(t, h, alicePassword)

	resolution, err := h.service.Resolve(context.Background(), result.Token, lanPeer())
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	restriction := resolution.Viewer.Restriction()
	if resolution.ProfileSelected || resolution.Viewer.ProfileID != "" || !restriction.DeniesAll() || !restriction.HideUnrated() {
		t.Fatalf("viewer without profile = %+v, want no libraries", resolution.Viewer)
	}
}

func TestPurgeExpiredDeletesOldSessions(t *testing.T) {
	t.Parallel()

	h, _ := withAlice(t)
	old := signedIn(t, h, alicePassword)
	h.clock.Advance(IdleTimeout)
	fresh := signedIn(t, h, alicePassword)

	deleted, err := h.service.PurgeExpired(context.Background())
	if err != nil || deleted != 1 {
		t.Fatalf("PurgeExpired = %d, %v; want 1", deleted, err)
	}

	now := h.clock.Now()
	if !h.store.purgeCutoffs[0].Equal(now.Add(-IdleTimeout)) || !h.store.purgeCutoffs[1].Equal(now.Add(-AbsoluteLifetime)) {
		t.Fatalf("cutoffs = %v", h.store.purgeCutoffs)
	}

	expectUnauthenticated(t, h, old.Token)

	if _, err := h.service.Resolve(context.Background(), fresh.Token, lanPeer()); err != nil {
		t.Fatalf("fresh session after purge: %v", err)
	}
}
