//go:build windows

package transcodedir

import (
	"errors"
	"fmt"
	"io/fs"
	"syscall"
	"testing"
)

func TestIsLockErrorOnWindows(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		err  error
		want bool
	}{
		{name: "sharing violation", err: &fs.PathError{Op: "RemoveAll", Path: "x", Err: errorSharingViolation}, want: true},
		{name: "access denied", err: &fs.PathError{Op: "RemoveAll", Path: "x", Err: syscall.ERROR_ACCESS_DENIED}, want: true},
		{name: "wrapped sharing violation", err: fmt.Errorf("remove: %w", errorSharingViolation), want: true},
		{name: "directory not empty", err: syscall.ERROR_DIR_NOT_EMPTY, want: false},
		{name: "path not found", err: syscall.ERROR_PATH_NOT_FOUND, want: false},
		{name: "plain error", err: errors.New("disk on fire"), want: false},
	}

	for _, testCase := range testCases {
		if got := isLockError(testCase.err); got != testCase.want {
			t.Errorf("%s: isLockError(%v) = %v, want %v", testCase.name, testCase.err, got, testCase.want)
		}
	}
}
