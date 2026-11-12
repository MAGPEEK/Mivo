package sqlite

import (
	"context"
	"fmt"

	"github.com/magpeek/mivo/internal/adapters/sqlite/sqlcgen"
	"github.com/magpeek/mivo/internal/domain/media"
)

// ExternalSubtitlesOfFile returns the stored external subtitles of one media file in the
// order the scan stored them; empty for a file without any and for an unknown file. It does
// not check visibility: callers resolve the file with a restriction first.
func (catalog *CatalogReader) ExternalSubtitlesOfFile(ctx context.Context, fileID string) ([]media.ExternalSubtitle, error) {
	rows, err := sqlcgen.New(catalog.reader).ListFileExternalSubtitles(ctx, fileID)
	if err != nil {
		return nil, fmt.Errorf("list external subtitles of file %q: %w", fileID, err)
	}

	subtitles := make([]media.ExternalSubtitle, 0, len(rows))

	for _, row := range rows {
		format := media.SubtitleFormat(row.Format)
		subtitles = append(subtitles, media.ExternalSubtitle{
			RelativePath: row.RelativePath, Format: format, Language: row.Language, Title: row.Title,
			Forced: row.Forced == 1, SDH: row.Sdh == 1, Default: row.IsDefault == 1, TextBased: format.TextBased(),
		})
	}

	return subtitles, nil
}

// FileKnown reports whether a media file with the ID exists, whatever its visibility; the
// subtitle cache sweep uses it to drop the entries of deleted files.
func (catalog *CatalogReader) FileKnown(ctx context.Context, fileID string) (bool, error) {
	found, err := sqlcgen.New(catalog.reader).MediaFileExists(ctx, fileID)
	if err != nil {
		return false, fmt.Errorf("look up file %q: %w", fileID, err)
	}

	return found, nil
}
