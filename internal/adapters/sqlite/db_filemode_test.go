package sqlite_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/magpeek/mivo/internal/adapters/sqlite"
)

// privateFileMode is the mode the database, WAL and SHM files must have on Unix.
const privateFileMode fs.FileMode = 0o600

// worldReadableFileMode is the mode SQLite would create the database with on its own.
const worldReadableFileMode fs.FileMode = 0o644

func TestOpenCreatesDatabaseFilesWithPrivateFileMode(t *testing.T) {
	t.Parallel()
	skipFileModeOnWindows(t)

	db, path := openTestDB(t, nil)

	// Migrate writes, so the WAL and SHM files exist while the database is open.
	if err := db.Migrate(t.Context()); err != nil {
		t.Fatalf("Migrate error = %v", err)
	}

	for _, name := range []string{path, path + "-wal", path + "-shm"} {
		assertFileMode(t, name, privateFileMode)
	}
}

func TestOpenTightensTheFileModeOfAnExistingDatabase(t *testing.T) {
	t.Parallel()
	skipFileModeOnWindows(t)

	path := filepath.Join(t.TempDir(), testDatabaseFile)
	for _, name := range []string{path, path + "-wal"} {
		createWorldReadableFile(t, name)
	}

	db, err := sqlite.Open(t.Context(), sqlite.Options{Path: path})
	if err != nil {
		t.Fatalf("Open(%q) error = %v", path, err)
	}

	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("Close error = %v", err)
		}
	})

	for _, name := range []string{path, path + "-wal", path + "-shm"} {
		assertFileMode(t, name, privateFileMode)
	}
}

// skipFileModeOnWindows skips: Windows has no Unix mode bits, the files inherit the ACL of
// the data directory.
func skipFileModeOnWindows(t *testing.T) {
	t.Helper()

	if runtime.GOOS == "windows" {
		t.Skip("Unix file modes do not apply on Windows; files inherit the data directory ACL")
	}
}

func createWorldReadableFile(t *testing.T, name string) {
	t.Helper()

	if err := os.WriteFile(name, nil, worldReadableFileMode); err != nil {
		t.Fatalf("create %s: %v", name, err)
	}

	// WriteFile is subject to the umask; Chmod sets the mode exactly.
	if err := os.Chmod(name, worldReadableFileMode); err != nil {
		t.Fatalf("chmod %s: %v", name, err)
	}
}

func assertFileMode(t *testing.T, name string, want fs.FileMode) {
	t.Helper()

	info, err := os.Stat(name)
	if err != nil {
		t.Fatalf("stat %s: %v", name, err)
	}

	if got := info.Mode().Perm(); got != want {
		t.Errorf("mode of %s = %v, want %v", filepath.Base(name), got, want)
	}
}
