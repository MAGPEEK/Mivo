package library

import "time"

// FileState is what a scan knows about a video file without opening it: its path relative to
// the library root (see ParseRelativePath), its size in bytes and its modification time.
type FileState struct {
	RelativePath string
	Size         int64
	ModifiedAt   time.Time
}

// Changed reports whether a file must be probed again: its path, size or modification time
// differs. Times are compared at millisecond precision, truncated, because the catalog stores
// Unix milliseconds (ADR 0004); a finer comparison would re-probe every file on every scan.
func Changed(previous, current FileState) bool {
	return previous.RelativePath != current.RelativePath ||
		previous.Size != current.Size ||
		previous.ModifiedAt.UnixMilli() != current.ModifiedAt.UnixMilli()
}
