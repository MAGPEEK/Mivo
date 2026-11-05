package sqlite_test

import (
	"database/sql"
	"fmt"
	"strconv"
	"testing"
	"time"

	"github.com/magpeek/mivo/internal/adapters/sqlite"
	appsearch "github.com/magpeek/mivo/internal/app/search"
	"github.com/magpeek/mivo/internal/domain/metadata"
)

// searchBenchmarkLimit is the size of the search dropdown (M5-15 "top").
const searchBenchmarkLimit = 8

// Sizes of the write benchmarks: a feature film's cast and crew as a provider delivers it, and
// a library large enough that a per-credit cost shows when it is deleted.
const (
	benchmarkCreditCount     = 30
	benchmarkDeleteItemCount = 2_000
)

// BenchmarkSearch measures searches in a library of benchmarkItemCount movies ("Movie 0" to
// "Movie 49999", genres from benchmarkGenres). Task M5-14 asks for < 100 ms; as with the
// listing benchmark the measured numbers are documented in the task file, not asserted.
// "movie" is the worst case: every item matches and must be ranked.
//
//	go test -run '^$' -bench BenchmarkSearch -benchtime 20x ./internal/adapters/sqlite/
func BenchmarkSearch(b *testing.B) {
	db, path := openBenchmarkDB(b)
	seedBenchmarkLibrary(b, path)

	index := sqlite.NewSearchIndex(db)
	filter := appsearch.Filter{LibraryIDs: []string{moviesLibraryID}}

	for _, query := range []string{"movie", "dra", "movie 4242", "comedy movie 12", "zzz"} {
		b.Run(query, func(b *testing.B) {
			for b.Loop() {
				if _, err := index.Search(b.Context(), query, filter, searchBenchmarkLimit); err != nil {
					b.Fatalf("Search(%q) error = %v", query, err)
				}
			}
		})
	}
}

// searchIndexVariants runs a write benchmark with the search triggers and, for comparison,
// with them dropped.
func searchIndexVariants() map[string]bool {
	return map[string]bool{"with index": true, "without index": false}
}

// BenchmarkSaveMetadataWithCredits measures SaveMetadata of a movie with benchmarkCreditCount
// credits and two genres, which replaces every credit and fires the search triggers per row.
//
//	go test -run '^$' -bench BenchmarkSaveMetadataWithCredits -benchtime 200x ./internal/adapters/sqlite/
func BenchmarkSaveMetadataWithCredits(b *testing.B) {
	for name, indexed := range searchIndexVariants() {
		b.Run(name, func(b *testing.B) {
			db, path := openBenchmarkDB(b)
			inspection := openBenchmarkInspection(b, path, indexed)
			benchmarkExec(b, inspection, `INSERT INTO libraries (id, name, kind, created_at, updated_at)
				VALUES (?, 'Movies', 'movies', 0, 0)`, moviesLibraryID)
			benchmarkExec(b, inspection, `INSERT INTO items (id, library_id, kind, match_key, title, sort_title, created_at, updated_at)
				VALUES ('m1', ?, 'movie', 'm1', 'Movie', 'Movie', 0, 0)`, moviesLibraryID)

			catalog := sqlite.NewLibraryCatalog(db, time.Now)
			records := []metadata.Record{creditsRecord("A"), creditsRecord("B")}

			saves := 0
			for b.Loop() {
				// Alternating records make every save change all credits.
				if err := catalog.SaveMetadata(b.Context(), "m1", records[saves%len(records)]); err != nil {
					b.Fatalf("SaveMetadata error = %v", err)
				}

				saves++
			}
		})
	}
}

