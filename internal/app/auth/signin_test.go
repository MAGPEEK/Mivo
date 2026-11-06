package auth

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/account"
)

const alicePassword = "alice secret"

func withAlice(t *testing.T) (harness, account.Profile) {
	t.Helper()

	h := newHarness(t)
	primary := h.store.addAccount("alice", "Alice", hashPrefix+alicePassword, account.RoleUser)

	return h, primary
}

func TestSignInStartsASessionWithTheAutoSelectedProfile(t *testing.T) {
	t.Parallel()

	h, primary := withAlice(t)

	result, err := h.service.SignIn(context.Background(), signInRequest("  ALICE ", alicePassword))
	if err != nil {
		t.Fatalf("SignIn: %v", err)
	}

	if result.Account.ID != "alice" || !result.ProfileSelected || result.Profile.ID != primary.ID {
		t.Fatalf("SignIn = %+v, want alice with the primary profile selected", result)
	}

	session := result.Session
	if session.Device != webDevice() || session.AddressClass != AddressLAN || !session.Remember ||
		!session.CreatedAt.Equal(h.clock.Now()) || !session.LastSeenAt.Equal(h.clock.Now()) {
		t.Fatalf("session = %+v", session)
	}

	stored, err := h.store.SessionByTokenHash(context.Background(), "sha:"+result.Token)
	if err != nil || stored.ID != session.ID {
		t.Fatalf("stored session = %+v, %v", stored, err)
	}
}

func TestSignInLeavesThePickerWhenTheProfileCannotBeAutoSelected(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		setup func(h harness, primary account.Profile)
	}{
		{"managed profile exists", func(h harness, _ account.Profile) {
			h.store.addProfile(account.Profile{ID: "kid", AccountID: "alice", Name: "Kid", AvatarColor: account.AvatarColorBlue})
		}},
		{"primary has a PIN", func(h harness, primary account.Profile) {
			primary.HasPIN = true
			h.store.profiles["alice"] = []account.Profile{primary}
		}},
		{"no profile at all", func(h harness, _ account.Profile) {
			h.store.profiles["alice"] = nil
		}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			h, primary := withAlice(t)
			test.setup(h, primary)

			result, err := h.service.SignIn(context.Background(), signInRequest("Alice", alicePassword))
			if err != nil {
				t.Fatalf("SignIn: %v", err)
			}

			if result.ProfileSelected || result.Session.ProfileID != "" || result.Session.Remember {
				t.Fatalf("SignIn selected a profile: %+v", result)
			}
		})
	}
}

func TestSignInFailuresLookAlike(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		username    string
		password    string
		wantVerify  bool
		wantDummy   bool
		prepareFake func(h harness)
	}{
		{name: "wrong password", username: "Alice", password: "wrong password", wantVerify: true},
		{name: "unknown name", username: "Bob", password: alicePassword, wantDummy: true},
		{name: "unusable name", username: string(rune(0x200B)), password: alicePassword, wantDummy: true},
		{name: "oversized name", username: strings.Repeat("a", maxNameInputBytes+1), password: alicePassword, wantDummy: true},
		{name: "oversized password", username: "Alice", password: strings.Repeat("p", MaxSecretBytes+1)},
		{
			name: "disabled account", username: "Alice", password: alicePassword, wantVerify: true,
			prepareFake: func(h harness) { h.store.setDisabled("alice", true) },
		},
		{
			name: "corrupt stored hash", username: "Alice", password: alicePassword, wantVerify: true,
			prepareFake: func(h harness) { _ = h.store.SetPasswordHash(context.Background(), "alice", "garbage") },
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			h, _ := withAlice(t)
			if test.prepareFake != nil {
				test.prepareFake(h)
			}

			_, err := h.service.SignIn(context.Background(), signInRequest(test.username, test.password))
			if !errors.Is(err, ErrInvalidCredentials) || !errors.Is(err, domain.ErrUnauthenticated) {
				t.Fatalf("SignIn error = %v, want ErrInvalidCredentials", err)
			}

			if err.Error() != ErrInvalidCredentials.Error() {
				t.Fatalf("error text %q differs from %q and could reveal the cause", err, ErrInvalidCredentials)
			}

			_, verifies, dummies := h.hasher.counts()
			if (verifies > 0) != test.wantVerify || (dummies > 0) != test.wantDummy {
				t.Fatalf("verifies = %d, dummies = %d; want verify %v, dummy %v", verifies, dummies, test.wantVerify, test.wantDummy)
			}
		})
	}
}

