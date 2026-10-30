// Package instancelock lets only one server process use a data directory at a time. Two
// servers on one directory would race the database migrations and purge each other's
// transcode sessions (ADR 0024).
//
// The lock is an exclusive, non-blocking OS lock on <data>/server.lock: flock on Unix-like
// systems, LockFileEx on Windows. The OS drops it when the process dies, so a crash never
// leaves a stale lock behind and the file itself can stay in place. Network file systems
// (SMB, NFS) may not honour these locks; the data directory belongs on a local disk.
package instancelock

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
)

const (
	// FileName is the lock file inside the data directory.
	FileName = "server.lock"

	// fileMode keeps the lock file private: a user who can open it could hold the lock and
	// keep the server from starting.
	fileMode fs.FileMode = 0o600

	goosWindows = "windows"
)

// ErrLocked reports that another process, or another Lock in this process, holds the lock.
var ErrLocked = errors.New("locked by another process")

// Lock is a held lock on a data directory. It is not safe for concurrent use.
type Lock struct {
	file *os.File
	path string
}

// Acquire takes the lock on dataDir without waiting. The directory must exist. When the lock
// is held elsewhere, the error wraps ErrLocked and names the lock file in dataDir.
func Acquire(dataDir string) (*Lock, error) {
	path := filepath.Join(dataDir, FileName)

	file, err := openLockFile(dataDir)
	if err != nil {
		return nil, fmt.Errorf("open lock file %s: %w", path, err)
	}

	err = lockFile(file)
	if err == nil {
		err = restrictMode(file)
	}

	if err != nil {
		return nil, errors.Join(fmt.Errorf("lock %s: %w", path, err), closeFile(file))
	}

	return &Lock{file: file, path: path}, nil
}

// Release unlocks and closes the lock file and leaves the file in place. Calls after the
// first, and calls on a nil Lock, do nothing.
func (lock *Lock) Release() error {
	if lock == nil || lock.file == nil {
		return nil
	}

	file := lock.file
	lock.file = nil

	// Closing alone would drop the lock as well, but Windows may release it only some time
	// after the close; unlocking first frees it at once for the next server.
	err := errors.Join(unlockFile(file), closeFile(file))
	if err != nil {
		return fmt.Errorf("release %s: %w", lock.path, err)
	}

	return nil
}

// openLockFile opens or creates the lock file through an os.Root, so a symbolic link at the
// lock path cannot make the server create or lock a file outside the data directory.
func openLockFile(dataDir string) (*os.File, error) {
	root, err := os.OpenRoot(dataDir)
	if err != nil {
		return nil, fmt.Errorf("open data directory: %w", err)
	}

	file, err := root.OpenFile(FileName, os.O_RDWR|os.O_CREATE, fileMode)

	closeErr := root.Close()
	if err != nil {
		return nil, errors.Join(err, closeErr)
	}

	if closeErr != nil {
		return nil, errors.Join(fmt.Errorf("close data directory: %w", closeErr), closeFile(file))
	}

	return file, nil
}

// restrictMode tightens a lock file that an older run or an operator left with a wider mode.
// Windows has no POSIX permission bits; there the data directory's ACL protects the file.
func restrictMode(file *os.File) error {
	if runtime.GOOS == goosWindows {
		return nil
	}

	info, err := file.Stat()
	if err != nil {
		return fmt.Errorf("stat: %w", err)
	}

	if info.Mode().Perm()&^fileMode == 0 {
		return nil
	}

	err = file.Chmod(fileMode)
	if err != nil {
		return fmt.Errorf("restrict mode to %o: %w", fileMode, err)
	}

	return nil
}

func closeFile(file *os.File) error {
	err := file.Close()
	if err != nil {
		return fmt.Errorf("close: %w", err)
	}

	return nil
}
