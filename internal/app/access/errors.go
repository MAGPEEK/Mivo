package access

import (
	"fmt"

	"github.com/magpeek/mivo/internal/domain"
)

// ErrProfileRequired reports a signed-in session without a selected profile on an operation
// that needs one (catalog, media, watch state; ADR 0032 §4). It wraps domain.ErrForbidden, so
// callers that only know the generic sentinel still refuse; adapters/http answers 403
// "profile_required" so the client shows the profile picker.
var ErrProfileRequired = fmt.Errorf("profile required: %w", domain.ErrForbidden)
