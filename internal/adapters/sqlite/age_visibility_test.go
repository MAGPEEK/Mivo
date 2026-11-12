package sqlite_test

import (
	"database/sql"
	"testing"
)

// ancestorVisibilitySchemaVersion is the migration that adds the visibility columns.
const ancestorVisibilitySchemaVersion = 16

// visibility is what the ancestor visibility columns of an item should hold: the highest own
// bucket of its chain (noBucket for none) and whether the chain has an unrated member.
type visibility struct {
	bucket  int64
	unrated bool
}

func assertVisibility(t *testing.T, inspection *sql.DB, itemID string, want visibility) {
	t.Helper()

	var (
		bucket  sql.NullInt64
		unrated bool
	)

	err := inspection.QueryRowContext(t.Context(),
		"SELECT visibility_age_rating, visibility_unrated FROM items WHERE id = ?", itemID).Scan(&bucket, &unrated)
	if err != nil {
		t.Fatalf("read visibility of %s: %v", itemID, err)
	}

	if got := (visibility{bucket: bucketOrNone(bucket), unrated: unrated}); got != want {
		t.Errorf("item %s visibility = %+v, want %+v", itemID, got, want)
	}
}

// The visibility of an item is decided by its whole chain: the highest own bucket of the item
// and its ancestors, and "unrated" exactly when the top ancestor has no own rating.
func TestAgeVisibilityFollowsTheAncestorChain(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                              string
		showRating, seasonRating, episode string
		wantShow, wantSeason, wantEpisode visibility
	}{
		{
			name:     "nothing rated",
			wantShow: visibility{noBucket, true}, wantSeason: visibility{noBucket, true}, wantEpisode: visibility{noBucket, true},
		},
		{
			name: "show rated, episode inherits", showRating: "FSK 6",
			wantShow: visibility{6, false}, wantSeason: visibility{6, false}, wantEpisode: visibility{6, false},
		},
		{
			name: "a lower rated episode does not open a path through its show", showRating: "FSK 16", episode: "FSK 0",
			wantShow: visibility{16, false}, wantSeason: visibility{16, false}, wantEpisode: visibility{16, false},
		},
		{
			name: "a higher rated episode is hidden on its own", showRating: "FSK 0", episode: "FSK 16",
			wantShow: visibility{0, false}, wantSeason: visibility{0, false}, wantEpisode: visibility{16, false},
		},
		{
			name: "a rated episode of an unrated show stays unrated", episode: "FSK 0",
			wantShow: visibility{noBucket, true}, wantSeason: visibility{noBucket, true}, wantEpisode: visibility{0, true},
		},
		{
			name: "rating on the season", seasonRating: "FSK 12", episode: "FSK 6",
			wantShow: visibility{noBucket, true}, wantSeason: visibility{12, true}, wantEpisode: visibility{12, true},
		},
		{
			name: "rating on every level", showRating: "FSK 6", seasonRating: "FSK 16", episode: "FSK 12",
			wantShow: visibility{6, false}, wantSeason: visibility{16, false}, wantEpisode: visibility{16, false},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			fixture := newPlacementFixture(t)
			show, season, episode := showChain(t, fixture, 1)

			// Children first, so the last write (the show) must propagate through rated levels.
			saveRating(t, fixture, episode, test.episode)
			saveRating(t, fixture, season, test.seasonRating)
			saveRating(t, fixture, show, test.showRating)

			assertVisibility(t, fixture.inspection, show, test.wantShow)
			assertVisibility(t, fixture.inspection, season, test.wantSeason)
			assertVisibility(t, fixture.inspection, episode, test.wantEpisode)
		})
	}
}

