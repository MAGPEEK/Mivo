package auth

import (
	"fmt"
	"math"
	"net/netip"
	"time"

	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/access"
	"github.com/magpeek/mivo/internal/domain/account"
)

var (
	// ErrInvalidCredentials reports a failed sign-in or a wrong current password. It never
	// says whether the name exists (ADR 0032 §2).
	ErrInvalidCredentials = fmt.Errorf("invalid credentials: %w", domain.ErrUnauthenticated)
	// ErrSetupComplete reports a setup attempt after an account exists.
	ErrSetupComplete = fmt.Errorf("setup already completed: %w", domain.ErrConflict)
	// ErrProfileRequired reports a request that needs a selected profile from a session that
	// has none (ADR 0032 §4); adapters/http maps it to 403 "profile_required".
	ErrProfileRequired = fmt.Errorf("profile required: %w", domain.ErrForbidden)
	// ErrPINInvalid reports a missing or wrong PIN; adapters/http maps it to 403 "pin_invalid".
	ErrPINInvalid = fmt.Errorf("pin invalid: %w", domain.ErrForbidden)
)

// ThrottledError reports a sign-in refused by the throttle of ADR 0032 §5. It unwraps to
// domain.ErrRateLimited; RetryAfter tells the client when to try again.
type ThrottledError struct {
	// Wait is the time until a retry may succeed.
	Wait time.Duration
}

// Error describes the refusal without naming the account.
func (throttled *ThrottledError) Error() string {
	return fmt.Sprintf("too many failed sign-ins, retry after %s: %s", throttled.Wait, domain.ErrRateLimited)
}

// Unwrap returns domain.ErrRateLimited.
func (throttled *ThrottledError) Unwrap() error {
	return domain.ErrRateLimited
}

// RetryAfter returns Wait.
func (throttled *ThrottledError) RetryAfter() time.Duration {
	return throttled.Wait
}

// RetryAfterSeconds returns Wait rounded up to whole seconds, at least 1, for the
// Retry-After header.
func (throttled *ThrottledError) RetryAfterSeconds() int {
	return retryAfterSeconds(throttled.Wait)
}

// ProfileLockedError reports a profile selection refused by the PIN lockout of ADR 0032 §4.
// It unwraps to domain.ErrRateLimited; adapters/http tells it apart from ThrottledError with
// errors.As and answers 429 "profile_locked".
type ProfileLockedError struct {
	// Wait is the time until the lock ends.
	Wait time.Duration
}

// Error describes the refusal.
func (locked *ProfileLockedError) Error() string {
	return fmt.Sprintf("profile locked after too many wrong PINs, retry after %s: %s", locked.Wait, domain.ErrRateLimited)
}

// Unwrap returns domain.ErrRateLimited.
func (locked *ProfileLockedError) Unwrap() error {
	return domain.ErrRateLimited
}

// RetryAfter returns Wait.
func (locked *ProfileLockedError) RetryAfter() time.Duration {
	return locked.Wait
}

// RetryAfterSeconds returns Wait rounded up to whole seconds, at least 1, for the
// Retry-After header.
func (locked *ProfileLockedError) RetryAfterSeconds() int {
	return retryAfterSeconds(locked.Wait)
}

func retryAfterSeconds(wait time.Duration) int {
	return max(1, int(math.Ceil(wait.Seconds())))
}

// Client is the kind of signed-in device (ADR 0032 §3).
type Client string

// The known clients. Only ClientWeb uses the session cookie; the others send a bearer token.
const (
	ClientWeb Client = "web"
	ClientTV  Client = "tv"
	ClientCLI Client = "cli"
)

// IsValid reports whether client is one of the known clients.
func (client Client) IsValid() bool {
	switch client {
	case ClientWeb, ClientTV, ClientCLI:
		return true
	default:
		return false
	}
}

// AddressClass tells whether a peer is in the local network or remote.
type AddressClass string

// The address classes stored with a device.
const (
	AddressLAN    AddressClass = "lan"
	AddressRemote AddressClass = "remote"
)

// Peer is the network origin of a request, classified by the HTTP adapter.
type Peer struct {
	// Address is the peer IP; the sign-in throttle counts failures per address.
	Address netip.Addr
	// Class is stored as the device's last address class.
	Class AddressClass
}

// Device describes the signed-in device; the session row is the device (US-25).
type Device struct {
	// Name is the user-visible device name, at most MaxDeviceNameRunes.
	Name string
	// Client is the kind of client.
	Client Client
	// Platform is a short platform label, at most MaxPlatformRunes.
	Platform string
}

// Session is one signed-in device. Only the token hash is stored.
type Session struct {
	ID        string
	TokenHash string
	AccountID string
	// ProfileID is the selected profile or empty when the client must show the picker.
	ProfileID string
	// Remember keeps the profile selection beyond SelectionLapse (ADR 0032 §4).
	Remember     bool
	Device       Device
	CreatedAt    time.Time
	LastSeenAt   time.Time
	AddressClass AddressClass
}

// AccountSession is a session with the name of its account, for the admin device list.
type AccountSession struct {
	Session     Session
	AccountName string
}

// Seen is what TouchSession records.
type Seen struct {
	At           time.Time
	AddressClass AddressClass
}

// FirstAdmin is the account, primary profile and password hash that Setup stores.
type FirstAdmin struct {
	Account      account.Account
	Profile      account.Profile
	PasswordHash string
}

// Credentials is an account with its stored password hash.
type Credentials struct {
	Account      account.Account
	PasswordHash string
}

// ViewerInputs are the stored inputs from which Resolve builds the viewer (ADR 0032 §7).
type ViewerInputs struct {
	Account account.Account
	// Profile is the zero value when no profile was requested.
	Profile          account.Profile
	AccountLibraries access.LibraryAccess
	ProfileLibraries access.LibraryAccess
	ProfileLimit     access.ContentLimit
	// PrimaryLimit is the limit of the account's primary profile; a managed profile never
	// gets a weaker limit than it.
	PrimaryLimit access.ContentLimit
}

// SignInRequest carries the sign-in form and its request context. Setup uses it too.
type SignInRequest struct {
	Name     string
	Password string
	Device   Device
	Peer     Peer
}

// SignInResult is a new session with the token the client must keep. The token is returned
// only here and never stored.
type SignInResult struct {
	Token   string
	Session Session
	Account account.Account
	// Profile is the auto-selected profile when ProfileSelected is true.
	Profile         account.Profile
	ProfileSelected bool
}

// Resolution is a resolved session token.
type Resolution struct {
	Session Session
	Account account.Account
	// Profile is the selected profile when ProfileSelected is true.
	Profile         account.Profile
	ProfileSelected bool
	Viewer          access.Viewer
}
