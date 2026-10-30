//go:build !(windows || darwin || dragonfly || freebsd || linux || netbsd || openbsd)

package instancelock

import (
	"errors"
	"fmt"
	"os"
	"runtime"
)

// lockFile fails on systems without flock or LockFileEx: running unprotected would let a
// second server corrupt the first one's data.
func lockFile(*os.File) error {
	return fmt.Errorf("file locks on %s: %w", runtime.GOOS, errors.ErrUnsupported)
}

func unlockFile(*os.File) error {
	return nil
}
