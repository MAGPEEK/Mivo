package watch_test

import (
	"errors"
	"slices"
	"testing"

	appwatch "github.com/magpeek/mivo/internal/app/watch"
	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/library"
	"github.com/magpeek/mivo/internal/domain/watch"
)

func TestStatesDecoratesAPageInTwoStoreCalls(t *testing.T) {
	t.Parallel()

	fixture := showFixture(t)
	inProgress := watch.State{PositionSeconds: 600, LastPlayed: fixedNow()}
	finished := watch.State{Watched: true, PlayCount: 1, LastPlayed: fixedNow()}
	fixture.store.put(profileID, "movie-1", inProgress)
	fixture.store.put(profileID, "episode-1", finished)
	fixture.store.put(profileID, "episode-2", finished)
	fixture.store.put(otherProfileID, "movie-2", finished)

	items := []appwatch.ItemRef{
		{ID: "movie-1", Kind: library.ItemKindMovie},
		{ID: "movie-2", Kind: library.ItemKindMovie},
		{ID: "photo-1", Kind: library.ItemKindPhoto},
		{ID: "show-1", Kind: library.ItemKindShow},
		{ID: "season-1", Kind: library.ItemKindSeason},
		{ID: "season-empty", Kind: library.ItemKindSeason},
		{ID: "movie-1", Kind: library.ItemKindMovie},
	}

	got, err := fixture.service.States(viewerContext(profileID), items)
	if err != nil {
		t.Fatalf("States: %v", err)
	}

	want := map[string]appwatch.ItemState{
		"movie-1":      {State: inProgress},
		"movie-2":      {},
		"show-1":       {Episodes: appwatch.EpisodeCount{Total: 3, Unwatched: 1}},
		"season-1":     {State: watch.State{Watched: true}, Episodes: appwatch.EpisodeCount{Total: 2}},
		"season-empty": {},
	}

	if len(got) != len(want) {
		t.Fatalf("States = %+v, want %+v", got, want)
	}

	for itemID, wantState := range want {
		if got[itemID] != wantState {
			t.Fatalf("States[%s] = %+v, want %+v", itemID, got[itemID], wantState)
		}
	}

	if fixture.store.stateReads != 1 || fixture.store.countReads != 1 {
		t.Fatalf("state reads %d, count reads %d; want 1 and 1", fixture.store.stateReads, fixture.store.countReads)
	}

	if !slices.Equal(fixture.store.readIDs[0], []string{"movie-1", "movie-2"}) ||
		!slices.Equal(fixture.store.countIDs[0], []string{"show-1", "season-1", "season-empty"}) {
		t.Fatalf("store asked for %v and %v, want distinct IDs by kind", fixture.store.readIDs, fixture.store.countIDs)
	}
}

func TestStatesSkipsStoreCallsWithoutMatchingItems(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		items          []appwatch.ItemRef
		wantStateReads int
		wantCountReads int
	}{
		{"empty page", nil, 0, 0},
		{"only photos", []appwatch.ItemRef{{ID: "photo-1", Kind: library.ItemKindPhoto}}, 0, 0},
		{"only movies", []appwatch.ItemRef{{ID: "movie-1", Kind: library.ItemKindMovie}}, 1, 0},
		{"only shows", []appwatch.ItemRef{{ID: "show-1", Kind: library.ItemKindShow}}, 0, 1},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			fixture := newFixture(t)

			if _, err := fixture.service.States(viewerContext(profileID), test.items); err != nil {
				t.Fatalf("States: %v", err)
			}

			if fixture.store.stateReads != test.wantStateReads || fixture.store.countReads != test.wantCountReads {
				t.Fatalf("state reads %d, count reads %d; want %d and %d",
					fixture.store.stateReads, fixture.store.countReads, test.wantStateReads, test.wantCountReads)
			}
		})
	}
}

func TestStatesWrapsStoreErrors(t *testing.T) {
	t.Parallel()

	for _, kind := range []library.ItemKind{library.ItemKindMovie, library.ItemKindShow} {
		fixture := newFixture(t)
		fixture.store.failWith = domain.ErrUnavailable

		_, err := fixture.service.States(viewerContext(profileID), []appwatch.ItemRef{{ID: "item-1", Kind: kind}})
		if !errors.Is(err, domain.ErrUnavailable) {
			t.Fatalf("States with a %s: error = %v, want ErrUnavailable", kind, err)
		}
	}
}
