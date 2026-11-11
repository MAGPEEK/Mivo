package sqlite

import (
	"database/sql"
	"encoding/base64"
	"errors"
	"strings"
	"testing"

	"github.com/magpeek/mivo/internal/adapters/sqlite/sqlcgen"
	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/access"
	"github.com/magpeek/mivo/internal/domain/library"
)

func TestChildCursorRoundTripIsBoundToItemAndDirection(t *testing.T) {
	t.Parallel()

	scope := childCursorScope{parentID: "parent"}
	keys := []childKey{
		{group: 0, number: 7, sortTitle: "title", id: "id"},
		{group: 1, number: 0, sortTitle: "", id: "id"},
		{group: 1, number: 0, sortTitle: "\xff nul \x00 title", id: "id"},
	}

	for _, key := range keys {
		cursor := encodeChildCursor(scope, key)

		got, found, err := decodeChildCursor(cursor, scope)
		if err != nil || !found || got != key {
			t.Errorf("decodeChildCursor(%+v) = %+v, %v, %v, want the key", key, got, found, err)
		}

		for _, other := range []childCursorScope{{parentID: "parent", descending: true}, {parentID: "other"}} {
			if _, _, err := decodeChildCursor(cursor, other); !errors.Is(err, domain.ErrInvalid) {
				t.Errorf("decodeChildCursor(%+v) in %+v error = %v, want domain.ErrInvalid", key, other, err)
			}
		}

		if _, err := decodeCursor(cursor); !errors.Is(err, domain.ErrInvalid) {
			t.Errorf("top-level decodeCursor of a child cursor error = %v, want domain.ErrInvalid", err)
		}
	}

	if _, found, err := decodeChildCursor("", scope); found || err != nil {
		t.Errorf("empty child cursor = %v, %v, want no key", found, err)
	}
}

func TestDecodeChildCursorRejectsMalformedFields(t *testing.T) {
	t.Parallel()

	scope := childCursorScope{parentID: "p"}
	encode := func(fields ...string) string {
		return base64.RawURLEncoding.EncodeToString([]byte(strings.Join(fields, cursorSeparator)))
	}

	for name, cursor := range map[string]string{
		"too few fields":    encode("children", "asc", "p", "0", "1", "id"),
		"wrong tag":         encode("kids", "asc", "p", "0", "1", "id", "t"),
		"group out of set":  encode("children", "asc", "p", "2", "1", "id", "t"),
		"number not number": encode("children", "asc", "p", "0", "x", "id", "t"),
		"empty id":          encode("children", "asc", "p", "0", "1", "", "t"),
		"line feed":         encodeChildCursor(scope, childKey{id: "i"}) + "\n",
	} {
		if _, _, err := decodeChildCursor(cursor, scope); !errors.Is(err, domain.ErrInvalid) {
			t.Errorf("%s: error = %v, want domain.ErrInvalid", name, err)
		}
	}
}

// Go's base64 decoder skips line breaks even in strict mode; the top-level cursor must refuse
// them so one key has exactly one cursor.
func TestDecodeCursorRejectsLineBreaks(t *testing.T) {
	t.Parallel()

	cursor := encodeCursor(itemKey{sortTitle: "t", id: "i"})

	for _, input := range []string{cursor + "\n", cursor + "\r", cursor[:2] + "\r\n" + cursor[2:]} {
		if _, err := decodeCursor(input); !errors.Is(err, domain.ErrInvalid) {
			t.Errorf("decodeCursor(%q) error = %v, want domain.ErrInvalid", input, err)
		}
	}
}

// The top-level listing must be a range search on the partial index in index order, without a
// temporary B-tree; the child listings must search items_children by parent. The correlated
// count and file subqueries may use their own indexes.
func TestListingQueriesUseTheCatalogIndexes(t *testing.T) {
	t.Parallel()

	db, _ := openInternalTestDB(t)
	if err := db.Migrate(t.Context()); err != nil {
		t.Fatalf("Migrate error = %v", err)
	}

	recorder := &queryRecorder{}
	queries := sqlcgen.New(recorder)
	parent := sql.NullString{String: "parent", Valid: true}
	childParams := sqlcgen.ListChildrenAscendingParams{
		ParentID: parent, HasAfter: storedTrue, AfterGroup: 0, AfterNumber: 3, AfterSortTitle: "t", AfterID: "i", RowLimit: 51,
	}

	recordings := map[string]func() error{
		"top level": func() error {
			recorder.query, recorder.args, _ = pageQuery(listingRequest{
				target: listingTarget{libraryID: "library", kind: library.ItemKindMovie, restriction: access.Unrestricted()}, query: library.ListingQuery{Filter: library.ListingFilter{Kind: library.ItemKindMovie}},
				after: listingKey{sortTitle: "title", id: "item"}, hasAfter: true, rowLimit: 101,
			})

			return errQueryRecorded
		},
		"children ascending": func() error {
			_, err := queries.ListChildrenAscending(t.Context(), childParams)

			return err
		},
		"children descending": func() error {
			_, err := queries.ListChildrenDescending(t.Context(), sqlcgen.ListChildrenDescendingParams(childParams))

			return err
		},
	}

	wantIndex := map[string]string{
		"top level":           "INDEX items_top_level (library_id=? AND sort_title>",
		"children ascending":  "INDEX items_children (parent_id=?",
		"children descending": "INDEX items_children (parent_id=?",
	}

	for name, record := range recordings {
		if err := record(); !errors.Is(err, errQueryRecorded) {
			t.Fatalf("%s: error = %v, want the recorded query", name, err)
		}

		assertListingPlan(t, name, queryPlan(t, db.reader, recorder.query, recorder.args), wantIndex[name])
	}
}

// assertListingPlan checks that the plan searches the named index of items, never scans items
// outside a subquery and sorts the top-level page only inside a subquery.
func assertListingPlan(t *testing.T, name string, steps map[int]planStep, wantIndex string) {
	t.Helper()

	usesIndex := false

	for id, step := range steps {
		usesIndex = usesIndex || (strings.HasPrefix(step.detail, "SEARCH items USING ") &&
			strings.Contains(step.detail, wantIndex))

		sortsPage := strings.Contains(step.detail, "USE TEMP B-TREE FOR ORDER BY") && !insideSubquery(steps, id)
		if sortsPage && name == "top level" {
			t.Errorf("%s: plan step %q sorts the page; plan: %+v", name, step.detail, steps)
		}

		if strings.HasPrefix(step.detail, "SCAN items") && !insideSubquery(steps, id) {
			t.Errorf("%s: plan step %q scans every item; plan: %+v", name, step.detail, steps)
		}
	}

	if !usesIndex {
		t.Errorf("%s: plan does not search %q; plan: %+v", name, wantIndex, steps)
	}
}
