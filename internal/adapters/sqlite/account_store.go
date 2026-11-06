package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/magpeek/mivo/internal/adapters/sqlite/sqlcgen"
	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/access"
	"github.com/magpeek/mivo/internal/domain/account"
)

// AccountStore stores accounts, their profiles and their library access (ADR 0032 §1, §7).
// Reads go to the reader pool; every change runs in one transaction on the writer pool, which
// starts it with BEGIN IMMEDIATE, so invariants checked inside a change (first admin, last
// admin, managed-profile limit) cannot race with another writer. Hashes pass through the
// store unchanged and are never logged or put into error messages.
//
// The record and callback types mirror the ports of app/users (M7-17) field for field, so the
// wiring converts them with plain type conversions.
type AccountStore struct {
	writer *sql.DB
	reader *sql.DB
	clock  func() time.Time
}

// NewAccountStore returns a store on db, which must be migrated. clock stamps created_at and
// updated_at (nil means time.Now).
func NewAccountStore(db *DB, clock func() time.Time) *AccountStore {
	if clock == nil {
		clock = time.Now
	}

	return &AccountStore{writer: db.writer, reader: db.reader, clock: clock}
}

// AccountRecord is an account as stored: the identity, its library access and the content
// limit of its primary profile.
type AccountRecord struct {
	Account      account.Account
	Libraries    access.LibraryAccess
	PrimaryLimit access.ContentLimit
}

// NewAccountRecord is everything a new account is stored with in one transaction: the account,
// its primary profile (name and avatar; the store forces the account id and the primary flag,
// gives it all of the account's libraries and the account's PrimaryLimit) and the password
// hash (an argon2id PHC string).
type NewAccountRecord struct {
	Account      AccountRecord
	Primary      account.Profile
	PasswordHash string
}

// AccountChange computes the new state of an account from the current one. The store calls it
// inside the write transaction with the number of enabled admins counted in that transaction.
type AccountChange func(current AccountRecord, enabledAdmins int) (AccountRecord, error)

// AccountDeletionCheck decides inside the delete transaction whether the account may go.
type AccountDeletionCheck func(current account.Account, enabledAdmins int) error

// HasAccounts reports whether at least one account exists; without one the server is in
// setup state (ADR 0032 §6).
func (store *AccountStore) HasAccounts(ctx context.Context) (bool, error) {
	count, err := sqlcgen.New(store.reader).CountUsers(ctx)
	if err != nil {
		return false, fmt.Errorf("count accounts: %w", err)
	}

	return count > 0, nil
}

// CreateFirstAdmin creates the first account, which must be an admin, with its primary
// profile. It succeeds only while no account exists, checked in the same transaction, so of
// two concurrent setups exactly one wins; the other gets domain.ErrConflict.
func (store *AccountStore) CreateFirstAdmin(ctx context.Context, record NewAccountRecord) error {
	admin := record.Account.Account
	if admin.Role != account.RoleAdmin {
		return fmt.Errorf("create first admin %s: role %q: %w", admin.ID, admin.Role, domain.ErrInvalid)
	}

	err := inWriteTransaction(ctx, store.writer, func(queries *sqlcgen.Queries) error {
		count, err := queries.CountUsers(ctx)
		if err != nil {
			return fmt.Errorf("count accounts: %w", err)
		}

		if count > 0 {
			return fmt.Errorf("an account already exists: %w", domain.ErrConflict)
		}

		return store.insertAccount(ctx, queries, record)
	})
	if err != nil {
		return fmt.Errorf("create first admin %s: %w", admin.ID, err)
	}

	return nil
}

// CreateAccount creates an account with its primary profile in one transaction. A name whose
// key another account holds wraps domain.ErrConflict; an unknown library id wraps
// domain.ErrInvalid.
func (store *AccountStore) CreateAccount(ctx context.Context, record NewAccountRecord) error {
	err := inWriteTransaction(ctx, store.writer, func(queries *sqlcgen.Queries) error {
		return store.insertAccount(ctx, queries, record)
	})
	if err != nil {
		return fmt.Errorf("create account %s: %w", record.Account.Account.ID, err)
	}

	return nil
}

// insertAccount stores the account, its library access and its primary profile, so an
// account never exists without one.
func (store *AccountStore) insertAccount(ctx context.Context, queries *sqlcgen.Queries, record NewAccountRecord) error {
	created := record.Account.Account
	if _, err := account.NewAccount(created); err != nil {
		return err
	}

	now := store.clock().UnixMilli()

	err := queries.InsertUser(ctx, sqlcgen.InsertUserParams{
		ID: created.ID, Name: created.Name, NameKey: account.NameKey(created.Name),
		Role: string(created.Role), Disabled: boolToInt(created.Disabled),
		PasswordHash: record.PasswordHash, AllLibraries: boolToInt(record.Account.Libraries.IsAll()),
		CreatedAt: now, UpdatedAt: now,
	})
	if err != nil {
		return accountWriteError("insert account", err)
	}

	if err := writeUserLibraries(ctx, queries, created.ID, record.Account.Libraries); err != nil {
		return err
	}

	primary := record.Primary
	primary.AccountID = created.ID
	primary.Primary = true

	return store.insertProfile(ctx, queries, ProfileWrite{Record: ProfileRecord{
		Profile: primary, Libraries: access.AllLibraries(), Limit: record.Account.PrimaryLimit,
	}})
}

