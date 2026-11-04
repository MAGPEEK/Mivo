package library_test

import (
	"context"
	"errors"
	"testing"

	"github.com/magpeek/mivo/internal/app/library"
	"github.com/magpeek/mivo/internal/domain"
	domainlibrary "github.com/magpeek/mivo/internal/domain/library"
)

type fakeCounts struct {
	counts map[string]int64
	err    error
}

func (counts fakeCounts) TopLevelItemCounts(context.Context) (map[string]int64, error) {
	return counts.counts, counts.err
}

func managedFixture() []domainlibrary.ManagedLibrary {
	return []domainlibrary.ManagedLibrary{
		{ID: "a", Name: "Anime", Kind: domainlibrary.KindShows, Folders: []domainlibrary.Folder{
			{ID: "a1", Path: "/media/anime"}, {ID: "a2", Path: "/mnt/offline"},
		}},
		{ID: "m", Name: "Movies", Kind: domainlibrary.KindMovies, Folders: []domainlibrary.Folder{
			{ID: "m1", Path: "/media/movies"},
		}},
	}
}

func newManagedCatalog(t *testing.T, counts fakeCounts, reachable ...string) *library.ManagedCatalog {
	t.Helper()

	catalog, err := library.NewManagedCatalog(library.ManagedCatalogDependencies{
		Libraries: newMemoryManaged(&callLog{}, managedFixture()...),
		Counts:    counts,
		Roots:     availableRoots(reachable...),
	})
	if err != nil {
		t.Fatalf("NewManagedCatalog() error = %v", err)
	}

	return catalog
}

func TestManagedCatalogListMarksAvailabilityAndCounts(t *testing.T) {
	t.Parallel()

	catalog := newManagedCatalog(t, fakeCounts{counts: map[string]int64{"m": 7}}, "/media/anime", "/media/movies")

	overviews, err := catalog.ListLibraries(t.Context())
	if err != nil {
		t.Fatalf("ListLibraries() error = %v", err)
	}

	if len(overviews) != 2 {
		t.Fatalf("ListLibraries() = %d overviews, want 2", len(overviews))
	}

	anime, movies := overviews[0], overviews[1]
	if anime.Available || !movies.Available {
		t.Errorf("Available = anime %v, movies %v; want false (one folder offline), true", anime.Available, movies.Available)
	}

	if anime.ItemCount != 0 || movies.ItemCount != 7 {
		t.Errorf("ItemCount = anime %d, movies %d; want 0 (no entry), 7", anime.ItemCount, movies.ItemCount)
	}

	wantFolders := []bool{true, false}
	for index, folder := range anime.Folders {
		if folder.Available != wantFolders[index] || folder.Folder.ID != managedFixture()[0].Folders[index].ID {
			t.Errorf("anime folder %d = %+v, want available %v", index, folder, wantFolders[index])
		}
	}
}

func TestManagedCatalogGetAndDescribe(t *testing.T) {
	t.Parallel()

	catalog := newManagedCatalog(t, fakeCounts{counts: map[string]int64{"a": 3}}, "/media/anime", "/mnt/offline")

	got, err := catalog.GetLibrary(t.Context(), "a")
	if err != nil {
		t.Fatalf("GetLibrary() error = %v", err)
	}

	if !got.Available || got.ItemCount != 3 || len(got.Folders) != 2 {
		t.Errorf("GetLibrary() = %+v, want available, 3 items, 2 folders", got)
	}

	described, err := catalog.Describe(t.Context(), managedFixture()[1])
	if err != nil {
		t.Fatalf("Describe() error = %v", err)
	}

	if described.Available || described.ItemCount != 0 {
		t.Errorf("Describe(movies) = %+v, want unavailable with 0 items", described)
	}
}

func TestManagedCatalogErrors(t *testing.T) {
	t.Parallel()

	catalog := newManagedCatalog(t, fakeCounts{err: errors.New("db down")})

	if _, err := catalog.GetLibrary(t.Context(), "missing"); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("GetLibrary(missing) error = %v, want ErrNotFound", err)
	}

	if _, err := catalog.ListLibraries(t.Context()); err == nil {
		t.Error("ListLibraries() with failing counts: error = nil")
	}

	cancelled, cancel := context.WithCancel(t.Context())
	cancel()

	healthy := newManagedCatalog(t, fakeCounts{}, "/media/movies")
	if _, err := healthy.ListLibraries(cancelled); !errors.Is(err, context.Canceled) {
		t.Errorf("ListLibraries(cancelled) error = %v, want context.Canceled", err)
	}
}

func TestNewManagedCatalogRejectsMissingDependencies(t *testing.T) {
	t.Parallel()

	if _, err := library.NewManagedCatalog(library.ManagedCatalogDependencies{}); !errors.Is(err, domain.ErrInvalid) {
		t.Errorf("NewManagedCatalog({}) error = %v, want ErrInvalid", err)
	}
}
