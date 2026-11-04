package sqlite_test

import (
	"errors"
	"testing"

	"github.com/magpeek/mivo/internal/domain"
)

// The store is the scan's library source: it returns libraries with several folders and their
// options, and an unknown ID is ErrNotFound.
func TestStoreServesTheScanLibrarySource(t *testing.T) {
	t.Parallel()

	store, _ := newManagedStore(t, sequentialIDs())
	created := mustCreate(t, store, movieCandidate("Movies", "/media/a", "/media/b"))

	got, err := store.GetLibrary(t.Context(), created.ID)
	if err != nil {
		t.Fatalf("GetLibrary error = %v", err)
	}

	if len(got.Folders) != 2 || !got.Options.ScanExtras {
		t.Errorf("GetLibrary = %+v, want two folders and the stored options", got)
	}

	listed, err := store.ListLibraries(t.Context())
	if err != nil || len(listed) != 1 || listed[0].ID != created.ID {
		t.Errorf("ListLibraries = %+v, %v; want the created library", listed, err)
	}

	if _, err := store.GetLibrary(t.Context(), "unknown"); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("GetLibrary(unknown) error = %v, want ErrNotFound", err)
	}
}
