package rows_test

import (
	"context"
	"errors"
	"testing"
	"time"

	approws "github.com/magpeek/mivo/internal/app/rows"
	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/library"
	"github.com/magpeek/mivo/internal/domain/rows"
)

func TestLibraryRowsHints(t *testing.T) {
	t.Parallel()

	sparse := func() *fakeCatalog {
		catalog := newFixtureCatalog()
		// Without release dates and genres only "recentlyAdded" can be built.
		catalog.items = []fakeItem{movie("m-bare", day(1), "")}

		return catalog
	}

	tests := []struct {
		name      string
		catalog   func() *fakeCatalog
		libraryID string
		scanning  bool
		wantRows  []rows.Kind
		wantHint  rows.Hint
	}{
		{name: "empty library", catalog: newFixtureCatalog, libraryID: emptyID, wantHint: rows.HintNoItems},
		{name: "empty library while scanning", catalog: newFixtureCatalog, libraryID: emptyID, scanning: true, wantHint: rows.HintScanning},
		{name: "missing metadata", catalog: sparse, libraryID: moviesID, wantRows: []rows.Kind{rows.KindRecentlyAdded}, wantHint: rows.HintNotEnoughData},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			catalog := test.catalog()
			catalog.scanning[test.libraryID] = test.scanning

			got, err := newTestService(t, catalog).LibraryRows(context.Background(), test.libraryID, 0)
			if err != nil {
				t.Fatalf("LibraryRows: %v", err)
			}

			kinds := []rows.Kind{}
			for _, row := range got.Rows {
				kinds = append(kinds, row.Kind)
			}

			if len(kinds) != len(test.wantRows) || got.Hint != test.wantHint {
				t.Errorf("rows %v hint %q, want %v and %q", kinds, got.Hint, test.wantRows, test.wantHint)
			}
		})
	}
}

func TestLibraryRowsErrors(t *testing.T) {
	t.Parallel()

	failure := errors.New("disk on fire")
	tests := []struct {
		name      string
		libraryID string
		limit     int
		failWith  error
		want      error
	}{
		{name: "unknown library", libraryID: "nope", want: domain.ErrNotFound},
		{name: "limit too small", libraryID: moviesID, limit: -1, want: domain.ErrInvalid},
		{name: "limit too large", libraryID: moviesID, limit: approws.MaxItemLimit + 1, want: domain.ErrInvalid},
		{name: "reader failure", libraryID: moviesID, failWith: failure, want: failure},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			catalog := newFixtureCatalog()
			catalog.failWith = test.failWith

			_, err := newTestService(t, catalog).LibraryRows(context.Background(), test.libraryID, test.limit)
			if !errors.Is(err, test.want) {
				t.Errorf("LibraryRows error = %v, want %v", err, test.want)
			}
		})
	}
}

func TestGenreRotationFollowsTheClock(t *testing.T) {
	t.Parallel()

	catalog := newFixtureCatalog()
	catalog.items = nil

	genres := []string{"A", "B", "C", "D", "E", "F", "G", "H"}
	for index, genre := range genres {
		catalog.items = append(catalog.items, movie("m-"+genre, day(index+1), "", genre))
	}

	pick := func(now time.Time) []string {
		service, err := approws.NewService(approws.Dependencies{
			Libraries: catalog, Rows: catalog, Summaries: catalog, Catalog: catalog, Scans: catalog,
			Clock: func() time.Time { return now },
		})
		if err != nil {
			t.Fatalf("NewService: %v", err)
		}

		got, err := service.LibraryRows(context.Background(), moviesID, 0)
		if err != nil {
			t.Fatalf("LibraryRows: %v", err)
		}

		var picked []string

		for _, row := range got.Rows {
			if row.Kind == rows.KindMoreGenre {
				picked = append(picked, row.Genre)
			}
		}

		return picked
	}

	morning, evening := pick(fixedNow().Add(-10*time.Hour)), pick(fixedNow())
	if len(morning) != rows.MaxGenreRows || !equalStrings(morning, evening) {
		t.Errorf("genres in the morning %v, in the evening %v, want the same three", morning, evening)
	}

	want := rows.PickGenres(genreCountsOf(genres), rows.DateSeed(fixedNow()))
	if !equalStrings(evening, want) {
		t.Errorf("genres = %v, want the domain pick %v", evening, want)
	}
}

func genreCountsOf(genres []string) []rows.GenreCount {
	counts := make([]rows.GenreCount, 0, len(genres))
	for _, genre := range genres {
		counts = append(counts, rows.GenreCount{Genre: genre, Count: 1})
	}

	return counts
}

func equalStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}

	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}

	return true
}

func TestNewServiceRequiresEveryDependency(t *testing.T) {
	t.Parallel()

	_, err := approws.NewService(approws.Dependencies{})
	if !errors.Is(err, domain.ErrInvalid) {
		t.Errorf("NewService error = %v, want ErrInvalid", err)
	}
}

// managedReader is the ManagedReader behind AllLibraries.
type managedReader struct{ libraries []library.ManagedLibrary }

func (reader managedReader) ListManaged(context.Context) ([]library.ManagedLibrary, error) {
	return reader.libraries, nil
}

func (reader managedReader) GetManaged(_ context.Context, id string) (library.ManagedLibrary, error) {
	for _, managed := range reader.libraries {
		if managed.ID == id {
			return managed, nil
		}
	}

	return library.ManagedLibrary{}, domain.ErrNotFound
}

func TestAllLibrariesShowsEveryLibrary(t *testing.T) {
	t.Parallel()

	source := approws.NewAllLibraries(managedReader{libraries: newFixtureCatalog().libraries})

	all, err := source.VisibleLibraries(context.Background())
	if err != nil || len(all) != 3 {
		t.Errorf("VisibleLibraries = %d libraries, %v; want 3", len(all), err)
	}

	if _, err := source.VisibleLibrary(context.Background(), moviesID); err != nil {
		t.Errorf("VisibleLibrary(movies): %v", err)
	}

	if _, err := source.VisibleLibrary(context.Background(), "nope"); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("VisibleLibrary(unknown) error = %v, want ErrNotFound", err)
	}
}
