package sqlite_test

import (
	"errors"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/magpeek/mivo/internal/adapters/sqlite"
	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/access"
	"github.com/magpeek/mivo/internal/domain/account"
)

// managedWrite returns a managed profile named name of the account named owner, without PIN.
func managedWrite(owner, name string) sqlite.ProfileWrite {
	return sqlite.ProfileWrite{Record: sqlite.ProfileRecord{
		Profile: account.Profile{
			ID: "profile-" + owner + "-" + name, AccountID: "account-" + owner, Name: name,
			AvatarColor: account.AvatarColorTeal,
		},
		Limit: access.NoLimit(), Libraries: access.AllLibraries(),
	}}
}

func mustCreateProfile(t *testing.T, store *sqlite.AccountStore, write sqlite.ProfileWrite) {
	t.Helper()

	if err := store.CreateProfile(t.Context(), write, nil); err != nil {
		t.Fatalf("CreateProfile(%s) error = %v", write.Record.Profile.ID, err)
	}
}

func mustMaxAge(t *testing.T, maxAge access.AgeBucket, hideUnrated bool) access.ContentLimit {
	t.Helper()

	limit, err := access.MaxAgeLimit(maxAge, hideUnrated)
	if err != nil {
		t.Fatalf("MaxAgeLimit(%d) error = %v", maxAge, err)
	}

	return limit
}

func mustReadProfile(t *testing.T, store *sqlite.AccountStore, profileID string) sqlite.ProfileRecord {
	t.Helper()

	record, err := store.GetProfile(t.Context(), profileID)
	if err != nil {
		t.Fatalf("GetProfile(%s) error = %v", profileID, err)
	}

	return record
}

func mustReadPIN(t *testing.T, store *sqlite.AccountStore, profileID string) string {
	t.Helper()

	hash, err := store.ProfilePINHash(t.Context(), profileID)
	if err != nil {
		t.Fatalf("ProfilePINHash(%s) error = %v", profileID, err)
	}

	return hash
}

func TestAccountProfileRoundTripWithPINLimitAndLibraries(t *testing.T) {
	t.Parallel()

	fixture := newAccountFixture(t)
	ctx := t.Context()
	libraryID := fixture.mustCreateLibrary(t, "Movies")
	fixture.mustCreateAccount(t, "Anna", account.RoleAdmin)

	managed := managedWrite("Anna", "Kid")
	managed.PINAction, managed.PINHash = sqlite.PINSet, opaqueHashText
	managed.Record.Limit = mustMaxAge(t, access.AgeBucket(6), true)
	managed.Record.Libraries = access.OnlyLibraries(libraryID)
	mustCreateProfile(t, fixture.store, managed)

	got := mustReadProfile(t, fixture.store, managed.Record.Profile.ID)
	want := managed.Record
	want.Profile.HasPIN = true

	if got.Profile != want.Profile || got.Limit != want.Limit ||
		!slices.Equal(got.Libraries.IDs(), []string{libraryID}) || got.Libraries.IsAll() {
		t.Errorf("GetProfile = %+v, want %+v", got, want)
	}

	if mustReadPIN(t, fixture.store, managed.Record.Profile.ID) != opaqueHashText {
		t.Error("ProfilePINHash returned a different hash")
	}

	profiles, err := fixture.store.ListProfiles(ctx, "account-Anna")
	if err != nil {
		t.Fatalf("ListProfiles error = %v", err)
	}

	if len(profiles) != 2 || !profiles[0].Profile.Primary || profiles[0].Profile.HasPIN ||
		profiles[1].Profile.ID != managed.Record.Profile.ID {
		t.Errorf("ListProfiles = %+v, want primary first, then the managed profile", profiles)
	}

	_, err = fixture.store.GetProfile(ctx, "missing")
	wantError(t, "GetProfile(missing)", err, domain.ErrNotFound)

	_, err = fixture.store.ProfilePINHash(ctx, "missing")
	wantError(t, "ProfilePINHash(missing)", err, domain.ErrNotFound)
}

