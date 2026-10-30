package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"runtime"
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

// The reader pool may open one connection per usable CPU (GOMAXPROCS, which respects
// container CPU limits) and keeps all of them idle instead of closing every connection above
// database/sql's default of two idle ones after a burst of parallel queries.
func TestReaderPoolKeepsEveryOpenConnectionIdle(t *testing.T) {
	t.Parallel()

	db, _ := openInternalTestDB(t)
	maxOpen := runtime.GOMAXPROCS(0)

	if got := db.reader.Stats().MaxOpenConnections; got != maxOpen {
		t.Fatalf("reader MaxOpenConnections = %d, want GOMAXPROCS %d", got, maxOpen)
	}

	connections := make([]*sql.Conn, 0, maxOpen)
	for range maxOpen {
		connection, err := db.reader.Conn(t.Context())
		if err != nil {
			t.Fatalf("check out connection: %v", err)
		}

		connections = append(connections, connection)
	}

	for _, connection := range connections {
		if err := connection.Close(); err != nil {
			t.Fatalf("return connection: %v", err)
		}
	}

	stats := db.reader.Stats()
	if stats.Idle != maxOpen || stats.MaxIdleClosed != 0 {
		t.Errorf("reader idle = %d, closed for exceeding the idle limit = %d, want %d and 0",
			stats.Idle, stats.MaxIdleClosed, maxOpen)
	}
}

// foreign_keys=ON is checked per connection by TestOpenAppliesPragmasOnEveryConnectionOfBothPools;
// this checks that the catalog schema relies on it: the writer refuses an orphan item.
func TestWriterEnforcesForeignKeysOfTheCatalog(t *testing.T) {
	t.Parallel()

	db, _ := openInternalTestDB(t)
	if err := db.Migrate(t.Context()); err != nil {
		t.Fatalf("Migrate error = %v", err)
	}

	_, err := db.writer.ExecContext(t.Context(),
		`INSERT INTO items (id, library_id, kind, title, sort_title, created_at, updated_at)
		VALUES ('item-1', 'missing-library', 'movie', 'A', 'a', 0, 0)`)
	if err == nil || !strings.Contains(strings.ToUpper(err.Error()), "FOREIGN KEY") {
		t.Errorf("insert of an orphan item error = %v, want a FOREIGN KEY constraint error", err)
	}
}

// A ping must start a read transaction on the database file. "SELECT 1", which the driver's
// own Ping runs, is answered without touching the file, so it reports a database whose file
// became unreadable as healthy. The VDBE program of a statement that reads the file contains
// a Transaction opcode; the control case shows the check tells the two apart.
func TestPingQueryStartsAReadTransaction(t *testing.T) {
	t.Parallel()

	db, _ := openInternalTestDB(t)

	tests := []struct {
		name  string
		query string
		want  bool
	}{
		{name: "ping query", query: pingQuery, want: true},
		{name: "driver ping (control)", query: "SELECT 1", want: false},
	}

	for _, test := range tests {
		if got := programHasOpcode(t, db.reader, test.query, "Transaction"); got != test.want {
			t.Errorf("%s: %q starts a transaction = %t, want %t", test.name, test.query, got, test.want)
		}
	}
}

func programHasOpcode(t *testing.T, pool *sql.DB, query, opcode string) bool {
	t.Helper()

	rows, err := pool.QueryContext(t.Context(), "EXPLAIN "+query)
	if err != nil {
		t.Fatalf("EXPLAIN %s: %v", query, err)
	}

	defer func() { _ = rows.Close() }()

	columns, err := rows.Columns()
	if err != nil {
		t.Fatalf("EXPLAIN columns: %v", err)
	}

	// Columns: addr, opcode, p1 … p5, comment. Only the opcode is read.
	const opcodeColumn = 1

	values := make([]any, len(columns))
	targets := make([]any, len(columns))

	for index := range values {
		targets[index] = &values[index]
	}

	found := false

	for rows.Next() {
		if err := rows.Scan(targets...); err != nil {
			t.Fatalf("scan EXPLAIN row: %v", err)
		}

		found = found || textValue(values[opcodeColumn]) == opcode
	}

	if err := rows.Err(); err != nil {
		t.Fatalf("EXPLAIN %s: %v", query, err)
	}

	return found
}

// textValue returns a TEXT column scanned into an any, which drivers deliver as string or
// []byte; other values yield "".
func textValue(value any) string {
	switch text := value.(type) {
	case string:
		return text
	case []byte:
		return string(text)
	default:
		return ""
	}
}
