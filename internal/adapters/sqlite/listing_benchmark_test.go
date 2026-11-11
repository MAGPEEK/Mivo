package sqlite_test

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/magpeek/mivo/internal/adapters/sqlite"
	applibrary "github.com/magpeek/mivo/internal/app/library"
	"github.com/magpeek/mivo/internal/domain/access"
	"github.com/magpeek/mivo/internal/domain/library"
)

// benchmarkItemCount is the library size of the performance check of task M5-11.
const benchmarkItemCount = 50_000

// Value pools the benchmark items cycle through.
func benchmarkGenres() []string {
	return []string{"Action", "Comedy", "Drama", "Horror", "Romance", "Thriller", "War", "Western"}
}

func benchmarkContainers() []string { return []string{"mkv", "mp4", "avi"} }

func benchmarkCodecs() []string { return []string{"h264", "hevc", "av1"} }

func benchmarkRatings() []string { return []string{"G", "PG", "PG-13", "R"} }

func benchmarkWidths() []int { return []int{720, 1280, 1920, 3840} }

// BenchmarkListingFirstPageAndFacets measures what the library grid asks for when a filter
// changes on a 50,000-item library: the first page of a filtered and sorted listing (with its
// total count) and every facet. Task M5-11 asks for < 300 ms; the measured number is
// documented in the task file, not asserted, so a slow CI machine cannot make it flaky.
//
//	go test -run '^$' -bench BenchmarkListingFirstPageAndFacets -benchtime 10x ./internal/adapters/sqlite/
func BenchmarkListingFirstPageAndFacets(b *testing.B) {
	db, path := openBenchmarkDB(b)
	seedBenchmarkLibrary(b, path)

	reader := sqlite.NewCatalogReader(db)
	filter := library.ListingFilter{Kind: library.ItemKindMovie, Genres: []string{"Drama", "War"}, Containers: []string{"mkv"}}
	query := library.ListingQuery{Restriction: access.Unrestricted(), Filter: filter, Order: library.ListingOrder{Sort: library.SortByYear, Descending: true}}
	ctx := context.Background()

	listPage := func(b *testing.B, query library.ListingQuery) {
		b.Helper()

		page, err := reader.ListItems(ctx, moviesLibraryID, library.PageRequest{Limit: library.DefaultPageLimit}, query)
		if err != nil || len(page.Items) != library.DefaultPageLimit {
			b.Fatalf("ListItems = %d items, %v", len(page.Items), err)
		}
	}
	countFacets := func(b *testing.B) {
		b.Helper()

		_, err := reader.CountFacets(ctx, moviesLibraryID, applibrary.FacetRequest{Restriction: access.Unrestricted(), Fields: library.FacetFields(), Filter: filter})
		if err != nil {
			b.Fatalf("CountFacets error = %v", err)
		}
	}

	b.Run("page and facets", func(b *testing.B) {
		for b.Loop() {
			listPage(b, query)
			countFacets(b)
		}
	})
	b.Run("page", func(b *testing.B) {
		for b.Loop() {
			listPage(b, query)
		}
	})
	b.Run("facets", func(b *testing.B) {
		for b.Loop() {
			countFacets(b)
		}
	})
	b.Run("default page", func(b *testing.B) {
		for b.Loop() {
			listPage(b, library.ListingQuery{Restriction: access.Unrestricted(), Filter: library.ListingFilter{Kind: library.ItemKindMovie}})
		}
	})
}

// BenchmarkListingRestrictedFirstPageAndFacets measures the same requests for a "Children"
// viewer (task M7-12), who sees half of the library (G and PG of G, PG, PG-13, R): the
// restriction must keep the title page on its index scan and the facets in one statement.
//
//	go test -run '^$' -bench BenchmarkListingRestricted -benchtime 10x ./internal/adapters/sqlite/
func BenchmarkListingRestrictedFirstPageAndFacets(b *testing.B) {
	db, path := openBenchmarkDB(b)
	seedBenchmarkLibrary(b, path)

	ctx := context.Background()
	children := benchmarkChildren(b, db)
	reader := sqlite.NewCatalogReader(db)
	// Action and Comedy are genres of movies a child may see (see insertBenchmarkMovie).
	filter := library.ListingFilter{Kind: library.ItemKindMovie, Genres: []string{"Action", "Comedy"}, Containers: []string{"mkv"}}
	queries := map[string]library.ListingQuery{
		"page":         {Restriction: children, Filter: filter, Order: library.ListingOrder{Sort: library.SortByYear, Descending: true}},
		"default page": {Restriction: children, Filter: library.ListingFilter{Kind: library.ItemKindMovie}},
	}

	for name, query := range queries {
		b.Run(name, func(b *testing.B) {
			for b.Loop() {
				page, err := reader.ListItems(ctx, moviesLibraryID, library.PageRequest{Limit: library.DefaultPageLimit}, query)
				if err != nil || len(page.Items) != library.DefaultPageLimit || page.TotalCount >= benchmarkItemCount {
					b.Fatalf("ListItems = %d of %d items, %v", len(page.Items), page.TotalCount, err)
				}
			}
		})
	}

	b.Run("facets", func(b *testing.B) {
		for b.Loop() {
			_, err := reader.CountFacets(ctx, moviesLibraryID, applibrary.FacetRequest{Restriction: children, Fields: library.FacetFields(), Filter: filter})
			if err != nil {
				b.Fatalf("CountFacets error = %v", err)
			}
		}
	})
}

