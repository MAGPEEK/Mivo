package auth

import (
	"context"
	"fmt"

	"github.com/magpeek/mivo/internal/domain/account"
)

// checkProfilePIN verifies pin against the profile's stored PIN hash and reports whether the
// profile has a PIN. The hash is read from the store rather than trusting Profile.HasPIN, so
// a PIN set a moment ago is already demanded. A missing PIN returns ErrPINInvalid without
// counting a failure (the client only learns that one is needed); a malformed or wrong PIN
// counts towards the lockout.
func (service *Service) checkProfilePIN(ctx context.Context, profileID, pin string) (bool, error) {
	pinHash, err := service.deps.PINs.ProfilePINHash(ctx, profileID)
	if err != nil {
		return false, fmt.Errorf("load pin hash: %w", err)
	}

	if pinHash == "" {
		return false, nil
	}

	attempt, err := service.pinLock.begin(profileID, service.deps.Clock())
	if err != nil {
		return true, err
	}

	if pin == "" {
		attempt.finish(outcomeAborted, service.deps.Clock())

		return true, ErrPINInvalid
	}

	result := outcomeFailed

	if account.ValidatePIN(pin) == nil {
		// No rehash: PINs are set by profile management (M7-17), which hashes them afresh.
		var matched bool

		matched, _, err = service.deps.Hasher.Verify(ctx, pin, pinHash)
		result = verifyOutcome(matched, err)
	}

	lockedFor := attempt.finish(result, service.deps.Clock())

	switch {
	case result == outcomeAborted:
		return true, fmt.Errorf("verify pin: %w", err)
	case result == outcomeSucceeded:
		return true, nil
	case lockedFor > 0:
		return true, &ProfileLockedError{Wait: lockedFor}
	default:
		return true, ErrPINInvalid
	}
}
