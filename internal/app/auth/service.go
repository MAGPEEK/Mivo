package auth

import (
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/account"
)

// Session lifetimes and cache of ADR 0032 §3.
const (
	// IdleTimeout ends a session that was not used for this long (sliding).
	IdleTimeout = 30 * 24 * time.Hour
	// AbsoluteLifetime ends every session this long after sign-in.
	AbsoluteLifetime = 365 * 24 * time.Hour
	// TouchInterval is the minimum time between two last_seen writes of one session.
	TouchInterval = 5 * time.Minute
	// CacheTTL is how long a resolved token is served from memory.
	CacheTTL = 30 * time.Second
)

// MaxSecretBytes caps a password before it reaches the hasher, so an oversized input cannot
// buy expensive work. It is far above account.MaxPasswordBytes, so it never refuses a valid
// password; longer input is refused like a wrong password.
const MaxSecretBytes = 1024

// maxNameInputBytes caps the raw sign-in name before normalisation for the same reason.
const maxNameInputBytes = 1024

// Dependencies lists everything Service needs. Every field except Logger is required; a nil
// Logger discards.
type Dependencies struct {
	Setup       SetupStore
	Credentials CredentialStore
	Profiles    ProfileStore
	Sessions    SessionStore
	Revoker     SessionRevoker
	PINs        PINStore
	Selector    ProfileSelector
	Devices     DeviceStore
	Hasher      Hasher
	Tokens      TokenSource
	IDs         IDSource
	Clock       Clock
	// Compose is the Unicode NFC composition for account names; cmd/mivo passes
	// norm.NFC.String, because the domain and app layers may use only the standard library.
	Compose account.Composer
	Logger  *slog.Logger
}

// Service implements the auth use cases. It is safe for concurrent use; the throttle, the PIN
// lockout and the session cache are owned by it and live in memory (ADR 0032 §4 and §5: a
// restart clears them).
type Service struct {
	deps     Dependencies
	throttle *throttle
	cache    *sessionCache
	pinLock  *pinLock
}

// NewService returns a Service or an error wrapping domain.ErrInvalid when a required
// dependency is missing.
func NewService(deps Dependencies) (*Service, error) {
	if err := checkDependencies(deps); err != nil {
		return nil, err
	}

	if deps.Logger == nil {
		deps.Logger = slog.New(slog.DiscardHandler)
	}

	return &Service{deps: deps, throttle: newThrottle(), cache: newSessionCache(), pinLock: newPINLock()}, nil
}

func checkDependencies(deps Dependencies) error {
	checks := []struct {
		name    string
		missing bool
	}{
		{"Setup", deps.Setup == nil},
		{"Credentials", deps.Credentials == nil},
		{"Profiles", deps.Profiles == nil},
		{"Sessions", deps.Sessions == nil},
		{"Revoker", deps.Revoker == nil},
		{"PINs", deps.PINs == nil},
		{"Selector", deps.Selector == nil},
		{"Devices", deps.Devices == nil},
		{"Hasher", deps.Hasher == nil},
		{"Tokens", deps.Tokens == nil},
		{"IDs", deps.IDs == nil},
		{"Clock", deps.Clock == nil},
		{"Compose", deps.Compose == nil},
	}

	var errs []error

	for _, check := range checks {
		if check.missing {
			errs = append(errs, fmt.Errorf("auth service: %s is required: %w", check.name, domain.ErrInvalid))
		}
	}

	return errors.Join(errs...)
}
