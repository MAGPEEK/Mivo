package sqlite

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/magpeek/mivo/internal/adapters/sqlite/sqlcgen"
	"github.com/magpeek/mivo/internal/domain/access"
	"github.com/magpeek/mivo/internal/domain/account"
)

// accountFromRow builds the domain account from a users row. The role text is trusted: the
// column's CHECK constraint allows only known roles.
func accountFromRow(row sqlcgen.User) account.Account {
	return account.Account{
		ID: row.ID, Name: row.Name, Role: account.Role(row.Role), Disabled: row.Disabled != 0,
	}
}

// profileFromRow builds the domain profile; HasPIN reflects whether a PIN hash is stored.
func profileFromRow(row sqlcgen.Profile) account.Profile {
	return account.Profile{
		ID: row.ID, AccountID: row.UserID, Name: row.Name, Primary: row.IsPrimary != 0,
		AvatarColor: account.AvatarColor(row.AvatarColor), HasPIN: row.PinHash.Valid,
	}
}

// profileRecord completes a profiles row with its library access and content limit.
func profileRecord(ctx context.Context, queries *sqlcgen.Queries, row sqlcgen.Profile) (ProfileRecord, error) {
	libraryIDs, err := queries.ListProfileLibraryIDs(ctx, row.ID)
	if err != nil {
		return ProfileRecord{}, fmt.Errorf("read libraries of profile %s: %w", row.ID, err)
	}

	limit, err := limitFromColumns(row.MaxAge, row.HideUnrated)
	if err != nil {
		return ProfileRecord{}, fmt.Errorf("read limit of profile %s: %w", row.ID, err)
	}

	return ProfileRecord{
		Profile: profileFromRow(row), Limit: limit, Libraries: libraryAccess(row.AllLibraries, libraryIDs),
	}, nil
}

// libraryAccess turns the all_libraries flag and the access rows into the domain value. With
// the flag unset and no rows it allows nothing, which fails closed.
func libraryAccess(allLibraries int64, libraryIDs []string) access.LibraryAccess {
	if allLibraries != 0 {
		return access.AllLibraries()
	}

	return access.OnlyLibraries(libraryIDs...)
}

// limitColumns stores a limit as max_age (NULL for no age limit) and hide_unrated.
func limitColumns(limit access.ContentLimit) (sql.NullInt64, int64) {
	maxAge, hasMaxAge := limit.MaxAge()

	return sql.NullInt64{Int64: int64(maxAge), Valid: hasMaxAge}, boolToInt(limit.HideUnrated())
}

// limitFromColumns is the inverse of limitColumns.
func limitFromColumns(maxAge sql.NullInt64, hideUnrated int64) (access.ContentLimit, error) {
	if !maxAge.Valid {
		return access.NoLimit().HidingUnrated(hideUnrated != 0), nil
	}

	limit, err := access.MaxAgeLimit(access.AgeBucket(maxAge.Int64), hideUnrated != 0)
	if err != nil {
		return access.ContentLimit{}, fmt.Errorf("stored max age: %w", err)
	}

	return limit, nil
}
