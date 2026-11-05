package restapi

import (
	"fmt"

	"github.com/magpeek/mivo/internal/adapters/http/apigen"
	applibrary "github.com/magpeek/mivo/internal/app/library"
)

// maxChaptersInDetail is the API's maxItems of ItemDetail.chapters. The catalog keeps at
// most that many chapters per file, so only a book of several long parts can exceed it; it
// lists the chapters of its first parts.
const maxChaptersInDetail = 2000

// applyChapters sends the chapters of an audiobook with offsets relative to their part
// file, so a client plays fileId from startMs; omitted when the book has none.
func applyChapters(response *apigen.ItemDetail, detail applibrary.ItemDetail) error {
	if len(detail.Chapters) == 0 {
		return nil
	}

	chapters := detail.Chapters[:min(len(detail.Chapters), maxChaptersInDetail)]
	mapped := make([]apigen.ItemChapter, 0, len(chapters))

	for index, chapter := range chapters {
		fileID, err := parseStoredID("file", chapter.FileID)
		if err != nil {
			return fmt.Errorf("chapter %d: %w", index, err)
		}

		start, end := chapter.FileRelative()
		mapped = append(mapped, apigen.ItemChapter{
			Title: chapter.Chapter.Title, FileID: fileID, StartMs: start.Milliseconds(), EndMs: end.Milliseconds(),
		})
	}

	response.Chapters = &mapped

	return nil
}
