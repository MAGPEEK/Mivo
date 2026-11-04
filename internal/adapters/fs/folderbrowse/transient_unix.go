//go:build unix

package folderbrowse

import "syscall"

// transientErrors are I/O errors of a disk or network share that may go away when the share
// is back (same set as adapters/fs/libraryfs).
func transientErrors() []error {
	return []error{syscall.EIO, syscall.ESTALE, syscall.ENOTCONN, syscall.ETIMEDOUT, syscall.EHOSTDOWN, syscall.ENODEV}
}
