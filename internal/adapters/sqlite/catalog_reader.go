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
	"github.com/magpeek/mivo/internal/domain/library"
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

// ListItems returns one page of a library's top-level items (movies and shows) ordered by
// (sort title, ID), each with its summary data (see summariesOf). page.Limit is normalised by
// library.NormalizeLimit. page.Cursor is empty for the first page and otherwise the
// NextCursor of the previous page.
//
// An invalid limit or a cursor this reader did not issue wraps domain.ErrInvalid, an unknown
// library domain.ErrNotFound.
func (catalog *CatalogReader) ListItems(ctx context.Context, libraryID string, page library.PageRequest) (applibrary.ItemPage, error) {
	limit, err := library.NormalizeLimit(page.Limit)
	if err != nil {
		return applibrary.ItemPage{}, fmt.Errorf("list items of library %q: %w", libraryID, err)
	}

	after, err := decodeCursor(page.Cursor)
	if err != nil {
		return applibrary.ItemPage{}, fmt.Errorf("list items of library %q: %w", libraryID, err)
	}

	var result applibrary.ItemPage

	err = inReadTransaction(ctx, catalog.reader, func(queries *sqlcgen.Queries) error {
		var readErr error

		result, readErr = readTopLevelPage(ctx, queries, topLevelRequest{libraryID: libraryID, after: after, limit: limit})

		return readErr
	})
	if err != nil {
		return applibrary.ItemPage{}, fmt.Errorf("list items of library %q: %w", libraryID, err)
	}

	return result, nil
}

// GetItem returns an item with its detail: files and streams (the primary file first),
// metadata, ancestors, siblings, play target, artwork, extras and counts (see
// applibrary.ItemDetail). An unknown ID wraps domain.ErrNotFound.
func (catalog *CatalogReader) GetItem(ctx context.Context, itemID string) (applibrary.ItemDetail, error) {
	var detail applibrary.ItemDetail

	err := inReadTransaction(ctx, catalog.reader, func(queries *sqlcgen.Queries) error {
		var err error

		detail, err = readItemDetail(ctx, queries, itemID)

		return err
	})
	if err != nil {
		return applibrary.ItemDetail{}, fmt.Errorf("get item %q: %w", itemID, err)
	}

	return detail, nil
}

// PlayableFile returns a file with its streams and the root of its library. It also returns
// a file whose probe failed; the caller decides whether it can play. An unknown ID wraps
// domain.ErrNotFound.
func (catalog *CatalogReader) PlayableFile(ctx context.Context, fileID string) (appplayback.PlayableFile, error) {
	var playable appplayback.PlayableFile

	err := inReadTransaction(ctx, catalog.reader, func(queries *sqlcgen.Queries) error {
		row, err := queries.GetFileWithRoot(ctx, fileID)
		if errors.Is(err, sql.ErrNoRows) {
			return domain.ErrNotFound
		}

		if err != nil {
			return fmt.Errorf("read file: %w", err)
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
