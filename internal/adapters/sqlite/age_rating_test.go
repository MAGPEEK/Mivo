package sqlite_test

import (
	"database/sql"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/pressly/goose/v3"

	"github.com/magpeek/mivo/internal/domain/metadata"
)

// ageRatingsSchemaVersion is the migration that adds the age bucket columns.
const ageRatingsSchemaVersion = 14

// noBucket marks an expected NULL bucket in the tables below.
const noBucket = -1

// storedAgeRating is what the age rating columns of one item hold.
type storedAgeRating struct {
	own       int64
	effective int64
	source    sql.NullString
}

func readAgeRating(t *testing.T, inspection *sql.DB, itemID string) storedAgeRating {
	t.Helper()

	var own, effective sql.NullInt64

	var stored storedAgeRating

	err := inspection.QueryRowContext(t.Context(),
		"SELECT age_rating, effective_age_rating, age_rating_source FROM items WHERE id = ?", itemID).
		Scan(&own, &effective, &stored.source)
	if err != nil {
		t.Fatalf("read age rating of %s: %v", itemID, err)
	}

	stored.own, stored.effective = bucketOrNone(own), bucketOrNone(effective)

	return stored
}

func bucketOrNone(value sql.NullInt64) int64 {
	if !value.Valid {
		return noBucket
	}

	return value.Int64
}

// buckets is the own and the effective bucket an item should hold.
type buckets struct {
	own       int64
	effective int64
}

func assertBuckets(t *testing.T, inspection *sql.DB, itemID string, want buckets) {
	t.Helper()

	got := readAgeRating(t, inspection, itemID)
	if got.own != want.own || got.effective != want.effective {
		t.Errorf("item %s buckets = (own %d, effective %d), want %+v", itemID, got.own, got.effective, want)
	}
}

// showChain places one episode and returns the show, season and episode IDs.
func showChain(t *testing.T, fixture *placementFixture, episode int) (show, season, episodeID string) {
	t.Helper()

	chain := fixture.save(t, placedFile(showsLibraryID, testID('f', episode), fmt.Sprintf("Show/S01E%02d.mkv", episode),
		episodePlacement("show", 1, episode)))

	return chain[0], chain[1], chain[2]
}

func saveRating(t *testing.T, fixture *placementFixture, itemID, rating string) {
	t.Helper()

	record := metadata.Record{Title: "Title " + itemID[:8], ContentRating: rating}
	if err := fixture.catalog.SaveMetadata(t.Context(), itemID, record); err != nil {
		t.Fatalf("SaveMetadata(%s, %q) error = %v", itemID, rating, err)
	}
}

func staleAgeRatingRows(t *testing.T, inspection *sql.DB) int {
	t.Helper()

	return countRows(t, inspection, "SELECT COUNT(*) FROM items WHERE age_rating_source IS NOT content_rating")
}

func TestAgeRatingEpisodeInheritsTheShowRating(t *testing.T) {
	t.Parallel()

	fixture := newPlacementFixture(t)
	show, season, episode := showChain(t, fixture, 1)

	saveRating(t, fixture, show, "TV-14")

	assertBuckets(t, fixture.inspection, show, buckets{12, 12})
	assertBuckets(t, fixture.inspection, season, buckets{noBucket, 12})
	assertBuckets(t, fixture.inspection, episode, buckets{noBucket, 12})

	// A new episode of the rated show inherits on insert, before any metadata is written.
	_, _, added := showChain(t, fixture, 2)
	assertBuckets(t, fixture.inspection, added, buckets{noBucket, 12})

	if got := staleAgeRatingRows(t, fixture.inspection); got != 0 {
		t.Errorf("stale rows = %d, want 0", got)
	}
}

