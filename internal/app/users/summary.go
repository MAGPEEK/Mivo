package users

import (
	"context"
	"fmt"

	"github.com/magpeek/mivo/internal/domain"
)

// ErrLibraryAccess reports a library access choice the rules refuse: none where one is
// required (an admin demoted to a user) or libraries outside the account's set. It wraps
// domain.ErrInvalid; the HTTP adapter names the request property "libraryAccess" for it.
var ErrLibraryAccess = fmt.Errorf("library access: %w", domain.ErrInvalid)

// AccountSummary returns one account as the users list shows it, with its profile count and
// last activity, for the admin. It reads the list, which a household keeps short, instead of
// widening the store port for a single row. An unknown id wraps domain.ErrNotFound.
func (s *Service) AccountSummary(ctx context.Context, accountID string) (AccountSummary, error) {
	summaries, err := s.ListAccounts(ctx)
	if err != nil {
		return AccountSummary{}, err
	}

	for _, summary := range summaries {
		if summary.Account.ID == accountID {
			return summary, nil
		}
	}

	return AccountSummary{}, fmt.Errorf("account %s: %w", accountID, domain.ErrNotFound)
}
