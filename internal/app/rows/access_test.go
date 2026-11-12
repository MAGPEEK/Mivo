package rows_test

import (
	"context"
	"errors"
	"testing"

	appaccess "github.com/magpeek/mivo/internal/app/access"
	approws "github.com/magpeek/mivo/internal/app/rows"
	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/access"
)

// adminContext carries an unrestricted administrator, the viewer of most tests.
func adminContext() context.Context {
	return appaccess.WithViewer(context.Background(), access.Viewer{
		AccountID: "admin", ProfileID: "admin", Admin: true, Libraries: access.AllLibraries(), Limit: access.NoLimit(),
	})
}

func limitedViewer(t *testing.T, maxAge access.AgeBucket, libraryIDs ...string) access.Viewer {
	t.Helper()

	limit, err := access.MaxAgeLimit(maxAge, true)
	if err != nil {
		t.Fatal(err)
	}

	return access.Viewer{AccountID: "kid", ProfileID: "kid", Libraries: access.OnlyLibraries(libraryIDs...), Limit: limit}
}

// scopedService reads the libraries through ScopedLibraries, as the wiring does.
func scopedService(t *testing.T, catalog *fakeCatalog) *approws.Service {
	t.Helper()

	service, err := approws.NewService(approws.Dependencies{
		Libraries: appaccess.NewScopedLibraries(catalog), Rows: catalog, Summaries: catalog, Catalog: catalog, Scans: catalog,
		Clock: fixedNow,
	})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	return service
}

func TestRowsWithoutViewerFailClosed(t *testing.T) {
	t.Parallel()

	catalog := newFixtureCatalog()
	service := scopedService(t, catalog)

	if _, err := service.Home(context.Background(), 0); !errors.Is(err, domain.ErrUnauthenticated) {
		t.Errorf("Home error = %v, want ErrUnauthenticated", err)
	}

	if _, err := service.LibraryRows(context.Background(), moviesID, 0); !errors.Is(err, domain.ErrUnauthenticated) {
		t.Errorf("LibraryRows error = %v, want ErrUnauthenticated", err)
	}

	if len(catalog.restrictions) != 0 {
		t.Errorf("readers were called without a viewer: %v", catalog.restrictions)
	}
}

func TestRowsOfADeniedLibraryAnswerNotFoundFirst(t *testing.T) {
	t.Parallel()

	catalog := newFixtureCatalog()
	service := scopedService(t, catalog)
	ctx := appaccess.WithViewer(context.Background(), limitedViewer(t, access.Age12, showsID))

	// The limit is invalid too: the denied library must still win.
	_, err := service.LibraryRows(ctx, moviesID, 9999)
	if !errors.Is(err, domain.ErrNotFound) || errors.Is(err, domain.ErrInvalid) {
		t.Errorf("LibraryRows error = %v, want ErrNotFound only", err)
	}

	if len(catalog.restrictions) != 0 {
		t.Errorf("readers were called for a denied library: %v", catalog.restrictions)
	}
}

func TestRowsPassTheViewersRestrictionToEveryRead(t *testing.T) {
	t.Parallel()

	catalog := newFixtureCatalog()
	viewer := limitedViewer(t, access.Age12, moviesID, showsID)
	ctx := appaccess.WithViewer(context.Background(), viewer)
	service := scopedService(t, catalog)

	if _, err := service.Home(ctx, 0); err != nil {
		t.Fatalf("Home error = %v", err)
	}

	for _, id := range []string{moviesID, showsID} {
		if _, err := service.LibraryRows(ctx, id, 0); err != nil {
			t.Fatalf("LibraryRows(%s) error = %v", id, err)
		}
	}

	for _, method := range []string{"ListRowItems", "ListEpisodeGroups", "ListRecentEpisodes", "ItemSummaries"} {
		seen := catalog.restrictions[method]
		if len(seen) == 0 {
			t.Errorf("%s was never called", method)
		}

		for _, restriction := range seen {
			assertSameRestriction(t, method, restriction, viewer.Restriction())
		}
	}
}

func TestHomeListsOnlyTheViewersLibraries(t *testing.T) {
	t.Parallel()

	catalog := newFixtureCatalog()
	ctx := appaccess.WithViewer(context.Background(), limitedViewer(t, access.Age18, showsID))

	home, err := scopedService(t, catalog).Home(ctx, 0)
	if err != nil {
		t.Fatalf("Home error = %v", err)
	}

	for _, row := range home.Rows {
		if row.LibraryID != showsID {
			t.Errorf("row %s of library %s leaked", row.ID, row.LibraryID)
		}
	}
}

func assertSameRestriction(t *testing.T, what string, got, want access.Restriction) {
	t.Helper()

	gotAge, gotLimited := got.MaxAge()
	wantAge, wantLimited := want.MaxAge()

	if got.AllLibraries() != want.AllLibraries() || got.HideUnrated() != want.HideUnrated() ||
		gotAge != wantAge || gotLimited != wantLimited || len(got.LibraryIDs()) != len(want.LibraryIDs()) {
		t.Errorf("%s restriction = %+v, want %+v", what, got, want)
	}
}
