package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/magpeek/mivo/internal/adapters/sqlite/sqlcgen"
	"github.com/magpeek/mivo/internal/app/auth"
	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/account"
)

// sessionTouchInterval is how old a session's last_seen_at must be before a touch writes it
// again (ADR 0032 §3). The TouchSession query enforces it, so a burst of requests resolving
// the same session costs at most one write per interval.
const sessionTouchInterval = auth.TouchInterval

// SessionStore stores sign-in sessions, which are also the signed-in devices (ADR 0032 §3).
// It keeps only the token hash; hashes are never logged or put into error messages. It
// implements the session ports of app/auth and app/users.
//
// A session's selected profile always belongs to the session's account: every write of
// profile_id checks it in the same statement or transaction, and the resolve query ignores a
// profile of another account.
type SessionStore struct {
	writer *sql.DB
	reader *sql.DB
}

// NewSessionStore returns a store on db, which must be migrated.
func NewSessionStore(db *DB) *SessionStore {
	return &SessionStore{writer: db.writer, reader: db.reader}
}

// SessionDetails is a session resolved with its account (disabled ones too; the caller
// decides) and its selected profile.
type SessionDetails struct {
	Session auth.Session
	Account account.Account
	// Profile is the zero value when ProfileSelected is false.
	Profile         account.Profile
	ProfileSelected bool
}

// AccountSession is a session listed across accounts, with the account's display name.
type AccountSession struct {
	Session     auth.Session
	AccountName string
}

// CreateSession stores a new session. An unknown account, or a selected profile that is
// missing or belongs to another account, returns an error wrapping domain.ErrNotFound; a
// reused session id or token hash returns one wrapping domain.ErrConflict.
func (store *SessionStore) CreateSession(ctx context.Context, session auth.Session) error {
	err := inWriteTransaction(ctx, store.writer, func(queries *sqlcgen.Queries) error {
		if err := checkProfileOwner(ctx, queries, session.AccountID, session.ProfileID); err != nil {
			return err
		}

		return insertSessionRow(ctx, queries, session)
	})
	if err != nil {
		return fmt.Errorf("create session %s: %w", session.ID, err)
	}

	return nil
}

// checkProfileOwner returns domain.ErrNotFound unless profileID is empty or a profile of
// accountID.
func checkProfileOwner(ctx context.Context, queries *sqlcgen.Queries, accountID, profileID string) error {
	if profileID == "" {
		return nil
	}

	count, err := queries.ProfileBelongsToUser(ctx, sqlcgen.ProfileBelongsToUserParams{
		ProfileID: profileID, UserID: accountID,
	})
	if err != nil {
		return fmt.Errorf("check profile %s: %w", profileID, err)
	}

	if count == 0 {
		return fmt.Errorf("profile %s of account %s: %w", profileID, accountID, domain.ErrNotFound)
	}

	return nil
}

func insertSessionRow(ctx context.Context, queries *sqlcgen.Queries, session auth.Session) error {
	err := queries.InsertSession(ctx, sqlcgen.InsertSessionParams{
		ID:           session.ID,
		TokenHash:    session.TokenHash,
		UserID:       session.AccountID,
		ProfileID:    sql.NullString{String: session.ProfileID, Valid: session.ProfileID != ""},
		Remember:     boolToInt(session.Remember),
		DeviceName:   session.Device.Name,
		Client:       string(session.Device.Client),
		Platform:     session.Device.Platform,
		AddressClass: string(session.AddressClass),
		CreatedAt:    session.CreatedAt.UnixMilli(),
		LastSeenAt:   session.LastSeenAt.UnixMilli(),
	})

	switch {
	case err == nil:
		return nil
	case isUniqueViolation(err) || isPrimaryKeyViolation(err):
		// The driver text would name the column; the token hash must not reach a message.
		return fmt.Errorf("session id or token already stored: %w", domain.ErrConflict)
	default:
		return fmt.Errorf("insert session: %w", foreignKeyAsNotFound(err))
	}
}

// SessionByTokenHash returns the session with tokenHash, or an error wrapping
// domain.ErrNotFound.
func (store *SessionStore) SessionByTokenHash(ctx context.Context, tokenHash string) (auth.Session, error) {
	details, err := store.SessionDetailsByTokenHash(ctx, tokenHash)

	return details.Session, err
}

// SessionDetailsByTokenHash resolves tokenHash to the session, its account and its selected
// profile in one query. An unknown hash returns an error wrapping domain.ErrNotFound.
func (store *SessionStore) SessionDetailsByTokenHash(ctx context.Context, tokenHash string) (SessionDetails, error) {
	row, err := sqlcgen.New(store.reader).GetSessionByTokenHash(ctx, tokenHash)
	if errors.Is(err, sql.ErrNoRows) {
		return SessionDetails{}, fmt.Errorf("resolve session: %w", domain.ErrNotFound)
	}

	if err != nil {
		return SessionDetails{}, fmt.Errorf("resolve session: %w", err)
	}

	return sessionDetailsFromRow(row), nil
}

// TouchSession records that the session was used. The write happens only when the stored
// last_seen_at is at least sessionTouchInterval older than seen.At or the address class
// changed; an unknown session is not an error.
func (store *SessionStore) TouchSession(ctx context.Context, sessionID string, seen auth.Seen) error {
	_, err := sqlcgen.New(store.writer).TouchSession(ctx, sqlcgen.TouchSessionParams{
		SeenAt:       seen.At.UnixMilli(),
		AddressClass: string(seen.AddressClass),
		ID:           sessionID,
		StaleBefore:  seen.At.Add(-sessionTouchInterval).UnixMilli(),
	})
	if err != nil {
		return fmt.Errorf("touch session %s: %w", sessionID, err)
	}

	return nil
}

// SelectProfile selects profileID for the session; remember keeps the selection beyond the
// short lapse (ADR 0032 §4). An unknown session, or a profile that is missing or belongs to
// another account, returns an error wrapping domain.ErrNotFound.
func (store *SessionStore) SelectProfile(ctx context.Context, sessionID, profileID string, remember bool) error {
	changed, err := sqlcgen.New(store.writer).SetSessionProfile(ctx, sqlcgen.SetSessionProfileParams{
		ProfileID: sql.NullString{String: profileID, Valid: true},
		Remember:  boolToInt(remember),
		ID:        sessionID,
	})

	return expectOneSessionChanged(fmt.Sprintf("select profile %s for session %s", profileID, sessionID), changed, err)
}

// ClearProfile drops the session's profile selection and its remember flag, so the client
// shows the picker. An unknown session returns an error wrapping domain.ErrNotFound.
func (store *SessionStore) ClearProfile(ctx context.Context, sessionID string) error {
	changed, err := sqlcgen.New(store.writer).ClearSessionProfile(ctx, sessionID)

	return expectOneSessionChanged("clear profile of session "+sessionID, changed, err)
}

func expectOneSessionChanged(operation string, changed int64, err error) error {
	if err != nil {
		return fmt.Errorf("%s: %w", operation, err)
	}

	if changed == 0 {
		return fmt.Errorf("%s: %w", operation, domain.ErrNotFound)
	}

	return nil
}

// fromUnixMilli reads an ADR 0004 timestamp column.
func fromUnixMilli(milliseconds int64) time.Time {
	return time.UnixMilli(milliseconds).UTC()
}
