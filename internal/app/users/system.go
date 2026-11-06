package users

import (
	"context"
	"fmt"

	"github.com/magpeek/mivo/internal/domain/access"
	"github.com/magpeek/mivo/internal/domain/account"
)

// System is the administration entry without a viewer. Only the host CLI uses it
// (`admin reset-password`, `admin list-accounts`, M7-26, ADR 0032 §6): whoever runs it already
// controls the data directory, so there is no session to check. It has its own constructor
// instead of a method on Service, so request-path code holding a *Service cannot reach these
// unchecked operations. Never wire it into a request path.
type System struct {
	service *Service
}

// NewSystem returns the viewer-less entry for the host CLI. Viewers and Cache are optional
// here: the CLI has no request viewer and no in-process resolve cache (a running server's
// cache entries expire within the ADR 0032 §3 cache lifetime).
func NewSystem(options Options) (*System, error) {
	if options.Viewers == nil {
		options.Viewers = noViewer
	}

	if options.Cache == nil {
		options.Cache = noCache{}
	}

	service, err := New(options)
	if err != nil {
		return nil, fmt.Errorf("new system: %w", err)
	}

	return &System{service: service}, nil
}

// ListAccounts returns every account, so the CLI user can find the right name.
func (system *System) ListAccounts(ctx context.Context) ([]AccountSummary, error) {
	summaries, err := system.service.options.Accounts.ListAccounts(ctx)
	if err != nil {
		return nil, fmt.Errorf("list accounts: %w", err)
	}

	return summaries, nil
}

// ResetPassword sets the password of the named account and revokes all its sessions. The
// name is matched like a sign-in name (NFC, case-insensitive).
func (system *System) ResetPassword(ctx context.Context, name, password string) error {
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

// noViewer is the viewer source of the CLI: there is never a viewer, so every viewer-checked
// use case of the embedded service fails closed.
func noViewer(context.Context) (access.Viewer, bool) {
	return access.Viewer{}, false
}

// noCache is the resolve cache of the CLI process, which has none.
type noCache struct{}

// EvictAccount does nothing.
func (noCache) EvictAccount(string) {}
