package sqlite_test

import (
	"database/sql"
	"errors"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/pressly/goose/v3"

	"github.com/magpeek/mivo/internal/adapters/sqlite"
	appsearch "github.com/magpeek/mivo/internal/app/search"
	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/access"
	"github.com/magpeek/mivo/internal/domain/library"
)

// lastSchemaVersionWithoutSearch is the schema the back-fill test starts from.
const lastSchemaVersionWithoutSearch = 9

// Items of the search fixture, in the format of testID.
const (
	searchMatrixID  = "m0000001-0000-4000-8000-000000000000"
	searchCafeID    = "m0000002-0000-4000-8000-000000000000"
	searchSpeedID   = "m0000003-0000-4000-8000-000000000000"
	searchTrailerID = "x0000001-0000-4000-8000-000000000000"
	searchDarkID    = "s0000001-0000-4000-8000-000000000000"
	searchSeasonID  = "s0000002-0000-4000-8000-000000000000"
	searchEpisodeID = "e0000001-0000-4000-8000-000000000000"
	searchBjorkID   = "a0000001-0000-4000-8000-000000000000"
	searchAlbumID   = "a0000002-0000-4000-8000-000000000000"
	searchTrackID   = "t0000001-0000-4000-8000-000000000000"
)

// searchFixtureItems are the items of the search fixture: id, library, parent, kind, title,
// sort title and original title. The extra and the season are not indexed.
func searchFixtureItems() [][]any {
	return [][]any{
		{searchMatrixID, moviesLibraryID, nil, "movie", "The Matrix", "Matrix", "Matrice"},
		{searchCafeID, moviesLibraryID, nil, "movie", "Café Society", "Cafe Society", ""},
		{searchSpeedID, moviesLibraryID, nil, "movie", "Speed", "Speed", ""},
		{searchTrailerID, moviesLibraryID, searchMatrixID, "extra", "Matrix Trailer", "Matrix Trailer", ""},
		{searchDarkID, showsLibraryID, nil, "show", "Dark", "Dark", ""},
		{searchSeasonID, showsLibraryID, searchDarkID, "season", "Season 1", "Season 1", ""},
		{searchEpisodeID, showsLibraryID, searchSeasonID, "episode", "Secrets", "Secrets", ""},
		{searchBjorkID, musicLibraryID, nil, "artist", "Björk", "Bjork", ""},
		{searchAlbumID, musicLibraryID, searchBjorkID, "album", "Homogenic", "Homogenic", ""},
		{searchTrackID, musicLibraryID, searchAlbumID, "track", "Jóga", "Joga", ""},
	}
}

type searchFixture struct {
	index      *sqlite.SearchIndex
	inspection *sql.DB
}

func allSearchLibraries() appsearch.Filter {
	return appsearch.Filter{
		LibraryIDs: []string{moviesLibraryID, showsLibraryID, musicLibraryID}, Restriction: access.Unrestricted(),
	}
}

func newSearchFixture(t *testing.T) searchFixture {
	t.Helper()

	db, path := openMigratedTestDB(t)
	fixture := searchFixture{index: sqlite.NewSearchIndex(db), inspection: openForeignKeyInspectionDB(t, path)}
	seedSearchFixture(t, fixture.inspection)

	return fixture
}

func seedSearchFixture(t *testing.T, inspection *sql.DB) {
	t.Helper()

	execute(t, inspection, `INSERT INTO libraries (id, name, kind, created_at, updated_at)
		VALUES (?, 'Movies', 'movies', 0, 0), (?, 'Shows', 'shows', 0, 0), (?, 'Music', 'music', 0, 0)`,
		moviesLibraryID, showsLibraryID, musicLibraryID)

	for _, item := range searchFixtureItems() {
		extraType := any(nil)
		if item[3] == "extra" {
			extraType = "trailer"
		}

		execute(t, inspection, `INSERT INTO items (id, library_id, parent_id, kind, match_key, title, sort_title,
			original_title, extra_type, created_at, updated_at) VALUES (?1, ?2, ?3, ?4, ?1, ?5, ?6, ?7, ?8, 0, 0)`,
			append(slices.Clone(item), extraType)...)
	}

	execute(t, inspection, `INSERT INTO people (id, name, sort_name)
		VALUES ('p1', 'Keanu Reeves', 'Reeves, Keanu'), ('p2', 'Matrix Fan', 'Fan, Matrix')`)
	execute(t, inspection, `INSERT INTO item_people (item_id, person_id, role, position)
		VALUES (?1, 'p1', 'actor', 0), (?1, 'p1', 'producer', 1), (?2, 'p1', 'actor', 0), (?2, 'p2', 'actor', 1)`,
		searchMatrixID, searchSpeedID)
	execute(t, inspection, `INSERT INTO tags (id, kind, name)
		VALUES ('g1', 'genre', 'Thriller'), ('g2', 'studio', 'Warner Bros')`)
	execute(t, inspection, `INSERT INTO item_tags (item_id, tag_id, position) VALUES (?1, 'g1', 0), (?1, 'g2', 1)`, searchSpeedID)
}

