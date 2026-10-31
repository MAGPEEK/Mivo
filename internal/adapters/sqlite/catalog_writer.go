package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	sqlitedriver "modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"

	"github.com/magpeek/mivo/internal/adapters/sqlite/sqlcgen"
	"github.com/magpeek/mivo/internal/app/scan"
	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/library"
)

// removeBatchSize bounds the IDs bound to one statement of RemoveFiles. SQLite accepts at
// most 32 766 parameters per statement; a vanished folder can hold more files than that.
const removeBatchSize = 500

// The writer is the catalog port of the scan use case.
var _ scan.CatalogWriter = (*CatalogWriter)(nil)

// CatalogWriter stores scan results (ADR 0022). Writes run in one transaction each on the
// writer pool; FileStates reads through the reader pool.
//
// It keeps the catalog's invariant that a file and its item belong to the same library:
// nothing in the schema enforces it, so every write checks it inside its transaction.
type CatalogWriter struct {
	writer *sql.DB
	reader *sqlcgen.Queries
}

// NewCatalogWriter returns a writer on db. db must be migrated before the first call.
func NewCatalogWriter(db *DB) *CatalogWriter {
	return &CatalogWriter{writer: db.writer, reader: sqlcgen.New(db.reader)}
}

// FileStates returns every stored file of a library ordered by relative path, for change
// detection. An unknown library has no files.
func (writer *CatalogWriter) FileStates(ctx context.Context, libraryID string) ([]scan.CatalogFile, error) {
	rows, err := writer.reader.ListFileStates(ctx, libraryID)
	if err != nil {
		return nil, fmt.Errorf("read file states of library %q: %w", libraryID, err)
	}

	files := make([]scan.CatalogFile, 0, len(rows))
	for _, row := range rows {
		files = append(files, scan.CatalogFile{
			ID:     row.ID,
			ItemID: row.ItemID,
			State: library.FileState{
				RelativePath: row.RelativePath,
				Size:         row.SizeBytes,
				ModifiedAt:   time.UnixMilli(row.ModifiedAt).UTC(),
			},
		})
	}

	return files, nil
}

// SaveProbedFile stores one probe result in one write transaction. A new file ID creates the
// file and, unless record.Item.ID is already stored in the same library, its item with
// created_at and updated_at from record.Item.CreatedAt. A stored file ID keeps its item
// (only record.Item.ID is used then) and gets the new path, size, time and probe result.
// The file's streams are replaced; a file whose probe failed is stored with its probe error
// and no streams. probed_at is record.ProbedAt. Times are stored in Unix milliseconds
// (ADR 0004). IDs come from the caller.
//
// It changes nothing and returns an error wrapping (the contract of
// scan.CatalogWriter.SaveProbedFile)
//   - domain.ErrInvalid for a record that breaks the rules of scan.ProbedFile (including a
//     zero ProbedAt, a zero CreatedAt of a new item, and a new item whose kind does not
//     follow from its library's kind, library.ItemKindForLibrary),
//   - domain.ErrNotFound for an unknown library,
//   - domain.ErrConflict when another file ID is stored at the same relative path of the
//     library (UNIQUE(library_id, relative_path)),
//   - domain.ErrInvalid when a stored file ID belongs to another item or library, or a
//     stored item to another library.
func (writer *CatalogWriter) SaveProbedFile(ctx context.Context, record scan.ProbedFile) error {
	if err := checkProbedFile(record); err != nil {
		return fmt.Errorf("save probed file %q: %w", record.File.ID, err)
	}

	err := inWriteTransaction(ctx, writer.writer, func(queries *sqlcgen.Queries) error {
		return catalogSave{queries: queries, record: record}.apply(ctx)
	})
	if err != nil {
		return fmt.Errorf("save probed file %q: %w", record.File.ID, err)
	}

	return nil
}

// RemoveFiles deletes the listed files of a library with their streams, and the items left
// without files, in one write transaction. IDs that are unknown or belong to another
// library are ignored, so a repeated call changes nothing and a library never loses a file
// of another one.
func (writer *CatalogWriter) RemoveFiles(ctx context.Context, libraryID string, fileIDs []string) error {
	if len(fileIDs) == 0 {
		return nil
	}

	err := inWriteTransaction(ctx, writer.writer, func(queries *sqlcgen.Queries) error {
		return removeFiles(ctx, queries, libraryID, fileIDs)
	})
	if err != nil {
		return fmt.Errorf("remove %d files of library %q: %w", len(fileIDs), libraryID, err)
	}

	return nil
}

// checkProbedFile rejects what would otherwise surface as a constraint error or as a file
// filed under the wrong item or library.
func checkProbedFile(record scan.ProbedFile) error {
	file := record.File

	switch {
	case file.ID == "":
		return fmt.Errorf("empty file id: %w", domain.ErrInvalid)
	case record.Item.ID == "":
		return fmt.Errorf("empty item id: %w", domain.ErrInvalid)
	case file.ItemID != record.Item.ID:
		return fmt.Errorf("file names item %q, record item is %q: %w", file.ItemID, record.Item.ID, domain.ErrInvalid)
	case file.LibraryID != record.Item.LibraryID:
		return fmt.Errorf("file is in library %q, its item in %q: %w",
			file.LibraryID, record.Item.LibraryID, domain.ErrInvalid)
	case record.ProbedAt.IsZero():
		// A zero time would be stored as year 1 and can only come from a caller that forgot it.
		return fmt.Errorf("zero probe time: %w", domain.ErrInvalid)
	}

	if _, err := library.ParseRelativePath(file.RelativePath); err != nil {
		return err
	}

	return file.Validate()
}

