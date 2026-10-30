package library

import (
	"fmt"

	"github.com/magpeek/mivo/internal/domain"
)

// Page size bounds of the item list (API contract: limit 1–500, default 100).
const (
	DefaultPageLimit = 100
	MaxPageLimit     = 500
	minPageLimit     = 1
	unsetPageLimit   = 0
)

// PageRequest asks for one page of a keyset-paginated list. Limit 0 means the default.
// Cursor is opaque to callers: empty for the first page, otherwise the value the previous
// page returned; only the adapter that produced it interprets it.
type PageRequest struct {
	Limit  int
	Cursor string
}

// NormalizeLimit returns DefaultPageLimit for an unset limit (0) and any limit between 1 and
// MaxPageLimit unchanged. Other values are rejected rather than clamped, so a client bug
// surfaces as an error instead of silently returning a different page size. The error wraps
// domain.ErrInvalid.
func NormalizeLimit(limit int) (int, error) {
	if limit == unsetPageLimit {
		return DefaultPageLimit, nil
	}

	if limit < minPageLimit || limit > MaxPageLimit {
		return 0, fmt.Errorf("page limit %d not between %d and %d: %w",
			limit, minPageLimit, MaxPageLimit, domain.ErrInvalid)
	}

	return limit, nil
}
