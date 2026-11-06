package auth

import (
	"context"
	"errors"
	"fmt"

	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/account"
)

// SetupRequired reports whether the server is in setup state: no account exists yet
// (ADR 0032 §6).
func (service *Service) SetupRequired(ctx context.Context) (bool, error) {
	hasAccounts, err := service.deps.Setup.HasAccounts(ctx)
	if err != nil {
		return false, fmt.Errorf("setup required: %w", err)
	}

	return !hasAccounts, nil
}

// Setup creates the first admin with its primary profile and signs it in. It succeeds only
// while no account exists; afterwards, also for a concurrent loser, it returns
// ErrSetupComplete. Invalid input wraps domain.ErrInvalid.
func (service *Service) Setup(ctx context.Context, request SignInRequest) (SignInResult, error) {
	device, err := validateDevice(request.Device)
	if err != nil {
		return SignInResult{}, fmt.Errorf("setup: %w", err)
	}

	name, err := account.NormalizeName(request.Name, service.deps.Compose)
	if err != nil {
		return SignInResult{}, fmt.Errorf("setup: %w", err)
	}

	if err := account.ValidatePassword(request.Password); err != nil {
		return SignInResult{}, fmt.Errorf("setup: %w", err)
	}

	// Checked before hashing, so a finished setup does not spend hash work per request.
	required, err := service.SetupRequired(ctx)
	if err != nil {
		return SignInResult{}, fmt.Errorf("setup: %w", err)
	}

	if !required {
		return SignInResult{}, ErrSetupComplete
	}

	admin, err := service.firstAdmin(ctx, name, request.Password)
	if err != nil {
		return SignInResult{}, err
	}

	if err := service.deps.Setup.CreateFirstAdmin(ctx, admin); err != nil {
		if errors.Is(err, domain.ErrConflict) {
			return SignInResult{}, fmt.Errorf("setup: %w: %w", ErrSetupComplete, err)
		}

		return SignInResult{}, fmt.Errorf("setup: store admin: %w", err)
	}

	return service.startSession(ctx, admin.Account, sessionOrigin{device: device, peer: request.Peer})
}

func (service *Service) firstAdmin(ctx context.Context, name, password string) (FirstAdmin, error) {
	accountID, err := service.deps.IDs.NewID()
	if err != nil {
		return FirstAdmin{}, fmt.Errorf("setup: new account id: %w", err)
	}

	profileID, err := service.deps.IDs.NewID()
	if err != nil {
		return FirstAdmin{}, fmt.Errorf("setup: new profile id: %w", err)
	}

	admin, err := account.NewAccount(account.Account{ID: accountID, Name: name, Role: account.RoleAdmin})
	if err != nil {
		return FirstAdmin{}, fmt.Errorf("setup: %w", err)
	}

	primary, err := account.NewProfile(account.Profile{
		ID: profileID, AccountID: accountID, Name: name, Primary: true, AvatarColor: account.DefaultAvatarColor,
	})
	if err != nil {
		return FirstAdmin{}, fmt.Errorf("setup: %w", err)
	}

	passwordHash, err := service.deps.Hasher.Hash(ctx, password)
	if err != nil {
		return FirstAdmin{}, fmt.Errorf("setup: hash password: %w", err)
	}

	return FirstAdmin{Account: admin, Profile: primary, PasswordHash: passwordHash}, nil
}