func searchIDs(t *testing.T, index *sqlite.SearchIndex, query string, filter appsearch.Filter) []string {
	t.Helper()

	hits, err := index.Search(t.Context(), query, filter, appsearch.MaxLimit)
	if err != nil {
		t.Fatalf("Search(%q) error = %v", query, err)
	}

	ids := make([]string, 0, len(hits))
	for _, hit := range hits {
		ids = append(ids, hit.ItemID)
	}

	return ids
}

func TestSearchMatchesPrefixesCaseAndAccentInsensitively(t *testing.T) {
	t.Parallel()

	fixture := newSearchFixture(t)

	tests := []struct {
		name  string
		query string
		want  []string
	}{
		{name: "prefix of a title word", query: "matr", want: []string{searchMatrixID, searchSpeedID}},
		{name: "upper case", query: "SECRETS", want: []string{searchEpisodeID}},
		{name: "accent folded in the query", query: "cafe", want: []string{searchCafeID}},
		{name: "accent in the query", query: "CAFÉ", want: []string{searchCafeID}},
		{name: "accented artist", query: "bjork", want: []string{searchBjorkID}},
		{name: "accented track", query: "jog", want: []string{searchTrackID}},
		{name: "every word must match", query: "the mat", want: []string{searchMatrixID}},
		{name: "words in any order", query: "society caf", want: []string{searchCafeID}},
		{name: "original title", query: "matrice", want: []string{searchMatrixID}},
		{name: "person", query: "keanu", want: []string{searchMatrixID, searchSpeedID}},
		{name: "genre", query: "thrill", want: []string{searchSpeedID}},
		{name: "studio is not searched", query: "warner", want: []string{}},
		{name: "season is not indexed", query: "season", want: []string{}},
		{name: "extra is not indexed", query: "trailer", want: []string{}},
		{name: "show and album", query: "dark", want: []string{searchDarkID}},
		{name: "punctuation splits words", query: "matrix!", want: []string{searchMatrixID, searchSpeedID}},
		{name: "no match", query: "zzz", want: []string{}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			got := searchIDs(t, fixture.index, test.query, allSearchLibraries())
			if !slices.Equal(got, test.want) {
				t.Errorf("Search(%q) = %v, want %v", test.query, got, test.want)
			}
		})
	}
}

func TestSearchRanksTitleMatchesAboveOtherFields(t *testing.T) {
	t.Parallel()

	fixture := newSearchFixture(t)

	// The Matrix matches in its title, Speed only through the person "Matrix Fan".
	got := searchIDs(t, fixture.index, "matrix", allSearchLibraries())
	if want := []string{searchMatrixID, searchSpeedID}; !slices.Equal(got, want) {
		t.Errorf("Search(matrix) = %v, want %v", got, want)
	}
}

func TestSearchReturnsKindAndLibrary(t *testing.T) {
	t.Parallel()

	fixture := newSearchFixture(t)

	hits, err := fixture.index.Search(t.Context(), "joga", allSearchLibraries(), 1)
	if err != nil {
		t.Fatalf("Search error = %v", err)
	}

	want := []appsearch.Hit{{ItemID: searchTrackID, LibraryID: musicLibraryID, Kind: library.ItemKindTrack}}
	if !slices.Equal(hits, want) {
		t.Errorf("Search(joga) = %+v, want %+v", hits, want)
	}
}

