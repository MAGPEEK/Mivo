package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/magpeek/mivo/internal/adapters/sqlite/sqlcgen"
	"github.com/magpeek/mivo/internal/domain/account"
)

// profileNameKeyColumn is how SQLite names the key column in the message of a violated
// profiles_unique_name index ("UNIQUE constraint failed: profiles.user_id, profiles.name_key").
// The result code is the same for every unique index of the table, so only the message tells
// the name index from profiles_one_primary.
const profileNameKeyColumn = "profiles.name_key"

// profileNameKey is the name_key column of a profile named name: account.NameKey, so profile
// names are unique within their account the way account names are unique on the server.
func profileNameKey(name string) sql.NullString {
	return sql.NullString{String: account.NameKey(name), Valid: true}
}

// profileWriteError maps a write to the profiles table: a violated name index is
// account.ErrNameTaken (the API's "name_taken"); every other constraint is mapped as
// accountWriteError does.
func profileWriteError(action string, err error) error {
	if isUniqueViolation(err) && strings.Contains(err.Error(), profileNameKeyColumn) {
		return fmt.Errorf("%s: %w", action, account.ErrNameTaken)
	}

	return accountWriteError(action, err)
}

// BackfillProfileNameKeys gives every profile written before migration 00015 its name key and
// returns how many profiles are still without one. It is idempotent and meant to run at
// start-up.
//
// A profile stays without a key when another profile of its account already holds that key:
// of two such legacy rows the older one (created_at, then id) gets it. The newer one is
// neither renamed nor deleted, because the server cannot know which of the two the household
// wants to keep; it stays usable under its name and gets its key with its next rename (any
// other edit of it answers account.ErrNameTaken until then). The caller should log a count
// above zero.
func (store *AccountStore) BackfillProfileNameKeys(ctx context.Context) (int, error) {
	unresolved := 0

	err := inWriteTransaction(ctx, store.writer, func(queries *sqlcgen.Queries) error {
		legacy, err := queries.ListProfilesWithoutNameKey(ctx)
		if err != nil {
			return fmt.Errorf("list profiles without a name key: %w", err)
		}

		unresolved = 0

		for _, profile := range legacy {
			changed, err := queries.SetProfileNameKeyIfFree(ctx, sqlcgen.SetProfileNameKeyIfFreeParams{
				NameKey: profileNameKey(profile.Name), ID: profile.ID,
			})
			if err != nil {
				return fmt.Errorf("set name key of profile %s: %w", profile.ID, err)
			}

			if changed == 0 {
				unresolved++
			}
		}

		return nil
	})
	if err != nil {
		return 0, fmt.Errorf("backfill profile name keys: %w", err)
	}

	return unresolved, nil
}
