package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"
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
