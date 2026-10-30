package instancelock_test

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/magpeek/mivo/internal/platform/instancelock"
)

// lockFileMode is the mode the acceptance criteria require for the lock file.
const lockFileMode fs.FileMode = 0o600

func TestAcquireCreatesThePrivateLockFileAndLeavesItAfterRelease(t *testing.T) {
	t.Parallel()

	dataDir := t.TempDir()
	lock := acquire(t, dataDir)

	path := filepath.Join(dataDir, instancelock.FileName)
	assertUnixMode(t, path, lockFileMode)

	if err := lock.Release(); err != nil {
		t.Fatalf("Release: %v", err)
	}

	if _, err := os.Stat(path); err != nil {
		t.Errorf("lock file after Release: %v, want it left in place", err)
	}
}

func TestAcquireTightensTheModeOfAnExistingLockFile(t *testing.T) {
	t.Parallel()

	if runtime.GOOS == "windows" {
		t.Skip("Windows has no POSIX permission bits; the data directory ACL protects the file")
	}

	dataDir := t.TempDir()
	path := filepath.Join(dataDir, instancelock.FileName)

	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatalf("write lock file: %v", err)
	}

	// Widened after creation, so the process umask cannot narrow it again.
	if err := os.Chmod(path, 0o644); err != nil { //nolint:gosec // G302: the test widens the mode on purpose
		t.Fatalf("widen lock file mode: %v", err)
	}

	lock := acquire(t, dataDir)
	defer releaseOrFail(t, lock)

	assertUnixMode(t, path, lockFileMode)
}

func TestAcquireFailsWhileTheDirectoryIsLocked(t *testing.T) {
	t.Parallel()

	dataDir := t.TempDir()
	lock := acquire(t, dataDir)

	// flock and LockFileEx both lock per open file, so a second open in the same process
	// conflicts like another process would.
	second, err := instancelock.Acquire(dataDir)
	if err == nil {
		releaseOrFail(t, second)
		t.Fatal("second Acquire succeeded, want ErrLocked")
	}

	assertLockedError(t, err, dataDir)

	releaseOrFail(t, lock)

	// The released lock is free for the next server.
	releaseOrFail(t, acquire(t, dataDir))
}

func TestReleaseIsIdempotent(t *testing.T) {
	t.Parallel()

	lock := acquire(t, t.TempDir())

	for attempt := range 2 {
		if err := lock.Release(); err != nil {
			t.Errorf("Release #%d: %v", attempt+1, err)
		}
	}

	var unset *instancelock.Lock
	if err := unset.Release(); err != nil {
		t.Errorf("Release of a nil lock: %v", err)
	}
}

func TestAcquireReportsUnusableDirectories(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		dataDir func(t *testing.T) string
	}{
		{
			name:    "missing directory",
			dataDir: func(t *testing.T) string { t.Helper(); return filepath.Join(t.TempDir(), "missing") },
		},
		{
			name:    "regular file",
			dataDir: regularFile,
		},
		{
			name:    "lock path is a directory",
			dataDir: directoryAtLockPath,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			dataDir := test.dataDir(t)

			lock, err := instancelock.Acquire(dataDir)
			if err == nil {
				releaseOrFail(t, lock)
				t.Fatal("Acquire succeeded, want an error")
			}

			if errors.Is(err, instancelock.ErrLocked) {
				t.Errorf("error %v wraps ErrLocked, want a plain failure", err)
			}

			if !strings.Contains(err.Error(), dataDir) {
				t.Errorf("error %q does not name %s", err, dataDir)
			}
		})
	}
}

func TestAcquireDoesNotFollowASymlinkOutOfTheDataDirectory(t *testing.T) {
	t.Parallel()

	dataDir := t.TempDir()
	outside := filepath.Join(t.TempDir(), "outside.lock")

	if err := os.Symlink(outside, filepath.Join(dataDir, instancelock.FileName)); err != nil {
		// Windows needs developer mode or an elevated shell for symbolic links.
		t.Skipf("create symlink: %v", err)
	}

	lock, err := instancelock.Acquire(dataDir)
	if err == nil {
		releaseOrFail(t, lock)
		t.Fatal("Acquire followed a symlink out of the data directory")
	}

	if _, statErr := os.Lstat(outside); !errors.Is(statErr, fs.ErrNotExist) {
		t.Errorf("file outside the data directory: %v, want it not created", statErr)
	}
}

func regularFile(t *testing.T) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}

	return path
}

func directoryAtLockPath(t *testing.T) string {
	t.Helper()

	dataDir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dataDir, instancelock.FileName), 0o750); err != nil {
		t.Fatalf("create directory at the lock path: %v", err)
	}

	return dataDir
}

func acquire(t *testing.T, dataDir string) *instancelock.Lock {
	t.Helper()

	lock, err := instancelock.Acquire(dataDir)
	if err != nil {
		t.Fatalf("Acquire(%s): %v", dataDir, err)
	}

	return lock
}

func releaseOrFail(t *testing.T, lock *instancelock.Lock) {
	t.Helper()

	if err := lock.Release(); err != nil {
		t.Errorf("Release: %v", err)
	}
}

// assertLockedError checks the contract of a refused lock: it wraps ErrLocked and names the
// data directory, so the operator knows which directory is in use.
func assertLockedError(t *testing.T, err error, dataDir string) {
	t.Helper()

	if !errors.Is(err, instancelock.ErrLocked) {
		t.Errorf("error %v does not wrap ErrLocked", err)
	}

	if !strings.Contains(err.Error(), dataDir) {
		t.Errorf("error %q does not name the data directory %s", err, dataDir)
	}
}

func assertUnixMode(t *testing.T, path string, want fs.FileMode) {
	t.Helper()

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}

	if runtime.GOOS == "windows" {
		return
	}

	if got := info.Mode().Perm(); got != want {
		t.Errorf("mode of %s = %o, want %o", path, got, want)
	}
}
