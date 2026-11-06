package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/magpeek/mivo/internal/adapters/sqlite/sqlcgen"
	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/access"
	"github.com/magpeek/mivo/internal/domain/account"

	sqlitedriver "modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"
)

// ProfileRecord is a profile as stored: the domain profile (HasPIN tells whether a PIN hash is
// stored; the hash itself is read only through ProfilePINHash), its own library access (all =
// the account's set) and its own content limit.
type ProfileRecord struct {
	Profile   account.Profile
	Libraries access.LibraryAccess
	Limit     access.ContentLimit
}

// PINAction says what a ProfileWrite does with the PIN hash.
type PINAction int

// PIN actions of a ProfileWrite.
const (
	// PINKeep leaves the stored PIN hash as it is; on create it means no PIN.
	PINKeep PINAction = iota
	// PINSet replaces the PIN hash with ProfileWrite.PINHash.
	PINSet
	// PINClear removes the PIN.
	PINClear
)

// ProfileWrite is a profile to create or update. PINHash is used only with PINSet.
// Profile.HasPIN is ignored.
type ProfileWrite struct {
	Record    ProfileRecord
	PINAction PINAction
	PINHash   string
}

// ManagedProfileCheck decides inside the create transaction whether one more managed profile
// fits, given the managed profiles the account has in that transaction.
type ManagedProfileCheck func(existingManaged int) error

// CreateProfile adds a managed profile when check (nil allows) agrees inside the transaction;
// independent of check, account.CheckManagedProfileLimit applies (domain.ErrConflict). A
// primary profile is created only with its account (domain.ErrInvalid); an unknown account
// wraps domain.ErrNotFound, an unknown library domain.ErrInvalid.
func (store *AccountStore) CreateProfile(ctx context.Context, write ProfileWrite, check ManagedProfileCheck) error {
	profile := write.Record.Profile
	if profile.Primary {
		return fmt.Errorf("create profile %s: a primary profile comes only with its account: %w", profile.ID, domain.ErrInvalid)
	}

	err := inWriteTransaction(ctx, store.writer, func(queries *sqlcgen.Queries) error {
		if _, err := currentAccount(ctx, queries, profile.AccountID); err != nil {
			return err
		}

		managed, err := queries.CountManagedProfiles(ctx, profile.AccountID)
		if err != nil {
			return fmt.Errorf("count managed profiles: %w", err)
		}

		if err := checkManagedProfiles(int(managed), check); err != nil {
			return err
		}

		return store.insertProfile(ctx, queries, write)
	})
	if err != nil {
		return fmt.Errorf("create profile %s: %w", profile.ID, err)
	}

	return nil
}

func checkManagedProfiles(existingManaged int, check ManagedProfileCheck) error {
	if check != nil {
		if err := check(existingManaged); err != nil {
			return err
		}
	}

	return account.CheckManagedProfileLimit(existingManaged)
}

func (store *AccountStore) insertProfile(ctx context.Context, queries *sqlcgen.Queries, write ProfileWrite) error {
	record := write.Record
	if _, err := account.NewProfile(record.Profile); err != nil {
		return err
	}

	now := store.clock().UnixMilli()
	maxAge, hideUnrated := limitColumns(record.Limit)

	err := queries.InsertProfile(ctx, sqlcgen.InsertProfileParams{
		ID: record.Profile.ID, UserID: record.Profile.AccountID, Name: record.Profile.Name,
		IsPrimary: boolToInt(record.Profile.Primary), AvatarColor: string(record.Profile.AvatarColor),
		PinHash: pinColumn(write), MaxAge: maxAge, HideUnrated: hideUnrated,
		AllLibraries: boolToInt(record.Libraries.IsAll()), CreatedAt: now, UpdatedAt: now,
	})
	if err != nil {
		return accountWriteError("insert profile", err)
	}

	return writeProfileLibraries(ctx, queries, record.Profile.ID, record.Libraries)
}

