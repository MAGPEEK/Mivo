package library

import "strings"

// IsHiddenFolderName reports whether the folder browser hides a directory name: dot
// folders, DSM "@" system folders and the recycle, snapshot and volume system folders
// (ADR 0022). Windows hidden attributes are the adapter's job.
func IsHiddenFolderName(name string) bool {
	if strings.HasPrefix(name, ".") || strings.HasPrefix(name, "@") {
		return true
	}

	switch strings.ToLower(name) {
	case "#recycle", "#snapshot", "$recycle.bin", "system volume information", "lost+found":
		return true
	default:
		return false
	}
}
