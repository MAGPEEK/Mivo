package sqlite_test

import (
	"errors"
	"testing"

	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/access"
	"github.com/magpeek/mivo/internal/domain/library"
)

// failClosedSchemaVersion is the migration that stores the uncomputed visibility as (18, 1).
const failClosedSchemaVersion = 17

// uncomputed is the stored visibility of a row the adapter has not computed yet.
func uncomputed() visibility {
	return visibility{bucket: 18, unrated: true}
}

// fixtureMovie inserts a movie row as a writer outside the adapter would: with its rating
// text, but without buckets.
func fixtureMovie(t *testing.T, fixture *placementFixture, id, rating string) {
	t.Helper()

	insertListingItem(t, fixture.inspection, listingItem{
		id: id, libraryID: moviesLibraryID, kind: "movie", title: "Movie " + rating, sortTitle: "movie " + rating, contentRating: rating,
	})
}

// upToSixShowingUnrated is the viewer an uncomputed row leaked to before migration 00017: a
// maximum age without "hide unrated".
func upToSixShowingUnrated(t *testing.T) access.Restriction {
	t.Helper()

	return restrictionOf(access.AllLibraries(), maxAgeLimit(t, access.Age6, false))
}

// visibleMovies lists the IDs of the movies that viewer sees; each must be found by ID too.
func visibleMovies(t *testing.T, fixture *placementFixture) []string {
	t.Helper()

	restriction := upToSixShowingUnrated(t)

	page, err := fixture.reader.ListItems(t.Context(), moviesLibraryID, library.PageRequest{}, library.ListingQuery{Restriction: restriction})
	if err != nil {
		t.Fatalf("ListItems error = %v", err)
	}

	ids := make([]string, 0, len(page.Items))
	for _, item := range page.Items {
		ids = append(ids, item.Item.ID)

		if _, err := fixture.reader.GetItem(t.Context(), item.Item.ID, restriction); err != nil {
			t.Errorf("GetItem(%s) of a listed item: %v", item.Item.ID, err)
		}
	}

	return ids
}

func backfill(t *testing.T, fixture *placementFixture) {
	t.Helper()

	if err := fixture.catalog.BackfillAgeRatings(t.Context()); err != nil {
		t.Fatalf("BackfillAgeRatings error = %v", err)
	}
}

// A row whose visibility was not computed yet is hidden from every limited viewer, also from
// one who shows unrated titles: an FSK 18 film must not be visible until the backfill ran.
func TestAgeVisibilityUncomputedRowsFailClosed(t *testing.T) {
	t.Parallel()

	fixture := newPlacementFixture(t)
	adult, child := testID('m', 18), testID('m', 1)

	fixtureMovie(t, fixture, adult, "FSK 18")
	fixtureMovie(t, fixture, child, "FSK 0")

	assertVisibility(t, fixture.inspection, adult, uncomputed())
	assertVisibility(t, fixture.inspection, child, uncomputed())
	assertTexts(t, "movies before the backfill", visibleMovies(t, fixture), []string{})

	if _, err := fixture.reader.GetItem(t.Context(), adult, upToSixShowingUnrated(t)); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("GetItem of the stale FSK 18 film: error = %v, want domain.ErrNotFound", err)
	}

	backfill(t, fixture)

	assertVisibility(t, fixture.inspection, adult, visibility{18, false})
	assertVisibility(t, fixture.inspection, child, visibility{0, false})
	assertTexts(t, "movies after the backfill", visibleMovies(t, fixture), []string{child})
}

// Migration 00017 closes the rows that 00016 left open: stale with (NULL, 1), which passes a
// maximum age.
func TestAgeVisibilityMigrationClosesStaleRows(t *testing.T) {
	t.Parallel()

	fixture := newPlacementFixture(t)
	adult, computed := testID('m', 18), testID('m', 1)

	fixtureMovie(t, fixture, adult, "FSK 18")
	fixtureMovie(t, fixture, computed, "FSK 0")
	backfill(t, fixture)

	rollBackTo(t, fixture.inspection, failClosedSchemaVersion-1)
	execute(t, fixture.inspection,
		"UPDATE items SET age_rating_source = NULL, visibility_age_rating = NULL, visibility_unrated = 1 WHERE id = ?", adult)

	if err := fixture.db.Migrate(t.Context()); err != nil {
		t.Fatalf("Migrate error = %v", err)
	}

	assertVisibility(t, fixture.inspection, adult, uncomputed())
	assertVisibility(t, fixture.inspection, computed, visibility{0, false})
	assertTexts(t, "movies after the migration", visibleMovies(t, fixture), []string{computed})
}

// A rating written without the adapter leaves the row stale; it is closed at once, not only
// when the backfill finds it.
func TestAgeVisibilityRatingChangedOutsideTheAdapterFailsClosed(t *testing.T) {
	t.Parallel()

	fixture := newPlacementFixture(t)
	movie := testID('m', 1)

	fixtureMovie(t, fixture, movie, "FSK 0")
	backfill(t, fixture)
	assertTexts(t, "movies while rated FSK 0", visibleMovies(t, fixture), []string{movie})

	execute(t, fixture.inspection, "UPDATE items SET content_rating = 'FSK 18' WHERE id = ?", movie)
	assertVisibility(t, fixture.inspection, movie, uncomputed())
	assertTexts(t, "movies after the change", visibleMovies(t, fixture), []string{})

	// The adapter's own write computes the pair in the same transaction.
	saveRating(t, fixture, movie, "FSK 6")
	assertVisibility(t, fixture.inspection, movie, visibility{6, false})
}
