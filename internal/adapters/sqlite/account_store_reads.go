package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/magpeek/mivo/internal/adapters/sqlite/sqlcgen"
	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/access"
	"github.com/magpeek/mivo/internal/domain/account"
)

// AccountSummary is one row of the users list (US-20).
type AccountSummary struct {
	AccountRecord

	// CreatedAt is when the account was created.
	CreatedAt time.Time
	// ProfileCount counts the primary and the managed profiles.
	ProfileCount int
	// LastActiveAt is the newest session activity; the zero time means never.
	LastActiveAt time.Time
}

// AccountCredentials is an account with its password hash, for sign-in and password changes.
type AccountCredentials struct {
	Record       AccountRecord
	PasswordHash string
}

// AccessInputs is what the viewer resolver (M7-15) combines into the effective library access
// and content limit of a profile (ADR 0032 §7): the account with its access, the profile with
// its access and limit, and the limit of the account's primary profile. The store applies no
// rule; for a primary profile PrimaryLimit equals ProfileLimit.
type AccessInputs struct {
	Account          account.Account
	AccountLibraries access.LibraryAccess
	Profile          account.Profile
	ProfileLibraries access.LibraryAccess
	ProfileLimit     access.ContentLimit
	PrimaryLimit     access.ContentLimit
}

// GetAccount returns one account by id. An unknown id wraps domain.ErrNotFound.
func (store *AccountStore) GetAccount(ctx context.Context, accountID string) (AccountRecord, error) {
	credentials, err := store.CredentialsByID(ctx, accountID)

	return credentials.Record, err
}

// FindAccountByNameKey returns the account whose account.NameKey equals nameKey. An unknown
// key wraps domain.ErrNotFound.
func (store *AccountStore) FindAccountByNameKey(ctx context.Context, nameKey string) (AccountRecord, error) {
	credentials, err := store.CredentialsByNameKey(ctx, nameKey)

	return credentials.Record, err
}

// CredentialsByID returns the account with its password hash. An unknown id wraps
// domain.ErrNotFound.
func (store *AccountStore) CredentialsByID(ctx context.Context, accountID string) (AccountCredentials, error) {
	return store.readCredentials(ctx, "account "+accountID, func(queries *sqlcgen.Queries) (sqlcgen.User, error) {
		return queries.GetUser(ctx, accountID)
	})
}

// CredentialsByNameKey returns the account whose name has the given account.NameKey with its
// password hash, for sign-in. An unknown key wraps domain.ErrNotFound.
func (store *AccountStore) CredentialsByNameKey(ctx context.Context, nameKey string) (AccountCredentials, error) {
	return store.readCredentials(ctx, "account by name", func(queries *sqlcgen.Queries) (sqlcgen.User, error) {
		return queries.GetUserByNameKey(ctx, nameKey)
	})
}

func (store *AccountStore) readCredentials(ctx context.Context, subject string,
	read func(*sqlcgen.Queries) (sqlcgen.User, error),
) (AccountCredentials, error) {
	var credentials AccountCredentials

	err := inReadTransaction(ctx, store.reader, func(queries *sqlcgen.Queries) error {
		row, err := read(queries)
		if errors.Is(err, sql.ErrNoRows) {
			return domain.ErrNotFound
		}

		if err != nil {
			return fmt.Errorf("read account row: %w", err)
		}

		record, err := completeAccountRecord(ctx, queries, row)
		credentials = AccountCredentials{Record: record, PasswordHash: row.PasswordHash}

		return err
	})
	if err != nil {
		return AccountCredentials{}, fmt.Errorf("read %s: %w", subject, err)
	}

	return credentials, nil
}

// readAccountRecord reads the account inside a transaction; an unknown id wraps
// domain.ErrNotFound.
func readAccountRecord(ctx context.Context, queries *sqlcgen.Queries, accountID string) (AccountRecord, error) {
	row, err := queries.GetUser(ctx, accountID)
	if errors.Is(err, sql.ErrNoRows) {
		return AccountRecord{}, fmt.Errorf("account %s: %w", accountID, domain.ErrNotFound)
	}

	if err != nil {
		return AccountRecord{}, fmt.Errorf("read account %s: %w", accountID, err)
	}

	return completeAccountRecord(ctx, queries, row)
}

// currentAccount reads only the domain account inside a transaction.
func currentAccount(ctx context.Context, queries *sqlcgen.Queries, accountID string) (account.Account, error) {
	row, err := queries.GetUser(ctx, accountID)
	if errors.Is(err, sql.ErrNoRows) {
		return account.Account{}, fmt.Errorf("account %s: %w", accountID, domain.ErrNotFound)
	}

	if err != nil {
		return account.Account{}, fmt.Errorf("read account %s: %w", accountID, err)
	}

	return accountFromRow(row), nil
}

// completeAccountRecord adds the library access and the primary profile's limit to a row.
func completeAccountRecord(ctx context.Context, queries *sqlcgen.Queries, row sqlcgen.User) (AccountRecord, error) {
	libraryIDs, err := queries.ListUserLibraryIDs(ctx, row.ID)
	if err != nil {
		return AccountRecord{}, fmt.Errorf("read account libraries: %w", err)
	}

	primary, err := queries.GetPrimaryProfile(ctx, row.ID)
	if err != nil {
		return AccountRecord{}, fmt.Errorf("read primary profile: %w", err)
	}

	limit, err := limitFromColumns(primary.MaxAge, primary.HideUnrated)
	if err != nil {
		return AccountRecord{}, err
	}

	return AccountRecord{
		Account: accountFromRow(row), Libraries: libraryAccess(row.AllLibraries, libraryIDs), PrimaryLimit: limit,
	}, nil
}

