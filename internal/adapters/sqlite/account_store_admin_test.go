package sqlite_test

import (
	"database/sql"
	"errors"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/magpeek/mivo/internal/adapters/sqlite"
	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/access"
	"github.com/magpeek/mivo/internal/domain/account"
)

// setRoleAndDisabled is a change that only sets the role and the disabled flag.
func setRoleAndDisabled(role account.Role, disabled bool) sqlite.AccountChange {
	return func(current sqlite.AccountRecord, _ int) (sqlite.AccountRecord, error) {
		current.Account.Role, current.Account.Disabled = role, disabled

		return current, nil
	}
}

func TestAccountLastAdminCannotBeDemotedDisabledOrDeleted(t *testing.T) {
	t.Parallel()

	fixture := newAccountFixture(t)
	ctx := t.Context()
	fixture.mustCreateAccount(t, "Anna", account.RoleAdmin)
	fixture.mustCreateAccount(t, "Ben", account.RoleUser)

	_, err := fixture.store.UpdateAccount(ctx, "account-Anna", setRoleAndDisabled(account.RoleUser, false))
	wantError(t, "demote last admin", err, domain.ErrConflict)

	_, err = fixture.store.UpdateAccount(ctx, "account-Anna", setRoleAndDisabled(account.RoleAdmin, true))
	wantError(t, "disable last admin", err, domain.ErrConflict)

	wantError(t, "delete last admin", fixture.store.DeleteAccount(ctx, "account-Anna", nil), domain.ErrConflict)

	if _, err := fixture.store.UpdateAccount(ctx, "account-Ben", setRoleAndDisabled(account.RoleAdmin, false)); err != nil {
		t.Fatalf("promote Ben error = %v", err)
	}

	if _, err := fixture.store.UpdateAccount(ctx, "account-Anna", setRoleAndDisabled(account.RoleUser, false)); err != nil {
		t.Fatalf("demote Anna with a second admin error = %v", err)
	}

	wantError(t, "delete Ben as last admin", fixture.store.DeleteAccount(ctx, "account-Ben", nil), domain.ErrConflict)

	if err := fixture.store.DeleteAccount(ctx, "account-Anna", nil); err != nil {
		t.Errorf("delete non-admin Anna error = %v", err)
	}
}

// The callbacks run inside the transaction with the admin count of that transaction, and
// their refusal aborts the change.
func TestAccountCallbacksSeeTheTransactionAdminCount(t *testing.T) {
	t.Parallel()

	fixture := newAccountFixture(t)
	ctx := t.Context()
	fixture.mustCreateAccount(t, "Anna", account.RoleAdmin)
	fixture.mustCreateAccount(t, "Ben", account.RoleAdmin)

	refused := errors.New("refused by the use case")
	seen := 0

	_, err := fixture.store.UpdateAccount(ctx, "account-Ben", func(_ sqlite.AccountRecord, admins int) (sqlite.AccountRecord, error) {
		seen = admins

		return sqlite.AccountRecord{}, refused
	})
	wantError(t, "UpdateAccount(refusing change)", err, refused)

	if seen != 2 {
		t.Errorf("change saw %d enabled admins, want 2", seen)
	}

	err = fixture.store.DeleteAccount(ctx, "account-Ben", func(current account.Account, admins int) error {
		return account.CheckAdminChange(current, account.AdminChangeDelete, admins-1)
	})
	wantError(t, "DeleteAccount(refusing check)", err, domain.ErrConflict)

	_, err = fixture.store.UpdateAccount(ctx, "account-Ben", func(current sqlite.AccountRecord, _ int) (sqlite.AccountRecord, error) {
		current.Account.ID = "account-Anna"

		return current, nil
	})
	wantError(t, "UpdateAccount(changed id)", err, domain.ErrInvalid)

	_, err = fixture.store.UpdateAccount(ctx, "missing", renameTo("Carl"))
	wantError(t, "UpdateAccount(missing)", err, domain.ErrNotFound)
	wantError(t, "DeleteAccount(missing)", fixture.store.DeleteAccount(ctx, "missing", nil), domain.ErrNotFound)
	wantError(t, "SetPasswordHash(missing)", fixture.store.SetPasswordHash(ctx, "missing", "x"), domain.ErrNotFound)

	_, err = fixture.store.UpdateAccount(ctx, "account-Ben", renameTo(""))
	wantError(t, "UpdateAccount(empty name)", err, domain.ErrInvalid)
}

