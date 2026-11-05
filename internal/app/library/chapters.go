package library

import "github.com/magpeek/mivo/internal/domain/media"

// ItemChapter is a chapter of an item on the item's whole timeline (M3-14): Chapter.Start
// and Chapter.End count from the beginning of the work across all its part files, and
// Chapter.FileRef is the library-relative path of the part file. FileID names that media
// file, so a client can seek within it (API ItemChapter.fileId).
type ItemChapter struct {
	Chapter media.Chapter
	FileID  string
}
