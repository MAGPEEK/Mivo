package sqlite_test

import (
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"sync"
	"testing"

	"github.com/magpeek/mivo/internal/adapters/sqlite"
	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/access"
	"github.com/magpeek/mivo/internal/domain/account"
)

// opaqueHashText stands in for an argon2id PHC string; the store never parses it.
const opaqueHashText = "phc-string-fixture"

// accountFixture is a migrated database with an account store, a library store on the same
// database and an inspection connection.
type accountFixture struct {
	store      *sqlite.AccountStore
	libraries  *sqlite.ManagedLibraryStore
	inspection *sql.DB
	clock      *testClock
}

func newAccountFixture(t *testing.T) accountFixture {
	t.Helper()

	db, path := openMigratedTestDB(t)
	clock := newTestClock()

	return accountFixture{
		store:      sqlite.NewAccountStore(db, clock.Now),
		libraries:  sqlite.NewManagedLibraryStore(db, clock.Now, sequentialIDs()),
		inspection: openInspectionDB(t, path),
		clock:      clock,
	}
}

// newAccount returns an account named name with all libraries and no limit, with its primary
// profile; the ids derive from name.
func newAccount(name string, role account.Role) sqlite.NewAccountRecord {
	return sqlite.NewAccountRecord{
		Account: sqlite.AccountRecord{
			Account:   account.Account{ID: "account-" + name, Name: name, Role: role},
			Libraries: access.AllLibraries(), PrimaryLimit: access.NoLimit(),
		},
		Primary:      account.Profile{ID: "profile-" + name, Name: name, AvatarColor: account.DefaultAvatarColor},
		PasswordHash: opaqueHashText,
	}
}

func (fixture accountFixture) mustCreateAccount(t *testing.T, name string, role account.Role) {
	t.Helper()

	if err := fixture.store.CreateAccount(t.Context(), newAccount(name, role)); err != nil {
		t.Fatalf("CreateAccount(%s) error = %v", name, err)
	}
}

func (fixture accountFixture) mustCreateLibrary(t *testing.T, name string) string {
	t.Helper()

	return mustCreate(t, fixture.libraries, movieCandidate(name, "/media/"+name)).ID
}

func wantError(t *testing.T, operation string, err, want error) {
	t.Helper()

	if !errors.Is(err, want) {
		t.Errorf("%s error = %v, want %v", operation, err, want)
	}
}

func TestAccountFirstAdminRaceHasExactlyOneWinner(t *testing.T) {
	t.Parallel()

	fixture := newAccountFixture(t)

	const contenders = 2

	results := make([]error, contenders)

	var group sync.WaitGroup

	for index := range contenders {
		group.Go(func() {
			results[index] = fixture.store.CreateFirstAdmin(t.Context(), newAccount(fmt.Sprintf("Admin%d", index), account.RoleAdmin))
		})
	}

	group.Wait()

	wins, conflicts := 0, 0

	for _, err := range results {
		switch {
		case err == nil:
			wins++
		case errors.Is(err, domain.ErrConflict):
			conflicts++
		default:
			t.Errorf("CreateFirstAdmin error = %v, want nil or ErrConflict", err)
		}
	}

	if wins != 1 || conflicts != 1 {
		t.Errorf("wins = %d, conflicts = %d, want 1 and 1", wins, conflicts)
	}

	if got := countRows(t, fixture.inspection, "SELECT COUNT(*) FROM users"); got != 1 {
		t.Errorf("accounts = %d, want 1", got)
	}
}

func TestAccountFirstAdminNeedsAnEmptyTableAndTheAdminRole(t *testing.T) {
	t.Parallel()

	fixture := newAccountFixture(t)
	ctx := t.Context()

	if has, err := fixture.store.HasAccounts(ctx); err != nil || has {
		t.Fatalf("HasAccounts on a fresh database = %t, %v; want false, nil", has, err)
	}

	wantError(t, "CreateFirstAdmin(user role)", fixture.store.CreateFirstAdmin(ctx, newAccount("Anna", account.RoleUser)), domain.ErrInvalid)

	fixture.mustCreateAccount(t, "Ben", account.RoleUser)

	if has, err := fixture.store.HasAccounts(ctx); err != nil || !has {
		t.Errorf("HasAccounts after create = %t, %v; want true, nil", has, err)
	}

	wantError(t, "CreateFirstAdmin(second)", fixture.store.CreateFirstAdmin(ctx, newAccount("Anna", account.RoleAdmin)), domain.ErrConflict)
}

