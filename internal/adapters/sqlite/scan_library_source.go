package sqlite

import (
	"context"
	"fmt"
	"time"

	"github.com/magpeek/mivo/internal/app/scan"
	"github.com/magpeek/mivo/internal/domain/library"
)

// ScanLibrarySource gives the scan the libraries of the single-root library table as
// libraries with one folder. The folder ID is the library ID, which is what migration 00004
// wrote for carried-over libraries. It is the stop-gap until the library store of M3-19g
// returns libraries with several folders and options (M3-19l replaces it).
type ScanLibrarySource struct {
	repository *LibraryRepository
}

var _ scan.LibrarySource = (*ScanLibrarySource)(nil)

// NewScanLibrarySource wraps a library repository for the scan.
func NewScanLibrarySource(db *DB, clock func() time.Time) *ScanLibrarySource {
	return &ScanLibrarySource{repository: NewLibraryRepository(db, clock)}
}

// GetLibrary returns the library with the default options; an unknown ID wraps
// domain.ErrNotFound.
func (source *ScanLibrarySource) GetLibrary(ctx context.Context, libraryID string) (library.ManagedLibrary, error) {
	summary, err := source.repository.GetLibrary(ctx, libraryID)
	if err != nil {
		return library.ManagedLibrary{}, fmt.Errorf("scan library source: %w", err)
	}

	return oneFolderLibrary(summary.Library), nil
}

// ListLibraries returns every library.
func (source *ScanLibrarySource) ListLibraries(ctx context.Context) ([]library.ManagedLibrary, error) {
	summaries, err := source.repository.ListLibraries(ctx)
	if err != nil {
		return nil, fmt.Errorf("scan library source: %w", err)
	}

	libraries := make([]library.ManagedLibrary, 0, len(summaries))
	for _, summary := range summaries {
		libraries = append(libraries, oneFolderLibrary(summary.Library))
	}

	return libraries, nil
}

func oneFolderLibrary(stored library.Library) library.ManagedLibrary {
	return library.ManagedLibrary{
		ID: stored.ID, Name: stored.Name, Kind: stored.Kind,
		Folders: []library.Folder{{ID: stored.ID, Path: stored.Root}},
		Options: library.DefaultOptions(),
	}
}
