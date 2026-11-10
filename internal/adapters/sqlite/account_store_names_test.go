package sqlite_test

import (
	"testing"

	"github.com/magpeek/mivo/internal/adapters/sqlite"
	"github.com/magpeek/mivo/internal/domain/account"
)

// TestAccountDuplicateNameIsNameTaken: a name another account holds, ignoring case, is the
// conflict the API answers as "name_taken", on create and on rename.
func TestAccountDuplicateNameIsNameTaken(t *testing.T) {
	t.Parallel()

	fixture := newAccountFixture(t)
	ctx := t.Context()
	fixture.mustCreateAccount(t, "Anna", account.RoleAdmin)
	fixture.mustCreateAccount(t, "Ben", account.RoleUser)

	wantError(t, "CreateAccount(duplicate name)", fixture.store.CreateAccount(ctx, newAccount("anna", account.RoleUser)), account.ErrNameTaken)

	_, err := fixture.store.UpdateAccount(ctx, "account-Ben", func(current sqlite.AccountRecord, _ int) (sqlite.AccountRecord, error) {
		current.Account.Name = "ANNA"

		return current, nil
	})
	wantError(t, "UpdateAccount(duplicate name)", err, account.ErrNameTaken)
}