func TestSignInRejectsAnInvalidDevice(t *testing.T) {
	t.Parallel()

	h, _ := withAlice(t)

	_, err := h.service.SignIn(context.Background(), SignInRequest{Name: "Alice", Password: alicePassword})
	if !errors.Is(err, domain.ErrInvalid) {
		t.Fatalf("SignIn error = %v, want domain.ErrInvalid", err)
	}
}

func TestSignInStoreFailureIsReportedAndNotCounted(t *testing.T) {
	t.Parallel()

	h, _ := withAlice(t)
	h.store.failLookup = errStoreDown

	for range nameFailuresBeforeLock + 1 {
		_, err := h.service.SignIn(context.Background(), signInRequest("Alice", "wrong password"))
		if !errors.Is(err, errStoreDown) {
			t.Fatalf("SignIn error = %v, want the store failure", err)
		}
	}

	h.store.failLookup = nil

	if _, err := h.service.SignIn(context.Background(), signInRequest("Alice", alicePassword)); err != nil {
		t.Fatalf("SignIn after store failures: %v (aborted attempts must not count)", err)
	}
}

func TestSignInCancelledVerifyIsAborted(t *testing.T) {
	t.Parallel()

	h, _ := withAlice(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := h.service.SignIn(ctx, signInRequest("Alice", alicePassword))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("SignIn error = %v, want context.Canceled", err)
	}

	_, err = h.service.SignIn(ctx, signInRequest("Nobody", alicePassword))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("SignIn(unknown) error = %v, want context.Canceled from DummyVerify", err)
	}
}

func TestSignInRehashesAWeakHash(t *testing.T) {
	t.Parallel()

	h, _ := withAlice(t)
	_ = h.store.SetPasswordHash(context.Background(), "alice", weakPrefix+alicePassword)

	if _, err := h.service.SignIn(context.Background(), signInRequest("Alice", alicePassword)); err != nil {
		t.Fatalf("SignIn: %v", err)
	}

	if got := h.store.passwordHash("alice"); got != hashPrefix+alicePassword {
		t.Fatalf("hash after sign-in = %q, want the rehashed %q", got, hashPrefix+alicePassword)
	}
}

func TestSignInSurvivesAFailedRehash(t *testing.T) {
	t.Parallel()

	var logs bytes.Buffer

	h, _ := withAlice(t)
	h.service.deps.Logger = slog.New(slog.NewTextHandler(&logs, nil))
	_ = h.store.SetPasswordHash(context.Background(), "alice", weakPrefix+alicePassword)
	h.store.failSetHash = errStoreDown

	if _, err := h.service.SignIn(context.Background(), signInRequest("Alice", alicePassword)); err != nil {
		t.Fatalf("SignIn with a failing rehash: %v", err)
	}

	if !strings.Contains(logs.String(), "rehash password") || strings.Contains(logs.String(), alicePassword) {
		t.Fatalf("log = %q, want a rehash warning without the password", logs.String())
	}
}

func TestSignOutDeletesTheSessionAndIsIdempotent(t *testing.T) {
	t.Parallel()

	h, _ := withAlice(t)
	ctx := context.Background()

	result, err := h.service.SignIn(ctx, signInRequest("Alice", alicePassword))
	if err != nil {
		t.Fatalf("SignIn: %v", err)
	}

	if _, err := h.service.Resolve(ctx, result.Token, lanPeer()); err != nil {
		t.Fatalf("Resolve before sign-out: %v", err)
	}

	for _, token := range []string{result.Token, result.Token, "", "unknown"} {
		if err := h.service.SignOut(ctx, token); err != nil {
			t.Fatalf("SignOut(%q): %v", token, err)
		}
	}

	_, err = h.service.Resolve(ctx, result.Token, lanPeer())
	if !errors.Is(err, domain.ErrUnauthenticated) {
		t.Fatalf("Resolve after sign-out error = %v, want domain.ErrUnauthenticated (cache evicted)", err)
	}
}