func TestAccountProfileUpdatePINActionsAndDelete(t *testing.T) {
	t.Parallel()

	fixture := newAccountFixture(t)
	ctx := t.Context()
	fixture.mustCreateAccount(t, "Anna", account.RoleAdmin)
	managed := managedWrite("Anna", "Kid")
	mustCreateProfile(t, fixture.store, managed)
	profileID := managed.Record.Profile.ID

	update := managed
	update.Record.Profile.Name, update.Record.Profile.AvatarColor = "Kiddo", account.AvatarColorPink
	update.Record.Limit, update.Record.Libraries = access.NoLimit().HidingUnrated(true), access.OnlyLibraries()
	update.PINAction, update.PINHash = sqlite.PINSet, opaqueHashText

	if err := fixture.store.UpdateProfile(ctx, update); err != nil {
		t.Fatalf("UpdateProfile(set PIN) error = %v", err)
	}

	got := mustReadProfile(t, fixture.store, profileID)
	if got.Profile.Name != "Kiddo" || got.Profile.AvatarColor != account.AvatarColorPink || !got.Profile.HasPIN ||
		got.Limit != update.Record.Limit || got.Libraries.IsAll() || len(got.Libraries.IDs()) != 0 {
		t.Errorf("updated profile = %+v, want %+v with a PIN", got, update.Record)
	}

	update.PINAction = sqlite.PINKeep
	if err := fixture.store.UpdateProfile(ctx, update); err != nil || mustReadPIN(t, fixture.store, profileID) != opaqueHashText {
		t.Errorf("UpdateProfile(keep PIN) error = %v or the PIN changed", err)
	}

	update.PINAction = sqlite.PINClear
	if err := fixture.store.UpdateProfile(ctx, update); err != nil || mustReadPIN(t, fixture.store, profileID) != "" {
		t.Errorf("UpdateProfile(clear PIN) error = %v or the PIN is still set", err)
	}

	wantError(t, "DeleteProfile(primary)", fixture.store.DeleteProfile(ctx, "profile-Anna"), domain.ErrConflict)

	if err := fixture.store.DeleteProfile(ctx, profileID); err != nil {
		t.Fatalf("DeleteProfile error = %v", err)
	}

	wantError(t, "DeleteProfile(again)", fixture.store.DeleteProfile(ctx, profileID), domain.ErrNotFound)

	wantError(t, "UpdateProfile(missing)", fixture.store.UpdateProfile(ctx, update), domain.ErrNotFound)

	update.Record.Profile.Name = ""
	wantError(t, "UpdateProfile(empty name)", fixture.store.UpdateProfile(ctx, update), domain.ErrInvalid)
}

func TestAccountProfileWithUnknownLibraryIsInvalid(t *testing.T) {
	t.Parallel()

	fixture := newAccountFixture(t)
	fixture.mustCreateAccount(t, "Anna", account.RoleAdmin)

	managed := managedWrite("Anna", "Kid")
	managed.Record.Libraries = access.OnlyLibraries("no-such-library")

	wantError(t, "CreateProfile(unknown library)", fixture.store.CreateProfile(t.Context(), managed, nil), domain.ErrInvalid)
}

func TestAccountManagedProfileLimitAndPrimaryRules(t *testing.T) {
	t.Parallel()

	fixture := newAccountFixture(t)
	ctx := t.Context()
	fixture.mustCreateAccount(t, "Anna", account.RoleAdmin)

	refused := errors.New("refused by the use case")
	seen := -1

	err := fixture.store.CreateProfile(ctx, managedWrite("Anna", "First"), func(existing int) error {
		seen = existing

		return refused
	})
	wantError(t, "CreateProfile(refusing check)", err, refused)

	if seen != 0 {
		t.Errorf("check saw %d managed profiles, want 0", seen)
	}

	for index := range account.MaxManagedProfiles {
		mustCreateProfile(t, fixture.store, managedWrite("Anna", fmt.Sprintf("Kid%d", index)))
	}

	wantError(t, "CreateProfile(over the limit)", fixture.store.CreateProfile(ctx, managedWrite("Anna", "OneTooMany"), nil), domain.ErrConflict)

	primary := managedWrite("Anna", "Second")
	primary.Record.Profile.Primary = true
	wantError(t, "CreateProfile(primary)", fixture.store.CreateProfile(ctx, primary, nil), domain.ErrInvalid)

	wantError(t, "CreateProfile(unknown account)", fixture.store.CreateProfile(ctx, managedWrite("Nobody", "Kid"), nil), domain.ErrNotFound)
}

