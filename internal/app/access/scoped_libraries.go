package access

import (
	"context"
	"fmt"

	"github.com/magpeek/mivo/internal/domain/library"
)

// LibraryReader is what ScopedLibraries needs of the library store: the managed libraries
// with their folders.
type LibraryReader interface {
	ListManaged(ctx context.Context) ([]library.ManagedLibrary, error)
	GetManaged(ctx context.Context, id string) (library.ManagedLibrary, error)
}

// ScopedLibraries is the library source of the catalog use cases: the visible libraries are
// the viewer's effective access set (ADR 0032 §7). It reads the viewer from the context and
// fails closed, so a request without a viewer sees no library.
type ScopedLibraries struct {
	reader LibraryReader
}

// NewScopedLibraries returns a ScopedLibraries over the library store.
func NewScopedLibraries(reader LibraryReader) ScopedLibraries {
	return ScopedLibraries{reader: reader}
}

// VisibleLibraries returns the libraries the viewer of ctx may browse, in store order.
// Without a viewer the error wraps domain.ErrUnauthenticated.
func (source ScopedLibraries) VisibleLibraries(ctx context.Context) ([]library.ManagedLibrary, error) {
	restriction, err := RestrictionFrom(ctx)
	if err != nil {
		return nil, fmt.Errorf("list visible libraries: %w", err)
	}

	if restriction.DeniesAll() {
		return nil, nil
	}

	libraries, err := source.reader.ListManaged(ctx)
	if err != nil {
		return nil, fmt.Errorf("list visible libraries: %w", err)
	}

	visible := make([]library.ManagedLibrary, 0, len(libraries))

	for _, managed := range libraries {
		if restriction.AllowsLibrary(managed.ID) {
			visible = append(visible, managed)
		}
	}

	return visible, nil
}

// VisibleLibrary returns one library the viewer may browse. A library outside the access set
// wraps domain.ErrNotFound without reading the store, like an unknown one.
func (source ScopedLibraries) VisibleLibrary(ctx context.Context, id string) (library.ManagedLibrary, error) {
	if _, err := LibraryRestriction(ctx, id); err != nil {
		return library.ManagedLibrary{}, fmt.Errorf("get visible library %q: %w", id, err)
	}

	managed, err := source.reader.GetManaged(ctx, id)
	if err != nil {
		return library.ManagedLibrary{}, fmt.Errorf("get visible library %q: %w", id, err)
	}

	return managed, nil
}
