package sqlite

import (
	"context"
	"fmt"
	"time"

	"github.com/magpeek/mivo/internal/adapters/sqlite/sqlcgen"
	applibrary "github.com/magpeek/mivo/internal/app/library"
	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/library"
	"github.com/magpeek/mivo/internal/domain/media"
)

// ReplaceFileChapters stores chapters as the whole chapter list of the media file fileID in
// one transaction; an empty list removes the stored chapters. Start and End count from the
// beginning of the file, FileRef is ignored. The list must satisfy media.ValidateChapters
// and every chapter must still span at least one millisecond after truncation to the stored
// precision. Errors wrap domain.ErrInvalid for an invalid list and domain.ErrNotFound for an
// unknown file; either way nothing changes.
func (catalog *LibraryCatalog) ReplaceFileChapters(ctx context.Context, fileID string, chapters []media.Chapter) error {
	rows, err := chapterRows(fileID, chapters)
	if err != nil {
		return fmt.Errorf("replace chapters of file %q: %w", fileID, err)
	}

	err = inWriteTransaction(ctx, catalog.writer, func(queries *sqlcgen.Queries) error {
		return replaceChapterRows(ctx, queries, fileID, rows)
	})
	if err != nil {
		return fmt.Errorf("replace chapters of file %q: %w", fileID, err)
	}

	return nil
}

// Limits of what is stored per file. maxChapterTitleRunes is the maxLength of the API's
// ItemChapter.title (JSON Schema counts characters, so the cut is per rune).
// maxChaptersPerFile bounds the rows a broken or hostile file can make the scan write; real
// audiobooks stay far below it.
const (
	maxChapterTitleRunes = 500
	maxChaptersPerFile   = 2000
)

// chapterRows validates chapters and converts them to rows in list order. It keeps the first
// maxChaptersPerFile chapters and cuts titles to maxChapterTitleRunes.
func chapterRows(fileID string, chapters []media.Chapter) ([]sqlcgen.InsertFileChapterParams, error) {
	if err := media.ValidateChapters(chapters); err != nil {
		return nil, fmt.Errorf("check chapters: %w", err)
	}

	chapters = chapters[:min(len(chapters), maxChaptersPerFile)]
	rows := make([]sqlcgen.InsertFileChapterParams, 0, len(chapters))

	for ordinal, chapter := range chapters {
		row := sqlcgen.InsertFileChapterParams{
			MediaFileID: fileID, Ordinal: int64(ordinal), Title: truncateRunes(chapter.Title, maxChapterTitleRunes),
			StartMs: chapter.Start.Milliseconds(), EndMs: chapter.End.Milliseconds(),
		}
		if row.EndMs <= row.StartMs {
			return nil, fmt.Errorf("chapter %d %q: shorter than a millisecond: %w", ordinal, row.Title, domain.ErrInvalid)
		}

		rows = append(rows, row)
	}

	return rows, nil
}

// truncateRunes returns text cut to at most limit runes, never inside a UTF-8 sequence.
func truncateRunes(text string, limit int) string {
	count := 0

	for index := range text {
		if count == limit {
			return text[:index]
		}

		count++
	}

	return text
}

func replaceChapterRows(ctx context.Context, queries *sqlcgen.Queries, fileID string, rows []sqlcgen.InsertFileChapterParams) error {
	found, err := queries.MediaFileExists(ctx, fileID)
	if err != nil {
		return fmt.Errorf("look up file: %w", err)
	}

	if !found {
		return fmt.Errorf("file: %w", domain.ErrNotFound)
	}

	if err := queries.DeleteFileChapters(ctx, fileID); err != nil {
		return fmt.Errorf("delete chapters: %w", err)
	}

	for _, row := range rows {
		if err := queries.InsertFileChapter(ctx, row); err != nil {
			return fmt.Errorf("insert chapter %d: %w", row.Ordinal, err)
		}
	}

	return nil
}

// ItemChapters returns the chapters of an item's media files in playback order on the
// item's whole timeline: the files are taken as consecutive parts in primary-file order
// (as ListItemFiles: part number, folder position, path), and the chapters of each file are
// shifted by the summed lengths of the files before it, including files without chapters.
// The length of a file is the larger of its probed duration and its last chapter end, so a
// file with an unknown (0) duration or chapters past its end never makes the next file's
// chapters overlap its own. This fits audiobooks; an item with alternative versions
// (milestone 4 video chapters) needs a per-file view. An item without chapters yields an
// empty list, an unknown item an error wrapping domain.ErrNotFound.
func (catalog *CatalogReader) ItemChapters(ctx context.Context, itemID string) ([]applibrary.ItemChapter, error) {
	chapters, err := readItemChapters(ctx, sqlcgen.New(catalog.reader), itemID)
	if err != nil {
		return nil, fmt.Errorf("read chapters of item %q: %w", itemID, err)
	}

	return chapters, nil
}

// readItemChapters is ItemChapters on the given queries, so the item detail reads the
// chapters in its own read transaction.
func readItemChapters(ctx context.Context, queries *sqlcgen.Queries, itemID string) ([]applibrary.ItemChapter, error) {
	rows, err := queries.ListItemChapters(ctx, itemID)
	if err != nil {
		return nil, fmt.Errorf("list chapters: %w", err)
	}

	if len(rows) == 0 {
		return nil, fmt.Errorf("item: %w", domain.ErrNotFound)
	}

	return itemChapters(rows), nil
}

// itemChapters turns the rows of ListItemChapters into chapters with work-relative offsets.
func itemChapters(rows []sqlcgen.ListItemChaptersRow) []applibrary.ItemChapter {
	chapters := make([]applibrary.ItemChapter, 0, len(rows))

	var (
		offset        time.Duration
		currentFileID string
		currentLength time.Duration
	)

	for _, row := range rows {
		if !row.FileID.Valid {
			continue
		}

		if row.FileID.String != currentFileID {
			offset += currentLength
			currentFileID, currentLength = row.FileID.String, milliseconds(row.DurationMs.Int64)
		}

		if !row.StartMs.Valid {
			continue
		}

		end := milliseconds(row.EndMs.Int64)
		currentLength = max(currentLength, end)
		chapters = append(chapters, applibrary.ItemChapter{
			Chapter: media.Chapter{
				Title: row.Title.String, FileRef: row.RelativePath.String,
				Start: offset + milliseconds(row.StartMs.Int64), End: offset + end,
			},
			FileID: currentFileID, FileStart: offset,
		})
	}

	return chapters
}

// audiobookDurations returns the summed file length of each listed audiobook; other kinds
// are skipped, and a page without audiobooks costs no query.
func audiobookDurations(ctx context.Context, queries *sqlcgen.Queries, listed []listedItem) (map[string]time.Duration, error) {
	var bookIDs []string

	for _, entry := range listed {
		if entry.item.Kind == library.ItemKindAudiobook.String() {
			bookIDs = append(bookIDs, entry.item.ID)
		}
	}

	if len(bookIDs) == 0 {
		return nil, nil
	}

	rows, err := queries.ListItemPartDurations(ctx, bookIDs)
	if err != nil {
		return nil, fmt.Errorf("read audiobook durations: %w", err)
	}

	durations := make(map[string]time.Duration, len(rows))
	for _, row := range rows {
		durations[row.ItemID] = milliseconds(row.DurationMs)
	}

	return durations, nil
}

func milliseconds(count int64) time.Duration {
	return time.Duration(count) * time.Millisecond
}