func TestAgeRatingEpisodeWithItsOwnRatingKeepsIt(t *testing.T) {
	t.Parallel()

	fixture := newPlacementFixture(t)
	show, season, episode := showChain(t, fixture, 1)

	saveRating(t, fixture, show, "TV-14")
	saveRating(t, fixture, episode, "TV-MA")
	saveRating(t, fixture, show, "TV-Y")

	assertBuckets(t, fixture.inspection, show, buckets{0, 0})
	assertBuckets(t, fixture.inspection, season, buckets{noBucket, 0})
	assertBuckets(t, fixture.inspection, episode, buckets{16, 16})
}

func TestAgeRatingShowRatingChangePropagates(t *testing.T) {
	t.Parallel()

	fixture := newPlacementFixture(t)
	show, season, episode := showChain(t, fixture, 1)

	steps := []struct {
		rating        string
		wantEffective int64
	}{
		{"TV-14", 12},
		{"TV-MA", 16},
		{"", noBucket},
		{"FSK 6", 6},
	}

	for _, step := range steps {
		saveRating(t, fixture, show, step.rating)

		assertBuckets(t, fixture.inspection, show, buckets{step.wantEffective, step.wantEffective})
		assertBuckets(t, fixture.inspection, season, buckets{noBucket, step.wantEffective})
		assertBuckets(t, fixture.inspection, episode, buckets{noBucket, step.wantEffective})
	}
}

func TestAgeRatingUnknownRatingStaysNull(t *testing.T) {
	t.Parallel()

	cases := []struct {
		rating string
		want   int64
	}{
		{"Not Rated", noBucket},
		{"1８", noBucket},
		{"", noBucket},
		{"XXX", 18},
		{"de/12", 12},
	}

	fixture := newPlacementFixture(t)

	for index, testCase := range cases {
		movie := fixture.save(t, placedFile(moviesLibraryID, testID('g', index+1), fmt.Sprintf("Movie%d.mkv", index),
			moviePlacement(fmt.Sprintf("movie%d", index), "Movie", 2000, "")))[0]

		saveRating(t, fixture, movie, testCase.rating)

		got := readAgeRating(t, fixture.inspection, movie)
		if got.own != testCase.want || got.effective != testCase.want || got.source.String != testCase.rating {
			t.Errorf("rating %q stored as %+v, want bucket %d with source %q", testCase.rating, got, testCase.want, testCase.rating)
		}
	}
}

// An upgraded database: the items existed with ratings before migration 00014 added the
// columns, so the backfill computes every bucket once and is a no-op afterwards.
func TestAgeRatingBackfillOnAnUpgradedDatabase(t *testing.T) {
	t.Parallel()

	fixture := newPlacementFixture(t)
	show, season, episode := showChain(t, fixture, 1)
	movie := fixture.save(t, placedFile(moviesLibraryID, testID('g', 1), "Movie.mkv", moviePlacement("movie", "Movie", 2000, "")))[0]

	rollBackAgeRatings(t, fixture.inspection)
	execute(t, fixture.inspection, "UPDATE items SET content_rating = 'TV-PG' WHERE id = ?", show)
	execute(t, fixture.inspection, "UPDATE items SET content_rating = 'R' WHERE id = ?", movie)

	if err := fixture.db.Migrate(t.Context()); err != nil {
		t.Fatalf("Migrate error = %v", err)
	}

	for run := 1; run <= 2; run++ {
		if err := fixture.catalog.BackfillAgeRatings(t.Context()); err != nil {
			t.Fatalf("BackfillAgeRatings run %d error = %v", run, err)
		}

		assertBuckets(t, fixture.inspection, show, buckets{6, 6})
		assertBuckets(t, fixture.inspection, season, buckets{noBucket, 6})
		assertBuckets(t, fixture.inspection, episode, buckets{noBucket, 6})
		assertBuckets(t, fixture.inspection, movie, buckets{16, 16})

		if got := staleAgeRatingRows(t, fixture.inspection); got != 0 {
			t.Errorf("run %d: stale rows = %d, want 0", run, got)
		}
	}
}

