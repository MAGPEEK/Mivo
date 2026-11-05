package match

import (
	"fmt"

	"github.com/magpeek/mivo/internal/domain"
)

// MatchState is where an item stands in online matching (ADR 0031 §9). The text values are
// stored in the database and used in the API. A user's choice is not a state of its own: it
// locks the provider_ids field (ADR 0031 §10), which Resolve respects.
//
//nolint:revive // the name is fixed by task M6-02 (match.MatchState)
type MatchState string

// Match states.
const (
	// MatchStateUnmatched means no match was made; the item shows under "not matched".
	MatchStateUnmatched MatchState = "unmatched"
	// MatchStateMatched means the item is linked to a provider entry.
	MatchStateMatched MatchState = "matched"
	// MatchStateDisabled means "Remove match": no auto-match until the user identifies it.
	MatchStateDisabled MatchState = "disabled"
)

// ParseMatchState accepts the exact stored text of a state. The error wraps domain.ErrInvalid.
func ParseMatchState(text string) (MatchState, error) {
	state := MatchState(text)
	if !state.IsValid() {
		return "", fmt.Errorf("parse match state %q: unknown state: %w", text, domain.ErrInvalid)
	}

	return state, nil
}

// IsValid reports whether the state is a known state.
func (state MatchState) IsValid() bool {
	switch state {
	case MatchStateUnmatched, MatchStateMatched, MatchStateDisabled:
		return true
	default:
		return false
	}
}

// String returns the text value of the state.
func (state MatchState) String() string {
	return string(state)
}
