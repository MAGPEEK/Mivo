//go:build windows

package imagecache

import (
	"errors"
	"syscall"
)

// errorSharingViolation is ERROR_SHARING_VIOLATION, which the syscall package does not name.
const errorSharingViolation syscall.Errno = 32

// isLockError reports whether an operation failed only because a file is still open
// somewhere, for example a cached picture a request is reading. ERROR_ACCESS_DENIED is
// included because Windows reports it for a file whose deletion is already pending.
func isLockError(err error) bool {
	return errors.Is(err, errorSharingViolation) || errors.Is(err, syscall.ERROR_ACCESS_DENIED)
}
