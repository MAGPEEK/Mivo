//go:build darwin || dragonfly || freebsd || linux || netbsd || openbsd

package instancelock

import (
	"errors"
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

// lockFile takes an exclusive flock and fails at once when it is taken. flock locks belong
// to the open file, so a second open in the same process conflicts as well.
func lockFile(file *os.File) error {
	err := unix.Flock(int(file.Fd()), unix.LOCK_EX|unix.LOCK_NB)
	if errors.Is(err, unix.EWOULDBLOCK) {
		return ErrLocked
	}

	if err != nil {
		return fmt.Errorf("flock: %w", err)
	}

	return nil
}

func unlockFile(file *os.File) error {
	err := unix.Flock(int(file.Fd()), unix.LOCK_UN)
	if err != nil {
		return fmt.Errorf("flock unlock: %w", err)
	}

	return nil
}
