package sqlite_test

import (
	"context"
	"database/sql"
	"log/slog"
	"path/filepath"
	"sync"
	"testing"

	"github.com/magpeek/mivo/internal/adapters/sqlite"
)

// testDatabaseFile is the file name the tests use inside t.TempDir().
const testDatabaseFile = "server.db"

// openTestDB opens a fresh database in t.TempDir() and closes it when the test ends.
func openTestDB(t *testing.T, logger *slog.Logger) (*sqlite.DB, string) {
	t.Helper()

	path := filepath.Join(t.TempDir(), testDatabaseFile)

	db, err := sqlite.Open(t.Context(), sqlite.Options{Path: path, Logger: logger})
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

// openMigratedTestDB is openTestDB followed by Migrate.
func openMigratedTestDB(t *testing.T) (*sqlite.DB, string) {
	t.Helper()

	db, path := openTestDB(t, nil)
	if err := db.Migrate(t.Context()); err != nil {
		t.Fatalf("Migrate error = %v", err)
	}

	return db, path
}

// openInspectionDB opens a plain connection to the file at path, independent of the adapter,
// so tests can check and tamper with what the adapter stored.
func openInspectionDB(t *testing.T, path string) *sql.DB {
	t.Helper()

	inspection, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open inspection connection: %v", err)
	}

	t.Cleanup(func() {
		if err := inspection.Close(); err != nil {
			t.Errorf("close inspection connection: %v", err)
		}
	})

	return inspection
}

// cancelledContext returns a context that is already cancelled.
func cancelledContext(t *testing.T) context.Context {
	t.Helper()

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	return ctx
}

// logRecord is the part of a slog.Record the tests assert on.
type logRecord struct {
	level   slog.Level
	message string
	attrs   map[string]slog.Value
}

// recordingHandler keeps every record at any level in memory.
type recordingHandler struct {
	mutex   *sync.Mutex
	records *[]logRecord
}

func newRecordingLogger() (*slog.Logger, func() []logRecord) {
	handler := recordingHandler{mutex: &sync.Mutex{}, records: &[]logRecord{}}

	snapshot := func() []logRecord {
		handler.mutex.Lock()
		defer handler.mutex.Unlock()

		return append([]logRecord(nil), *handler.records...)
	}

	return slog.New(handler), snapshot
}

func (recordingHandler) Enabled(context.Context, slog.Level) bool { return true }

func (handler recordingHandler) Handle(_ context.Context, record slog.Record) error {
	attrs := map[string]slog.Value{}
	record.Attrs(func(attr slog.Attr) bool {
		attrs[attr.Key] = attr.Value
		return true
	})

	handler.mutex.Lock()
	defer handler.mutex.Unlock()

	*handler.records = append(*handler.records, logRecord{level: record.Level, message: record.Message, attrs: attrs})

	return nil
}

// WithAttrs and WithGroup are not used by the adapter; returning the handler unchanged is
// enough for these tests.
func (handler recordingHandler) WithAttrs([]slog.Attr) slog.Handler { return handler }

func (handler recordingHandler) WithGroup(string) slog.Handler { return handler }
