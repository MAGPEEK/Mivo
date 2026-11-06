package sqlite

import (
	"github.com/magpeek/mivo/internal/adapters/sqlite/sqlcgen"
	"github.com/magpeek/mivo/internal/app/auth"
	"github.com/magpeek/mivo/internal/domain/account"
)

// sessionFromRow builds the session from an auth_sessions row. Client and address class are
// trusted: the columns' CHECK constraints allow only known values.
func sessionFromRow(row sqlcgen.AuthSession) auth.Session {
	return auth.Session{
		ID:        row.ID,
		TokenHash: row.TokenHash,
		AccountID: row.UserID,
		ProfileID: row.ProfileID.String,
		Remember:  row.Remember != 0,
		Device: auth.Device{
			Name: row.DeviceName, Client: auth.Client(row.Client), Platform: row.Platform,
		},
		CreatedAt:    fromUnixMilli(row.CreatedAt),
		LastSeenAt:   fromUnixMilli(row.LastSeenAt),
		AddressClass: auth.AddressClass(row.AddressClass),
	}
}

// sessionDetailsFromRow splits the resolve row. The profile columns come from a join on the
// profile id and the account id, so a profile of another account reads as no selection.
func sessionDetailsFromRow(row sqlcgen.GetSessionByTokenHashRow) SessionDetails {
	session := sessionFromRow(sqlcgen.AuthSession{
		ID: row.ID, TokenHash: row.TokenHash, UserID: row.UserID, ProfileID: row.ProfileID,
		Remember: row.Remember, DeviceName: row.DeviceName, Client: row.Client, Platform: row.Platform,
		AddressClass: row.AddressClass, CreatedAt: row.CreatedAt, LastSeenAt: row.LastSeenAt,
	})

	details := SessionDetails{
		Session: session,
		Account: account.Account{
			ID: row.UserID, Name: row.UserName, Role: account.Role(row.UserRole), Disabled: row.UserDisabled != 0,
		},
	}

	if row.ProfileID.Valid {
		details.ProfileSelected = true
		details.Profile = account.Profile{
			ID: row.ProfileID.String, AccountID: row.UserID, Name: row.ProfileName.String,
			Primary: row.ProfileIsPrimary.Int64 != 0, AvatarColor: account.AvatarColor(row.ProfileAvatarColor.String),
			HasPIN: row.ProfileHasPin,
		}
	}

	return details
}

func accountSessionFromRow(row sqlcgen.ListAllSessionsRow) AccountSession {
	return AccountSession{
		Session: sessionFromRow(sqlcgen.AuthSession{
			ID: row.ID, TokenHash: row.TokenHash, UserID: row.UserID, ProfileID: row.ProfileID,
			Remember: row.Remember, DeviceName: row.DeviceName, Client: row.Client, Platform: row.Platform,
			AddressClass: row.AddressClass, CreatedAt: row.CreatedAt, LastSeenAt: row.LastSeenAt,
		}),
		AccountName: row.UserName,
	}
}
