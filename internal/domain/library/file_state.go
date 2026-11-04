package library

import (
	"time"

	"github.com/magpeek/mivo/internal/domain/media"
)

// FileState is what a scan knows about a library file without opening it: its path relative
// to the library root (see ParseRelativePath), its size in bytes, its modification time and
// its class (see ClassifyFile).
//
// RelativePath is the path exactly as the file system reports it; it opens the file and is
// stored. NormalizedPath is the same path in Unicode NFC, produced by the adapter (ADR 0027
// §3: the domain is standard-library only and expects NFC text); it is used for parsing and
// match keys. An empty NormalizedPath means the relative path is already NFC.
//
// RootID is the ID of the library folder the path is relative to (ADR 0028); the same
// relative path in two folders of a library names two files. Catalogs store and return it
// for every file and sidecar.
type FileState struct {
	RootID         string
	RelativePath   string
	NormalizedPath string
	Class          FileClass
	Size           int64
	ModifiedAt     time.Time
	// Layout says how the video is stored; empty means a single file. For a disc folder
	// RelativePath is the BDMV or VIDEO_TS folder, and Size and ModifiedAt are those of its
	// index file (ADR 0029).
	Layout media.Layout
}

// PathForParsing returns the NFC path the name parser and match keys work on.
func (state FileState) PathForParsing() string {
	if state.NormalizedPath != "" {
		return state.NormalizedPath
	}

	return state.RelativePath
}

// Changed reports whether a file must be probed again: its path, size or modification time
// differs. Times are compared at millisecond precision, truncated, because the catalog stores
// Unix milliseconds (ADR 0004); a finer comparison would re-probe every file on every scan.
// Class and NormalizedPath are derived from the path, so they never change on their own.
func Changed(previous, current FileState) bool {
	return previous.RelativePath != current.RelativePath ||
		previous.Size != current.Size ||
		previous.ModifiedAt.UnixMilli() != current.ModifiedAt.UnixMilli()
}
