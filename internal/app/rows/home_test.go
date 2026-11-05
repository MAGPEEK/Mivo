package rows_test

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"testing"

	approws "github.com/magpeek/mivo/internal/app/rows"
	"github.com/magpeek/mivo/internal/domain/library"
	"github.com/magpeek/mivo/internal/domain/rows"
)

func TestHomeRows(t *testing.T) {
	t.Parallel()

	catalog := newFixtureCatalog()

	got, err := newTestService(t, catalog).Home(context.Background(), 0)
	if err != nil {
		t.Fatalf("Home: %v", err)
	}

	// One "recentlyAdded" row per library with items, in library order; the empty library
	// has none and Home carries no "recentEpisodes" row (task M5-12 decision).
	wantLibraries := []struct{ id, name string }{{id: moviesID, name: "Movies"}, {id: showsID, name: "Shows"}}
	if len(got.Rows) != len(wantLibraries) {
		t.Fatalf("rows = %+v, want one per library with items", got.Rows)
	}

	for index, want := range wantLibraries {
		row := got.Rows[index]
		if row.Kind != rows.KindRecentlyAdded || row.LibraryID != want.id || row.TitleKey != rows.TitleRecentlyAddedIn ||
			row.TitleParams[rows.ParamLibrary] != want.name || len(row.Items) == 0 {
			t.Errorf("row %d = %+v, want recentlyAdded of %s", index, row, want.name)
		}
	}

	if got.Hint != rows.HintNone || len(got.Hero) != 0 {
		t.Errorf("hint %q hero %+v, want none and an empty hero (no backdrops)", got.Hint, got.Hero)
	}
}

func TestHomeHero(t *testing.T) {
	t.Parallel()

	catalog := newFixtureCatalog()
	catalog.items = nil

	// Seven movies with backdrops, added on days 1 to 7, and a newer one without.
	for index := range 7 {
		item := movie(fmt.Sprintf("m-%d", index+1), day(index+1), "")
		item.backdrop, item.summaryText = true, "A summary\nof movie."
		catalog.items = append(catalog.items, item)
	}

	catalog.items = append(catalog.items, movie("m-plain", day(20), ""))

	got, err := newTestService(t, catalog).Home(context.Background(), 0)
	if err != nil {
		t.Fatalf("Home: %v", err)
	}

	ids := make([]string, 0, len(got.Hero))
	for _, hero := range got.Hero {
		ids = append(ids, hero.ItemID)

		if hero.SummaryLine != "A summary of movie." || hero.Backdrop.ID != "backdrop-"+hero.ItemID || hero.Kind != library.ItemKindMovie {
			t.Errorf("hero %+v", hero)
		}
	}

	if want := []string{"m-7", "m-6", "m-5", "m-4", "m-3"}; !slices.Equal(ids, want) {
		t.Errorf("hero = %v, want the five newest with a backdrop %v", ids, want)
	}
}

func TestHomeCapsTheRows(t *testing.T) {
	t.Parallel()

	catalog := &fakeCatalog{scanning: map[string]bool{}}

	for index := range approws.MaxHomeRows + 1 {
		id := fmt.Sprintf("lib-%03d", index)
		catalog.libraries = append(catalog.libraries, library.ManagedLibrary{ID: id, Name: id, Kind: library.KindMovies})

		item := movie("m-"+id, day(1), "")
		item.summary.Item.LibraryID = id
		catalog.items = append(catalog.items, item)
	}

	got, err := newTestService(t, catalog).Home(context.Background(), 0)
	if err != nil {
		t.Fatalf("Home: %v", err)
	}

	if len(got.Rows) != approws.MaxHomeRows || got.Rows[0].LibraryID != "lib-000" {
		t.Errorf("Home has %d rows starting with %q, want the first %d libraries", len(got.Rows), got.Rows[0].LibraryID, approws.MaxHomeRows)
	}
}

func TestHomeHints(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		catalog  func() *fakeCatalog
		wantHint rows.Hint
	}{
		{name: "no libraries", catalog: func() *fakeCatalog { return &fakeCatalog{scanning: map[string]bool{}} }, wantHint: rows.HintNoLibraries},
		{name: "no items", catalog: func() *fakeCatalog {
			catalog := newFixtureCatalog()
			catalog.items = nil

			return catalog
		}, wantHint: rows.HintNoItems},
		{name: "scanning", catalog: func() *fakeCatalog {
			catalog := newFixtureCatalog()
			catalog.items, catalog.scanning[showsID] = nil, true

			return catalog
		}, wantHint: rows.HintScanning},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			got, err := newTestService(t, test.catalog()).Home(context.Background(), 0)
			if err != nil {
				t.Fatalf("Home: %v", err)
			}

			if got.Hint != test.wantHint || got.Rows == nil || len(got.Rows) != 0 || got.Hero == nil {
				t.Errorf("Home = %+v, want hint %q with empty, non-nil rows and hero", got, test.wantHint)
			}
		})
	}
}

func TestHomeErrors(t *testing.T) {
	t.Parallel()

	failure := errors.New("disk on fire")

	catalog := newFixtureCatalog()
	catalog.failWith = failure

	if _, err := newTestService(t, catalog).Home(context.Background(), 0); !errors.Is(err, failure) {
		t.Errorf("Home error = %v, want %v", err, failure)
	}

	if _, err := newTestService(t, newFixtureCatalog()).Home(context.Background(), 51); err == nil {
		t.Error("Home with limit 51 succeeded")
	}
}
