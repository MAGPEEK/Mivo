package auth

import (
	"context"
	"errors"
	"testing"

	"github.com/magpeek/mivo/internal/app/access"
	"github.com/magpeek/mivo/internal/domain"
	domainaccess "github.com/magpeek/mivo/internal/domain/access"
	"github.com/magpeek/mivo/internal/domain/account"
)

const newPassword = "brand new secret"

func TestChangePasswordNeedsThePrimaryProfile(t *testing.T) {
	t.Parallel()

	h, _ := withAlice(t)
	h.store.addProfile(account.Profile{ID: "kid", AccountID: "alice", Name: "Kid", AvatarColor: account.AvatarColorBlue})
	h.store.addAccount("bob", "Bob", hashPrefix+"bob secret", account.RoleUser)

	tests := []struct {
		name      string
		profileID string
	}{
		{"managed profile", "kid"},
		{"no profile", ""},
		{"primary of another account", "bob-primary"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			viewer := domainaccess.Viewer{AuthSessionID: "session", AccountID: "alice", ProfileID: test.profileID}
			ctx := access.WithViewer(context.Background(), viewer)

			err := h.service.ChangePassword(ctx, PasswordChange{Current: alicePassword, New: newPassword, Peer: lanPeer()})
			if !errors.Is(err, domain.ErrForbidden) {
				t.Fatalf("ChangePassword error = %v, want domain.ErrForbidden", err)
			}

			if got := h.store.passwordHash("alice"); got != hashPrefix+alicePassword {
				t.Fatalf("a forbidden change stored %q", got)
			}
		})
	}
}

func viewerContext(t *testing.T, h harness, token string) context.Context {
	t.Helper()

	resolution, err := h.service.Resolve(context.Background(), token, lanPeer())
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	return access.WithViewer(context.Background(), resolution.Viewer)
}

func TestChangePasswordRevokesTheOtherSessions(t *testing.T) {
	t.Parallel()

	h, _ := withAlice(t)
	current := signedIn(t, h, alicePassword)
	other := signedIn(t, h, alicePassword)

	// Cache the other session, so the test proves the eviction and not only the delete.
	if _, err := h.service.Resolve(context.Background(), other.Token, lanPeer()); err != nil {
		t.Fatalf("Resolve(other): %v", err)
	}

	ctx := viewerContext(t, h, current.Token)

	if err := h.service.ChangePassword(ctx, PasswordChange{Current: alicePassword, New: newPassword, Peer: lanPeer()}); err != nil {
		t.Fatalf("ChangePassword: %v", err)
	}

	if got := h.store.passwordHash("alice"); got != hashPrefix+newPassword {
		t.Fatalf("stored hash = %q", got)
	}

	expectUnauthenticated(t, h, other.Token)

	if _, err := h.service.Resolve(context.Background(), current.Token, lanPeer()); err != nil {
		t.Fatalf("current session after the change: %v", err)
	}

	if _, err := h.service.SignIn(context.Background(), signInRequest("Alice", newPassword)); err != nil {
		t.Fatalf("SignIn with the new password: %v", err)
	}
}

func TestChangePasswordRefusals(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		change   PasswordChange
		noViewer bool
		want     error
	}{
		{name: "no viewer", change: PasswordChange{Current: alicePassword, New: newPassword}, noViewer: true, want: domain.ErrUnauthenticated},
		{name: "wrong current", change: PasswordChange{Current: "wrong password", New: newPassword}, want: ErrInvalidCredentials},
		{name: "invalid new", change: PasswordChange{Current: alicePassword, New: "short"}, want: domain.ErrInvalid},
		{name: "oversized current", change: PasswordChange{Current: string(make([]byte, MaxSecretBytes+1)), New: newPassword}, want: ErrInvalidCredentials},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			h, _ := withAlice(t)
			result := signedIn(t, h, alicePassword)

			ctx := context.Background()
			if !test.noViewer {
				ctx = viewerContext(t, h, result.Token)
			}

			if err := h.service.ChangePassword(ctx, test.change); !errors.Is(err, test.want) {
				t.Fatalf("ChangePassword error = %v, want %v", err, test.want)
			}

			if got := h.store.passwordHash("alice"); got != hashPrefix+alicePassword {
				t.Fatalf("a refused change stored %q", got)
			}
		})
	}
}

func TestChangePasswordIsThrottled(t *testing.T) {
	t.Parallel()

	h, _ := withAlice(t)
	ctx := viewerContext(t, h, signedIn(t, h, alicePassword).Token)
	wrong := PasswordChange{Current: "wrong password", New: newPassword, Peer: lanPeer()}

	for range nameFailuresBeforeLock {
		_ = h.service.ChangePassword(ctx, wrong)
	}

	right := PasswordChange{Current: alicePassword, New: newPassword, Peer: lanPeer()}

	var throttled *ThrottledError
	if err := h.service.ChangePassword(ctx, right); !errors.As(err, &throttled) {
		t.Fatalf("ChangePassword after %d failures = %v, want *ThrottledError", nameFailuresBeforeLock, err)
	}
}

func TestChangePasswordReportsStoreFailures(t *testing.T) {
	t.Parallel()

	t.Run("hash", func(t *testing.T) {
		t.Parallel()

		h, _ := withAlice(t)
		ctx := viewerContext(t, h, signedIn(t, h, alicePassword).Token)
		h.hasher.failHash = errStoreDown

		err := h.service.ChangePassword(ctx, PasswordChange{Current: alicePassword, New: newPassword})
		if !errors.Is(err, errStoreDown) {
			t.Fatalf("ChangePassword error = %v, want the hash failure", err)
		}
	})

	t.Run("store", func(t *testing.T) {
		t.Parallel()

		h, _ := withAlice(t)
		ctx := viewerContext(t, h, signedIn(t, h, alicePassword).Token)
		h.store.failSetHash = errStoreDown

		err := h.service.ChangePassword(ctx, PasswordChange{Current: alicePassword, New: newPassword})
		if !errors.Is(err, errStoreDown) {
			t.Fatalf("ChangePassword error = %v, want the store failure", err)
		}
	})

	t.Run("account gone", func(t *testing.T) {
		t.Parallel()

		h, _ := withAlice(t)
		ctx := viewerContext(t, h, signedIn(t, h, alicePassword).Token)
		h.store.mutex.Lock()
		delete(h.store.credentials, "alice")
		h.store.mutex.Unlock()

		err := h.service.ChangePassword(ctx, PasswordChange{Current: alicePassword, New: newPassword})
		if !errors.Is(err, domain.ErrUnauthenticated) {
			t.Fatalf("ChangePassword error = %v, want domain.ErrUnauthenticated", err)
		}
	})
}