// ListAccounts returns every account ordered by name key.
func (store *AccountStore) ListAccounts(ctx context.Context) ([]AccountSummary, error) {
	var summaries []AccountSummary

	err := inReadTransaction(ctx, store.reader, func(queries *sqlcgen.Queries) error {
		rows, err := queries.ListUserSummaries(ctx)
		if err != nil {
			return fmt.Errorf("read accounts: %w", err)
		}

		grants, err := queries.ListAllUserLibraryIDs(ctx)
		if err != nil {
			return fmt.Errorf("read account libraries: %w", err)
		}

		summaries, err = summariesFromRows(rows, grants)

		return err
	})
	if err != nil {
		return nil, fmt.Errorf("list accounts: %w", err)
	}

	return summaries, nil
}

func summariesFromRows(rows []sqlcgen.ListUserSummariesRow, grants []sqlcgen.UserLibrary) ([]AccountSummary, error) {
	libraryIDs := make(map[string][]string)
	for _, grant := range grants {
		libraryIDs[grant.UserID] = append(libraryIDs[grant.UserID], grant.LibraryID)
	}

	summaries := make([]AccountSummary, 0, len(rows))

	for _, row := range rows {
		limit, err := limitFromColumns(row.PrimaryMaxAge, row.PrimaryHideUnrated)
		if err != nil {
			return nil, fmt.Errorf("account %s: %w", row.ID, err)
		}

		summary := AccountSummary{
			AccountRecord: AccountRecord{
				Account: account.Account{
					ID: row.ID, Name: row.Name, Role: account.Role(row.Role), Disabled: row.Disabled != 0,
				},
				Libraries: libraryAccess(row.AllLibraries, libraryIDs[row.ID]), PrimaryLimit: limit,
			},
			CreatedAt:    time.UnixMilli(row.CreatedAt).UTC(),
			ProfileCount: int(row.ProfileCount),
		}

		if row.LastActivityAt > 0 {
			summary.LastActiveAt = time.UnixMilli(row.LastActivityAt).UTC()
		}

		summaries = append(summaries, summary)
	}

	return summaries, nil
}

// GetProfile returns the profile with its limit and library access. An unknown id wraps
// domain.ErrNotFound.
func (store *AccountStore) GetProfile(ctx context.Context, profileID string) (ProfileRecord, error) {
	var record ProfileRecord

	err := inReadTransaction(ctx, store.reader, func(queries *sqlcgen.Queries) error {
		row, err := readProfileRow(ctx, queries, profileID)
		if err != nil {
			return err
		}

		record, err = profileRecord(ctx, queries, row)

		return err
	})
	if err != nil {
		return ProfileRecord{}, fmt.Errorf("read profile %s: %w", profileID, err)
	}

	return record, nil
}

// ProfilePINHash returns the profile's PIN hash, empty when it has no PIN, for the PIN check
// (M7-16). An unknown id wraps domain.ErrNotFound.
func (store *AccountStore) ProfilePINHash(ctx context.Context, profileID string) (string, error) {
	row, err := readProfileRow(ctx, sqlcgen.New(store.reader), profileID)
	if err != nil {
		return "", fmt.Errorf("read pin hash of profile %s: %w", profileID, err)
	}

	return row.PinHash.String, nil
}

func readProfileRow(ctx context.Context, queries *sqlcgen.Queries, profileID string) (sqlcgen.Profile, error) {
	row, err := queries.GetProfile(ctx, profileID)
	if errors.Is(err, sql.ErrNoRows) {
		return sqlcgen.Profile{}, domain.ErrNotFound
	}

	if err != nil {
		return sqlcgen.Profile{}, fmt.Errorf("read profile row: %w", err)
	}

	return row, nil
}

// ListProfiles returns the account's profiles, the primary one first, then the managed ones
// in creation order. An unknown account has no profiles.
func (store *AccountStore) ListProfiles(ctx context.Context, accountID string) ([]ProfileRecord, error) {
	var records []ProfileRecord

	err := inReadTransaction(ctx, store.reader, func(queries *sqlcgen.Queries) error {
		rows, err := queries.ListProfilesOfUser(ctx, accountID)
		if err != nil {
			return fmt.Errorf("read profile rows: %w", err)
		}

		records = make([]ProfileRecord, 0, len(rows))

		for _, row := range rows {
			record, err := profileRecord(ctx, queries, row)
			if err != nil {
				return err
			}

			records = append(records, record)
		}

		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("list profiles of account %s: %w", accountID, err)
	}

	return records, nil
}

// AccessInputs reads, in one snapshot, what the effective access and limit of the profile are
// built from. An unknown profile wraps domain.ErrNotFound.
func (store *AccountStore) AccessInputs(ctx context.Context, profileID string) (AccessInputs, error) {
	var inputs AccessInputs

	err := inReadTransaction(ctx, store.reader, func(queries *sqlcgen.Queries) error {
		row, err := readProfileRow(ctx, queries, profileID)
		if err != nil {
			return err
		}

		profile, err := profileRecord(ctx, queries, row)
		if err != nil {
			return err
		}

		owner, err := readAccountRecord(ctx, queries, row.UserID)
		if err != nil {
			return err
		}

		inputs = AccessInputs{
			Account: owner.Account, AccountLibraries: owner.Libraries,
			Profile: profile.Profile, ProfileLibraries: profile.Libraries,
			ProfileLimit: profile.Limit, PrimaryLimit: owner.PrimaryLimit,
		}

		return nil
	})
	if err != nil {
		return AccessInputs{}, fmt.Errorf("read access inputs of profile %s: %w", profileID, err)
	}

	return inputs, nil
}
