//go:build !windows

package folderbrowse

import "io/fs"

// hiddenByAttribute is Windows-only; Unix hides by name.
func hiddenByAttribute(fs.DirEntry) bool { return false }

// hiddenInfo is Windows-only; Unix hides by name.
func hiddenInfo(fs.FileInfo) bool { return false }
