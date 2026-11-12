package access

import (
	"context"
	"fmt"

	"github.com/magpeek/mivo/internal/domain"
	domainaccess "github.com/magpeek/mivo/internal/domain/access"
)

// RestrictionFrom returns what the viewer of ctx may read. Without a viewer it fails closed
// with an error wrapping domain.ErrUnauthenticated, never with an unrestricted answer. It does
// not demand a selected profile: catalog reads act for the effective access the HTTP adapter
// resolved.
func RestrictionFrom(ctx context.Context) (domainaccess.Restriction, error) {
	viewer, ok := ViewerFrom(ctx)
	if !ok {
		return domainaccess.Restriction{}, fmt.Errorf("viewer: %w", domain.ErrUnauthenticated)
	}

	return viewer.Restriction(), nil
}

// CheckLibrary reports a library outside the restriction as domain.ErrNotFound, the answer for
// a missing one, so a viewer cannot tell a hidden library from an unknown one (ADR 0032 §7).
// Use cases call it first, before validating other input and before any store call.
func CheckLibrary(restriction domainaccess.Restriction, libraryID string) error {
	if !restriction.AllowsLibrary(libraryID) {
		return fmt.Errorf("library %q: %w", libraryID, domain.ErrNotFound)
	}

	return nil
}

// LibraryRestriction combines RestrictionFrom and CheckLibrary for use cases that take a
// library id.
func LibraryRestriction(ctx context.Context, libraryID string) (domainaccess.Restriction, error) {
	restriction, err := RestrictionFrom(ctx)
	if err != nil {
		return domainaccess.Restriction{}, err
	}

	if err := CheckLibrary(restriction, libraryID); err != nil {
		return domainaccess.Restriction{}, err
	}

	return restriction, nil
}
