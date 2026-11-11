package sqlite

import (
	"strings"
	"testing"

	applibrary "github.com/magpeek/mivo/internal/app/library"
	"github.com/magpeek/mivo/internal/domain/access"
	"github.com/magpeek/mivo/internal/domain/library"
)

func restrictedTo(t *testing.T, libraries access.LibraryAccess, maxAge access.AgeBucket, hideUnrated bool) access.Restriction {
	t.Helper()

	limit, err := access.MaxAgeLimit(maxAge, hideUnrated)
	if err != nil {
		t.Fatalf("MaxAgeLimit(%d) error = %v", maxAge, err)
	}

	return access.Viewer{Libraries: libraries, Limit: limit}.Restriction()
}

func TestListingRestrictionConditionSQL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		restriction access.Restriction
		want        string
		wantArgs    int
	}{
		{name: "unrestricted adds no text", restriction: access.Unrestricted()},
		{name: "zero value never matches", want: neverMatches},
		{name: "no library never matches", restriction: restrictedTo(t, access.OnlyLibraries(), access.Age6, true), want: neverMatches + " AND i.effective_age_rating <= :p1", wantArgs: 1},
		{
			name: "libraries and hidden unrated", restriction: restrictedTo(t, access.OnlyLibraries("b", "a"), access.Age6, true),
			want: "i.library_id IN (:p1, :p2) AND i.effective_age_rating <= :p3", wantArgs: 3,
		},
		{
			name: "unrated shown", restriction: restrictedTo(t, access.AllLibraries(), access.Age12, false),
			want: "(i.effective_age_rating IS NULL OR i.effective_age_rating <= :p1)", wantArgs: 1,
		},
		{
			name:        "only unrated hidden",
			restriction: access.Viewer{Libraries: access.AllLibraries(), Limit: access.NoLimit().HidingUnrated(true)}.Restriction(),
			want:        "i.effective_age_rating IS NOT NULL",
		},
	}
	for _, test := range tests {
		builder := &sqlBuilder{}
		if got := restrictionConditionSQL(builder, test.restriction, "i"); got != test.want || len(builder.args) != test.wantArgs {
			t.Errorf("%s: condition = %q with %d args, want %q with %d", test.name, got, len(builder.args), test.want, test.wantArgs)
		}
	}
}

// restrictedStatements returns every statement the listing builds for the restriction, by
// name.
func restrictedStatements(restriction access.Restriction) map[string]queryText {
	target := listingTarget{libraryID: "library", kind: library.ItemKindShow, restriction: restriction}
	filter := library.ListingFilter{Kind: library.ItemKindShow}
	files := library.ListingFilter{Kind: library.ItemKindShow, Containers: []string{"mkv"}}
	statements := map[string]queryText{}

	add := func(name, text string, args []any) { statements[name] = queryText{text: text, args: args} }

	text, args, _ := pageQuery(listingRequest{target: target, query: library.ListingQuery{Filter: filter}, rowLimit: 51})
	add("first page", text, args)

	text, args, _ = pageQuery(listingRequest{
		target: target, query: library.ListingQuery{Filter: filter},
		after: listingKey{sortTitle: "title", id: "item"}, hasAfter: true, rowLimit: 51,
	})
	add("next page", text, args)

	text, args, _ = pageQuery(listingRequest{target: target, query: library.ListingQuery{Filter: files}, rowLimit: 51})
	add("file filter page", text, args)

	text, args = countQuery(target, filter)
	add("count", text, args)

	text, args = firstCharactersQuery(target, filter, false)
	add("letters", text, args)

	text, args = facetsQuery(target, library.FacetFields(), files, applibrary.MaxFacetValues+1)
	add("facets", text, args)

	scope := folderScope{rootID: "root", directory: "Show/Season 1", restriction: restriction}

	text, args = folderItemsQuery(scope)
	add("folder items", text, args)

	text, args = folderDirectoriesQuery(scope)
	add("folder directories", text, args)

	return statements
}

// A restriction must not turn a listing into a table scan: with a content limit and a library
// set, every statement still finds its items and files through an index, and the title page
// still reads items_top_level in order without sorting.
func TestListingRestrictedQueriesUseTheCatalogIndexes(t *testing.T) {
	t.Parallel()

	db, _ := openInternalTestDB(t)
	if err := db.Migrate(t.Context()); err != nil {
		t.Fatalf("Migrate error = %v", err)
	}

	restriction := restrictedTo(t, access.OnlyLibraries("library", "other"), access.Age6, true)

	for name, statement := range restrictedStatements(restriction) {
		if !strings.Contains(statement.text, "effective_age_rating <= ") {
			t.Errorf("%s: statement does not filter by the age bucket: %s", name, statement.text)
		}

		steps := queryPlan(t, db.reader, statement.text, statement.args)
		assertIndexedPlan(t, name, steps)

		if strings.HasSuffix(name, "page") && !strings.Contains(name, "file") {
			assertListingPlan(t, "top level", steps, "INDEX items_top_level (library_id=?")
		}
	}
}

// assertIndexedPlan checks that the plan searches items through an index and scans neither
// items nor media_files as a whole.
func assertIndexedPlan(t *testing.T, name string, steps map[int]planStep) {
	t.Helper()

	searchesItems := false

	for _, step := range steps {
		searchesItems = searchesItems || strings.HasPrefix(step.detail, "SEARCH items USING ")

		if strings.HasPrefix(step.detail, "SCAN items") || strings.HasPrefix(step.detail, "SCAN media_files") {
			t.Errorf("%s: plan step %q scans a whole table; plan: %+v", name, step.detail, steps)
		}
	}

	if !searchesItems {
		t.Errorf("%s: plan does not search items through an index; plan: %+v", name, steps)
	}
}

// Without a restriction the statements are the ones of before task M7-12, so results and
// query plans are unchanged; the zero restriction closes every statement.
func TestListingUnrestrictedStatementsCarryNoRestriction(t *testing.T) {
	t.Parallel()

	for name, statement := range restrictedStatements(access.Unrestricted()) {
		if strings.Contains(statement.text, "effective_age_rating") || strings.Contains(statement.text, neverMatches) ||
			strings.Contains(statement.text, "items.library_id IN") {
			t.Errorf("%s: unrestricted statement carries a restriction: %s", name, statement.text)
		}
	}

	for name, statement := range restrictedStatements(access.Restriction{}) {
		if !strings.Contains(statement.text, neverMatches) {
			t.Errorf("%s: statement of the zero restriction can match: %s", name, statement.text)
		}
	}
}
