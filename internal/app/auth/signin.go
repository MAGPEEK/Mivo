package auth

import (
	"context"
	"errors"
	"fmt"

	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/account"
)

// invalidNameKey is the throttle key shared by names that cannot be normalised; such a name
// never exists, and a valid name's key is never empty.
const invalidNameKey = ""

// SignIn checks name and password and starts a session. A wrong name or password, and a
// disabled account, return ErrInvalidCredentials; a throttled attempt returns a
// *ThrottledError (ADR 0032 §5). The answer never reveals whether the name exists.
func (service *Service) SignIn(ctx context.Context, request SignInRequest) (SignInResult, error) {
	device, err := validateDevice(request.Device)
	if err != nil {
		return SignInResult{}, fmt.Errorf("sign in: %w", err)
	}

	nameKey := service.signInNameKey(request.Name)

	attempt, err := service.throttle.begin(nameKey, request.Peer.Address, service.deps.Clock())
	if err != nil {
		return SignInResult{}, fmt.Errorf("sign in: %w", err)
	}

	signedIn, result, err := service.checkCredentials(ctx, nameKey, request.Password)
	attempt.finish(result, service.deps.Clock())

	if err != nil {
		return SignInResult{}, fmt.Errorf("sign in: %w", err)
	}

	if result != outcomeSucceeded {
		return SignInResult{}, ErrInvalidCredentials
	}

	return service.startSession(ctx, signedIn, sessionOrigin{device: device, peer: request.Peer})
}

// signInNameKey normalises like account creation does, so throttling and lookup use one key
// per account however the name is typed.
func (service *Service) signInNameKey(raw string) string {
	if len(raw) > maxNameInputBytes {
		return invalidNameKey
	}

	name, err := account.NormalizeName(raw, service.deps.Compose)
	if err != nil {
		return invalidNameKey
	}

	return account.NameKey(name)
}

// checkCredentials verifies the password of the account with nameKey. A returned error is
// a cancellation or store failure and goes with outcomeAborted.
func (service *Service) checkCredentials(ctx context.Context, nameKey, password string) (account.Account, outcome, error) {
	// Oversized input is refused before any hash work; no stored password can be that long.
	if len(password) > MaxSecretBytes {
		return account.Account{}, outcomeFailed, nil
	}

	credentials, err := service.findCredentials(ctx, nameKey)
	if errors.Is(err, domain.ErrNotFound) {
		if err := service.deps.Hasher.DummyVerify(ctx, password); err != nil {
			return account.Account{}, outcomeAborted, fmt.Errorf("dummy verify: %w", err)
		}

		return account.Account{}, outcomeFailed, nil
	}

	if err != nil {
		return account.Account{}, outcomeAborted, err
	}

	matched, err := service.verifyPassword(ctx, credentials, password)
	if err != nil {
		return account.Account{}, outcomeAborted, err
	}

	if !matched || credentials.Account.Disabled {
		return account.Account{}, outcomeFailed, nil
	}

	return credentials.Account, outcomeSucceeded, nil
}

func (service *Service) findCredentials(ctx context.Context, nameKey string) (Credentials, error) {
	if nameKey == invalidNameKey {
		return Credentials{}, fmt.Errorf("unusable name: %w", domain.ErrNotFound)
	}

	credentials, err := service.deps.Credentials.CredentialsByName(ctx, nameKey)
	if err != nil {
		return Credentials{}, fmt.Errorf("load credentials: %w", err)
	}

	return credentials, nil
}

// verifyPassword checks password against the stored hash and stores a fresh hash when the
// stored one uses weaker parameters. A corrupt stored hash is logged and counts as a wrong
// password, so the client never sees a validation error for its own input.
func (service *Service) verifyPassword(ctx context.Context, credentials Credentials, password string) (bool, error) {
	matched, needsRehash, err := service.verifyStored(ctx, credentials, password)
	if err != nil {
		return false, err
	}

	if matched && needsRehash {
		service.rehash(ctx, credentials.Account.ID, password)
	}

	return matched, nil
}

// verifyStored wraps Hasher.Verify; only a cancellation is returned as an error.
func (service *Service) verifyStored(ctx context.Context, credentials Credentials, password string) (matched, needsRehash bool, err error) {
	matched, needsRehash, err = service.deps.Hasher.Verify(ctx, password, credentials.PasswordHash)
	if err == nil {
		return matched, needsRehash, nil
	}

	if ctx.Err() != nil {
		return false, false, fmt.Errorf("verify password: %w", ctx.Err())
	}

	service.deps.Logger.ErrorContext(ctx, "stored password hash is unusable",
		"account", credentials.Account.ID, "error", err)

	// A corrupt hash fails before any hash work; spend it anyway, so the answer takes as
	// long as a wrong password.
	if err := service.deps.Hasher.DummyVerify(ctx, password); err != nil {
		return false, false, fmt.Errorf("dummy verify: %w", err)
	}

	return false, false, nil
}

// rehash upgrades a hash with weaker parameters. A failure only delays the upgrade to the
// next sign-in, so it is logged and the sign-in goes on.
func (service *Service) rehash(ctx context.Context, accountID, password string) {
	passwordHash, err := service.deps.Hasher.Hash(ctx, password)
	if err == nil {
		err = service.deps.Credentials.SetPasswordHash(ctx, accountID, passwordHash)
	}

	if err != nil {
		service.deps.Logger.WarnContext(ctx, "rehash password", "account", accountID, "error", err)
	}
}

// SignOut deletes the session of token and drops it from the cache. An unknown token is not
// an error.
func (service *Service) SignOut(ctx context.Context, token string) error {
	if token == "" {
		return nil
	}

	tokenHash := service.deps.Tokens.HashToken(token)
	service.cache.evict(func(resolution Resolution) bool { return resolution.Session.TokenHash == tokenHash })

	session, err := service.deps.Sessions.SessionByTokenHash(ctx, tokenHash)
	if errors.Is(err, domain.ErrNotFound) {
		return nil
	}

	if err != nil {
		return fmt.Errorf("sign out: load session: %w", err)
	}

	if err := service.deps.Revoker.DeleteSession(ctx, session.ID); err != nil {
		return fmt.Errorf("sign out: delete session: %w", err)
	}

	service.EvictSession(session.ID)

	return nil
}