// UpdateProfile stores name, avatar colour, content limit, library access and the PIN action
// of a profile; the account and the primary flag never change. An unknown profile wraps
// domain.ErrNotFound, an unknown library domain.ErrInvalid.
func (store *AccountStore) UpdateProfile(ctx context.Context, write ProfileWrite) error {
	record := write.Record
	if _, err := account.NewProfile(record.Profile); err != nil {
		return fmt.Errorf("update profile %s: %w", record.Profile.ID, err)
	}

	err := inWriteTransaction(ctx, store.writer, func(queries *sqlcgen.Queries) error {
		now := store.clock().UnixMilli()
		maxAge, hideUnrated := limitColumns(record.Limit)

		changed, err := queries.UpdateProfile(ctx, sqlcgen.UpdateProfileParams{
			Name: record.Profile.Name, AvatarColor: string(record.Profile.AvatarColor),
			MaxAge: maxAge, HideUnrated: hideUnrated, AllLibraries: boolToInt(record.Libraries.IsAll()),
			UpdatedAt: now, ID: record.Profile.ID,
		})
		if err != nil {
			return fmt.Errorf("update profile row: %w", err)
		}

		if changed == 0 {
			return domain.ErrNotFound
		}

		if err := writePIN(ctx, queries, write, now); err != nil {
			return err
		}

		return writeProfileLibraries(ctx, queries, record.Profile.ID, record.Libraries)
	})
	if err != nil {
		return fmt.Errorf("update profile %s: %w", record.Profile.ID, err)
	}

	return nil
}

func writePIN(ctx context.Context, queries *sqlcgen.Queries, write ProfileWrite, now int64) error {
	if write.PINAction == PINKeep {
		return nil
	}

	_, err := queries.SetProfilePINHash(ctx, sqlcgen.SetProfilePINHashParams{
		PinHash: pinColumn(write), UpdatedAt: now, ID: write.Record.Profile.ID,
	})
	if err != nil {
		return fmt.Errorf("update pin hash: %w", err)
	}

	return nil
}

// pinColumn is the stored PIN hash for a write: NULL unless the action sets a non-empty hash.
func pinColumn(write ProfileWrite) sql.NullString {
	if write.PINAction != PINSet {
		return sql.NullString{}
	}

	return sql.NullString{String: write.PINHash, Valid: write.PINHash != ""}
}

// DeleteProfile removes a managed profile; sessions that had it selected keep running without
// a profile. The primary profile goes only with its account (domain.ErrConflict); an unknown
// profile wraps domain.ErrNotFound.
func (store *AccountStore) DeleteProfile(ctx context.Context, profileID string) error {
	err := inWriteTransaction(ctx, store.writer, func(queries *sqlcgen.Queries) error {
		row, err := queries.GetProfile(ctx, profileID)
		if errors.Is(err, sql.ErrNoRows) {
			return domain.ErrNotFound
		}

		if err != nil {
			return fmt.Errorf("read profile: %w", err)
		}

		if err := account.CheckProfileDeletion(profileFromRow(row)); err != nil {
			return err
		}

		if _, err := queries.DeleteProfile(ctx, profileID); err != nil {
			return fmt.Errorf("delete profile row: %w", err)
		}

		return nil
	})
	if err != nil {
		return fmt.Errorf("delete profile %s: %w", profileID, err)
	}

	return nil
}

// writeUserLibraries replaces the account's library rows; "all" keeps none.
func writeUserLibraries(ctx context.Context, queries *sqlcgen.Queries, accountID string, libraries access.LibraryAccess) error {
	if err := queries.DeleteUserLibraries(ctx, accountID); err != nil {
		return fmt.Errorf("clear account libraries: %w", err)
	}

	if libraries.IsAll() {
		return nil
	}

	for _, libraryID := range libraries.IDs() {
		err := queries.InsertUserLibrary(ctx, sqlcgen.InsertUserLibraryParams{UserID: accountID, LibraryID: libraryID})
		if err != nil {
			return grantError(libraryID, err)
		}
	}

	return nil
}

// writeProfileLibraries replaces the profile's library rows; "all" keeps none.
func writeProfileLibraries(ctx context.Context, queries *sqlcgen.Queries, profileID string, libraries access.LibraryAccess) error {
	if err := queries.DeleteProfileLibraries(ctx, profileID); err != nil {
		return fmt.Errorf("clear profile libraries: %w", err)
	}

	if libraries.IsAll() {
		return nil
	}

	for _, libraryID := range libraries.IDs() {
		err := queries.InsertProfileLibrary(ctx, sqlcgen.InsertProfileLibraryParams{ProfileID: profileID, LibraryID: libraryID})
		if err != nil {
			return grantError(libraryID, err)
		}
	}

	return nil
}

// grantError reports an unknown library id as domain.ErrInvalid: it is bad input to an access
// list, not a missing account or profile. The driver detail is dropped.
func grantError(libraryID string, err error) error {
	var driverError *sqlitedriver.Error
	if errors.As(err, &driverError) && driverError.Code() == sqlite3.SQLITE_CONSTRAINT_FOREIGNKEY {
		return fmt.Errorf("grant library %s: unknown library: %w", libraryID, domain.ErrInvalid)
	}

	return fmt.Errorf("grant library %s: %w", libraryID, err)
}
