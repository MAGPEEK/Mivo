package auth

import (
	"context"
	"errors"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/magpeek/mivo/internal/app/access"
	"github.com/magpeek/mivo/internal/domain"
	domainaccess "github.com/magpeek/mivo/internal/domain/access"
	"github.com/magpeek/mivo/internal/domain/account"
)

const kidPIN = "1234"

// withFamily returns Alice with her primary profile and the managed profiles "Zoe" (no PIN)
// and "Kid" (PIN kidPIN), signed in without a selected profile.
func withFamily(t *testing.T) (harness, SignInResult, account.Profile) {
	t.Helper()

	h, _ := withAlice(t)
	h.store.addProfile(account.Profile{ID: "zoe", AccountID: "alice", Name: "Zoe", AvatarColor: account.DefaultAvatarColor})
	h.store.addProfile(account.Profile{ID: "kid", AccountID: "alice", Name: "Kid", AvatarColor: account.DefaultAvatarColor})
	h.store.setPIN("kid", kidPIN)

	result := signedIn(t, h, alicePassword)
	if result.ProfileSelected {
		t.Fatalf("sign-in selected %+v despite managed profiles", result.Profile)
	}

	kid := account.Profile{ID: "kid", AccountID: "alice", Name: "Kid", AvatarColor: account.DefaultAvatarColor, HasPIN: true}

	return h, result, kid
}

func selectKid(ctx context.Context, h harness, pin string) error {
	_, err := h.service.SelectProfile(ctx, ProfileSelection{ProfileID: "kid", PIN: pin})

	return err
}

func TestMeListsThePickerWithoutAProfile(t *testing.T) {
	t.Parallel()

	h, result, _ := withFamily(t)

	me, err := h.service.Me(viewerContext(t, h, result.Token))
	if err != nil {
		t.Fatalf("Me: %v", err)
	}

	if me.Account.ID != "alice" || me.ProfileSelected || me.Profile.ID != "" {
		t.Fatalf("Me = %+v, want Alice without profile", me)
	}

	var order []string
	for _, picked := range me.Profiles {
		order = append(order, picked.Profile.ID)
	}

	if len(order) != 3 || order[0] != "alice-primary" || order[1] != "kid" || order[2] != "zoe" {
		t.Fatalf("picker order = %v, want primary, then managed by name", order)
	}

	if !me.Profiles[1].Profile.HasPIN || !me.Profiles[0].Profile.Primary || me.Profiles[0].Admin {
		t.Fatalf("picker flags = %+v", me.Profiles)
	}
}

func TestMeMarksTheAdminProfile(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.store.addAccount("root", "Root", hashPrefix+alicePassword, account.RoleAdmin)
	h.store.addProfile(account.Profile{ID: "guest", AccountID: "root", Name: "Guest", AvatarColor: account.DefaultAvatarColor})

	result, err := h.service.SignIn(context.Background(), signInRequest("Root", alicePassword))
	if err != nil {
		t.Fatalf("SignIn: %v", err)
	}

	ctx := viewerContext(t, h, result.Token)

	me, err := h.service.SelectProfile(ctx, ProfileSelection{ProfileID: "root-primary"})
	if err != nil {
		t.Fatalf("SelectProfile: %v", err)
	}

	if !me.ProfileSelected || me.Profile.ID != "root-primary" || !me.Profiles[0].Admin || me.Profiles[1].Admin {
		t.Fatalf("Me = %+v, want only the primary profile admin", me)
	}
}

func TestSelectProfileWithoutPINRemembersByDefault(t *testing.T) {
	t.Parallel()

	h, result, _ := withFamily(t)
	ctx := viewerContext(t, h, result.Token)

	me, err := h.service.SelectProfile(ctx, ProfileSelection{ProfileID: "zoe"})
	if err != nil {
		t.Fatalf("SelectProfile: %v", err)
	}

	if !me.ProfileSelected || me.Profile.ID != "zoe" {
		t.Fatalf("Me = %+v, want zoe selected", me)
	}

	if session, _ := h.store.session(result.Session.ID); session.ProfileID != "zoe" || !session.Remember {
		t.Fatalf("stored session = %+v, want zoe remembered", session)
	}

	resolution, err := h.service.Resolve(context.Background(), result.Token, lanPeer())
	if err != nil || resolution.Viewer.ProfileID != "zoe" {
		t.Fatalf("Resolve after select = %+v, %v; want profile zoe (cache evicted)", resolution.Viewer, err)
	}
}

