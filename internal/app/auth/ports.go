// Package auth contains the account sign-in use cases of ADR 0032: first-run setup, sign-in
// and sign-out, password change, session resolution with a short cache, sign-in throttling,
// profile selection with the PIN lockout, the device list with revoke and the periodic purge
// of expired sessions. It declares the ports it needs; adapters
// implement them (adapters/sqlite, adapters/credentials) and cmd/mivo wires them.
package auth

import (
	"context"
	"time"

	"github.com/magpeek/mivo/internal/domain/account"
)

// SetupStore creates the first admin account. Implemented by adapters/sqlite (M7-08).
type SetupStore interface {
	// HasAccounts reports whether at least one account exists.
	HasAccounts(ctx context.Context) (bool, error)
	// CreateFirstAdmin stores the admin account with its primary profile in one transaction
	// that only succeeds while no account exists; otherwise it returns an error wrapping
	// domain.ErrConflict.
	CreateFirstAdmin(ctx context.Context, admin FirstAdmin) error
}

// CredentialStore reads and changes password hashes. Implemented by adapters/sqlite (M7-08).
type CredentialStore interface {
	// CredentialsByName returns the account whose account.NameKey equals nameKey, or an error
	// wrapping domain.ErrNotFound.
	CredentialsByName(ctx context.Context, nameKey string) (Credentials, error)
	// CredentialsByID returns the account with accountID, or an error wrapping
	// domain.ErrNotFound.
	CredentialsByID(ctx context.Context, accountID string) (Credentials, error)
	// SetPasswordHash replaces the account's password hash.
	SetPasswordHash(ctx context.Context, accountID, passwordHash string) error
}

// ProfileStore reads the profiles and access inputs of an account. Implemented by
// adapters/sqlite (M7-08).
type ProfileStore interface {
	// Profiles returns every profile of the account, the primary one included.
	Profiles(ctx context.Context, accountID string) ([]account.Profile, error)
	// ViewerInputs returns the account and, when profileID is not empty, the profile with its
	// access inputs. A missing account or profile, or a profile of another account, returns
	// an error wrapping domain.ErrNotFound.
	ViewerInputs(ctx context.Context, accountID, profileID string) (ViewerInputs, error)
}

// SessionStore creates and reads sessions (one per signed-in device). Implemented by
// adapters/sqlite (M7-08); the store keeps only the token hash.
type SessionStore interface {
	// CreateSession stores a new session.
	CreateSession(ctx context.Context, session Session) error
	// SessionByTokenHash returns the session with tokenHash, or an error wrapping
	// domain.ErrNotFound.
	SessionByTokenHash(ctx context.Context, tokenHash string) (Session, error)
	// TouchSession records that the session was used; the store writes last_seen and the
	// address class. Resolve calls it at most once per TouchInterval and session.
	TouchSession(ctx context.Context, sessionID string, seen Seen) error
}

// SessionRevoker deletes sessions. Implemented by adapters/sqlite (M7-08).
type SessionRevoker interface {
	// DeleteSession deletes one session; an unknown ID is not an error.
	DeleteSession(ctx context.Context, sessionID string) error
	// DeleteOtherSessions deletes every session of the account except keepSessionID (which
	// may be empty to delete all).
	DeleteOtherSessions(ctx context.Context, accountID, keepSessionID string) error
	// PurgeSessions deletes the sessions last seen at or before idleBefore or created at or
	// before createdBefore (the bounds count as expired, like Resolve) and returns how many
	// it deleted.
	PurgeSessions(ctx context.Context, idleBefore, createdBefore time.Time) (int64, error)
}

// PINStore reads profile PIN hashes. Implemented by adapters/sqlite (M7-08).
type PINStore interface {
	// ProfilePINHash returns the profile's PIN hash, empty when it has no PIN, or an error
	// wrapping domain.ErrNotFound.
	ProfilePINHash(ctx context.Context, profileID string) (string, error)
}

// ProfileSelector stores the profile selection of a session. Implemented by adapters/sqlite.
type ProfileSelector interface {
	// SetSessionProfile sets the session's profile and remember flag; an empty profileID
	// clears the selection. An unknown session returns an error wrapping domain.ErrNotFound.
	SetSessionProfile(ctx context.Context, sessionID, profileID string, remember bool) error
}

// DeviceStore lists sessions as devices (US-25). Implemented by adapters/sqlite.
type DeviceStore interface {
	// AccountSessions returns every session of the account.
	AccountSessions(ctx context.Context, accountID string) ([]Session, error)
	// AllSessions returns every session of every account with the account name.
	AllSessions(ctx context.Context) ([]AccountSession, error)
	// SessionByID returns the session with sessionID, or an error wrapping
	// domain.ErrNotFound.
	SessionByID(ctx context.Context, sessionID string) (Session, error)
}

// Hasher hashes and verifies passwords and PINs. Implemented by adapters/credentials (M7-07).
type Hasher interface {
	// Hash returns an encoded hash of secret with a fresh salt.
	Hash(ctx context.Context, secret string) (string, error)
	// Verify reports whether secret matches encoded; needsRehash asks for a fresh Hash
	// because encoded uses weaker parameters.
	Verify(ctx context.Context, secret, encoded string) (ok, needsRehash bool, err error)
	// DummyVerify spends the work of one Verify, so unknown names cost the same time.
	DummyVerify(ctx context.Context, secret string) error
}

// TokenSource creates session tokens and hashes them for storage. Implemented by
// adapters/credentials (M7-07).
type TokenSource interface {
	// New returns a fresh random token and its HashToken.
	New() (token, hash string, err error)
	// HashToken returns the storage hash of token.
	HashToken(token string) string
}

// IDSource creates new random entity IDs. Implemented by adapters/uuidgen.
type IDSource interface {
	// NewID returns a new random ID.
	NewID() (string, error)
}

// Clock returns the current time; injected so throttling and expiry are testable.
type Clock func() time.Time