func TestAccountRoundTripByIDAndNameKey(t *testing.T) {
	t.Parallel()

	fixture := newAccountFixture(t)
	ctx := t.Context()
	libraryID := fixture.mustCreateLibrary(t, "Movies")

	record := newAccount("Zoë", account.RoleUser)
	record.Account.Libraries = access.OnlyLibraries(libraryID)
	record.Account.PrimaryLimit = mustMaxAge(t, access.AgeBucket(12), true)

	if err := fixture.store.CreateAccount(ctx, record); err != nil {
		t.Fatalf("CreateAccount error = %v", err)
	}

	byID, err := fixture.store.GetAccount(ctx, record.Account.Account.ID)
	if err != nil {
		t.Fatalf("GetAccount error = %v", err)
	}

	byName, err := fixture.store.FindAccountByNameKey(ctx, account.NameKey("ZOË"))
	if err != nil {
		t.Fatalf("FindAccountByNameKey error = %v", err)
	}

	credentials, err := fixture.store.CredentialsByNameKey(ctx, account.NameKey("zoë"))
	if err != nil {
		t.Fatalf("CredentialsByNameKey error = %v", err)
	}

	for _, got := range []sqlite.AccountRecord{byID, byName, credentials.Record} {
		if got.Account != record.Account.Account || got.PrimaryLimit != record.Account.PrimaryLimit ||
			got.Libraries.IsAll() || !slices.Equal(got.Libraries.IDs(), []string{libraryID}) {
			t.Errorf("read account = %+v, want %+v", got, record.Account)
		}
	}

	if credentials.PasswordHash != opaqueHashText {
		t.Error("CredentialsByNameKey returned a different password hash")
	}

	_, err = fixture.store.GetAccount(ctx, "missing")
	wantError(t, "GetAccount(missing)", err, domain.ErrNotFound)

	_, err = fixture.store.FindAccountByNameKey(ctx, "missing")
	wantError(t, "FindAccountByNameKey(missing)", err, domain.ErrNotFound)
}

func TestAccountNamesAreUniqueCaseInsensitively(t *testing.T) {
	t.Parallel()

	fixture := newAccountFixture(t)
	ctx := t.Context()
	fixture.mustCreateAccount(t, "Anna", account.RoleAdmin)
	fixture.mustCreateAccount(t, "Ben", account.RoleUser)

	wantError(t, "CreateAccount(anna)", fixture.store.CreateAccount(ctx, newAccount("anna", account.RoleUser)), domain.ErrConflict)

	_, err := fixture.store.UpdateAccount(ctx, "account-Ben", renameTo("ANNA"))
	wantError(t, "UpdateAccount(rename to ANNA)", err, domain.ErrConflict)

	if got := countRows(t, fixture.inspection, "SELECT COUNT(*) FROM profiles"); got != 2 {
		t.Errorf("profiles after refused create = %d, want 2 (rolled back)", got)
	}
}

func renameTo(name string) sqlite.AccountChange {
	return func(current sqlite.AccountRecord, _ int) (sqlite.AccountRecord, error) {
		current.Account.Name = name

		return current, nil
	}
}

func TestAccountCreateWithUnknownLibraryIsInvalid(t *testing.T) {
	t.Parallel()

	fixture := newAccountFixture(t)
	record := newAccount("Anna", account.RoleUser)
	record.Account.Libraries = access.OnlyLibraries("no-such-library")

	wantError(t, "CreateAccount(unknown library)", fixture.store.CreateAccount(t.Context(), record), domain.ErrInvalid)

	if got := countRows(t, fixture.inspection, "SELECT COUNT(*) FROM users"); got != 0 {
		t.Errorf("accounts after refused create = %d, want 0", got)
	}
}

func TestAccountCreateRefusesAnInvalidAccount(t *testing.T) {
	t.Parallel()

	fixture := newAccountFixture(t)

	wantError(t, "CreateAccount(invalid role)", fixture.store.CreateAccount(t.Context(), newAccount("Anna", "owner")), domain.ErrInvalid)
}

func TestAccountStoreFailsWithACancelledContext(t *testing.T) {
	t.Parallel()

	fixture := newAccountFixture(t)
	ctx := cancelledContext(t)

	if _, err := fixture.store.HasAccounts(ctx); err == nil {
		t.Error("HasAccounts with a cancelled context succeeded")
	}

	if _, err := fixture.store.ListAccounts(ctx); err == nil {
		t.Error("ListAccounts with a cancelled context succeeded")
	}

	if err := fixture.store.CreateAccount(ctx, newAccount("Anna", account.RoleAdmin)); err == nil {
		t.Error("CreateAccount with a cancelled context succeeded")
	}
}
