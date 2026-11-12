package sqlite_test

import (
	"testing"

	"github.com/magpeek/mivo/internal/adapters/sqlite"
	appwatch "github.com/magpeek/mivo/internal/app/watch"
	"github.com/magpeek/mivo/internal/domain/access"
	"github.com/magpeek/mivo/internal/domain/watch"
)

// TestEpisodeCountsLeaveOutHiddenEpisodes pins the ancestor rule for the badges: Chain Show
// has two episodes, one below an FSK 16 season, and Sixteen Show only an FSK 0 episode below
// an FSK 16 show.
func TestEpisodeCountsLeaveOutHiddenEpisodes(t *testing.T) {
	t.Parallel()

	fixture := watchFixtureOn(t, newAncestorFixture(t))
	containers := []string{chainShow, chainHiddenSeason, chainOpenSeason, sixteenShow, sixteenSeason}

	upToSix, err := access.MaxAgeLimit(access.Age6, false)
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name        string
		restriction access.Restriction
		want        map[string]appwatch.EpisodeCount
	}{
		{
			name: "unrestricted", restriction: access.Unrestricted(),
			want: map[string]appwatch.EpisodeCount{
				chainShow: {Total: 2, Unwatched: 2}, chainHiddenSeason: {Total: 1, Unwatched: 1}, chainOpenSeason: {Total: 1, Unwatched: 1},
				sixteenShow: {Total: 1, Unwatched: 1}, sixteenSeason: {Total: 1, Unwatched: 1},
			},
		},
		{
			name: "up to 6", restriction: access.Viewer{Libraries: access.AllLibraries(), Limit: upToSix}.Restriction(),
			want: map[string]appwatch.EpisodeCount{chainShow: {Total: 1, Unwatched: 1}, chainOpenSeason: {Total: 1, Unwatched: 1}},
		},
		{name: "no library", restriction: access.Restriction{}, want: map[string]appwatch.EpisodeCount{}},
	}

	for _, test := range tests {
		got, err := fixture.store.EpisodeCounts(t.Context(), watchProfileID, containers, test.restriction)
		if err != nil {
			t.Fatalf("%s: EpisodeCounts error = %v", test.name, err)
		}

		if len(got) != len(test.want) {
			t.Errorf("%s: counts = %+v, want %+v", test.name, got, test.want)

			continue
		}

		for id, want := range test.want {
			if got[id] != want {
				t.Errorf("%s: count of %s = %+v, want %+v", test.name, id, got[id], want)
			}
		}
	}
}

// TestUpNextNeverPicksAHiddenEpisode watches the visible episode of Chain Show; the next
// episode among the visible ones does not exist, whereas the hidden season's one does for
// an unrestricted viewer.
func TestUpNextNeverPicksAHiddenEpisode(t *testing.T) {
	t.Parallel()

	fixture := watchFixtureOn(t, newAncestorFixture(t))
	fixture.put(t, watchProfileID, chainOpen, watch.State{Watched: true, PlayCount: 1, LastPlayed: watchNow()})

	upToSix, err := access.MaxAgeLimit(access.Age6, false)
	if err != nil {
		t.Fatal(err)
	}

	query := sqlite.WatchRowQuery{ProfileID: watchProfileID, PlayedSince: watchSince(), Limit: watch.UpNextLimit}

	for name, restriction := range map[string]access.Restriction{
		"up to 6":  access.Viewer{Libraries: access.AllLibraries(), Limit: upToSix}.Restriction(),
		"no right": {},
	} {
		query.Restriction = restriction

		itemIDs, err := fixture.store.UpNext(t.Context(), query)
		if err != nil {
			t.Fatalf("%s: UpNext error = %v", name, err)
		}

		for _, id := range itemIDs {
			if id == chainHidden {
				t.Errorf("%s: Up next picked the hidden episode", name)
			}
		}
	}
}
