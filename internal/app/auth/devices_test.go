package auth

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/account"
)

const (
	bobPassword  = "bob secret"
	rootPassword = "root secret"
)

// deviceFamily has Alice with two devices, Bob with one and the admin Root with one.
type deviceFamily struct {
	harness
	alice, aliceTV, bob, root SignInResult
}

func withDevices(t *testing.T) deviceFamily {
	t.Helper()

	h, _ := withAlice(t)
	h.store.addAccount("bob", "Bob", hashPrefix+bobPassword, account.RoleUser)
	h.store.addAccount("root", "Root", hashPrefix+rootPassword, account.RoleAdmin)

	signIn := func(name, password string) SignInResult {
		result, err := h.service.SignIn(context.Background(), signInRequest(name, password))
		if err != nil {
			t.Fatalf("SignIn %s: %v", name, err)
		}

		h.clock.Advance(time.Minute)

		return result
	}

	family := deviceFamily{harness: h}
	family.alice = signIn("Alice", alicePassword)
	family.bob = signIn("Bob", bobPassword)
	family.aliceTV = signIn("Alice", alicePassword)
	family.root = signIn("Root", rootPassword)

	return family
}

func deviceIDs(views []DeviceView) []string {
	ids := make([]string, 0, len(views))
	for _, view := range views {
		ids = append(ids, view.ID)
	}

	return ids
}

func TestListDevicesShowsOnlyOwnDevicesNewestFirst(t *testing.T) {
	t.Parallel()

	family := withDevices(t)

	views, err := family.service.ListDevices(viewerContext(t, family.harness, family.alice.Token))
	if err != nil {
		t.Fatalf("ListDevices: %v", err)
	}

	ids := deviceIDs(views)
	if len(ids) != 2 || ids[0] != family.aliceTV.Session.ID || ids[1] != family.alice.Session.ID {
		t.Fatalf("devices = %v, want Alice's TV then her first device", ids)
	}

	// The first device was not touched yet: last use is the sign-in.
	if !views[1].Current || views[0].Current || views[1].Device != webDevice() || views[1].AccountName != "" {
		t.Fatalf("views = %+v, want the caller's device marked", views)
	}
}

func TestListDevicesLeavesOutExpiredSessions(t *testing.T) {
	t.Parallel()

	family := withDevices(t)
	ctx := viewerContext(t, family.harness, family.alice.Token)

	family.clock.Advance(IdleTimeout - 3*time.Minute)

	views, err := family.service.ListDevices(ctx)
	if err != nil {
		t.Fatalf("ListDevices: %v", err)
	}

	if ids := deviceIDs(views); len(ids) != 1 || ids[0] != family.aliceTV.Session.ID {
		t.Fatalf("devices = %v, want only the recent TV session", ids)
	}
}

func TestListAllDevicesNeedsAdminPowers(t *testing.T) {
	t.Parallel()

	family := withDevices(t)

	if _, err := family.service.ListAllDevices(viewerContext(t, family.harness, family.alice.Token)); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("ListAllDevices as a user: %v, want domain.ErrForbidden", err)
	}

	views, err := family.service.ListAllDevices(viewerContext(t, family.harness, family.root.Token))
	if err != nil {
		t.Fatalf("ListAllDevices as admin: %v", err)
	}

	if len(views) != 4 || views[0].ID != family.root.Session.ID || !views[0].Current || views[0].AccountName != "Root" {
		t.Fatalf("all devices = %+v", views)
	}
}

func TestAdminPowersAreCheckedAgainstTheStore(t *testing.T) {
	t.Parallel()

	family := withDevices(t)
	ctx := viewerContext(t, family.harness, family.root.Token)

	family.store.setDisabled("root", true)

	if _, err := family.service.ListAllDevices(ctx); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("ListAllDevices by a disabled admin: %v, want domain.ErrForbidden", err)
	}

	if _, err := family.service.RevokeDevice(ctx, family.bob.Session.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("RevokeDevice by a disabled admin: %v, want domain.ErrNotFound", err)
	}

	delete(family.store.credentials, "root")

	if _, err := family.service.ListAllDevices(ctx); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("ListAllDevices by a gone admin: %v, want domain.ErrForbidden", err)
	}
}

