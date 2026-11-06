package users

import (
	"context"
	"fmt"

	"github.com/magpeek/mivo/internal/domain/account"
)

// System is the administration entry without a viewer. Only the host CLI uses it
// (`admin reset-password`, `admin list-accounts`, M7-26, ADR 0032 §6): whoever runs it already
// controls the data directory, so there is no session to check. Never wire it into a request
// path.
type System struct {
	service *Service
}

// System returns the viewer-less entry for the host CLI.
func (s *Service) System() System {
	return System{service: s}
}

// ListAccounts returns every account, so the CLI user can find the right name.
func (system System) ListAccounts(ctx context.Context) ([]AccountSummary, error) {
	summaries, err := system.service.options.Accounts.ListAccounts(ctx)
	if err != nil {
		return nil, fmt.Errorf("list accounts: %w", err)
	}

	return summaries, nil
}

// ResetPassword sets the password of the named account and revokes all its sessions. The
// name is matched like a sign-in name (NFC, case-insensitive).
func (system System) ResetPassword(ctx context.Context, name, password string) error {
	normalized, err := system.service.normalizeName(name)
	if err != nil {
		return err
	}

	target, err := system.service.options.Accounts.FindAccountByNameKey(ctx, account.NameKey(normalized))
	if err != nil {
		return fmt.Errorf("find account %q: %w", normalized, err)
	}

	return system.service.setPassword(ctx, target.Account.ID, password)
}