func TestAccountAccessInputsCombineAccountProfileAndPrimary(t *testing.T) {
	t.Parallel()

	fixture := newAccountFixture(t)
	ctx := t.Context()
	libraryID := fixture.mustCreateLibrary(t, "Movies")

	record := newAccount("Ben", account.RoleUser)
	record.Account.Libraries = access.OnlyLibraries(libraryID)
	record.Account.PrimaryLimit = mustMaxAge(t, access.AgeBucket(16), false)

	if err := fixture.store.CreateAccount(ctx, record); err != nil {
		t.Fatalf("CreateAccount error = %v", err)
	}

	managed := managedWrite("Ben", "Kid")
	managed.Record.Limit = mustMaxAge(t, access.AgeBucket(6), true)
	mustCreateProfile(t, fixture.store, managed)

	inputs, err := fixture.store.AccessInputs(ctx, managed.Record.Profile.ID)
	if err != nil {
		t.Fatalf("AccessInputs(managed) error = %v", err)
	}

	if inputs.Account != record.Account.Account || inputs.Profile.ID != managed.Record.Profile.ID ||
		!slices.Equal(inputs.AccountLibraries.IDs(), []string{libraryID}) || !inputs.ProfileLibraries.IsAll() ||
		inputs.ProfileLimit != managed.Record.Limit || inputs.PrimaryLimit != record.Account.PrimaryLimit {
		t.Errorf("AccessInputs(managed) = %+v", inputs)
	}

	primaryInputs, err := fixture.store.AccessInputs(ctx, "profile-Ben")
	if err != nil {
		t.Fatalf("AccessInputs(primary) error = %v", err)
	}

	if primaryInputs.ProfileLimit != record.Account.PrimaryLimit || primaryInputs.PrimaryLimit != record.Account.PrimaryLimit {
		t.Errorf("AccessInputs(primary) limits = %+v / %+v, want %+v",
			primaryInputs.ProfileLimit, primaryInputs.PrimaryLimit, record.Account.PrimaryLimit)
	}

	_, err = fixture.store.AccessInputs(ctx, "missing")
	wantError(t, "AccessInputs(missing)", err, domain.ErrNotFound)
}

func TestAccountListHasProfileCountAndLastActivity(t *testing.T) {
	t.Parallel()

	fixture := newAccountFixture(t)
	ctx := t.Context()
	libraryID := fixture.mustCreateLibrary(t, "Movies")
	fixture.mustCreateAccount(t, "Ben", account.RoleUser)

	record := newAccount("anna", account.RoleAdmin)
	record.Account.Libraries = access.OnlyLibraries(libraryID)
	record.Account.PrimaryLimit = mustMaxAge(t, access.AgeBucket(18), false)

	if err := fixture.store.CreateAccount(ctx, record); err != nil {
		t.Fatalf("CreateAccount error = %v", err)
	}

	mustCreateProfile(t, fixture.store, managedWrite("Ben", "Kid"))
	fixture.insertSession(t, "session-1", "account-Ben", "profile-Ben")
	fixture.clock.Advance(time.Hour)
	fixture.insertSession(t, "session-2", "account-Ben", "")

	summaries, err := fixture.store.ListAccounts(ctx)
	if err != nil {
		t.Fatalf("ListAccounts error = %v", err)
	}

	if len(summaries) != 2 || summaries[0].Account.Name != "anna" || summaries[1].Account.Name != "Ben" {
		t.Fatalf("ListAccounts = %+v, want anna then Ben", summaries)
	}

	anna, ben := summaries[0], summaries[1]
	if anna.ProfileCount != 1 || !anna.LastActiveAt.IsZero() || anna.PrimaryLimit != record.Account.PrimaryLimit ||
		!slices.Equal(anna.Libraries.IDs(), []string{libraryID}) {
		t.Errorf("anna summary = %+v, want 1 profile, no activity, one library, max age 18", anna)
	}

	wantActivity := fixture.clock.Now().Add(time.Hour)
	if ben.ProfileCount != 2 || !ben.LastActiveAt.Equal(wantActivity) || !ben.Libraries.IsAll() || ben.PrimaryLimit != access.NoLimit() {
		t.Errorf("Ben summary = %+v, want 2 profiles, last activity %v, all libraries", ben, wantActivity)
	}
}