func TestSearchAppliesTheFilterAndLimit(t *testing.T) {
	t.Parallel()

	fixture := newSearchFixture(t)

	tests := []struct {
		name   string
		query  string
		filter appsearch.Filter
		limit  int
		want   []string
	}{
		{name: "one library", query: "keanu", filter: appsearch.Filter{LibraryIDs: []string{showsLibraryID}, Restriction: access.Unrestricted()}, limit: 10, want: []string{}},
		{name: "no library finds nothing", query: "keanu", filter: appsearch.Filter{}, limit: 10, want: []string{}},
		{
			name: "kinds", query: "d", limit: 10, want: []string{searchDarkID},
			filter: appsearch.Filter{LibraryIDs: allSearchLibraries().LibraryIDs, Kinds: []library.ItemKind{library.ItemKindShow}, Restriction: access.Unrestricted()},
		},
		{
			name: "several kinds", query: "s", limit: 10, want: []string{searchEpisodeID, searchCafeID, searchSpeedID},
			filter: appsearch.Filter{LibraryIDs: allSearchLibraries().LibraryIDs, Kinds: []library.ItemKind{library.ItemKindEpisode, library.ItemKindMovie}, Restriction: access.Unrestricted()},
		},
		{name: "limit", query: "keanu", filter: allSearchLibraries(), limit: 1, want: []string{searchMatrixID}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			hits, err := fixture.index.Search(t.Context(), test.query, test.filter, test.limit)
			if err != nil {
				t.Fatalf("Search error = %v", err)
			}

			got := make([]string, 0, len(hits))
			for _, hit := range hits {
				got = append(got, hit.ItemID)
			}

			slices.Sort(got)
			slices.Sort(test.want)

			if !slices.Equal(got, test.want) {
				t.Errorf("Search(%q) = %v, want %v", test.query, got, test.want)
			}
		})
	}
}

func TestSearchRejectsAnInvalidLimitOrKind(t *testing.T) {
	t.Parallel()

	fixture := newSearchFixture(t)

	tests := []struct {
		name   string
		filter appsearch.Filter
		limit  int
	}{
		{name: "zero limit", filter: allSearchLibraries(), limit: 0},
		{name: "limit above the maximum", filter: allSearchLibraries(), limit: appsearch.MaxLimit + 1},
		{name: "unknown kind", filter: appsearch.Filter{LibraryIDs: allSearchLibraries().LibraryIDs, Kinds: []library.ItemKind{"hologram"}, Restriction: access.Unrestricted()}, limit: 1},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			_, err := fixture.index.Search(t.Context(), "matrix", test.filter, test.limit)
			if !errors.Is(err, domain.ErrInvalid) {
				t.Errorf("Search error = %v, want domain.ErrInvalid", err)
			}
		})
	}
}

func TestSearchAcceptsHostileInput(t *testing.T) {
	t.Parallel()

	fixture := newSearchFixture(t)

	tests := []struct {
		name  string
		query string
		want  []string
	}{
		{name: "empty", query: "", want: []string{}},
		{name: "blanks", query: " \t\n ", want: []string{}},
		{name: "quote", query: `"`, want: []string{}},
		{name: "unterminated quote", query: `"matrix`, want: []string{searchMatrixID, searchSpeedID}},
		{name: "doubled quotes", query: `""matrix""`, want: []string{searchMatrixID, searchSpeedID}},
		{name: "star", query: "*", want: []string{}},
		{name: "leading star", query: "*matrix", want: []string{searchMatrixID, searchSpeedID}},
		{name: "NEAR", query: "NEAR", want: []string{}},
		{name: "NEAR group", query: "NEAR(matrix keanu, 2)", want: []string{}},
		{name: "operators", query: "matrix OR AND NOT speed", want: []string{}},
		{name: "column filter", query: "title:matrix", want: []string{}},
		{name: "column set", query: "{title people}: keanu", want: []string{}},
		{name: "caret and minus", query: "^matrix -keanu", want: []string{searchMatrixID, searchSpeedID}},
		{name: "parentheses", query: "(((", want: []string{}},
		{name: "plus", query: "the + matrix", want: []string{searchMatrixID}},
		{name: "nul byte", query: "matrix\x00keanu", want: []string{searchMatrixID, searchSpeedID}},
		{name: "invalid utf-8", query: "matrix\xff\xfe", want: []string{searchMatrixID, searchSpeedID}},
		{name: "10 kB of one word", query: strings.Repeat("a", 10*1024), want: []string{}},
		{name: "10 kB of words", query: strings.Repeat("matrix ", 10*1024/7), want: []string{searchMatrixID, searchSpeedID}},
		{name: "10 kB of syntax", query: strings.Repeat(`"*NEAR(`, 10*1024/7), want: []string{}},
		// Adlam (Unicode 9) is a word for Go and the tokenizer ("title in a newer script" in
		// TestSearchFollowsCatalogChanges); no fixture item has it, and every word must match.
		{name: "letter of a newer script", query: "\U0001E900 matrix", want: []string{}},
		{name: "newer script only", query: "\U0001E900\U0001E901", want: []string{}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			got := searchIDs(t, fixture.index, test.query, allSearchLibraries())
			if !slices.Equal(got, test.want) {
				t.Errorf("Search(%.40q) = %v, want %v", test.query, got, test.want)
			}
		})
	}
}

