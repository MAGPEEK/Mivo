package library

import (
	"time"

	"github.com/magpeek/mivo/internal/domain/media"
)

// ItemChapter is a chapter of an item on the item's whole timeline (M3-14): Chapter.Start
// and Chapter.End count from the beginning of the work across all its part files, and
// Chapter.FileRef is the library-relative path of the part file. FileID names that media
// file and FileStart is where the file begins on the work's timeline, so a client can seek
// within the file (API ItemChapter.fileId and its file-relative startMs and endMs).
type ItemChapter struct {
	Chapter   media.Chapter
	FileID    string
	FileStart time.Duration
}

// FileRelative returns the chapter's start and end within its part file.
func (chapter ItemChapter) FileRelative() (start, end time.Duration) {
	return chapter.Chapter.Start - chapter.FileStart, chapter.Chapter.End - chapter.FileStart
}