func TestSelectProfileLocksAfterFiveWrongPINs(t *testing.T) {
	t.Parallel()

	h, result, _ := withFamily(t)
	ctx := viewerContext(t, h, result.Token)

	for attempt := 1; attempt < PINFailuresBeforeLock; attempt++ {
		if err := selectKid(ctx, h, "0000"); !errors.Is(err, ErrPINInvalid) || !errors.Is(err, domain.ErrForbidden) {
			t.Fatalf("wrong PIN %d: %v, want ErrPINInvalid", attempt, err)
		}
	}

	var locked *ProfileLockedError
	if err := selectKid(ctx, h, "0000"); !errors.As(err, &locked) || locked.Wait != PINLockDuration {
		t.Fatalf("fifth wrong PIN: %v, want ProfileLockedError for %s", err, PINLockDuration)
	}

	_, verifies, _ := h.hasher.counts()

	h.clock.Advance(PINLockDuration - time.Second)

	err := selectKid(ctx, h, kidPIN)
	if !errors.As(err, &locked) || !errors.Is(err, domain.ErrRateLimited) || locked.RetryAfterSeconds() != 1 {
		t.Fatalf("right PIN while locked: %v, want locked with 1 s left", err)
	}

	if _, after, _ := h.hasher.counts(); after != verifies {
		t.Fatalf("a locked profile ran %d verifies", after-verifies)
	}

	h.clock.Advance(time.Second)

	if err := selectKid(ctx, h, kidPIN); err != nil {
		t.Fatalf("right PIN after the lock: %v", err)
	}

	if session, _ := h.store.session(result.Session.ID); session.ProfileID != "kid" || session.Remember {
		t.Fatalf("stored session = %+v, want kid not remembered", session)
	}
}

func TestSelectProfileGivesAFreshBudgetAfterTheLock(t *testing.T) {
	t.Parallel()

	h, result, _ := withFamily(t)
	ctx := viewerContext(t, h, result.Token)

	for range PINFailuresBeforeLock {
		_ = selectKid(ctx, h, "0000")
	}

	h.clock.Advance(PINLockDuration)

	for attempt := 1; attempt < PINFailuresBeforeLock; attempt++ {
		if err := selectKid(ctx, h, "0000"); !errors.Is(err, ErrPINInvalid) {
			t.Fatalf("wrong PIN %d after the lock: %v, want ErrPINInvalid", attempt, err)
		}
	}
}

func TestSelectProfileSuccessResetsTheCounter(t *testing.T) {
	t.Parallel()

	h, result, _ := withFamily(t)
	ctx := viewerContext(t, h, result.Token)

	for range 2 {
		for attempt := 1; attempt < PINFailuresBeforeLock; attempt++ {
			if err := selectKid(ctx, h, "9999"); !errors.Is(err, ErrPINInvalid) {
				t.Fatalf("wrong PIN %d: %v, want ErrPINInvalid", attempt, err)
			}
		}

		if err := selectKid(ctx, h, kidPIN); err != nil {
			t.Fatalf("right PIN: %v", err)
		}
	}
}

func TestSelectProfileForgetsOldFailures(t *testing.T) {
	t.Parallel()

	h, result, _ := withFamily(t)
	ctx := viewerContext(t, h, result.Token)

	for attempt := 1; attempt < PINFailuresBeforeLock; attempt++ {
		_ = selectKid(ctx, h, "0000")
	}

	h.clock.Advance(pinFailureMemory + time.Second)

	if err := selectKid(ctx, h, "0000"); !errors.Is(err, ErrPINInvalid) {
		t.Fatalf("wrong PIN after the memory: %v, want ErrPINInvalid, not a lock", err)
	}
}

func TestSelectProfilePINInputs(t *testing.T) {
	t.Parallel()

	h, result, _ := withFamily(t)
	ctx := viewerContext(t, h, result.Token)

	// A missing PIN only tells the client one is needed and never locks.
	for range PINFailuresBeforeLock + 1 {
		if err := selectKid(ctx, h, ""); !errors.Is(err, ErrPINInvalid) {
			t.Fatalf("missing PIN: %v, want ErrPINInvalid", err)
		}
	}

	_, verifies, _ := h.hasher.counts()

	// Malformed PINs count but never reach the hasher.
	for _, pin := range []string{"12345", "abcd", "１２３４"} {
		if err := selectKid(ctx, h, pin); !errors.Is(err, ErrPINInvalid) {
			t.Fatalf("PIN %q: %v, want ErrPINInvalid", pin, err)
		}
	}

	if _, after, _ := h.hasher.counts(); after != verifies {
		t.Fatalf("malformed PINs ran %d verifies", after-verifies)
	}

	_ = selectKid(ctx, h, "0000")

	var locked *ProfileLockedError
	if err := selectKid(ctx, h, "0001"); !errors.As(err, &locked) {
		t.Fatalf("fifth counted failure: %v, want ProfileLockedError", err)
	}
}

