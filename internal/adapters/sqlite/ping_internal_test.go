package sqlite

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"io"
	"log/slog"
	"slices"
	"sync"
	"testing"
)

// TestPingQueryStartsAReadTransaction (pools_internal_test.go) shows that pingQuery reads the
// database file. This test pins that Ping really runs it, and nothing else, so a change to
// the driver's own Ping ("SELECT 1") fails here. The reader pool is replaced by a recording
// driver because SQLite offers no hook that lists executed statements.
func TestPingRunsThePingQueryOnTheReaderPool(t *testing.T) {
	t.Parallel()

	connector := &recordingConnector{}
	reader := sql.OpenDB(connector)

	t.Cleanup(func() { _ = reader.Close() })

	db := &DB{reader: reader, logger: slog.New(slog.DiscardHandler)}

	if err := db.Ping(t.Context()); err != nil {
		t.Fatalf("Ping error = %v", err)
	}

	if got := connector.recordedQueries(); !slices.Equal(got, []string{pingQuery}) {
		t.Errorf("queries run by Ping = %q, want only %q", got, pingQuery)
	}
}

var errNotSupportedByRecorder = errors.New("not supported by the recording driver")

// recordingConnector is a database/sql driver that records the text of every query and
// answers each with a single row holding the integer 1.
type recordingConnector struct {
	mutex   sync.Mutex
	queries []string
}

func (connector *recordingConnector) Connect(context.Context) (driver.Conn, error) {
	return recordingConnection{connector: connector}, nil
}

func (connector *recordingConnector) Driver() driver.Driver { return recordingDriver{} }

func (connector *recordingConnector) record(query string) {
	connector.mutex.Lock()
	defer connector.mutex.Unlock()

	connector.queries = append(connector.queries, query)
}

func (connector *recordingConnector) recordedQueries() []string {
	connector.mutex.Lock()
	defer connector.mutex.Unlock()

	return slices.Clone(connector.queries)
}

type recordingDriver struct{}

func (recordingDriver) Open(string) (driver.Conn, error) { return nil, errNotSupportedByRecorder }

type recordingConnection struct {
	connector *recordingConnector
}

func (recordingConnection) Prepare(string) (driver.Stmt, error) {
	return nil, errNotSupportedByRecorder
}

func (recordingConnection) Close() error { return nil }

func (recordingConnection) Begin() (driver.Tx, error) { return nil, errNotSupportedByRecorder }

// QueryContext makes database/sql run queries directly instead of preparing them.
func (connection recordingConnection) QueryContext(_ context.Context, query string,
	_ []driver.NamedValue,
) (driver.Rows, error) {
	connection.connector.record(query)

	return &singleIntegerRows{}, nil
}

// singleIntegerRows is a result of one row with the integer 1 in one column.
type singleIntegerRows struct {
	done bool
}

func (*singleIntegerRows) Columns() []string { return []string{"value"} }

func (*singleIntegerRows) Close() error { return nil }

func (rows *singleIntegerRows) Next(destination []driver.Value) error {
	if rows.done {
		return io.EOF
	}

	rows.done = true
	destination[0] = int64(1)

	return nil
}
