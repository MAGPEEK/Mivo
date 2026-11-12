package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/magpeek/mivo/internal/adapters/sqlite/sqlcgen"
	"github.com/magpeek/mivo/internal/app/images"
	applibrary "github.com/magpeek/mivo/internal/app/library"
	appplayback "github.com/magpeek/mivo/internal/app/playback"
	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/access"
	"github.com/magpeek/mivo/internal/domain/media"
)

// The reader is the item port of the library catalog use cases and the file source of
// playback.
var (
	_ applibrary.ItemReader  = (*CatalogReader)(nil)
	_ applibrary.ChildReader = (*CatalogReader)(nil)
	_ appplayback.FileSource = (*CatalogReader)(nil)
	_ images.ArtworkSource   = (*CatalogReader)(nil)
)

// CatalogReader reads the media catalog for the library grid, the item detail and playback
// (ADR 0022). Each call reads in one transaction on the reader pool, so its queries see one
// snapshot even while a scan writes.
//
// Stored values the domain does not accept (an unknown kind, container or frame rate, an
// edited database or a downgrade) are reported as errors that do not wrap
// domain.ErrInvalid (see errUndecodableRow) instead of being handed to clients.
type CatalogReader struct {
	reader *sql.DB
}

// NewCatalogReader returns a reader on db. db must be migrated before the first call.
func NewCatalogReader(db *DB) *CatalogReader {
	return &CatalogReader{reader: db.reader}
}

// GetItem returns an item with its detail: files and streams (the primary file first),
// metadata, ancestors, siblings, play target, artwork, extras and counts (see
// applibrary.ItemDetail), as the restriction allows: siblings, play target, extras and counts
// cover visible items only. An unknown ID wraps domain.ErrNotFound, and so does an item the
// restriction hides, itself or through an ancestor; the zero restriction hides every item.
func (catalog *CatalogReader) GetItem(
	ctx context.Context, itemID string, restriction access.Restriction,
) (applibrary.ItemDetail, error) {
	var detail applibrary.ItemDetail

	err := inReadTransaction(ctx, catalog.reader, func(queries *sqlcgen.Queries) error {
		var err error

		detail, err = readItemDetail(ctx, queries, itemID, viewOf(restriction))

		return err
	})
	if err != nil {
		return applibrary.ItemDetail{}, fmt.Errorf("get item %q: %w", itemID, err)
	}

	return detail, nil
}

// PlayableFile returns a file with its streams and the root of its library. It also returns
// a file whose probe failed; the caller decides whether it can play. An unknown ID and a file
// whose item the restriction hides wrap domain.ErrNotFound; the zero restriction hides all.
func (catalog *CatalogReader) PlayableFile(ctx context.Context, fileID string, restriction access.Restriction) (appplayback.PlayableFile, error) {
	var playable appplayback.PlayableFile

	err := inReadTransaction(ctx, catalog.reader, func(queries *sqlcgen.Queries) error {
		row, err := queries.GetFileWithRoot(ctx, fileID)
		if errors.Is(err, sql.ErrNoRows) {
			return domain.ErrNotFound
		}

		if err != nil {
			return fmt.Errorf("read file: %w", err)
		}

		// The item of the file decides: hidden itself, through an ancestor or by its library, the file
		// answers like an unknown one.
		if _, err := viewOf(restriction).item(ctx, queries, row.MediaFile.ItemID); err != nil {
			return err
		}

		files, err := filesWithStreams(ctx, queries, []sqlcgen.MediaFile{row.MediaFile})
		if err != nil {
			return err
		}

		playable = appplayback.PlayableFile{File: files[0], LibraryRoot: row.RootPath}

		return nil
	})
	if err != nil {
		return appplayback.PlayableFile{}, fmt.Errorf("playable file %q: %w", fileID, err)
	}

	return playable, nil
}

// filesWithStreams loads the streams of all given files with one query and returns the
// files in the given order.
func filesWithStreams(ctx context.Context, queries *sqlcgen.Queries, fileRows []sqlcgen.MediaFile) ([]media.File, error) {
	fileIDs := make([]string, 0, len(fileRows))
	for _, row := range fileRows {
		fileIDs = append(fileIDs, row.ID)
	}

	streamRows, err := queries.ListStreamsOfFiles(ctx, fileIDs)
	if err != nil {
		return nil, fmt.Errorf("read streams: %w", err)
	}

	streamsByFile := make(map[string][]sqlcgen.MediaStream, len(fileRows))
	for _, stream := range streamRows {
		streamsByFile[stream.MediaFileID] = append(streamsByFile[stream.MediaFileID], stream)
	}

	files := make([]media.File, 0, len(fileRows))

	for _, row := range fileRows {
		file, err := fileFromRow(row, streamsByFile[row.ID])
		if err != nil {
			return nil, err
		}

		files = append(files, file)
	}

	return files, nil
}