func TestAdminManagedProfileHasNoAdminPowers(t *testing.T) {
	t.Parallel()

	family := withDevices(t)
	family.store.addProfile(account.Profile{ID: "guest", AccountID: "root", Name: "Guest", AvatarColor: account.DefaultAvatarColor})
	ctx := viewerContext(t, family.harness, family.root.Token)

	if _, err := family.service.SelectProfile(ctx, ProfileSelection{ProfileID: "guest"}); err != nil {
		t.Fatalf("SelectProfile: %v", err)
	}

	guest := viewerContext(t, family.harness, family.root.Token)

	if _, err := family.service.ListAllDevices(guest); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("ListAllDevices from a managed profile: %v, want domain.ErrForbidden", err)
	}

	if err := family.service.ClearProfile(guest); err != nil {
		t.Fatalf("ClearProfile: %v", err)
	}

	if _, err := family.service.ListAllDevices(viewerContext(t, family.harness, family.root.Token)); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("ListAllDevices without a profile: %v, want domain.ErrForbidden", err)
	}
}

func TestRevokeIsScopedToTheOwnAccount(t *testing.T) {
	t.Parallel()

	family := withDevices(t)
	ctx := viewerContext(t, family.harness, family.alice.Token)

	for _, revoke := range []func(context.Context, string) (bool, error){
		family.service.RevokeOwnDevice, family.service.RevokeDevice,
	} {
		for _, deviceID := range []string{family.bob.Session.ID, "unknown"} {
			if _, err := revoke(ctx, deviceID); !errors.Is(err, domain.ErrNotFound) {
				t.Fatalf("revoke %s: %v, want domain.ErrNotFound", deviceID, err)
			}
		}
	}

	if _, ok := family.store.session(family.bob.Session.ID); !ok {
		t.Fatal("Bob's session was deleted by Alice")
	}

	// Warm the cache, so the revoke must evict it.
	if _, err := family.service.Resolve(context.Background(), family.aliceTV.Token, lanPeer()); err != nil {
		t.Fatalf("Resolve TV: %v", err)
	}

	signedOut, err := family.service.RevokeOwnDevice(ctx, family.aliceTV.Session.ID)
	if err != nil || signedOut {
		t.Fatalf("RevokeOwnDevice(TV) = %t, %v; want the other device revoked", signedOut, err)
	}

	expectUnauthenticated(t, family.harness, family.aliceTV.Token)
}

func TestRevokeTheCurrentDeviceSignsOut(t *testing.T) {
	t.Parallel()

	family := withDevices(t)

	signedOut, err := family.service.RevokeDevice(viewerContext(t, family.harness, family.alice.Token), family.alice.Session.ID)
	if err != nil || !signedOut {
		t.Fatalf("RevokeDevice(own) = %t, %v; want signed out", signedOut, err)
	}

	expectUnauthenticated(t, family.harness, family.alice.Token)
}

func TestAdminRevokesAnyDeviceOnlyThroughRevokeDevice(t *testing.T) {
	t.Parallel()

	family := withDevices(t)
	ctx := viewerContext(t, family.harness, family.root.Token)

	if _, err := family.service.RevokeOwnDevice(ctx, family.bob.Session.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("RevokeOwnDevice(Bob) by admin: %v, want domain.ErrNotFound", err)
	}

	if _, err := family.service.Resolve(context.Background(), family.bob.Token, lanPeer()); err != nil {
		t.Fatalf("Resolve Bob: %v", err)
	}

	signedOut, err := family.service.RevokeDevice(ctx, family.bob.Session.ID)
	if err != nil || signedOut {
		t.Fatalf("RevokeDevice(Bob) by admin = %t, %v", signedOut, err)
	}

	expectUnauthenticated(t, family.harness, family.bob.Token)
}

func TestDevicesReportStoreFailures(t *testing.T) {
	t.Parallel()

	family := withDevices(t)
	aliceCtx := viewerContext(t, family.harness, family.alice.Token)
	rootCtx := viewerContext(t, family.harness, family.root.Token)

	family.store.failDevices = errStoreDown

	if _, err := family.service.ListDevices(aliceCtx); !errors.Is(err, errStoreDown) {
		t.Errorf("ListDevices: %v, want the store error", err)
	}

	if _, err := family.service.ListAllDevices(rootCtx); !errors.Is(err, errStoreDown) {
		t.Errorf("ListAllDevices: %v, want the store error", err)
	}
}