// Two admins demoting each other at the same time: the count is read inside the write
// transaction, so exactly one demotion succeeds.
func TestAccountTwoAdminsDemotingEachOtherLeaveOneAdmin(t *testing.T) {
	t.Parallel()

	fixture := newAccountFixture(t)
	fixture.mustCreateAccount(t, "Anna", account.RoleAdmin)
	fixture.mustCreateAccount(t, "Ben", account.RoleAdmin)

	targets := map[string]sqlite.AccountChange{
		"account-Anna": setRoleAndDisabled(account.RoleUser, false),
		"account-Ben":  setRoleAndDisabled(account.RoleAdmin, true),
	}

	var (
		group     sync.WaitGroup
		mutex     sync.Mutex
		conflicts int
	)

	for accountID, change := range targets {
		group.Go(func() {
			_, err := fixture.store.UpdateAccount(t.Context(), accountID, change)
			if err != nil && !errors.Is(err, domain.ErrConflict) {
				t.Errorf("UpdateAccount(%s) error = %v, want nil or ErrConflict", accountID, err)
			}

			mutex.Lock()
			defer mutex.Unlock()

			if err != nil {
				conflicts++
			}
		})
	}

	group.Wait()

	admins := countRows(t, fixture.inspection, "SELECT COUNT(*) FROM users WHERE role = 'admin' AND disabled = 0")
	if conflicts != 1 || admins != 1 {
		t.Errorf("conflicts = %d, enabled admins = %d; want 1 and 1", conflicts, admins)
	}
}

func TestAccountUpdateRenamesThePrimaryProfileAndStoresTheLimit(t *testing.T) {
	t.Parallel()

	fixture := newAccountFixture(t)
	ctx := t.Context()
	fixture.mustCreateAccount(t, "Anna", account.RoleAdmin)
	fixture.mustCreateAccount(t, "Ben", account.RoleUser)
	libraryID := fixture.mustCreateLibrary(t, "Movies")
	limit := mustMaxAge(t, access.AgeBucket(16), false)

	updated, err := fixture.store.UpdateAccount(ctx, "account-Ben", func(current sqlite.AccountRecord, _ int) (sqlite.AccountRecord, error) {
		current.Account.Name, current.Account.Disabled = "Benjamin", true
		current.Libraries, current.PrimaryLimit = access.OnlyLibraries(libraryID), limit

		return current, nil
	})
	if err != nil {
		t.Fatalf("UpdateAccount error = %v", err)
	}

	if err := fixture.store.SetPasswordHash(ctx, "account-Ben", "new-hash"); err != nil {
		t.Fatalf("SetPasswordHash error = %v", err)
	}

	credentials, err := fixture.store.CredentialsByID(ctx, "account-Ben")
	if err != nil {
		t.Fatalf("CredentialsByID error = %v", err)
	}

	got := credentials.Record
	if got.Account != updated.Account || got.PrimaryLimit != limit || credentials.PasswordHash != "new-hash" ||
		!slices.Equal(got.Libraries.IDs(), []string{libraryID}) {
		t.Errorf("stored account = %+v, want %+v", got, updated)
	}

	primary := mustReadProfile(t, fixture.store, "profile-Ben")
	if primary.Profile.Name != "Benjamin" || primary.Limit != limit {
		t.Errorf("primary profile = %+v, want name Benjamin and limit %+v", primary, limit)
	}
}

