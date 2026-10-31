//go:build unix

package libraryfs

import (
	"fmt"
	"io/fs"
	"os"
	"syscall"
)

const (
	// openFileFlags opens without blocking: opening a FIFO for reading would otherwise wait for
	// a writer. The type is checked on the open handle; O_NONBLOCK has no effect on reads of
	// regular files.
	openFileFlags = os.O_RDONLY | syscall.O_NONBLOCK
	// openDirectoryFlags fails with ENOTDIR instead of blocking when a directory was replaced by
	// a FIFO between listing its parent and opening it.
	openDirectoryFlags = os.O_RDONLY | syscall.O_DIRECTORY
)

// transientErrors are I/O errors of a disk or network share that may go away when the share is
// back; they map to domain.ErrUnavailable.
func transientErrors() []error {
	return []error{syscall.EIO, syscall.ESTALE, syscall.ENOTCONN, syscall.ETIMEDOUT, syscall.EHOSTDOWN, syscall.ENODEV}
}

// directoryIdentity returns device and inode of an open directory. Inode 0, which some FUSE
// and network file systems report, leaves the identity unknown.
func directoryIdentity(_ *os.File, info fs.FileInfo) (directoryID, error) {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return directoryID{}, fmt.Errorf("no device and inode for %s", info.Name())
	}

	// The conversion is needed where Dev is not uint64 (for example int32 on macOS).
	return directoryID{device: uint64(stat.Dev), indexLow: stat.Ino}, nil
}

// videoFileInfo reads the metadata of a regular file found by listing its directory. In Go
// 1.27 the entries of a directory opened through an os.Root already carry metadata read with
// lstatat on the directory handle, so DirEntry.Info is confined too. That is an
// implementation detail, though: the documented DirEntry.Info may lstat a path built from the
// directory's name, outside the root. The root's Lstat is confined by contract and costs one
// metadata read per video file.
func videoFileInfo(root *os.Root, relativePath string, _ fs.DirEntry) (fs.FileInfo, error) {
	info, err := root.Lstat(relativePath)
	if err != nil {
		return nil, fmt.Errorf("lstat: %w", err)
	}

	return info, nil
}
