// Package access carries the signed-in viewer through a request's context.Context
// (ADR 0032 §7). The HTTP adapter resolves the viewer and stores it with WithViewer; use cases
// read it with ViewerFrom and fail closed when it is missing.
package access

import (
	"context"

	domainaccess "github.com/magpeek/mivo/internal/domain/access"
)

// viewerKey is the unexported context key type, so no other package can overwrite or read
// the viewer by accident.
type viewerKey struct{}

// WithViewer returns a copy of ctx that carries viewer.
func WithViewer(ctx context.Context, viewer domainaccess.Viewer) context.Context {
	return context.WithValue(ctx, viewerKey{}, viewer)
}

// ViewerFrom returns the viewer stored by WithViewer and whether there was one. Callers must
// treat a missing viewer as unauthenticated, never as unrestricted.
func ViewerFrom(ctx context.Context) (domainaccess.Viewer, bool) {
	viewer, ok := ctx.Value(viewerKey{}).(domainaccess.Viewer)

	return viewer, ok
}
