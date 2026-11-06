package auth

import (
	"context"
	"errors"
	"fmt"

	"github.com/magpeek/mivo/internal/app/access"
	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/account"
)

// PasswordChange is a signed-in viewer's request to replace its account password.
type PasswordChange struct {
	Current string
	New     string
	// Peer feeds the throttle, so the current password cannot be guessed through a session.
	Peer Peer
}

// ChangePassword replaces the password of the viewer's account (from ctx) and revokes the
// account's other sessions; the current one stays signed in. Without a viewer it returns
// domain.ErrUnauthenticated, without the account's primary profile selected
// domain.ErrForbidden (a child's profile must not change the account password even when it
// knows it), a wrong current password ErrInvalidCredentials, an invalid new one
// domain.ErrInvalid; guessing is throttled like sign-in.
func (service *Service) ChangePassword(ctx context.Context, change PasswordChange) error {
	viewer, ok := access.ViewerFrom(ctx)
	if !ok || viewer.AccountID == "" {
		return fmt.Errorf("change password: %w", domain.ErrUnauthenticated)
	}

	if err := service.requirePrimaryProfile(ctx, viewer.AccountID, viewer.ProfileID); err != nil {
		return fmt.Errorf("change password: %w", err)
	}

	if err := account.ValidatePassword(change.New); err != nil {
		return fmt.Errorf("change password: %w", err)
	}

	credentials, err := service.deps.Credentials.CredentialsByID(ctx, viewer.AccountID)
	if errors.Is(err, domain.ErrNotFound) {
		return fmt.Errorf("change password: account gone: %w", domain.ErrUnauthenticated)
	}

	if err != nil {
		return fmt.Errorf("change password: load credentials: %w", err)
	}

	if err := service.checkCurrentPassword(ctx, credentials, change); err != nil {
		return fmt.Errorf("change password: %w", err)
	}

	return service.replacePassword(ctx, viewer.AccountID, viewer.AuthSessionID, change.New)
}

// requirePrimaryProfile returns domain.ErrForbidden unless profileID is the primary profile of
// the account. It reads the store, not the viewer, so it also holds for a hand-built viewer.
func (service *Service) requirePrimaryProfile(ctx context.Context, accountID, profileID string) error {
	if profileID == "" {
		return fmt.Errorf("no profile selected: %w", domain.ErrForbidden)
	}

	profiles, err := service.deps.Profiles.Profiles(ctx, accountID)
	if err != nil {
		return fmt.Errorf("load profiles: %w", err)
	}

	for _, profile := range profiles {
		if profile.ID == profileID && profile.Primary && profile.AccountID == accountID {
			return nil
		}
	}

	return fmt.Errorf("only the primary profile may change the password: %w", domain.ErrForbidden)
}

func (service *Service) checkCurrentPassword(ctx context.Context, credentials Credentials, change PasswordChange) error {
	nameKey := account.NameKey(credentials.Account.Name)

	attempt, err := service.throttle.begin(nameKey, change.Peer.Address, service.deps.Clock())
	if err != nil {
		return err
	}

	result := outcomeFailed

	if len(change.Current) <= MaxSecretBytes {
		var matched bool

		// No rehash: the hash is replaced right after a match anyway.
		matched, _, err = service.verifyStored(ctx, credentials, change.Current)
		result = verifyOutcome(matched, err)
	}

	attempt.finish(result, service.deps.Clock())

	if result == outcomeAborted {
		return fmt.Errorf("verify current password: %w", err)
	}

	if result == outcomeFailed {
		return ErrInvalidCredentials
	}

	return nil
}

func verifyOutcome(matched bool, err error) outcome {
	switch {
	case err != nil:
		return outcomeAborted
	case matched:
		return outcomeSucceeded
	default:
		return outcomeFailed
	}
}

func (service *Service) replacePassword(ctx context.Context, accountID, keepSessionID, password string) error {
	passwordHash, err := service.deps.Hasher.Hash(ctx, password)
	if err != nil {
		return fmt.Errorf("change password: hash: %w", err)
	}

	if err := service.deps.Credentials.SetPasswordHash(ctx, accountID, passwordHash); err != nil {
		return fmt.Errorf("change password: store hash: %w", err)
	}

	// Evict before and after the delete: a failed delete must not leave the old sessions
	// served from the cache, and the generation guard stops a concurrent Resolve from
	// putting them back.
	service.evictOtherSessions(accountID, keepSessionID)

	if err := service.deps.Revoker.DeleteOtherSessions(ctx, accountID, keepSessionID); err != nil {
		return fmt.Errorf("change password: revoke other sessions: %w", err)
	}

	service.evictOtherSessions(accountID, keepSessionID)

	return nil
}

func (service *Service) evictOtherSessions(accountID, keepSessionID string) {
	service.cache.evict(func(resolution Resolution) bool {
		return resolution.Session.AccountID == accountID && resolution.Session.ID != keepSessionID
	})
}
