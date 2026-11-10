package account

import (
	"fmt"

	"github.com/magpeek/mivo/internal/domain"
)

// Conflicts of the account rules. Each wraps domain.ErrConflict, so callers that only need
// the category test for that; the HTTP adapter answers them with their own problem codes
// (ADR 0032, api/openapi.yaml: "last_admin", "profile_limit", "name_taken").
var (
	// ErrLastAdmin reports a change that would delete, disable or demote the last enabled
	// admin account.
	ErrLastAdmin = fmt.Errorf("last enabled admin: %w", domain.ErrConflict)
	// ErrProfileLimit reports a new managed profile for an account that already has
	// MaxManagedProfiles of them.
	ErrProfileLimit = fmt.Errorf("managed profile limit: %w", domain.ErrConflict)
	// ErrNameTaken reports a name another account, or another profile of the account,
	// already holds, ignoring case. Stores report it for their unique name keys.
	ErrNameTaken = fmt.Errorf("name taken: %w", domain.ErrConflict)
)
