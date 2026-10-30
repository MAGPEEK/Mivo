//go:build windows

package instancelock

import (
	"errors"
	"fmt"
	"os"

	"golang.org/x/sys/windows"
)

// allBytes locks the whole file range, the usual convention for LockFileEx (the low and the
// high half of the range length are both set to it).
const allBytes = ^uint32(0)

// lockFile takes an exclusive LockFileEx lock and fails at once when it is taken.
func lockFile(file *os.File) error {
	err := windows.LockFileEx(windows.Handle(file.Fd()),
		windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY,
		0, allBytes, allBytes, new(windows.Overlapped))
	if errors.Is(err, windows.ERROR_LOCK_VIOLATION) {
		return ErrLocked
	}

	if err != nil {
		return fmt.Errorf("LockFileEx: %w", err)
	}

	return nil
}

func unlockFile(file *os.File) error {
	err := windows.UnlockFileEx(windows.Handle(file.Fd()), 0, allBytes, allBytes, new(windows.Overlapped))
	if err != nil {
		return fmt.Errorf("UnlockFileEx: %w", err)
	}

	return nil
}