// catalogSave is one SaveProbedFile run inside its write transaction.
type catalogSave struct {
	queries *sqlcgen.Queries
	record  scan.ProbedFile
}

func (run catalogSave) apply(ctx context.Context) error {
	file := run.record.File

	owner, err := run.queries.GetFileOwner(ctx, file.ID)

	switch {
	case errors.Is(err, sql.ErrNoRows):
		err = run.ensureItem(ctx)
	case err != nil:
		err = fmt.Errorf("read stored file: %w", err)
	case owner.ItemID != file.ItemID || owner.LibraryID != file.LibraryID:
		err = fmt.Errorf("stored file belongs to item %q in library %q: %w",
			owner.ItemID, owner.LibraryID, domain.ErrInvalid)
	}

	if err != nil {
		return err
	}

	if err := run.queries.UpsertMediaFile(ctx, fileParams(file, run.record.ProbedAt.UnixMilli())); err != nil {
		if isUniqueViolation(err) {
			return fmt.Errorf("write file %q: another file is stored at this path: %w", file.RelativePath, domain.ErrConflict)
		}

		return fmt.Errorf("write file %q: %w", file.RelativePath, err)
	}

	return run.replaceStreams(ctx)
}

// ensureItem keeps a stored item of the file's library and creates a missing one.
func (run catalogSave) ensureItem(ctx context.Context) error {
	item := run.record.Item

	libraryID, err := run.queries.GetItemLibrary(ctx, item.ID)
	if errors.Is(err, sql.ErrNoRows) {
		return run.insertItem(ctx)
	}

	if err != nil {
		return fmt.Errorf("read stored item %q: %w", item.ID, err)
	}

	if libraryID != item.LibraryID {
		return fmt.Errorf("stored item %q belongs to library %q, not %q: %w",
			item.ID, libraryID, item.LibraryID, domain.ErrInvalid)
	}

	return nil
}

func (run catalogSave) insertItem(ctx context.Context) error {
	item := run.record.Item

	if err := run.checkNewItem(ctx); err != nil {
		return fmt.Errorf("new item %q: %w", item.ID, err)
	}

	createdAt := item.CreatedAt.UnixMilli()

	err := run.queries.InsertItem(ctx, sqlcgen.InsertItemParams{
		ID:        item.ID,
		LibraryID: item.LibraryID,
		Kind:      item.Kind.String(),
		Title:     item.Title,
		SortTitle: item.SortTitle,
		CreatedAt: createdAt,
		UpdatedAt: createdAt,
	})
	if err != nil {
		return fmt.Errorf("insert item %q: %w", item.ID, err)
	}

	return nil
}

// checkNewItem requires a title, a creation time, an existing library and the item kind
// that follows from the library's kind, which the startup sync keeps for every stored item.
func (run catalogSave) checkNewItem(ctx context.Context) error {
	item := run.record.Item

	if strings.TrimSpace(item.Title) == "" {
		return fmt.Errorf("empty title: %w", domain.ErrInvalid)
	}

	if item.CreatedAt.IsZero() {
		return fmt.Errorf("zero creation time: %w", domain.ErrInvalid)
	}

	storedKind, err := run.queries.GetLibraryKind(ctx, item.LibraryID)
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("library %q: %w", item.LibraryID, domain.ErrNotFound)
	}

	if err != nil {
		return fmt.Errorf("read library %q: %w", item.LibraryID, err)
	}

	libraryKind, err := library.ParseKind(storedKind)
	if err != nil {
		return undecodable(fmt.Sprintf("library %q: stored kind", item.LibraryID), err)
	}

	wantKind, err := library.ItemKindForLibrary(libraryKind)
	if err != nil {
		return fmt.Errorf("library %q: %w", item.LibraryID, err)
	}

	if item.Kind != wantKind {
		return fmt.Errorf("kind %q in a %s library, want %q: %w", item.Kind, libraryKind, wantKind, domain.ErrInvalid)
	}

	return nil
}

func (run catalogSave) replaceStreams(ctx context.Context) error {
	file := run.record.File

	if err := run.queries.DeleteFileStreams(ctx, file.ID); err != nil {
		return fmt.Errorf("delete streams of %q: %w", file.RelativePath, err)
	}

	for _, stream := range file.Description.Streams {
		if err := run.queries.InsertStream(ctx, streamParams(file.ID, stream)); err != nil {
			return fmt.Errorf("insert stream %d of %q: %w", stream.Index, file.RelativePath, err)
		}
	}

	return nil
}

func removeFiles(ctx context.Context, queries *sqlcgen.Queries, libraryID string, fileIDs []string) error {
	var itemIDs []string

	for batch := range slices.Chunk(fileIDs, removeBatchSize) {
		owners, err := queries.DeleteLibraryFiles(ctx, sqlcgen.DeleteLibraryFilesParams{
			LibraryID: libraryID,
			FileIds:   batch,
		})
		if err != nil {
			return fmt.Errorf("delete files: %w", err)
		}

		itemIDs = append(itemIDs, owners...)
	}

	slices.Sort(itemIDs)
	itemIDs = slices.Compact(itemIDs)

	for batch := range slices.Chunk(itemIDs, removeBatchSize) {
		if err := queries.DeleteItemsWithoutFiles(ctx, batch); err != nil {
			return fmt.Errorf("delete items without files: %w", err)
		}
	}

	return nil
}

// isUniqueViolation reports a UNIQUE constraint failure. The connections report extended
// result codes, so the code tells it apart from other constraint errors.
func isUniqueViolation(err error) bool {
	var driverError *sqlitedriver.Error

	return errors.As(err, &driverError) && driverError.Code() == sqlite3.SQLITE_CONSTRAINT_UNIQUE
}