func TestSignInThrottlesPerNameWithBackOff(t *testing.T) {
	t.Parallel()

	h, _ := withAlice(t)
	ctx := context.Background()

	for range nameFailuresBeforeLock {
		if _, err := h.service.SignIn(ctx, signInRequest("Alice", "wrong password")); !errors.Is(err, ErrInvalidCredentials) {
			t.Fatalf("failure before the lock: %v", err)
		}
	}

	// Locked: even the right password, typed differently and from another peer, is refused.
	request := signInRequest("alice ", alicePassword)
	request.Peer = otherPeer()
	expectThrottled(t, h, request, nameFirstLock)

	for _, wantLock := range []time.Duration{2 * time.Minute, 4 * time.Minute, 8 * time.Minute, nameMaxLock, nameMaxLock} {
		h.clock.Advance(nameMaxLock)

		if _, err := h.service.SignIn(ctx, signInRequest("Alice", "wrong password")); !errors.Is(err, ErrInvalidCredentials) {
			t.Fatalf("failure after the lock ended: %v", err)
		}

		expectThrottled(t, h, signInRequest("Alice", alicePassword), wantLock)
	}

	h.clock.Advance(nameMaxLock)

	if _, err := h.service.SignIn(ctx, signInRequest("Alice", alicePassword)); err != nil {
		t.Fatalf("SignIn after the lock: %v", err)
	}

	// The success cleared the counter: one more failure does not lock again.
	_, _ = h.service.SignIn(ctx, signInRequest("Alice", "wrong password"))

	if _, err := h.service.SignIn(ctx, signInRequest("Alice", alicePassword)); err != nil {
		t.Fatalf("SignIn after a reset: %v", err)
	}
}

func TestSignInForgetsOldNameFailures(t *testing.T) {
	t.Parallel()

	h, _ := withAlice(t)
	ctx := context.Background()

	for range nameFailuresBeforeLock - 1 {
		_, _ = h.service.SignIn(ctx, signInRequest("Alice", "wrong password"))
	}

	h.clock.Advance(nameFailureMemory + time.Second)
	_, _ = h.service.SignIn(ctx, signInRequest("Alice", "wrong password"))

	if _, err := h.service.SignIn(ctx, signInRequest("Alice", alicePassword)); err != nil {
		t.Fatalf("SignIn after forgotten failures: %v", err)
	}
}

func TestSignInThrottlesPerPeer(t *testing.T) {
	t.Parallel()

	h, _ := withAlice(t)
	ctx := context.Background()

	for index := range peerFailureLimit {
		// Distinct unknown names, so the name lock never triggers.
		name := "Guess " + strings.Repeat("x", index+1)
		if _, err := h.service.SignIn(ctx, signInRequest(name, "whatever pw")); !errors.Is(err, ErrInvalidCredentials) {
			t.Fatalf("failure %d: %v", index, err)
		}
	}

	h.clock.Advance(time.Minute)
	expectThrottled(t, h, signInRequest("Alice", alicePassword), peerWindow-time.Minute)

	other := signInRequest("Alice", alicePassword)
	other.Peer = otherPeer()

	if _, err := h.service.SignIn(ctx, other); err != nil {
		t.Fatalf("SignIn from another peer: %v", err)
	}

	h.clock.Advance(peerWindow)

	if _, err := h.service.SignIn(ctx, signInRequest("Alice", alicePassword)); err != nil {
		t.Fatalf("SignIn after the peer window: %v", err)
	}
}

func expectThrottled(t *testing.T, h harness, request SignInRequest, wantWait time.Duration) {
	t.Helper()

	_, verifiesBefore, _ := h.hasher.counts()

	_, err := h.service.SignIn(context.Background(), request)

	var throttled *ThrottledError
	if !errors.As(err, &throttled) || !errors.Is(err, domain.ErrRateLimited) {
		t.Fatalf("SignIn error = %v, want a *ThrottledError", err)
	}

	if throttled.RetryAfter() != wantWait {
		t.Fatalf("RetryAfter = %s, want %s", throttled.RetryAfter(), wantWait)
	}

	if _, verifiesAfter, _ := h.hasher.counts(); verifiesAfter != verifiesBefore {
		t.Fatal("a throttled attempt reached the hasher")
	}
}

func TestThrottledErrorRetryAfterSeconds(t *testing.T) {
	t.Parallel()

	tests := []struct {
		wait time.Duration
		want int
	}{
		{0, 1},
		{300 * time.Millisecond, 1},
		{time.Minute, 60},
		{61500 * time.Millisecond, 62},
	}

	for _, test := range tests {
		throttled := &ThrottledError{Wait: test.wait}
		if got := throttled.RetryAfterSeconds(); got != test.want {
			t.Errorf("RetryAfterSeconds(%s) = %d, want %d", test.wait, got, test.want)
		}

		if !strings.Contains(throttled.Error(), "retry after") {
			t.Errorf("Error() = %q", throttled.Error())
		}
	}
}