func TestSelectProfileRefusesAnotherAccountsProfile(t *testing.T) {
	t.Parallel()

	h, result, _ := withFamily(t)
	h.store.addAccount("bob", "Bob", hashPrefix+"bob secret", account.RoleUser)
	h.store.setPIN("bob-primary", kidPIN)
	ctx := viewerContext(t, h, result.Token)

	for _, profileID := range []string{"bob-primary", "missing", ""} {
		_, err := h.service.SelectProfile(ctx, ProfileSelection{ProfileID: profileID, PIN: kidPIN})
		if !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("select %q: %v, want domain.ErrNotFound", profileID, err)
		}
	}

	if _, verifies, _ := h.hasher.counts(); verifies != 1 {
		t.Fatalf("verifies = %d, want only the sign-in", verifies)
	}

	if session, _ := h.store.session(result.Session.ID); session.ProfileID != "" {
		t.Fatalf("session profile = %q, want none", session.ProfileID)
	}
}

func TestSelectProfileCapsParallelGuesses(t *testing.T) {
	t.Parallel()

	h, result, _ := withFamily(t)
	ctx := viewerContext(t, h, result.Token)

	for attempt := 1; attempt < PINFailuresBeforeLock; attempt++ {
		_ = selectKid(ctx, h, "0000")
	}

	_, before, _ := h.hasher.counts()

	gate := make(chan struct{})
	h.hasher.mutex.Lock()
	h.hasher.gate = gate
	h.hasher.mutex.Unlock()

	var (
		waiting  sync.WaitGroup
		firstErr error
	)

	waiting.Go(func() { firstErr = selectKid(ctx, h, "0000") })

	// Wait until the first guess is inside Verify, holding the last remaining try.
	for {
		if _, verifies, _ := h.hasher.counts(); verifies > before {
			break
		}

		runtime.Gosched()
	}

	var locked *ProfileLockedError
	if err := selectKid(ctx, h, "0001"); !errors.As(err, &locked) || locked.Wait != inFlightRetry {
		t.Fatalf("parallel guess: %v, want the in-flight refusal", err)
	}

	close(gate)
	waiting.Wait()

	if !errors.As(firstErr, &locked) || locked.Wait != PINLockDuration {
		t.Fatalf("first guess: %v, want it to lock the profile", firstErr)
	}
}

func TestSelectionWithoutRememberLapses(t *testing.T) {
	t.Parallel()

	h, result, _ := withFamily(t)

	if err := selectKid(viewerContext(t, h, result.Token), h, kidPIN); err != nil {
		t.Fatalf("SelectProfile: %v", err)
	}

	// Requests keep the selection alive: the lapse slides with last use.
	for range 2 {
		h.clock.Advance(SelectionLapse - time.Hour)

		resolution, err := h.service.Resolve(context.Background(), result.Token, lanPeer())
		if err != nil || !resolution.ProfileSelected {
			t.Fatalf("Resolve while in use = %+v, %v; want kid selected", resolution, err)
		}
	}

	h.clock.Advance(SelectionLapse)

	resolution, err := h.service.Resolve(context.Background(), result.Token, lanPeer())
	if err != nil || resolution.ProfileSelected || resolution.Viewer.ProfileID != "" {
		t.Fatalf("Resolve after the lapse = %+v, %v; want no profile", resolution, err)
	}

	if session, _ := h.store.session(result.Session.ID); session.ProfileID != "" {
		t.Fatalf("stored profile = %q after the lapse, want cleared", session.ProfileID)
	}
}

func TestRememberedSelectionDoesNotLapse(t *testing.T) {
	t.Parallel()

	h, result, _ := withFamily(t)
	remember := true

	_, err := h.service.SelectProfile(viewerContext(t, h, result.Token),
		ProfileSelection{ProfileID: "kid", PIN: kidPIN, Remember: &remember})
	if err != nil {
		t.Fatalf("SelectProfile: %v", err)
	}

	h.clock.Advance(2 * SelectionLapse)

	resolution, err := h.service.Resolve(context.Background(), result.Token, lanPeer())
	if err != nil || resolution.Viewer.ProfileID != "kid" {
		t.Fatalf("Resolve = %+v, %v; want kid still selected", resolution.Viewer, err)
	}
}

func TestLapseSurvivesAFailedClear(t *testing.T) {
	t.Parallel()

	h, result, _ := withFamily(t)

	if err := selectKid(viewerContext(t, h, result.Token), h, kidPIN); err != nil {
		t.Fatalf("SelectProfile: %v", err)
	}

	h.store.failSelect = errStoreDown

	h.clock.Advance(SelectionLapse)

	resolution, err := h.service.Resolve(context.Background(), result.Token, lanPeer())
	if err != nil || resolution.ProfileSelected {
		t.Fatalf("Resolve = %+v, %v; want no profile even when the clear fails", resolution, err)
	}
}