// BenchmarkDeleteLibraryWithCredits measures deleting a library of benchmarkDeleteItemCount
// movies with benchmarkCreditCount credits and two genres each.
//
//	go test -run '^$' -bench BenchmarkDeleteLibraryWithCredits -benchtime 3x ./internal/adapters/sqlite/
func BenchmarkDeleteLibraryWithCredits(b *testing.B) {
	for name, indexed := range searchIndexVariants() {
		b.Run(name, func(b *testing.B) {
			_, path := openBenchmarkDB(b)
			inspection := openBenchmarkInspection(b, path, indexed)

			for range b.N {
				b.StopTimer()
				seedCreditedLibrary(b, inspection)
				b.StartTimer()
				benchmarkExec(b, inspection, `DELETE FROM libraries WHERE id = ?`, moviesLibraryID)
			}
		})
	}
}

func creditsRecord(variant string) metadata.Record {
	record := metadata.Record{Title: "Movie " + variant, SortTitle: "movie " + variant, Genres: []string{"Drama " + variant, "War"}}
	for order := range benchmarkCreditCount {
		record.People = append(record.People, metadata.Credit{
			Name: fmt.Sprintf("Person %s%d", variant, order), Role: metadata.RoleActor, Character: "Role", Order: order,
		})
	}

	return record
}

// openBenchmarkInspection opens one connection with foreign keys on, so cascades run as on
// the writer, and drops the search triggers unless indexed.
func openBenchmarkInspection(b *testing.B, path string, indexed bool) *sql.DB {
	b.Helper()

	inspection, err := sql.Open("sqlite", path)
	if err != nil {
		b.Fatalf("open inspection: %v", err)
	}

	b.Cleanup(func() { _ = inspection.Close() })
	inspection.SetMaxOpenConns(1)
	benchmarkExec(b, inspection, `PRAGMA foreign_keys = ON`)

	if !indexed {
		for _, trigger := range searchTriggers() {
			benchmarkExec(b, inspection, `DROP TRIGGER `+trigger)
		}
	}

	return inspection
}

func searchTriggers() []string {
	return []string{
		"search_docs_insert", "search_docs_delete", "items_search_insert", "items_search_update",
		"item_people_search_insert", "item_people_search_delete", "item_people_search_update", "people_search_rename",
		"item_tags_search_insert", "item_tags_search_delete", "item_tags_search_update", "tags_search_rename",
	}
}

func seedCreditedLibrary(b *testing.B, inspection *sql.DB) {
	b.Helper()

	statements := []string{
		`INSERT INTO libraries (id, name, kind, created_at, updated_at) VALUES ('` + moviesLibraryID + `', 'Movies', 'movies', 0, 0)`,
		`INSERT OR IGNORE INTO tags (id, kind, name) VALUES ('g1', 'genre', 'Drama'), ('g2', 'genre', 'War')`,
		seriesSQL(benchmarkCreditCount) + `INSERT OR IGNORE INTO people (id, name, sort_name)
			SELECT 'p' || value, 'Person ' || value, 'Person ' || value FROM series`,
		seriesSQL(benchmarkDeleteItemCount) + `INSERT INTO items (id, library_id, kind, match_key, title, sort_title, created_at, updated_at)
			SELECT 'i' || value, '` + moviesLibraryID + `', 'movie', 'i' || value, 'Movie ' || value, 'movie ' || value, 0, 0
			FROM series`,
		`INSERT INTO item_people (item_id, person_id, role, position) SELECT items.id, people.id, 'actor', 0 FROM items, people`,
		`INSERT INTO item_tags (item_id, tag_id, position) SELECT items.id, tags.id, 0 FROM items, tags`,
	}
	for _, statement := range statements {
		benchmarkExec(b, inspection, statement)
	}
}

// seriesSQL starts a statement with the common table expression series(value) of 1..count;
// the driver's SQLite has no generate_series.
func seriesSQL(count int) string {
	return `WITH RECURSIVE series(value) AS (SELECT 1 UNION ALL SELECT value + 1 FROM series WHERE value < ` +
		strconv.Itoa(count) + `) `
}

func benchmarkExec(b *testing.B, inspection *sql.DB, query string, args ...any) {
	b.Helper()

	if _, err := inspection.ExecContext(b.Context(), query, args...); err != nil {
		b.Fatalf("%s: %v", query, err)
	}
}