func TestSearchFollowsCatalogChanges(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		statements []string
		query      string
		want       []string
	}{
		{name: "renamed item loses the old title", statements: []string{`UPDATE items SET title = 'Heat', sort_title = 'Heat' WHERE id = '` + searchSpeedID + `'`}, query: "speed", want: []string{}},
		{name: "renamed item has the new title", statements: []string{`UPDATE items SET title = 'Heat', sort_title = 'Heat' WHERE id = '` + searchSpeedID + `'`}, query: "heat", want: []string{searchSpeedID}},
		{name: "renamed item keeps its people", statements: []string{`UPDATE items SET title = 'Heat' WHERE id = '` + searchSpeedID + `'`}, query: "heat keanu thriller", want: []string{searchSpeedID}},
		{name: "deleted item", statements: []string{`DELETE FROM items WHERE id = '` + searchMatrixID + `'`}, query: "matrice", want: []string{}},
		{name: "deleted library", statements: []string{`DELETE FROM libraries WHERE id = '` + musicLibraryID + `'`}, query: "bjork", want: []string{}},
		{name: "new item", statements: []string{`INSERT INTO items (id, library_id, kind, match_key, title, sort_title, created_at, updated_at) VALUES ('n1', '` + moviesLibraryID + `', 'movie', 'n1', 'Amélie', 'Amelie', 0, 0)`}, query: "amelie", want: []string{"n1"}},
		{name: "title in a newer script", statements: []string{`INSERT INTO items (id, library_id, kind, match_key, title, sort_title, created_at, updated_at) VALUES ('n1', '` + moviesLibraryID + "', 'movie', 'n1', '\U0001E900\U0001E901 Film', 'Film', 0, 0)"}, query: "\U0001E900 film", want: []string{"n1"}},
		{name: "added person", statements: []string{`INSERT INTO item_people (item_id, person_id, role, position) VALUES ('` + searchCafeID + `', 'p2', 'actor', 0)`}, query: "fan", want: []string{searchCafeID, searchSpeedID}},
		{name: "removed person", statements: []string{`DELETE FROM item_people WHERE person_id = 'p1' AND item_id = '` + searchSpeedID + `'`}, query: "keanu", want: []string{searchMatrixID}},
		{name: "deleted person", statements: []string{`DELETE FROM people WHERE id = 'p1'`}, query: "keanu", want: []string{}},
		{name: "renamed person", statements: []string{`UPDATE people SET name = 'Carrie-Anne Moss' WHERE id = 'p1'`}, query: "carrie", want: []string{searchMatrixID, searchSpeedID}},
		{name: "added genre", statements: []string{`INSERT INTO item_tags (item_id, tag_id, position) VALUES ('` + searchDarkID + `', 'g1', 0)`}, query: "thriller", want: []string{searchDarkID, searchSpeedID}},
		{name: "removed genre", statements: []string{`DELETE FROM item_tags WHERE tag_id = 'g1'`}, query: "thriller", want: []string{}},
		{name: "renamed genre", statements: []string{`UPDATE tags SET name = 'Krimi' WHERE id = 'g1'`}, query: "krimi", want: []string{searchSpeedID}},
		{name: "studio turned genre", statements: []string{`UPDATE tags SET kind = 'genre' WHERE id = 'g2'`}, query: "warner", want: []string{searchSpeedID}},
		{name: "deleted genre", statements: []string{`DELETE FROM tags WHERE id = 'g1'`}, query: "thriller", want: []string{}},
		{name: "unchanged title", statements: []string{`UPDATE items SET title = title, year = 1994 WHERE id = '` + searchSpeedID + `'`}, query: "speed thriller keanu", want: []string{searchSpeedID}},
		{name: "moved person", statements: []string{`UPDATE item_people SET item_id = '` + searchDarkID + `' WHERE person_id = 'p2'`}, query: "fan", want: []string{searchDarkID}},
		{name: "moved genre", statements: []string{`UPDATE item_tags SET item_id = '` + searchDarkID + `' WHERE tag_id = 'g1'`}, query: "thriller", want: []string{searchDarkID}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			fixture := newSearchFixture(t)
			for _, statement := range test.statements {
				execute(t, fixture.inspection, statement)
			}

			got := searchIDs(t, fixture.index, test.query, allSearchLibraries())
			slices.Sort(got)
			slices.Sort(test.want)

			if !slices.Equal(got, test.want) {
				t.Errorf("Search(%q) = %v, want %v", test.query, got, test.want)
			}
		})
	}
}

