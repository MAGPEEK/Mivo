package sqlite_test

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/magpeek/mivo/internal/adapters/sqlite"
	appsystem "github.com/magpeek/mivo/internal/app/system"
)

// The adapter must satisfy the ports cmd/mivo wires it into.
var (
	_ appsystem.IdentityStore   = (*sqlite.IdentityRepository)(nil)
	_ appsystem.DatabaseChecker = (*sqlite.DB)(nil)
)

// Windows paths with a drive letter, spaces and characters that are special in URIs ('#',
// '%') must reach exactly that file, not a decoded or truncated variant of the name.
func TestOpenUsesTheExactPathWithSpacesAndURICharacters(t *testing.T) {
	t.Parallel()

	directory := filepath.Join(t.TempDir(), "Jane Doe #1 %20 ä")
	if err := os.Mkdir(directory, 0o700); err != nil {
		t.Fatalf("create directory: %v", err)
	}

	path := filepath.Join(directory, "server.db")

	db, err := sqlite.Open(t.Context(), sqlite.Options{Path: path})
	if err != nil {
		t.Fatalf("Open(%q) error = %v", path, err)
	}

	if err := db.Migrate(t.Context()); err != nil {
		t.Fatalf("Migrate error = %v", err)
	}

	if err := db.Close(); err != nil {
		t.Fatalf("Close error = %v", err)
	}

	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatalf("read directory: %v", err)
	}

	if len(entries) != 1 || entries[0].Name() != "server.db" {
		t.Errorf("directory entries after Close = %v, want only server.db", entryNames(entries))
	}
}

func entryNames(entries []os.DirEntry) []string {
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Name())
	}

	return names
}

func TestOpenFails(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		ctx     func(t *testing.T) context.Context
		path    func(t *testing.T) string
		wantErr error
	}{
		{
			name:    "empty path",
			ctx:     func(t *testing.T) context.Context { return t.Context() },
			path:    func(*testing.T) string { return "" },
			wantErr: sqlite.ErrNoPath,
		},
		{
			name: "missing directory",
			ctx:  func(t *testing.T) context.Context { return t.Context() },
			path: func(t *testing.T) string {
				return filepath.Join(t.TempDir(), "missing", "server.db")
			},
		},
		{
			name: "path is a directory",
			ctx:  func(t *testing.T) context.Context { return t.Context() },
			path: func(t *testing.T) string { return t.TempDir() },
		},
		{
			name:    "cancelled context",
			ctx:     cancelledContext,
			path:    func(t *testing.T) string { return filepath.Join(t.TempDir(), "server.db") },
			wantErr: context.Canceled,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			db, err := sqlite.Open(test.ctx(t), sqlite.Options{Path: test.path(t)})
			if err == nil {
				_ = db.Close()

				t.Fatal("Open error = nil, want an error")
			}

			if test.wantErr != nil && !errors.Is(err, test.wantErr) {
				t.Errorf("Open error = %v, want %v", err, test.wantErr)
			}
		})
	}
}

func TestCloseClosesBothPoolsAndIsIdempotent(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), testDatabaseFile)

	db, err := sqlite.Open(t.Context(), sqlite.Options{Path: path})
	if err != nil {
		t.Fatalf("Open error = %v", err)
	}

	if err := db.Close(); err != nil {
		t.Fatalf("first Close error = %v", err)
	}

	if err := db.Close(); err != nil {
		t.Errorf("second Close error = %v, want nil", err)
	}

	// Ping uses the reader pool and Migrate the writer pool; both must be closed.
	if err := db.Ping(t.Context()); err == nil {
		t.Error("Ping after Close error = nil, want an error")
	}

	if err := db.Migrate(t.Context()); err == nil {
		t.Error("Migrate after Close error = nil, want an error")
	}
}

func TestPingSucceedsOnAnOpenDatabase(t *testing.T) {
	t.Parallel()

	logger, records := newRecordingLogger()
	db, _ := openTestDB(t, logger)

	if err := db.Ping(t.Context()); err != nil {
		t.Fatalf("Ping error = %v", err)
	}

	if got := records(); len(got) != 0 {
		t.Errorf("log records = %+v, want none", got)
	}
}

// The DatabaseChecker port leaves logging of the raw error to the adapter: a cancelled
// caller is logged at debug level, real failures (incl. timeouts) at warn level.
func TestPingFailureIsReturnedAndLoggedAtItsLevel(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		ctx       func(t *testing.T) context.Context
		closeDB   bool
		wantErr   error
		wantLevel slog.Level
		wantText  string
	}{
		{name: "cancelled context", ctx: cancelledContext, wantErr: context.Canceled, wantLevel: slog.LevelDebug},
		{name: "deadline exceeded", ctx: expiredContext, wantErr: context.DeadlineExceeded, wantLevel: slog.LevelWarn},
		{
			name: "closed database", ctx: func(t *testing.T) context.Context { return t.Context() },
			closeDB: true, wantLevel: slog.LevelWarn, wantText: "closed",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			logger, records := newRecordingLogger()
			db, _ := openTestDB(t, logger)

			if test.closeDB {
				if err := db.Close(); err != nil {
					t.Fatalf("Close error = %v", err)
				}
			}

			err := db.Ping(test.ctx(t))
			if err == nil || (test.wantErr != nil && !errors.Is(err, test.wantErr)) {
				t.Fatalf("Ping error = %v, want %v", err, test.wantErr)
			}

			assertSinglePingFailureRecord(t, records(), test.wantLevel, test.wantText)
		})
	}
}

func assertSinglePingFailureRecord(t *testing.T, records []logRecord, wantLevel slog.Level, wantText string) {
	t.Helper()

	if len(records) != 1 {
		t.Fatalf("log records = %+v, want exactly one", records)
	}

	record := records[0]
	if record.level != wantLevel || record.message != "database ping failed" {
		t.Errorf("log record = %v %q, want %v %q", record.level, record.message, wantLevel, "database ping failed")
	}

	loggedError, found := record.attrs["error"]
	if !found || !strings.Contains(loggedError.String(), wantText) {
		t.Errorf("logged error = %v, want the raw error containing %q", loggedError, wantText)
	}
}

func expiredContext(t *testing.T) context.Context {
	t.Helper()

	ctx, cancel := context.WithDeadline(t.Context(), time.Now().Add(-time.Second))
	t.Cleanup(cancel)

	return ctx
}
