package sqlite

import (
	"context"

	"github.com/magpeek/mivo/internal/app/scan"
	"github.com/magpeek/mivo/internal/app/scantrigger"
	"github.com/magpeek/mivo/internal/domain/library"
)

// The scan and the scan triggers read the libraries through the store, so an edit applies to
// the next scan without any copy to keep in sync.
var (
	_ scan.LibrarySource        = (*ManagedLibraryStore)(nil)
	_ scantrigger.LibraryLister = (*ManagedLibraryStore)(nil)
)

// GetLibrary returns one library with its folders and options; an unknown ID wraps
// domain.ErrNotFound. It is GetManaged under the name the scan ports use.
func (store *ManagedLibraryStore) GetLibrary(ctx context.Context, libraryID string) (library.ManagedLibrary, error) {
	return store.GetManaged(ctx, libraryID)
}

// ListLibraries returns every library, ordered by name. It is ListManaged under the name the
// scan ports use.
func (store *ManagedLibraryStore) ListLibraries(ctx context.Context) ([]library.ManagedLibrary, error) {
	return store.ListManaged(ctx)
}
