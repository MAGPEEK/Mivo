//go:build windows

package folderbrowse

import (
	"io/fs"
	"syscall"
)

// hiddenByAttribute reports the hidden or system attribute of a listed entry. An entry whose
// attributes cannot be read is hidden, which errs on the safe side.
func hiddenByAttribute(entry fs.DirEntry) bool {
	info, err := entry.Info()
	if err != nil {
		return true
	}

	return hiddenInfo(info)
}

// hiddenInfo reports the hidden or system attribute in file metadata.
func hiddenInfo(info fs.FileInfo) bool {
	data, ok := info.Sys().(*syscall.Win32FileAttributeData)
	if !ok {
		return false
	}

	return data.FileAttributes&(syscall.FILE_ATTRIBUTE_HIDDEN|syscall.FILE_ATTRIBUTE_SYSTEM) != 0
}
