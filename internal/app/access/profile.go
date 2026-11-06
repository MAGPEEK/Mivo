package access

import (
	"context"
	"fmt"

	"github.com/magpeek/mivo/internal/domain"
	domainaccess "github.com/magpeek/mivo/internal/domain/access"
)

// ErrProfileRequired reports a signed-in viewer without a selected profile. Catalog, media
// and watch operations act for a profile, so they refuse such a session (API 403
// "profile_required", ADR 0032 §7). It wraps domain.ErrForbidden: the identity is known,
// only the profile choice is missing.
var ErrProfileRequired = fmt.Errorf("profile required: %w", domain.ErrForbidden)

// ProfileViewerFrom returns the viewer of ctx when it has a selected profile. Without a
// viewer (or with one without an account) it returns an error wrapping
// domain.ErrUnauthenticated, without a profile ErrProfileRequired.
func ProfileViewerFrom(ctx context.Context) (domainaccess.Viewer, error) {
	viewer, ok := ViewerFrom(ctx)
	if !ok || viewer.AccountID == "" {
		return domainaccess.Viewer{}, fmt.Errorf("viewer: %w", domain.ErrUnauthenticated)
	}

	if viewer.ProfileID == "" {
		return domainaccess.Viewer{}, ErrProfileRequired
	}

	return viewer, nil
}
