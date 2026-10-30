//go:build windows

package transcodedir

import (
	"errors"
	"syscall"
)

// errorSharingViolation is ERROR_SHARING_VIOLATION, which the syscall package does not name.
// Windows returns it while another process still holds a file open without delete sharing,
// as FFmpeg does with its output for a moment after it was killed.
const errorSharingViolation syscall.Errno = 32

// isLockError reports whether deleting failed only because a file is still open somewhere.
// ERROR_ACCESS_DENIED is included because Windows reports it for a file whose deletion is
// already pending while a handle to it remains open.
func isLockError(err error) bool {
	return errors.Is(err, errorSharingViolation) || errors.Is(err, syscall.ERROR_ACCESS_DENIED)
}