// A row changed behind the adapter's back (content_rating without its buckets) is stale; the
// backfill recomputes it and the subtree that inherits from it.
func TestAgeRatingBackfillRecomputesChangedRows(t *testing.T) {
	t.Parallel()

	fixture := newPlacementFixture(t)
	show, _, episode := showChain(t, fixture, 1)

	saveRating(t, fixture, show, "TV-14")
	execute(t, fixture.inspection, "UPDATE items SET content_rating = 'NC-17' WHERE id = ?", show)

	if err := fixture.catalog.BackfillAgeRatings(t.Context()); err != nil {
		t.Fatalf("BackfillAgeRatings error = %v", err)
	}

	assertBuckets(t, fixture.inspection, show, buckets{18, 18})
	assertBuckets(t, fixture.inspection, episode, buckets{noBucket, 18})
}

// More stale rows than one backfill batch holds are recomputed across several transactions.
func TestAgeRatingBackfillCoversMoreRowsThanOneBatch(t *testing.T) {
	t.Parallel()

	const movies = 1203

	fixture := newPlacementFixture(t)

	for number := 1; number <= movies; number++ {
		insertItem(t, fixture.inspection, newItemSpec(testID('m', number), moviesLibraryID, "movie", fmt.Sprintf("Movie %d", number)))
	}

	execute(t, fixture.inspection, "UPDATE items SET content_rating = 'PG-13'")

	if err := fixture.catalog.BackfillAgeRatings(t.Context()); err != nil {
		t.Fatalf("BackfillAgeRatings error = %v", err)
	}

	if got := staleAgeRatingRows(t, fixture.inspection); got != 0 {
		t.Errorf("stale rows = %d, want 0", got)
	}

	rated := countRows(t, fixture.inspection, "SELECT COUNT(*) FROM items WHERE effective_age_rating = 12")
	if rated != movies {
		t.Errorf("items with effective bucket 12 = %d, want %d", rated, movies)
	}
}

// The backfill's lookup reads the partial index of stale rows, so a run without changes does
// not scan items.
func TestAgeRatingBackfillLookupUsesTheStaleIndex(t *testing.T) {
	t.Parallel()

	_, path := openMigratedTestDB(t)
	inspection := openInspectionDB(t, path)

	rows, err := inspection.QueryContext(t.Context(), `EXPLAIN QUERY PLAN SELECT id, content_rating FROM items
		WHERE age_rating_source IS NOT content_rating ORDER BY id LIMIT 500`)
	if err != nil {
		t.Fatalf("EXPLAIN QUERY PLAN: %v", err)
	}

	defer func() { _ = rows.Close() }()

	var plan []string

	for rows.Next() {
		var (
			id, parent, unused int
			detail             string
		)

		if err := rows.Scan(&id, &parent, &unused, &detail); err != nil {
			t.Fatalf("scan plan step: %v", err)
		}

		plan = append(plan, detail)
	}

	if err := rows.Err(); err != nil {
		t.Fatalf("EXPLAIN QUERY PLAN: %v", err)
	}

	if !strings.Contains(strings.Join(plan, "\n"), "items_age_rating_stale") {
		t.Errorf("query plan = %q, want the index items_age_rating_stale", plan)
	}
}

func TestAgeRatingBackfillFailsWithACancelledContext(t *testing.T) {
	t.Parallel()

	fixture := newPlacementFixture(t)

	if err := fixture.catalog.BackfillAgeRatings(cancelledContext(t)); err == nil {
		t.Error("BackfillAgeRatings with a cancelled context succeeded, want an error")
	}
}

// rollBackAgeRatings undoes migration 00014 alone, leaving the database as a server before
// M7-11 stored it.
func rollBackAgeRatings(t *testing.T, inspection *sql.DB) {
	t.Helper()

	provider, err := goose.NewProvider(goose.DialectSQLite3, inspection, os.DirFS("migrations"),
		goose.WithDisableGlobalRegistry(true))
	if err != nil {
		t.Fatalf("NewProvider error = %v", err)
	}

	if _, err := provider.DownTo(t.Context(), ageRatingsSchemaVersion-1); err != nil {
		t.Fatalf("DownTo(%d) error = %v", ageRatingsSchemaVersion-1, err)
	}
}
