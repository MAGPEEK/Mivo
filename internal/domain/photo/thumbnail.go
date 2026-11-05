package photo

import (
	"fmt"

	"github.com/magpeek/mivo/internal/domain"
)

// String returns the stored and transferred text of the state.
func (state ThumbnailState) String() string {
	return string(state)
}

// ParseThumbnailState returns the state named by text; anything else wraps domain.ErrInvalid.
func ParseThumbnailState(text string) (ThumbnailState, error) {
	switch state := ThumbnailState(text); state {
	case ThumbnailReady, ThumbnailUnsupported:
		return state, nil
	default:
		return "", fmt.Errorf("parse thumbnail state %q: %w", text, domain.ErrInvalid)
	}
}
