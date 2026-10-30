package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"

	"github.com/magpeek/mivo/internal/adapters/sqlite/sqlcgen"
)

// errQueryRecorded stops a generated query after queryRecorder captured it.
var errQueryRecorded = errors.New("query recorded, not run")

// queryRecorder is a sqlcgen.DBTX that records the SQL and arguments of a query instead of
// running it, so a test can inspect exactly what the generated code sends.
type queryRecorder struct {
	query string
	args  []any
}

func (*queryRecorder) ExecContext(context.Context, string, ...any) (sql.Result, error) {
	return nil, errQueryRecorded
}

func (*queryRecorder) PrepareContext(context.Context, string) (*sql.Stmt, error) {
	return nil, errQueryRecorded
}

func (recorder *queryRecorder) QueryContext(_ context.Context, query string, args ...any) (*sql.Rows, error) {
	recorder.query, recorder.args = query, args

	return nil, errQueryRecorded
}

// QueryRowContext is not used by the :many queries under test.
func (*queryRecorder) QueryRowContext(context.Context, string, ...any) *sql.Row { return nil }

// planStep is one row of EXPLAIN QUERY PLAN.
type planStep struct {
	parent int
	detail string
}

// The keyset page must be a range search on items_library_sort_title in index order: a
// sort of the library's items in a temporary B-tree would read the whole library for every
// page. The only sort allowed is the one inside the correlated subquery that picks an
// item's primary file among its (in milestone 2: one) files.
func TestListItemPageSearchesTheSortTitleIndexWithoutSortingItems(t *testing.T) {
	t.Parallel()

	db, _ := openInternalTestDB(t)
	if err := db.Migrate(t.Context()); err != nil {
		t.Fatalf("Migrate error = %v", err)
	}

	recorder := &queryRecorder{}

	_, err := sqlcgen.New(recorder).ListItemPage(t.Context(), sqlcgen.ListItemPageParams{
		LibraryID: "library", AfterSortTitle: "title", AfterID: "item", RowLimit: 101,
	})
	if !errors.Is(err, errQueryRecorded) {
		t.Fatalf("ListItemPage error = %v, want the recorded query", err)
	}

	steps := queryPlan(t, db.reader, recorder.query, recorder.args)

	usesIndex := false

	for id, step := range steps {
		usesIndex = usesIndex || (strings.HasPrefix(step.detail, "SEARCH items USING ") &&
			strings.Contains(step.detail, "INDEX items_library_sort_title (library_id=? AND sort_title>"))

		if strings.Contains(step.detail, "USE TEMP B-TREE FOR ORDER BY") && !insideSubquery(steps, id) {
			t.Errorf("plan step %q sorts outside the primary-file subquery; plan: %+v", step.detail, steps)
		}
	}

	if !usesIndex {
		t.Errorf("plan does not search items_library_sort_title; plan: %+v", steps)
	}
}

// insideSubquery reports whether a step of the plan tree lies below a subquery step.
func insideSubquery(steps map[int]planStep, id int) bool {
	for parent := steps[id].parent; parent != 0; parent = steps[parent].parent {
		if strings.Contains(steps[parent].detail, "SUBQUERY") {
			return true
		}
	}

	return false
}

func queryPlan(t *testing.T, pool *sql.DB, query string, args []any) map[int]planStep {
	t.Helper()

	rows, err := pool.QueryContext(t.Context(), "EXPLAIN QUERY PLAN "+query, args...)
	if err != nil {
		t.Fatalf("EXPLAIN QUERY PLAN: %v", err)
	}

	defer func() { _ = rows.Close() }()

	steps := map[int]planStep{}

	for rows.Next() {
		var (
			id, unused int
			step       planStep
		)

		if err := rows.Scan(&id, &step.parent, &unused, &step.detail); err != nil {
			t.Fatalf("scan plan step: %v", err)
		}

		steps[id] = step
	}

	if err := rows.Err(); err != nil {
		t.Fatalf("EXPLAIN QUERY PLAN: %v", err)
	}

	return steps
}
