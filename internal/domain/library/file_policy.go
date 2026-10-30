package library

import (
	"path"
	"strings"
)

// hiddenEntryPrefix marks hidden files and folders on Unix-like systems and on NAS shares
// (macOS resource forks "._x", ".DS_Store", ".@__thumb").
const hiddenEntryPrefix = "."

// IsVideoFileName reports whether a file name has one of the video extensions of ADR 0022.
// The comparison ignores case, so "MOVIE.MKV" counts. Audio-only containers such as .m4a and
// sidecar files such as .srt do not; a file whose probe finds no video stream is skipped
// later by the scan.
func IsVideoFileName(name string) bool {
	extension := strings.ToLower(path.Ext(name))

	switch extension {
	case ".mkv", ".mk3d", ".mp4", ".m4v", ".mov", ".avi", ".divx",
		".ts", ".m2ts", ".mts", ".webm", ".flv", ".wmv", ".asf", ".ogv",
		".3gp", ".3g2", ".mpg", ".mpeg", ".vob", ".rm", ".rmvb", ".mxf":
		return true
	default:
		return false
	}
}

// IsIgnoredDirectory reports whether a directory is a NAS or operating system folder that a
// scan never enters (ADR 0022). The comparison ignores case because Windows shows the
// recycle bin as "$Recycle.Bin" and file systems such as exFAT do not preserve case reliably.
func IsIgnoredDirectory(name string) bool {
	switch strings.ToLower(name) {
	case "@eadir", "#recycle", "#snapshot", ".@__thumb",
		"$recycle.bin", "system volume information", "lost+found":
		return true
	default:
		return false
	}
}

// IsHiddenEntry reports whether a file or directory name is hidden by the leading-dot
// convention. Scans skip hidden entries; the Windows hidden attribute is not considered
// because the domain cannot read attributes and NAS shares rarely set it.
func IsHiddenEntry(name string) bool {
	return strings.HasPrefix(name, hiddenEntryPrefix)
}
