package sqlite

import (
	"context"
	"fmt"

	"github.com/magpeek/mivo/internal/adapters/sqlite/sqlcgen"
	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/media"
)

// ReplaceExternalSubtitles stores subtitles as the whole set of external subtitles of the
// media file fileID in one transaction; an empty list removes the stored ones. The order of
// the list is kept. Errors wrap domain.ErrNotFound for an unknown file and domain.ErrInvalid
// for two subtitles with the same path; either way nothing changes.
func (catalog *LibraryCatalog) ReplaceExternalSubtitles(ctx context.Context, fileID string, subtitles []media.ExternalSubtitle) error {
	err := inWriteTransaction(ctx, catalog.writer, func(queries *sqlcgen.Queries) error {
		return replaceSubtitleRows(ctx, queries, fileID, subtitles)
	})
	if err != nil {
		return fmt.Errorf("replace external subtitles of file %q: %w", fileID, err)
	}

	return nil
}

func replaceSubtitleRows(ctx context.Context, queries *sqlcgen.Queries, fileID string, subtitles []media.ExternalSubtitle) error {
	found, err := queries.MediaFileExists(ctx, fileID)
	if err != nil {
		return fmt.Errorf("look up file: %w", err)
	}

	if !found {
		return fmt.Errorf("file: %w", domain.ErrNotFound)
	}

	if err := queries.DeleteFileExternalSubtitles(ctx, fileID); err != nil {
		return fmt.Errorf("delete subtitles: %w", err)
	}

	for ordinal, subtitle := range subtitles {
		err := queries.InsertExternalSubtitle(ctx, subtitleRow(fileID, ordinal, subtitle))
		if isPrimaryKeyViolation(err) || isUniqueViolation(err) {
			return fmt.Errorf("subtitle %q listed twice: %w", subtitle.RelativePath, domain.ErrInvalid)
		}

		if err != nil {
			return fmt.Errorf("insert subtitle %q: %w", subtitle.RelativePath, err)
		}
	}

	return nil
}

func subtitleRow(fileID string, ordinal int, subtitle media.ExternalSubtitle) sqlcgen.InsertExternalSubtitleParams {
	return sqlcgen.InsertExternalSubtitleParams{
		MediaFileID: fileID, TrackID: subtitle.TrackID(), Ordinal: int64(ordinal),
		RelativePath: subtitle.RelativePath, Format: string(subtitle.Format), Language: subtitle.Language,
		Title: subtitle.Title, Forced: flag(subtitle.Forced), Sdh: flag(subtitle.SDH), IsDefault: flag(subtitle.Default),
	}
}

func flag(value bool) int64 {
	if value {
		return 1
	}

	return 0
}

// ExternalSubtitles returns the stored external subtitles of the library's files by file ID,
// each file's list in the order ReplaceExternalSubtitles received it. Files without
// subtitles are absent.
func (catalog *LibraryCatalog) ExternalSubtitles(ctx context.Context, libraryID string) (map[string][]media.ExternalSubtitle, error) {
	rows, err := sqlcgen.New(catalog.reader).ListLibraryExternalSubtitles(ctx, libraryID)
	if err != nil {
		return nil, fmt.Errorf("list external subtitles of library %q: %w", libraryID, err)
	}

	result := map[string][]media.ExternalSubtitle{}

	for _, row := range rows {
		format := media.SubtitleFormat(row.Format)
		result[row.MediaFileID] = append(result[row.MediaFileID], media.ExternalSubtitle{
			RelativePath: row.RelativePath, Format: format, Language: row.Language, Title: row.Title,
			Forced: row.Forced == 1, SDH: row.Sdh == 1, Default: row.IsDefault == 1, TextBased: format.TextBased(),
		})
	}

	return result, nil
}
