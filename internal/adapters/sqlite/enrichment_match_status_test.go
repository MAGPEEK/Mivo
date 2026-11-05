package sqlite_test

import (
	"maps"
	"slices"
	"testing"
	"time"

	"github.com/magpeek/mivo/internal/adapters/sqlite"
	"github.com/magpeek/mivo/internal/domain/library"
	"github.com/magpeek/mivo/internal/domain/metadata/match"
)

// seedListingMatches stores a TMDB match for Alpha, a removed match for Bravo and an
// explicit unmatched row for Charlie; 1917 and Ärger have no row (unmatched as well).
func seedListingMatches(t *testing.T, fixture catalogFixture) *sqlite.EnrichmentStore {
	t.Helper()

	store := sqlite.NewEnrichmentStore(fixture.db, nil)
	matches := map[string]sqlite.ItemMatch{
		movieAlpha:   {State: match.MatchStateMatched, Provider: string(match.RefTMDB), Confidence: 1, MatchedAt: time.UnixMilli(1000)},
		movieBravo:   {State: match.MatchStateDisabled},
		movieCharlie: {State: match.MatchStateUnmatched},
	}

	for itemID, itemMatch := range matches {
		if err := store.SetItemMatch(t.Context(), itemID, itemMatch); err != nil {
			t.Fatalf("SetItemMatch(%s) error = %v", itemID, err)
		}
	}

	return store
}

func TestListItemsFiltersByMatchState(t *testing.T) {
	t.Parallel()

	fixture := newListingFixture(t)
	seedListingMatches(t, fixture)

	tests := []struct {
		state library.MatchStateFilter
		want  []string
	}{
		{library.MatchFilterAll, []string{movie1917, movieAlpha, movieBravo, movieCharlie, movieAerger}},
		{library.MatchFilterMatched, []string{movieAlpha}},
		{library.MatchFilterDisabled, []string{movieBravo}},
		{library.MatchFilterUnmatched, []string{movie1917, movieCharlie, movieAerger}},
		{"bogus", nil},
	}
	for _, test := range tests {
		t.Run(string(test.state), func(t *testing.T) {
			t.Parallel()

			filter := library.ListingFilter{Kind: library.ItemKindMovie, MatchState: test.state}

			got, total := fixture.listAllIDs(t, moviesLibraryID, library.ListingQuery{Filter: filter}, 2)
			if !slices.Equal(got, test.want) || total != int64(len(test.want)) {
				t.Errorf("ids = %v (total %d), want %v", got, total, test.want)
			}
		})
	}
}

func TestItemMatchStatusesReadsStoredRowsOnly(t *testing.T) {
	t.Parallel()

	fixture := newListingFixture(t)
	store := seedListingMatches(t, fixture)

	got, err := store.ItemMatchStatuses(t.Context(), []string{movieAlpha, movieBravo, movieCharlie, movie1917, "unknown"})
	if err != nil {
		t.Fatalf("ItemMatchStatuses error = %v", err)
	}

	want := map[string]match.ItemStatus{
		movieAlpha:   {State: match.MatchStateMatched, Provider: match.RefTMDB},
		movieBravo:   {State: match.MatchStateDisabled},
		movieCharlie: {State: match.MatchStateUnmatched},
	}
	if !maps.Equal(got, want) {
		t.Errorf("ItemMatchStatuses = %v, want %v", got, want)
	}

	empty, err := store.ItemMatchStatuses(t.Context(), nil)
	if err != nil || len(empty) != 0 {
		t.Errorf("ItemMatchStatuses(nil) = %v, %v, want empty", empty, err)
	}
}

func TestItemMatchStatusesReadsLargeBatches(t *testing.T) {
	t.Parallel()

	fixture := newListingFixture(t)
	store := seedListingMatches(t, fixture)

	const padding = 1200

	itemIDs := make([]string, 0, padding+1)
	for range padding {
		itemIDs = append(itemIDs, "missing")
	}

	itemIDs = append(itemIDs, movieAlpha)

	got, err := store.ItemMatchStatuses(t.Context(), itemIDs)
	if err != nil || len(got) != 1 || got[movieAlpha].State != match.MatchStateMatched {
		t.Errorf("ItemMatchStatuses = %v, %v, want only Alpha matched", got, err)
	}
}
