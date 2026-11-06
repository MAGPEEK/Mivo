package sqlite

import (
	"context"
	"fmt"
	"time"

	"github.com/magpeek/mivo/internal/adapters/sqlite/sqlcgen"
	"github.com/magpeek/mivo/internal/app/auth"
)

// ListSessions returns the sessions of the account, most recently used first.
func (store *SessionStore) ListSessions(ctx context.Context, accountID string) ([]auth.Session, error) {
	rows, err := sqlcgen.New(store.reader).ListUserSessions(ctx, accountID)
	if err != nil {
		return nil, fmt.Errorf("list sessions of account %s: %w", accountID, err)
	}

	sessions := make([]auth.Session, 0, len(rows))
	for _, row := range rows {
		sessions = append(sessions, sessionFromRow(row))
	}

	return sessions, nil
}

// ListAllSessions returns the sessions of every account with the account name, most recently
// used first; it serves the admin device list.
func (store *SessionStore) ListAllSessions(ctx context.Context) ([]AccountSession, error) {
	rows, err := sqlcgen.New(store.reader).ListAllSessions(ctx)
	if err != nil {
		return nil, fmt.Errorf("list all sessions: %w", err)
	}

	sessions := make([]AccountSession, 0, len(rows))
	for _, row := range rows {
		sessions = append(sessions, accountSessionFromRow(row))
	}

	return sessions, nil
}

// DeleteSession deletes one session of any account (the admin path); an unknown id is not an
// error.
func (store *SessionStore) DeleteSession(ctx context.Context, sessionID string) error {
	if _, err := sqlcgen.New(store.writer).DeleteSession(ctx, sessionID); err != nil {
		return fmt.Errorf("delete session %s: %w", sessionID, err)
	}

	return nil
}

// DeleteAccountSession deletes the session only when it belongs to accountID, so one user
// cannot end another user's device. It returns the token hashes it deleted (none for an
// unknown id or another account's session) so the caller can evict them from its cache.
func (store *SessionStore) DeleteAccountSession(ctx context.Context, accountID, sessionID string) ([]string, error) {
	tokenHashes, err := sqlcgen.New(store.writer).DeleteUserSession(ctx, sqlcgen.DeleteUserSessionParams{
		ID: sessionID, UserID: accountID,
	})
	if err != nil {
		return nil, fmt.Errorf("delete session %s of account %s: %w", sessionID, accountID, err)
	}

	return tokenHashes, nil
}

// DeleteOtherSessions deletes every session of the account except keepSessionID, which may be
// empty to delete all.
func (store *SessionStore) DeleteOtherSessions(ctx context.Context, accountID, keepSessionID string) error {
	_, err := store.RevokeOtherSessions(ctx, accountID, keepSessionID)

	return err
}

// RevokeOtherSessions is DeleteOtherSessions returning the deleted token hashes.
func (store *SessionStore) RevokeOtherSessions(ctx context.Context, accountID, keepSessionID string) ([]string, error) {
	tokenHashes, err := sqlcgen.New(store.writer).DeleteUserSessionsExcept(ctx, sqlcgen.DeleteUserSessionsExceptParams{
		UserID: accountID, KeepID: keepSessionID,
	})
	if err != nil {
		return nil, fmt.Errorf("delete other sessions of account %s: %w", accountID, err)
	}

	return tokenHashes, nil
}

// RevokeAccountSessions deletes every session of the account (app/users port, used where no
// server cache needs updating, such as the CLI).
func (store *SessionStore) RevokeAccountSessions(ctx context.Context, accountID string) error {
	return store.DeleteOtherSessions(ctx, accountID, "")
}

// PurgeSessions deletes the sessions last seen at or before idleBefore or created at or
// before createdBefore and returns how many it deleted.
func (store *SessionStore) PurgeSessions(ctx context.Context, idleBefore, createdBefore time.Time) (int64, error) {
	tokenHashes, err := store.PurgeExpiredSessions(ctx, idleBefore, createdBefore)

	return int64(len(tokenHashes)), err
}

// PurgeExpiredSessions is PurgeSessions returning the deleted token hashes.
func (store *SessionStore) PurgeExpiredSessions(ctx context.Context, idleBefore, createdBefore time.Time) ([]string, error) {
	tokenHashes, err := sqlcgen.New(store.writer).DeleteExpiredSessions(ctx, sqlcgen.DeleteExpiredSessionsParams{
		IdleBefore: idleBefore.UnixMilli(), CreatedBefore: createdBefore.UnixMilli(),
	})
	if err != nil {
		return nil, fmt.Errorf("purge expired sessions: %w", err)
	}

	return tokenHashes, nil
}
