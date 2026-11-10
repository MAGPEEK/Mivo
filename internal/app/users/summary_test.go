package users_test

import (
	"testing"

	"github.com/magpeek/mivo/internal/app/users"
	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/account"
)

func TestAccountSummary(t *testing.T) {
	t.Parallel()

	fixture := newFixture(t)

	summary, err := fixture.service.AccountSummary(adminContext(), ownerID)
	wantNoError(t, err)

	if summary.Account.ID != ownerID || summary.ProfileCount != 2 {
		t.Fatalf("AccountSummary() = %+v, want the owner with 2 profiles", summary)
	}

	_, err = fixture.service.AccountSummary(adminContext(), "missing-account")
	wantError(t, err, domain.ErrNotFound)

	_, err = fixture.service.AccountSummary(ownerContext(), ownerID)
	wantError(t, err, domain.ErrForbidden)

	fixture.store.failList = true

	_, err = fixture.service.AccountSummary(adminContext(), ownerID)
	wantError(t, err, errFake)
}

// TestDemotionWithoutLibrariesNamesLibraryAccess: the HTTP adapter points the client at
// libraryAccess, so the refusal carries its own sentinel.
func TestDemotionWithoutLibrariesNamesLibraryAccess(t *testing.T) {
	t.Parallel()

	fixture := newFixture(t)

	_, err := fixture.service.CreateAccount(adminContext(), users.CreateAccountInput{
		Name: "Second Admin", Password: validPassword, Role: account.RoleAdmin,
	})
	wantNoError(t, err)

	_, err = fixture.service.UpdateAccount(adminContext(), adminID, users.AccountPatch{Role: pointer(account.RoleUser)})
	wantError(t, err, users.ErrLibraryAccess)
	wantError(t, err, domain.ErrInvalid)
}
