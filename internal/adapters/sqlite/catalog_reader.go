package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/magpeek/mivo/internal/adapters/sqlite/sqlcgen"
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
	_ appplayback.FileSource = (*CatalogReader)(nil)
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

// ListItems returns one page of a library's items ordered by (sort title, ID), each with its
// primary file and the file's streams; the streams of the whole page are read with one query.
// page.Limit is normalised by library.NormalizeLimit. page.Cursor is empty for the first page
// and otherwise the NextCursor of the previous page.
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

		result, readErr = itemPageQuery{libraryID: libraryID, after: after, limit: limit}.read(ctx, queries)

		return readErr
	})
	if err != nil {
		return applibrary.ItemPage{}, fmt.Errorf("list items of library %q: %w", libraryID, err)
	}

	return result, nil
}

// GetItem returns an item with all its files and their streams, the primary file first. An
// unknown ID wraps domain.ErrNotFound.
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

// itemPageQuery asks for the limit items of a library after a key.
type itemPageQuery struct {
	libraryID string
	after     itemKey
	limit     int
}

// read reads one row more than the limit to learn whether another page follows.
func (query itemPageQuery) read(ctx context.Context, queries *sqlcgen.Queries) (applibrary.ItemPage, error) {
	found, err := queries.LibraryExists(ctx, query.libraryID)
	if err != nil {
		return applibrary.ItemPage{}, fmt.Errorf("read library: %w", err)
	}

	if !found {
		return applibrary.ItemPage{}, domain.ErrNotFound
	}

	rows, err := queries.ListItemPage(ctx, sqlcgen.ListItemPageParams{
		LibraryID:      query.libraryID,
		AfterSortTitle: query.after.sortTitle,
		AfterID:        query.after.id,
		RowLimit:       int64(query.limit) + 1,
	})
	if err != nil {
		return applibrary.ItemPage{}, fmt.Errorf("read items: %w", err)
	}

	hasMore := len(rows) > query.limit
	rows = rows[:min(len(rows), query.limit)]

	items, err := itemSummaries(ctx, queries, rows)
	if err != nil {
		return applibrary.ItemPage{}, err
	}

	page := applibrary.ItemPage{Items: items}
	if hasMore {
		last := rows[len(rows)-1].Item
		page.NextCursor = encodeCursor(itemKey{sortTitle: last.SortTitle, id: last.ID})
	}

	return page, nil
}

func itemSummaries(ctx context.Context, queries *sqlcgen.Queries, rows []sqlcgen.ListItemPageRow) ([]applibrary.ItemSummary, error) {
	fileRows := make([]sqlcgen.MediaFile, 0, len(rows))
	for _, row := range rows {
		fileRows = append(fileRows, row.MediaFile)
	}

	files, err := filesWithStreams(ctx, queries, fileRows)
	if err != nil {
		return nil, err
	}

	summaries := make([]applibrary.ItemSummary, 0, len(rows))

	for index, row := range rows {
		item, err := itemFromRow(row.Item)
		if err != nil {
			return nil, err
		}

		summaries = append(summaries, applibrary.ItemSummary{Item: item, PrimaryFile: files[index]})
	}

	return summaries, nil
}

func readItemDetail(ctx context.Context, queries *sqlcgen.Queries, itemID string) (applibrary.ItemDetail, error) {
	row, err := queries.GetItem(ctx, itemID)
	if errors.Is(err, sql.ErrNoRows) {
		return applibrary.ItemDetail{}, domain.ErrNotFound
	}

	if err != nil {
		return applibrary.ItemDetail{}, fmt.Errorf("read item: %w", err)
	}

	item, err := itemFromRow(row)
	if err != nil {
		return applibrary.ItemDetail{}, err
	}

	fileRows, err := queries.ListItemFiles(ctx, itemID)
	if err != nil {
		return applibrary.ItemDetail{}, fmt.Errorf("read files: %w", err)
	}

	files, err := filesWithStreams(ctx, queries, fileRows)
	if err != nil {
		return applibrary.ItemDetail{}, err
	}

	return applibrary.ItemDetail{Item: item, Files: files}, nil
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
