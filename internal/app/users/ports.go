package users

import (
	"context"
	"time"

	"github.com/magpeek/mivo/internal/domain/access"
	"github.com/magpeek/mivo/internal/domain/account"
)

// AccountRecord is an account as the administration stores it: the identity, its library
// access (admins always have all libraries) and the content limit of its primary profile.
type AccountRecord struct {
	Account      account.Account
	Libraries    access.LibraryAccess
	PrimaryLimit access.ContentLimit
}

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

// ProfileRecord is a profile with its own library access (all = the account's set) and its own
// content limit, persisted as maxAge/hideUnrated (ADR 0032 §7).
type ProfileRecord struct {
	Profile   account.Profile
	Libraries access.LibraryAccess
	Limit     access.ContentLimit
}

// NewAccountRecord is everything the store writes for a new account in one transaction: the
// account, its primary profile and the password hash.
type NewAccountRecord struct {
	Account      AccountRecord
	Primary      account.Profile
	PasswordHash string
}

// PINAction says what an update does with the PIN hash of a profile.
type PINAction int

// PIN actions of a ProfileWrite.
const (
	// PINKeep leaves the stored PIN hash as it is.
	PINKeep PINAction = iota
	// PINSet replaces the PIN hash with ProfileWrite.PINHash.
	PINSet
	// PINClear removes the PIN.
	PINClear
)

// ProfileWrite is a profile to create or update. PINHash is used only with PINSet; on create
// PINKeep means no PIN.
type ProfileWrite struct {
	Record    ProfileRecord
	PINAction PINAction
	PINHash   string
}

// AccountChange computes the new state of an account from the current one. The store calls it
// inside the write transaction with the number of enabled admins counted in that transaction,
// so the last-admin rule cannot be raced by two concurrent changes (ADR 0032 §1).
type AccountChange func(current AccountRecord, enabledAdmins int) (AccountRecord, error)

// AccountDeletionCheck decides inside the delete transaction whether the account may go.
type AccountDeletionCheck func(current account.Account, enabledAdmins int) error

// ManagedProfileCheck decides inside the create transaction whether one more managed profile
// fits, given the managed profiles the account has in that transaction.
type ManagedProfileCheck func(existingManaged int) error

// AccountReader reads accounts. Missing accounts are domain.ErrNotFound.
type AccountReader interface {
	// ListAccounts returns every account ordered by name.
	ListAccounts(ctx context.Context) ([]AccountSummary, error)
	// GetAccount returns one account by id.
	GetAccount(ctx context.Context, accountID string) (AccountRecord, error)
	// FindAccountByNameKey returns the account whose account.NameKey equals nameKey.
	FindAccountByNameKey(ctx context.Context, nameKey string) (AccountRecord, error)
}

// AccountWriter writes accounts. Duplicate name keys are domain.ErrConflict, unknown library
// ids domain.ErrInvalid, missing accounts domain.ErrNotFound.
type AccountWriter interface {
	// CreateAccount stores the account with its primary profile and password hash.
	CreateAccount(ctx context.Context, record NewAccountRecord) error
	// UpdateAccount applies change inside one transaction and stores its result; a new name
	// also renames the primary profile, which carries the account name (ADR 0032 §1).
	UpdateAccount(ctx context.Context, accountID string, change AccountChange) (AccountRecord, error)
	// DeleteAccount removes the account, its profiles and sessions when check allows it.
	DeleteAccount(ctx context.Context, accountID string, check AccountDeletionCheck) error
}

// PasswordWriter stores password hashes; the hash never leaves the store otherwise.
type PasswordWriter interface {
	// SetPasswordHash replaces the account's password hash.
	SetPasswordHash(ctx context.Context, accountID, hash string) error
}

// ProfileReader reads profiles. Missing profiles are domain.ErrNotFound.
type ProfileReader interface {
	// GetProfile returns one profile by id.
	GetProfile(ctx context.Context, profileID string) (ProfileRecord, error)
	// ListProfiles returns the account's profiles, primary first.
	ListProfiles(ctx context.Context, accountID string) ([]ProfileRecord, error)
}

// ProfileWriter writes managed profiles and profile settings.
type ProfileWriter interface {
	// CreateProfile stores a managed profile when check allows it inside the transaction.
	CreateProfile(ctx context.Context, profile ProfileWrite, check ManagedProfileCheck) error
	// UpdateProfile stores the profile's fields, library access, limit and PIN action.
	UpdateProfile(ctx context.Context, profile ProfileWrite) error
	// DeleteProfile removes a profile of the account; the caller has checked it is not the
	// primary. A profile of another account is domain.ErrNotFound.
	DeleteProfile(ctx context.Context, accountID, profileID string) error
}

// Hasher hashes passwords and PINs (argon2id, adapters/credentials).
type Hasher interface {
	// Hash returns the encoded hash of secret.
	Hash(ctx context.Context, secret string) (string, error)
}

// IDSource creates the ids of new accounts and profiles (adapters/uuidgen).
type IDSource interface {
	// NewID returns a new random id.
	NewID() (string, error)
}

// SessionRevoker ends sign-in sessions. cmd/mivo wires the auth service while the server runs
// and the session store for the CLI, which has no server cache to update.
type SessionRevoker interface {
	// RevokeAccountSessions deletes every session of the account.
	RevokeAccountSessions(ctx context.Context, accountID string) error
}

// ResolveCache forgets cached session resolutions (app/auth), so a changed access, limit,
// disabled state or PIN applies to the next request instead of after the cache lifetime.
type ResolveCache interface {
	// EvictAccount drops every cached session resolution of the account.
	EvictAccount(accountID string)
}

// ViewerSource returns the viewer of the request; cmd/mivo wires app/access.ViewerFrom.
type ViewerSource func(ctx context.Context) (access.Viewer, bool)
