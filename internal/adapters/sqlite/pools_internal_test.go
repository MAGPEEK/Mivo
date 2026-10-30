package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"strings"
	"testing"
)

// openInternalTestDB opens a database in t.TempDir() for tests that need the unexported pools.
func openInternalTestDB(t *testing.T) (*DB, string) {
	t.Helper()

	path := filepath.Join(t.TempDir(), "server.db")

	db, err := Open(t.Context(), Options{Path: path})
	if err != nil {
		t.Fatalf("Open(%q) error = %v", path, err)
	}

	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("Close error = %v", err)
		}
	})

	return db, path
}

// checkOutConnection takes a connection from pool and returns it when the test ends.
func checkOutConnection(t *testing.T, pool *sql.DB) *sql.Conn {
	t.Helper()

	connection, err := pool.Conn(t.Context())
	if err != nil {
		t.Fatalf("check out connection: %v", err)
	}

	t.Cleanup(func() {
		if err := connection.Close(); err != nil {
			t.Errorf("return connection: %v", err)
		}
	})

	return connection
}

// connectionPragmas are the per-connection settings of ADR 0004 plus query_only.
type connectionPragmas struct {
	journalMode string
	busyTimeout int
	foreignKeys int
	synchronous int
	queryOnly   int
}

// synchronousNormalValue is what PRAGMA synchronous reports for NORMAL.
const synchronousNormalValue = 1

func readPragmas(ctx context.Context, t *testing.T, connection *sql.Conn) connectionPragmas {
	t.Helper()

	var pragmas connectionPragmas

	queries := []struct {
		pragma string
		target any
	}{
		{"journal_mode", &pragmas.journalMode},
		{"busy_timeout", &pragmas.busyTimeout},
		{"foreign_keys", &pragmas.foreignKeys},
		{"synchronous", &pragmas.synchronous},
		{"query_only", &pragmas.queryOnly},
	}

	for _, query := range queries {
		err := connection.QueryRowContext(ctx, "PRAGMA "+query.pragma).Scan(query.target)
		if err != nil {
			t.Fatalf("PRAGMA %s: %v", query.pragma, err)
		}
	}

	return pragmas
}

// TestOpenAppliesPragmasOnEveryConnectionOfBothPools takes two reader connections at once, so
// the second one is newly opened by database/sql and must carry the settings as well.
func TestOpenAppliesPragmasOnEveryConnectionOfBothPools(t *testing.T) {
	t.Parallel()

	db, _ := openInternalTestDB(t)
	ctx := t.Context()

	tests := []struct {
		name          string
		pool          *sql.DB
		connections   int
		wantQueryOnly int
	}{
		{name: "writer", pool: db.writer, connections: 1, wantQueryOnly: 0},
		{name: "reader", pool: db.reader, connections: 2, wantQueryOnly: 1},
	}

	for _, test := range tests {
		want := connectionPragmas{
			journalMode: "wal", busyTimeout: 5000, foreignKeys: 1,
			synchronous: synchronousNormalValue, queryOnly: test.wantQueryOnly,
		}

		for index := range test.connections {
			// The connections stay checked out until the test ends, so each Conn call
			// gets a different one.
			connection := checkOutConnection(t, test.pool)

			if got := readPragmas(ctx, t, connection); got != want {
				t.Errorf("%s connection %d pragmas = %+v, want %+v", test.name, index, got, want)
			}
		}
	}
}

func TestOpenLimitsWriterToOneConnection(t *testing.T) {
	t.Parallel()

	db, _ := openInternalTestDB(t)

	if got := db.writer.Stats().MaxOpenConnections; got != writerMaxOpenConnections {
		t.Errorf("writer MaxOpenConnections = %d, want %d", got, writerMaxOpenConnections)
	}

	if got := db.reader.Stats().MaxOpenConnections; got < 1 {
		t.Errorf("reader MaxOpenConnections = %d, want at least 1", got)
	}
}

// With _txlock=immediate, BEGIN on the writer takes the write lock at once. A second writer
// that does not wait (busy_timeout 0) must then be refused immediately.
func TestWriterTransactionsTakeTheWriteLockAtBegin(t *testing.T) {
	t.Parallel()

	db, path := openInternalTestDB(t)
	ctx := t.Context()

	transaction, err := db.writer.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("BeginTx error = %v", err)
	}

	// database/sql rolls the transaction back itself when t.Context() is cancelled, which
	// happens before cleanup runs.
	t.Cleanup(func() {
		if err := transaction.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
			t.Errorf("Rollback error = %v", err)
		}
	})

	other, err := sql.Open(driverName, path+"?_busy_timeout=0")
	if err != nil {
		t.Fatalf("open second connection: %v", err)
	}

	t.Cleanup(func() {
		if err := other.Close(); err != nil {
			t.Errorf("close second connection: %v", err)
		}
	})

	_, err = other.ExecContext(ctx, "BEGIN IMMEDIATE")
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "locked") {
		t.Errorf("BEGIN IMMEDIATE on second connection error = %v, want database is locked", err)
	}
}

func TestReaderPoolRejectsWrites(t *testing.T) {
	t.Parallel()

	db, _ := openInternalTestDB(t)

	_, err := db.reader.ExecContext(t.Context(), "CREATE TABLE reader_write_probe (value INTEGER)")
	if err == nil {
		t.Fatal("write through the reader pool succeeded, want a query_only error")
	}
}