func TestSearchIndexIsConsistentAfterChanges(t *testing.T) {
	t.Parallel()

	fixture := newSearchFixture(t)
	execute(t, fixture.inspection, `UPDATE items SET title = 'Heat' WHERE id = ?`, searchSpeedID)
	execute(t, fixture.inspection, `DELETE FROM items WHERE id = ?`, searchMatrixID)
	execute(t, fixture.inspection, `DELETE FROM item_people WHERE item_id = ?`, searchSpeedID)

	// FTS5's integrity check compares the index with the stored rows.
	execute(t, fixture.inspection, `INSERT INTO item_search (item_search) VALUES ('integrity-check')`)

	var documents, rows int

	err := fixture.inspection.QueryRowContext(t.Context(),
		`SELECT (SELECT COUNT(*) FROM search_docs), (SELECT COUNT(*) FROM item_search)`).Scan(&documents, &rows)
	if err != nil {
		t.Fatalf("count search rows: %v", err)
	}

	// Ten fixture items, minus the season, the extra and the deleted movie.
	if documents != 7 || rows != 7 {
		t.Errorf("search_docs = %d, item_search = %d, want 7 each", documents, rows)
	}
}

// Deleting a library cascades to its items and from there to their credits and genres; the
// delete triggers of item_people and item_tags skip those rows (their item is gone) and the
// index must still be exact.
func TestSearchIndexIsConsistentAfterALibraryDeletion(t *testing.T) {
	t.Parallel()

	fixture := newSearchFixture(t)
	execute(t, fixture.inspection, `DELETE FROM libraries WHERE id = ?`, moviesLibraryID)
	execute(t, fixture.inspection, `INSERT INTO item_search (item_search) VALUES ('integrity-check')`)

	var documents, rows int

	err := fixture.inspection.QueryRowContext(t.Context(),
		`SELECT (SELECT COUNT(*) FROM search_docs), (SELECT COUNT(*) FROM item_search)`).Scan(&documents, &rows)
	if err != nil {
		t.Fatalf("count search rows: %v", err)
	}

	// Left: the show and its episode, the artist, album and track.
	if documents != 5 || rows != 5 {
		t.Errorf("search_docs = %d, item_search = %d, want 5 each", documents, rows)
	}

	for _, query := range []string{"keanu", "thriller", "matrix", "speed"} {
		if got := searchIDs(t, fixture.index, query, allSearchLibraries()); len(got) != 0 {
			t.Errorf("Search(%q) after deleting the library = %v, want none", query, got)
		}
	}
}

func TestSearchMigrationBackFillsAnExistingCatalog(t *testing.T) {
	t.Parallel()

	db, path := openTestDB(t, nil)
	inspection := openForeignKeyInspectionDB(t, path)

	provider, err := goose.NewProvider(goose.DialectSQLite3, inspection, os.DirFS("migrations"),
		goose.WithDisableGlobalRegistry(true))
	if err != nil {
		t.Fatalf("NewProvider error = %v", err)
	}

	if _, err := provider.UpTo(t.Context(), lastSchemaVersionWithoutSearch); err != nil {
		t.Fatalf("UpTo(%d) error = %v", lastSchemaVersionWithoutSearch, err)
	}

	seedSearchFixture(t, inspection)

	if err := db.Migrate(t.Context()); err != nil {
		t.Fatalf("Migrate error = %v", err)
	}

	index := sqlite.NewSearchIndex(db)
	for query, want := range map[string][]string{"keanu": {searchMatrixID, searchSpeedID}, "thriller": {searchSpeedID}, "cafe": {searchCafeID}} {
		got := searchIDs(t, index, query, allSearchLibraries())
		slices.Sort(got)

		if !slices.Equal(got, want) {
			t.Errorf("Search(%q) after back-fill = %v, want %v", query, got, want)
		}
	}
}

func TestSearchReportsAnUndecodableKind(t *testing.T) {
	t.Parallel()

	fixture := newSearchFixture(t)
	execute(t, fixture.inspection, `UPDATE search_docs SET kind = 'hologram' WHERE item_id = ?`, searchDarkID)

	_, err := fixture.index.Search(t.Context(), "dark", allSearchLibraries(), 1)
	assertUndecodableRowError(t, "Search", err)
}

func TestSearchFailsWithACancelledContext(t *testing.T) {
	t.Parallel()

	fixture := newSearchFixture(t)

	_, err := fixture.index.Search(cancelledContext(t), "matrix", allSearchLibraries(), 1)
	if err == nil {
		t.Error("Search with a cancelled context succeeded, want an error")
	}
}