func TestAgeVisibilityShowRatingChangePropagates(t *testing.T) {
	t.Parallel()

	fixture := newPlacementFixture(t)
	show, _, episode := showChain(t, fixture, 1)

	saveRating(t, fixture, episode, "FSK 6")

	steps := []struct {
		rating string
		want   visibility
	}{
		{"FSK 16", visibility{16, false}},
		{"FSK 0", visibility{6, false}},
		{"", visibility{6, true}},
		{"FSK 12", visibility{12, false}},
	}
	for _, step := range steps {
		saveRating(t, fixture, show, step.rating)
		assertVisibility(t, fixture.inspection, episode, step.want)
	}
}

// An item stored without a rating (the placement chain) inherits the visibility of its parent
// through the insert trigger, without a write by the adapter.
func TestAgeVisibilityNewEpisodeInheritsItsChain(t *testing.T) {
	t.Parallel()

	fixture := newPlacementFixture(t)
	show, _, _ := showChain(t, fixture, 1)

	saveRating(t, fixture, show, "FSK 16")

	_, _, second := showChain(t, fixture, 2)
	assertVisibility(t, fixture.inspection, second, visibility{16, false})

	movie := fixture.save(t, placedFile(moviesLibraryID, testID('g', 1), "Movie.mkv", moviePlacement("movie", "Movie", 2000, "")))[0]
	assertVisibility(t, fixture.inspection, movie, visibility{noBucket, true})
}

// A database migrated from 00015 holds buckets but no visibility: the migration marks every
// row stale, and the backfill computes the visibility whatever the order of the rows.
func TestAgeVisibilityBackfillAfterTheMigration(t *testing.T) {
	t.Parallel()

	fixture := newPlacementFixture(t)
	show, season, episode := showChain(t, fixture, 1)

	saveRating(t, fixture, show, "FSK 16")
	saveRating(t, fixture, episode, "FSK 0")

	rollBackTo(t, fixture.inspection, ancestorVisibilitySchemaVersion-1)

	if err := fixture.db.Migrate(t.Context()); err != nil {
		t.Fatalf("Migrate error = %v", err)
	}

	if got := staleAgeRatingRows(t, fixture.inspection); got != 3 {
		t.Fatalf("stale rows after the migration = %d, want 3", got)
	}

	for _, id := range []string{show, season, episode} {
		assertVisibility(t, fixture.inspection, id, uncomputed())
	}

	if err := fixture.catalog.BackfillAgeRatings(t.Context()); err != nil {
		t.Fatalf("BackfillAgeRatings error = %v", err)
	}

	assertVisibility(t, fixture.inspection, show, visibility{16, false})
	assertVisibility(t, fixture.inspection, season, visibility{16, false})
	assertVisibility(t, fixture.inspection, episode, visibility{16, false})
	assertBuckets(t, fixture.inspection, episode, buckets{0, 0})
}

// No writer reparents an item today; one that does leaves the item stale, so the backfill
// recomputes its subtree from the new chain.
func TestAgeVisibilityReparentedSubtreeIsRecomputed(t *testing.T) {
	t.Parallel()

	fixture := newPlacementFixture(t)
	show, season, episode := showChain(t, fixture, 1)
	other := fixture.save(t, placedFile(showsLibraryID, testID('f', 9), "Other/S02E01.mkv", episodePlacement("other", 2, 1)))[0]

	saveRating(t, fixture, show, "FSK 0")
	saveRating(t, fixture, other, "FSK 18")
	execute(t, fixture.inspection, "UPDATE items SET parent_id = ? WHERE id = ?", other, season)

	if got := staleAgeRatingRows(t, fixture.inspection); got != 1 {
		t.Fatalf("stale rows after reparenting = %d, want 1", got)
	}

	// Until the backfill runs, the moved row is closed (migration 00017).
	assertVisibility(t, fixture.inspection, season, uncomputed())

	if err := fixture.catalog.BackfillAgeRatings(t.Context()); err != nil {
		t.Fatalf("BackfillAgeRatings error = %v", err)
	}

	assertVisibility(t, fixture.inspection, season, visibility{18, false})
	assertVisibility(t, fixture.inspection, episode, visibility{18, false})
	assertBuckets(t, fixture.inspection, episode, buckets{noBucket, 18})
}
