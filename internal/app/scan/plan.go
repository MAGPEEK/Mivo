package scan

import (
	"strings"

	"github.com/magpeek/mivo/internal/domain/library"
)

// relativePathSeparator separates the segments of a root-relative catalog path.
const relativePathSeparator = "/"

// probeJob is one file to probe: its walked state, the placement computed for it and, for a
// changed file, the stored file it replaces.
type probeJob struct {
	state     library.FileState
	placement library.Placement
	placed    *PlacedFileState
	// fileID, when set, is the ID a new file is saved with; the audiobook pipeline needs it
	// before the save to write the file's chapters.
	fileID string
}

// belowAny reports whether relativePath equals one of the skipped paths or lies below one.
// Trailing separators of a skipped path are ignored ("Locked/" protects "Locked/a.mkv"); an
// empty or "." skipped path stands for the whole root.
func belowAny(relativePath string, skipped []string) bool {
	for _, skippedPath := range skipped {
		skippedPath = strings.TrimRight(skippedPath, relativePathSeparator)

		if skippedPath == "" || skippedPath == "." || relativePath == skippedPath ||
			strings.HasPrefix(relativePath, skippedPath+relativePathSeparator) {
			return true
		}
	}

	return false
}