// benchmarkChildren computes the age buckets of the seeded library and returns the
// restriction of a "Children" profile with access to the movies library only.
func benchmarkChildren(b *testing.B, db *sqlite.DB) access.Restriction {
	b.Helper()

	if err := sqlite.NewLibraryCatalog(db, time.Now).BackfillAgeRatings(context.Background()); err != nil {
		b.Fatalf("BackfillAgeRatings error = %v", err)
	}

	limit, err := access.LimitForPreset(access.PresetChildren, access.AgeAll)
	if err != nil {
		b.Fatalf("LimitForPreset error = %v", err)
	}

	return access.Viewer{Libraries: access.OnlyLibraries(moviesLibraryID), Limit: limit}.Restriction()
}

func openBenchmarkDB(b *testing.B) (*sqlite.DB, string) {
	b.Helper()

	path := filepath.Join(b.TempDir(), testDatabaseFile)

	db, err := sqlite.Open(context.Background(), sqlite.Options{Path: path})
	if err != nil {
		b.Fatalf("Open error = %v", err)
	}

	b.Cleanup(func() { _ = db.Close() })

	if err := db.Migrate(context.Background()); err != nil {
		b.Fatalf("Migrate error = %v", err)
	}

	return db, path
}

// seedBenchmarkLibrary writes the movies library with benchmarkItemCount movies, each with
// one or two genres, one file, a video and an audio stream, in one transaction.
func seedBenchmarkLibrary(b *testing.B, path string) {
	b.Helper()

	inspection, err := sql.Open("sqlite", path)
	if err != nil {
		b.Fatalf("open inspection: %v", err)
	}

	defer func() { _ = inspection.Close() }()

	ctx := context.Background()

	transaction, err := inspection.BeginTx(ctx, nil)
	if err != nil {
		b.Fatalf("begin: %v", err)
	}

	seedStatements := []string{
		`INSERT INTO libraries (id, name, kind, created_at, updated_at) VALUES ('` + moviesLibraryID + `', 'Movies', 'movies', 0, 0)`,
		`INSERT INTO library_roots (id, library_id, path, path_key, position, created_at)
			VALUES ('` + moviesLibraryID + `', '` + moviesLibraryID + `', '/media/movies', '/media/movies', 0, 0)`,
	}
	for index, genre := range benchmarkGenres() {
		seedStatements = append(seedStatements, fmt.Sprintf(`INSERT INTO tags (id, kind, name) VALUES ('g%d', 'genre', '%s')`, index, genre))
	}

	for _, statement := range seedStatements {
		if _, err := transaction.ExecContext(ctx, statement); err != nil {
			b.Fatalf("%s: %v", statement, err)
		}
	}

	for index := range benchmarkItemCount {
		insertBenchmarkMovie(b, transaction, index)
	}

	if err := transaction.Commit(); err != nil {
		b.Fatalf("commit: %v", err)
	}
}

func insertBenchmarkMovie(b *testing.B, transaction *sql.Tx, index int) {
	b.Helper()

	ctx := context.Background()
	itemID := fmt.Sprintf("i%07d-0000-4000-8000-000000000000", index)
	fileID := fmt.Sprintf("f%07d-0000-4000-8000-000000000000", index)
	width := benchmarkWidths()[index%len(benchmarkWidths())]

	statements := []struct {
		query string
		args  []any
	}{
		{
			`INSERT INTO items (id, library_id, kind, match_key, title, sort_title, year, content_rating, created_at, updated_at)
			VALUES (?, ?, 'movie', ?, ?, ?, ?, ?, ?, ?)`,
			[]any{
				itemID, moviesLibraryID, itemID, fmt.Sprintf("Movie %d", index), fmt.Sprintf("movie %07d", (index*7919)%benchmarkItemCount),
				1950 + index%75, benchmarkRatings()[index%len(benchmarkRatings())], index, index,
			},
		},
		{`INSERT INTO item_tags (item_id, tag_id, position) VALUES (?, ?, 0)`, []any{itemID, fmt.Sprintf("g%d", index%len(benchmarkGenres()))}},
		{`INSERT OR IGNORE INTO item_tags (item_id, tag_id, position) VALUES (?, ?, 1)`, []any{itemID, fmt.Sprintf("g%d", (index/3+1)%len(benchmarkGenres()))}},
		{
			`INSERT INTO media_files (id, item_id, library_id, root_id, relative_path, size_bytes, modified_at, container,
			format_name, duration_ms, bit_rate, probed_at) VALUES (?, ?, ?, ?, ?, ?, 0, ?, '', ?, ?, 0)`,
			[]any{
				fileID, itemID, moviesLibraryID, moviesLibraryID, fmt.Sprintf("Movie %d/movie.mkv", index), index * 1000,
				benchmarkContainers()[index%len(benchmarkContainers())], index * 10, index * 3,
			},
		},
		{benchmarkStreamSQL, []any{fileID, 0, "video", benchmarkCodecs()[index%len(benchmarkCodecs())], width, width * 9 / 16, ""}},
		{benchmarkStreamSQL, []any{fileID, 1, "audio", "aac", 0, 0, "eng"}},
	}

	for _, statement := range statements {
		if _, err := transaction.ExecContext(ctx, statement.query, statement.args...); err != nil {
			b.Fatalf("seed movie %d: %v", index, err)
		}
	}
}

const benchmarkStreamSQL = `INSERT INTO media_streams (media_file_id, stream_index, kind, codec, profile, level,
	pixel_format, bit_depth, width, height, frame_rate, color_primaries, color_transfer, color_space, hdr,
	channels, channel_layout, sample_rate, language, title, is_default, is_forced, is_attached_picture, bit_rate)
	VALUES (?, ?, ?, ?, '', 0, '', 0, ?, ?, '', '', '', '', '', 0, '', 0, ?, '', 0, 0, 0, 0)`
