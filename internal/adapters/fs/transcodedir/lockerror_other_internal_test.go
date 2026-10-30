//go:build !windows

package transcodedir

import (
	"errors"
	"io/fs"
	"syscall"
	"testing"
)

func TestIsLockErrorNeverRetriesOutsideWindows(t *testing.T) {
	t.Parallel()

	errs := []error{
		&fs.PathError{Op: "unlinkat", Path: "x", Err: syscall.EACCES},
		&fs.PathError{Op: "unlinkat", Path: "x", Err: syscall.EBUSY},
		errors.New("disk on fire"),
	}

	for _, err := range errs {
		if isLockError(err) {
			t.Errorf("isLockError(%v) = true, want false", err)
		}
	}
}
