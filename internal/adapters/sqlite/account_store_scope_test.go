package sqlite_test

import (
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/magpeek/mivo/internal/adapters/sqlite"
	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/access"
	"github.com/magpeek/mivo/internal/domain/account"
)

// A profile id of another account is answered like an unknown id and changes nothing.
func TestAccountProfileWritesAreScopedToTheirAccount(t *testing.T) {
	t.Parallel()

	fixture := newAccountFixture(t)
	ctx := t.Context()
	fixture.mustCreateAccount(t, "Anna", account.RoleAdmin)
	fixture.mustCreateAccount(t, "Ben", account.RoleUser)
	victim := managedWrite("Anna", "Kid")
	victim.PINAction, victim.PINHash = sqlite.PINSet, opaqueHashText
	mustCreateProfile(t, fixture.store, victim)

	foreign := victim
	foreign.Record.Profile.AccountID = "account-Ben"
	foreign.Record.Profile.Name = "Taken over"
	wantError(t, "UpdateProfile(other account)", fixture.store.UpdateProfile(ctx, foreign), domain.ErrNotFound)

	foreign.Record.Profile.Name = victim.Record.Profile.Name
	foreign.PINAction = sqlite.PINClear
	wantError(t, "UpdateProfile(clear PIN of other account)", fixture.store.UpdateProfile(ctx, foreign), domain.ErrNotFound)

	wantError(t, "DeleteProfile(other account)",
		fixture.store.DeleteProfile(ctx, "account-Ben", victim.Record.Profile.ID), domain.ErrNotFound)

	got := mustReadProfile(t, fixture.store, victim.Record.Profile.ID)
	if got.Profile.Name != "Kid" || mustReadPIN(t, fixture.store, victim.Record.Profile.ID) != opaqueHashText {
		t.Errorf("victim profile after foreign writes = %+v, want it unchanged with its PIN", got)
	}
}

// The primary profile carries the account name and access; UpdateAccount owns both.
func TestAccountPrimaryProfileKeepsNameAndLibraries(t *testing.T) {
	t.Parallel()

	fixture := newAccountFixture(t)
	ctx := t.Context()
	libraryID := fixture.mustCreateLibrary(t, "Movies")
	fixture.mustCreateAccount(t, "Anna", account.RoleAdmin)

	primary := sqlite.ProfileWrite{Record: mustReadProfile(t, fixture.store, "profile-Anna")}

	renamed := primary
	renamed.Record.Profile.Name = "Annie"
	wantError(t, "UpdateProfile(rename primary)", fixture.store.UpdateProfile(ctx, renamed), domain.ErrInvalid)

	narrowed := primary
	narrowed.Record.Libraries = access.OnlyLibraries(libraryID)
	wantError(t, "UpdateProfile(narrow primary)", fixture.store.UpdateProfile(ctx, narrowed), domain.ErrInvalid)

	allowed := primary
	allowed.Record.Profile.AvatarColor = account.AvatarColorBlue
	allowed.Record.Limit = mustMaxAge(t, access.AgeBucket(12), true)
	allowed.PINAction, allowed.PINHash = sqlite.PINSet, opaqueHashText

	if err := fixture.store.UpdateProfile(ctx, allowed); err != nil {
		t.Fatalf("UpdateProfile(avatar, limit, PIN of primary) error = %v", err)
	}

	got := mustReadProfile(t, fixture.store, "profile-Anna")
	if got.Profile.AvatarColor != account.AvatarColorBlue || got.Limit != allowed.Record.Limit || !got.Profile.HasPIN {
		t.Errorf("primary profile = %+v, want blue, limit 12 and a PIN", got)
	}
}

func TestAccountDuplicateIDsAreConflicts(t *testing.T) {
	t.Parallel()

	fixture := newAccountFixture(t)
	ctx := t.Context()
	fixture.mustCreateAccount(t, "Anna", account.RoleAdmin)

	sameAccountID := newAccount("Ben", account.RoleUser)
	sameAccountID.Account.Account.ID = "account-Anna"
	wantError(t, "CreateAccount(duplicate account id)", fixture.store.CreateAccount(ctx, sameAccountID), domain.ErrConflict)

	sameProfileID := newAccount("Carl", account.RoleUser)
	sameProfileID.Primary.ID = "profile-Anna"
	wantError(t, "CreateAccount(duplicate profile id)", fixture.store.CreateAccount(ctx, sameProfileID), domain.ErrConflict)

	managed := managedWrite("Anna", "Kid")
	managed.Record.Profile.ID = "profile-Anna"
	wantError(t, "CreateProfile(duplicate profile id)", fixture.store.CreateProfile(ctx, managed, nil), domain.ErrConflict)
}

// With one managed slot left, two concurrent creates fill it exactly once.
func TestAccountConcurrentProfileCreatesRespectTheLimit(t *testing.T) {
	t.Parallel()

	fixture := newAccountFixture(t)
	fixture.mustCreateAccount(t, "Anna", account.RoleAdmin)

	for index := range account.MaxManagedProfiles - 1 {
		mustCreateProfile(t, fixture.store, managedWrite("Anna", fmt.Sprintf("Kid%d", index)))
	}

	const contenders = 2

	results := make([]error, contenders)

	var group sync.WaitGroup

	for index := range contenders {
		group.Go(func() {
			results[index] = fixture.store.CreateProfile(t.Context(), managedWrite("Anna", fmt.Sprintf("Last%d", index)), nil)
		})
	}

	group.Wait()

	conflicts := 0

	for _, err := range results {
		if errors.Is(err, domain.ErrConflict) {
			conflicts++
		} else if err != nil {
			t.Errorf("CreateProfile error = %v, want nil or ErrConflict", err)
		}
	}

	managed := countRows(t, fixture.inspection, "SELECT COUNT(*) FROM profiles WHERE user_id = 'account-Anna' AND is_primary = 0")
	if conflicts != 1 || managed != account.MaxManagedProfiles {
		t.Errorf("conflicts = %d, managed profiles = %d; want 1 and %d", conflicts, managed, account.MaxManagedProfiles)
	}
}