// UpdateAccount reads the account, lets change compute its new state and stores it, all in
// one transaction. A new name also renames the primary profile, which carries the account
// name, and PrimaryLimit becomes the primary profile's limit. Independent of change, demoting
// or disabling the last enabled admin wraps domain.ErrConflict.
func (store *AccountStore) UpdateAccount(ctx context.Context, accountID string, change AccountChange) (AccountRecord, error) {
	var updated AccountRecord

	err := inWriteTransaction(ctx, store.writer, func(queries *sqlcgen.Queries) error {
		current, err := readAccountRecord(ctx, queries, accountID)
		if err != nil {
			return err
		}

		admins, err := queries.CountEnabledAdmins(ctx)
		if err != nil {
			return fmt.Errorf("count enabled admins: %w", err)
		}

		next, err := change(current, int(admins))
		if err != nil {
			return err
		}

		if err := checkAccountChange(current.Account, next.Account, int(admins)); err != nil {
			return err
		}

		updated = next

		return store.writeAccountRow(ctx, queries, next)
	})
	if err != nil {
		return AccountRecord{}, fmt.Errorf("update account %s: %w", accountID, err)
	}

	return updated, nil
}

// checkAccountChange refuses a changed id, an invalid account and the loss of the last
// enabled admin.
func checkAccountChange(current, next account.Account, enabledAdmins int) error {
	if next.ID != current.ID {
		return fmt.Errorf("change must keep the account id: %w", domain.ErrInvalid)
	}

	if _, err := account.NewAccount(next); err != nil {
		return err
	}

	return checkAdminChanges(current, next, enabledAdmins)
}

func (store *AccountStore) writeAccountRow(ctx context.Context, queries *sqlcgen.Queries, next AccountRecord) error {
	now := store.clock().UnixMilli()

	// The row was read in this transaction, so the update always finds it.
	_, err := queries.UpdateUser(ctx, sqlcgen.UpdateUserParams{
		Name: next.Account.Name, NameKey: account.NameKey(next.Account.Name),
		Role: string(next.Account.Role), Disabled: boolToInt(next.Account.Disabled),
		AllLibraries: boolToInt(next.Libraries.IsAll()), UpdatedAt: now, ID: next.Account.ID,
	})
	if err != nil {
		return accountWriteError("update account row", err)
	}

	maxAge, hideUnrated := limitColumns(next.PrimaryLimit)

	err = queries.UpdatePrimaryProfile(ctx, sqlcgen.UpdatePrimaryProfileParams{
		Name: next.Account.Name, MaxAge: maxAge, HideUnrated: hideUnrated, UpdatedAt: now, UserID: next.Account.ID,
	})
	if err != nil {
		return fmt.Errorf("update primary profile: %w", err)
	}

	return writeUserLibraries(ctx, queries, next.Account.ID, next.Libraries)
}

// DeleteAccount removes the account with its profiles, library access and sessions when check
// (nil allows) agrees inside the transaction. Independent of check, deleting the last enabled
// admin wraps domain.ErrConflict.
func (store *AccountStore) DeleteAccount(ctx context.Context, accountID string, check AccountDeletionCheck) error {
	err := inWriteTransaction(ctx, store.writer, func(queries *sqlcgen.Queries) error {
		current, err := currentAccount(ctx, queries, accountID)
		if err != nil {
			return err
		}

		admins, err := queries.CountEnabledAdmins(ctx)
		if err != nil {
			return fmt.Errorf("count enabled admins: %w", err)
		}

		if err := checkDeletion(current, int(admins), check); err != nil {
			return err
		}

		if _, err := queries.DeleteUser(ctx, accountID); err != nil {
			return fmt.Errorf("delete account row: %w", err)
		}

		return nil
	})
	if err != nil {
		return fmt.Errorf("delete account %s: %w", accountID, err)
	}

	return nil
}

func checkDeletion(current account.Account, enabledAdmins int, check AccountDeletionCheck) error {
	if check != nil {
		if err := check(current, enabledAdmins); err != nil {
			return err
		}
	}

	return account.CheckAdminChange(current, account.AdminChangeDelete, enabledAdmins)
}

// SetPasswordHash replaces the account's password hash. An unknown account wraps
// domain.ErrNotFound.
func (store *AccountStore) SetPasswordHash(ctx context.Context, accountID, passwordHash string) error {
	err := inWriteTransaction(ctx, store.writer, func(queries *sqlcgen.Queries) error {
		changed, err := queries.SetUserPasswordHash(ctx, sqlcgen.SetUserPasswordHashParams{
			PasswordHash: passwordHash, UpdatedAt: store.clock().UnixMilli(), ID: accountID,
		})
		if err != nil {
			// The driver error may quote the statement but never its bound values.
			return fmt.Errorf("update password hash: %w", err)
		}

		if changed == 0 {
			return domain.ErrNotFound
		}

		return nil
	})
	if err != nil {
		return fmt.Errorf("set password hash of account %s: %w", accountID, err)
	}

	return nil
}

// checkAdminChanges applies the last-admin rule to every admin power the change takes away.
func checkAdminChanges(current, next account.Account, enabledAdmins int) error {
	if current.Role == account.RoleAdmin && next.Role != account.RoleAdmin {
		if err := account.CheckAdminChange(current, account.AdminChangeDemote, enabledAdmins); err != nil {
			return err
		}
	}

	if !current.Disabled && next.Disabled {
		return account.CheckAdminChange(current, account.AdminChangeDisable, enabledAdmins)
	}

	return nil
}

// accountWriteError maps a unique violation (the name key, or an id the caller reused) to
// domain.ErrConflict.
func accountWriteError(action string, err error) error {
	if isUniqueViolation(err) {
		return fmt.Errorf("%s: name or id already taken: %w", action, domain.ErrConflict)
	}

	return fmt.Errorf("%s: %w", action, err)
}