func TestAccountDeleteCascadesToProfilesAccessAndSessions(t *testing.T) {
	t.Parallel()

	fixture := newAccountFixture(t)
	ctx := t.Context()
	libraryID := fixture.mustCreateLibrary(t, "Movies")
	fixture.mustCreateAccount(t, "Anna", account.RoleAdmin)

	record := newAccount("Ben", account.RoleUser)
	record.Account.Libraries = access.OnlyLibraries(libraryID)

	if err := fixture.store.CreateAccount(ctx, record); err != nil {
		t.Fatalf("CreateAccount error = %v", err)
	}

	managed := managedWrite("Ben", "Kid")
	managed.Record.Libraries = access.OnlyLibraries(libraryID)
	mustCreateProfile(t, fixture.store, managed)
	fixture.insertSession(t, "session-ben", "account-Ben", managed.Record.Profile.ID)

	if err := fixture.store.DeleteAccount(ctx, "account-Ben", nil); err != nil {
		t.Fatalf("DeleteAccount error = %v", err)
	}

	for _, table := range []string{"profiles", "user_libraries", "profile_libraries", "auth_sessions"} {
		left := countRows(t, fixture.inspection, "SELECT COUNT(*) FROM "+table+
			" WHERE "+ownerColumn(table)+" IN ('account-Ben', 'profile-Ben', 'profile-Ben-Kid')")
		if left != 0 {
			t.Errorf("%s rows of the deleted account = %d, want 0", table, left)
		}
	}

	if got := countRows(t, fixture.inspection, "SELECT COUNT(*) FROM profiles"); got != 1 {
		t.Errorf("profiles left = %d, want Anna's 1", got)
	}
}

func ownerColumn(table string) string {
	if table == "profile_libraries" {
		return "profile_id"
	}

	return "user_id"
}

// Deleting a profile keeps a session that had it selected, without a profile.
func TestAccountProfileDeleteKeepsTheSessionWithoutProfile(t *testing.T) {
	t.Parallel()

	fixture := newAccountFixture(t)
	fixture.mustCreateAccount(t, "Anna", account.RoleAdmin)
	managed := managedWrite("Anna", "Kid")
	mustCreateProfile(t, fixture.store, managed)
	fixture.insertSession(t, "session-anna", "account-Anna", managed.Record.Profile.ID)

	if err := fixture.store.DeleteProfile(t.Context(), "account-Anna", managed.Record.Profile.ID); err != nil {
		t.Fatalf("DeleteProfile error = %v", err)
	}

	if got := countRows(t, fixture.inspection, "SELECT COUNT(*) FROM auth_sessions WHERE profile_id IS NULL"); got != 1 {
		t.Errorf("sessions without profile = %d, want 1", got)
	}
}

func TestAccountLibraryDeleteRemovesAccessRows(t *testing.T) {
	t.Parallel()

	fixture := newAccountFixture(t)
	ctx := t.Context()
	kept := fixture.mustCreateLibrary(t, "Kept")
	removed := fixture.mustCreateLibrary(t, "Removed")

	record := newAccount("Ben", account.RoleUser)
	record.Account.Libraries = access.OnlyLibraries(kept, removed)

	if err := fixture.store.CreateAccount(ctx, record); err != nil {
		t.Fatalf("CreateAccount error = %v", err)
	}

	managed := managedWrite("Ben", "Kid")
	managed.Record.Libraries = access.OnlyLibraries(removed)
	mustCreateProfile(t, fixture.store, managed)

	if err := fixture.libraries.DeleteLibrary(ctx, removed); err != nil {
		t.Fatalf("DeleteLibrary error = %v", err)
	}

	inputs, err := fixture.store.AccessInputs(ctx, managed.Record.Profile.ID)
	if err != nil {
		t.Fatalf("AccessInputs error = %v", err)
	}

	if inputs.AccountLibraries.Allows(removed) || !inputs.AccountLibraries.Allows(kept) {
		t.Errorf("account libraries = %v, want only %s", inputs.AccountLibraries.IDs(), kept)
	}

	if inputs.ProfileLibraries.IsAll() || len(inputs.ProfileLibraries.IDs()) != 0 {
		t.Errorf("profile libraries = %v, want none (fail closed)", inputs.ProfileLibraries.IDs())
	}
}

// insertSession stores a session row directly; the session queries are M7-09. An empty
// profileID stores no selected profile.
func (fixture accountFixture) insertSession(t *testing.T, sessionID, accountID, profileID string) {
	t.Helper()

	execute(t, fixture.inspection, `INSERT INTO auth_sessions (id, token_hash, user_id, profile_id, device_name,
		client, address_class, created_at, last_seen_at) VALUES (?, ?, ?, ?, 'Laptop', 'web', 'lan', ?, ?)`,
		sessionID, "token-"+sessionID, accountID, sql.NullString{String: profileID, Valid: profileID != ""},
		fixture.clock.Now().UnixMilli(), fixture.clock.Now().Add(time.Hour).UnixMilli())
}
