package sqlite_test

import (
	"database/sql"
	"runtime"
	"strings"
	"testing"
)

// carriedKey is the path_key Migrate leaves on a carried-over folder: the SQL migration keys
// without case folding and the re-keying after it folds on Windows only.
func carriedKey(key string) string {
	if runtime.GOOS == "windows" {
		return strings.ToLower(key)
	}

	return key
}

// queryStrings runs a query that returns one text column and returns its rows.
func queryStrings(t *testing.T, inspection *sql.DB, query string) []string {
	t.Helper()

	rows, err := inspection.QueryContext(t.Context(), query)
	if err != nil {
		t.Fatalf("%s: %v", query, err)
	}

	defer func() { _ = rows.Close() }()

	var values []string

	for rows.Next() {
		var value string
		if err := rows.Scan(&value); err != nil {
			t.Fatalf("scan: %v", err)
		}

		values = append(values, value)
	}

	if err := rows.Err(); err != nil {
		t.Fatalf("%s: %v", query, err)
	}

	return values
}