func TestClearProfileShowsThePickerAgain(t *testing.T) {
	t.Parallel()

	h, primary := withAlice(t)
	result := signedIn(t, h, alicePassword)
	ctx := viewerContext(t, h, result.Token)

	if viewer, err := RequireProfile(ctx); err != nil || viewer.ProfileID != primary.ID {
		t.Fatalf("RequireProfile = %+v, %v; want the primary profile", viewer, err)
	}

	if err := h.service.ClearProfile(ctx); err != nil {
		t.Fatalf("ClearProfile: %v", err)
	}

	cleared := viewerContext(t, h, result.Token)

	_, err := RequireProfile(cleared)
	if !errors.Is(err, ErrProfileRequired) || !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("RequireProfile after clear: %v, want ErrProfileRequired", err)
	}

	me, err := h.service.Me(cleared)
	if err != nil || me.ProfileSelected {
		t.Fatalf("Me after clear = %+v, %v; want no profile", me, err)
	}
}

func TestSelectionUseCasesNeedASession(t *testing.T) {
	t.Parallel()

	h, _ := withAlice(t)
	noViewer := context.Background()
	noSession := access.WithViewer(context.Background(), viewerWithout("alice", ""))

	if _, err := RequireProfile(noViewer); !errors.Is(err, domain.ErrUnauthenticated) {
		t.Errorf("RequireProfile: %v", err)
	}

	for _, ctx := range []context.Context{noViewer, noSession} {
		if _, err := h.service.Me(ctx); !errors.Is(err, domain.ErrUnauthenticated) {
			t.Errorf("Me: %v", err)
		}

		if _, err := h.service.SelectProfile(ctx, ProfileSelection{ProfileID: "alice-primary"}); !errors.Is(err, domain.ErrUnauthenticated) {
			t.Errorf("SelectProfile: %v", err)
		}

		if err := h.service.ClearProfile(ctx); !errors.Is(err, domain.ErrUnauthenticated) {
			t.Errorf("ClearProfile: %v", err)
		}

		if _, err := h.service.ListDevices(ctx); !errors.Is(err, domain.ErrUnauthenticated) {
			t.Errorf("ListDevices: %v", err)
		}

		if _, err := h.service.RevokeDevice(ctx, "id-1"); !errors.Is(err, domain.ErrUnauthenticated) {
			t.Errorf("RevokeDevice: %v", err)
		}
	}
}

func TestSelectionRefusesGoneSessionsAndAccounts(t *testing.T) {
	t.Parallel()

	h, result, _ := withFamily(t)
	ctx := viewerContext(t, h, result.Token)

	if err := h.store.DeleteSession(context.Background(), result.Session.ID); err != nil {
		t.Fatalf("DeleteSession: %v", err)
	}

	if _, err := h.service.SelectProfile(ctx, ProfileSelection{ProfileID: "zoe"}); !errors.Is(err, domain.ErrUnauthenticated) {
		t.Errorf("SelectProfile on a gone session: %v", err)
	}

	h.store.setDisabled("alice", true)

	if _, err := h.service.Me(ctx); !errors.Is(err, domain.ErrUnauthenticated) {
		t.Errorf("Me of a disabled account: %v", err)
	}

	delete(h.store.credentials, "alice")

	if _, err := h.service.Me(ctx); !errors.Is(err, domain.ErrUnauthenticated) {
		t.Errorf("Me of a gone account: %v", err)
	}
}

func TestSelectionReportsStoreFailures(t *testing.T) {
	t.Parallel()

	h, result, _ := withFamily(t)
	ctx := viewerContext(t, h, result.Token)

	h.store.failSelect = errStoreDown

	if _, err := h.service.SelectProfile(ctx, ProfileSelection{ProfileID: "zoe"}); !errors.Is(err, errStoreDown) {
		t.Errorf("SelectProfile: %v, want the store error", err)
	}

	if err := h.service.ClearProfile(ctx); !errors.Is(err, errStoreDown) {
		t.Errorf("ClearProfile: %v, want the store error", err)
	}

	h.store.failSelect = nil
	h.store.pins["kid"] = "corrupt"

	if err := selectKid(ctx, h, kidPIN); !errors.Is(err, domain.ErrInvalid) || errors.Is(err, ErrPINInvalid) {
		t.Errorf("corrupt PIN hash: %v, want the hasher error", err)
	}

	// An aborted check counts nothing: the right PIN still works after many of them.
	h.store.pins["kid"] = hashPrefix + kidPIN

	if err := selectKid(ctx, h, kidPIN); err != nil {
		t.Errorf("right PIN after an aborted check: %v", err)
	}
}

func viewerWithout(accountID, sessionID string) domainaccess.Viewer {
	return domainaccess.Viewer{AccountID: accountID, AuthSessionID: sessionID}
}
