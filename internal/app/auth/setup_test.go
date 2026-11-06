package auth

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/account"
)

func TestNewServiceRequiresEveryDependency(t *testing.T) {
	t.Parallel()

	_, err := NewService(Dependencies{})
	if !errors.Is(err, domain.ErrInvalid) {
		t.Fatalf("NewService(empty) error = %v, want domain.ErrInvalid", err)
	}

	for _, name := range []string{"Setup", "Credentials", "Profiles", "Sessions", "Revoker", "PINs", "Selector", "Devices", "Hasher", "Tokens", "IDs", "Clock", "Compose"} {
		if !strings.Contains(err.Error(), name+" is required") {
			t.Errorf("error %q does not name missing %s", err, name)
		}
	}
}

func TestSetupCreatesTheAdminAndSignsIn(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	ctx := context.Background()

	required, err := h.service.SetupRequired(ctx)
	if err != nil || !required {
		t.Fatalf("SetupRequired before setup = %v, %v; want true", required, err)
	}

	result, err := h.service.Setup(ctx, signInRequest("  René   Admin ", "correct horse"))
	if err != nil {
		t.Fatalf("Setup: %v", err)
	}

	if result.Account.Name != "René Admin" || result.Account.Role != account.RoleAdmin {
		t.Fatalf("account = %+v, want composed, collapsed admin name", result.Account)
	}

	if !result.ProfileSelected || !result.Profile.Primary || result.Session.ProfileID != result.Profile.ID {
		t.Fatalf("setup did not select the primary profile: %+v", result)
	}

	if result.Token == "" || result.Session.TokenHash == result.Token {
		t.Fatalf("token %q / hash %q: the store must get only the hash", result.Token, result.Session.TokenHash)
	}

	if got := h.store.passwordHash(result.Account.ID); got != hashPrefix+"correct horse" {
		t.Fatalf("stored hash = %q", got)
	}

	required, err = h.service.SetupRequired(ctx)
	if err != nil || required {
		t.Fatalf("SetupRequired after setup = %v, %v; want false", required, err)
	}

	resolved, err := h.service.Resolve(ctx, result.Token, lanPeer())
	if err != nil || !resolved.Viewer.Admin {
		t.Fatalf("Resolve(setup token) = %+v, %v; want an admin viewer", resolved.Viewer, err)
	}
}

func TestSetupTwiceIsRefusedWithoutHashing(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	ctx := context.Background()

	if _, err := h.service.Setup(ctx, signInRequest("Admin", "correct horse")); err != nil {
		t.Fatalf("first Setup: %v", err)
	}

	hashesBefore, _, _ := h.hasher.counts()

	_, err := h.service.Setup(ctx, signInRequest("Intruder", "another password"))
	if !errors.Is(err, ErrSetupComplete) || !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("second Setup error = %v, want ErrSetupComplete (conflict)", err)
	}

	if hashesAfter, _, _ := h.hasher.counts(); hashesAfter != hashesBefore {
		t.Fatalf("second Setup hashed a password (%d -> %d)", hashesBefore, hashesAfter)
	}
}

func TestConcurrentSetupHasExactlyOneWinner(t *testing.T) {
	t.Parallel()

	const contenders = 8

	h := newHarness(t)

	var (
		wait      sync.WaitGroup
		mutex     sync.Mutex
		winners   int
		conflicts int
	)

	for index := range contenders {
		wait.Add(1)

		go func() {
			defer wait.Done()

			_, err := h.service.Setup(context.Background(), signInRequest(fmt.Sprintf("Admin %d", index), "correct horse"))

			mutex.Lock()
			defer mutex.Unlock()

			switch {
			case err == nil:
				winners++
			case errors.Is(err, ErrSetupComplete):
				conflicts++
			default:
				t.Errorf("Setup error = %v, want nil or ErrSetupComplete", err)
			}
		}()
	}

	wait.Wait()

	if winners != 1 || conflicts != contenders-1 {
		t.Fatalf("winners = %d, conflicts = %d; want 1 and %d", winners, conflicts, contenders-1)
	}
}

func TestSetupRejectsInvalidInput(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		request SignInRequest
	}{
		{"empty name", signInRequest("   ", "correct horse")},
		{"short password", signInRequest("Admin", "short")},
		{"unknown client", SignInRequest{Name: "Admin", Password: "correct horse", Device: Device{Client: "fax"}}},
		{"long device name", SignInRequest{
			Name: "Admin", Password: "correct horse",
			Device: Device{Client: ClientWeb, Name: strings.Repeat("x", MaxDeviceNameRunes+1)},
		}},
		{"control in platform", SignInRequest{
			Name: "Admin", Password: "correct horse",
			Device: Device{Client: ClientTV, Platform: "web\x00os"},
		}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			h := newHarness(t)

			_, err := h.service.Setup(context.Background(), test.request)
			if !errors.Is(err, domain.ErrInvalid) {
				t.Fatalf("Setup error = %v, want domain.ErrInvalid", err)
			}

			if required, _ := h.service.SetupRequired(context.Background()); !required {
				t.Fatal("an invalid setup created an account")
			}
		})
	}
}

func TestSetupReportsAHashFailure(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.hasher.failHash = errStoreDown

	_, err := h.service.Setup(context.Background(), signInRequest("Admin", "correct horse"))
	if !errors.Is(err, errStoreDown) {
		t.Fatalf("Setup error = %v, want the hash failure", err)
	}
}
