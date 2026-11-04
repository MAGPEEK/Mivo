package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/magpeek/mivo/internal/adapters/sqlite/sqlcgen"
	"github.com/magpeek/mivo/internal/app/scan"
	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/library"
	"github.com/magpeek/mivo/internal/domain/media"
)

// PlacedFiles returns every media file of the library ordered by relative path, with the
// placement key built from the item chain it is stored under. An unknown library has no files.
func (catalog *LibraryCatalog) PlacedFiles(ctx context.Context, libraryID string) ([]scan.PlacedFileState, error) {
	var (
		files []sqlcgen.ListPlacedFilesRow
		items []sqlcgen.ListLibraryChainItemsRow
	)

	err := inReadTransaction(ctx, catalog.reader, func(queries *sqlcgen.Queries) error {
		var err error

		if files, err = queries.ListPlacedFiles(ctx, libraryID); err != nil {
			return fmt.Errorf("list files: %w", err)
		}

		if items, err = queries.ListLibraryChainItems(ctx, libraryID); err != nil {
			return fmt.Errorf("list items: %w", err)
		}

		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("read placed files of library %q: %w", libraryID, err)
	}

	states, err := placedStates(files, newChainIndex(items))
	if err != nil {
		return nil, fmt.Errorf("read placed files of library %q: %w", libraryID, err)
	}

	return states, nil
}

// SavePlacedFile stores one probe result and upserts the item chain of its placement in one
// write transaction (the contract of scan.PlacedFileCatalog.SavePlacedFile). Items are
// created with the title, sort title, year and numbers of the placement; existing items keep
// theirs. A part number of 0 is stored like an absent one.
func (catalog *LibraryCatalog) SavePlacedFile(
	ctx context.Context, record scan.PlacedFile, newID func() (string, error),
) ([]string, error) {
	if err := checkPlacedFile(record); err != nil {
		return nil, fmt.Errorf("save placed file %q: %w", record.File.RelativePath, err)
	}

	var chain []string

	err := inWriteTransaction(ctx, catalog.writer, func(queries *sqlcgen.Queries) error {
		var err error

		chain, err = placedSave{queries: queries, record: record, newID: newID, now: catalog.now()}.apply(ctx)

		return err
	})
	if err != nil {
		return nil, fmt.Errorf("save placed file %q: %w", record.File.RelativePath, err)
	}

	return chain, nil
}

// MoveFile re-parents a file under the chain of the new placement in one write transaction
// and removes the former ancestors that end up empty. Streams stay as they are.
func (catalog *LibraryCatalog) MoveFile(ctx context.Context, request scan.MoveRequest) ([]string, error) {
	if err := request.Placement.Validate(); err != nil {
		return nil, fmt.Errorf("move file %q: %w", request.FileID, err)
	}

	var chain []string

	err := inWriteTransaction(ctx, catalog.writer, func(queries *sqlcgen.Queries) error {
		var err error

		chain, err = moveInTransaction(ctx, queries, request, catalog.now())

		return err
	})
	if err != nil {
		return nil, fmt.Errorf("move file %q: %w", request.FileID, err)
	}

	return chain, nil
}

// RemovePlacedFiles deletes the files and then every item left without files, bottom-up, in
// one write transaction. IDs that are unknown or belong to another library are ignored.
func (catalog *LibraryCatalog) RemovePlacedFiles(ctx context.Context, libraryID string, fileIDs []string) error {
	if len(fileIDs) == 0 {
		return nil
	}

	err := inWriteTransaction(ctx, catalog.writer, func(queries *sqlcgen.Queries) error {
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

		return pruneItems(ctx, queries, itemIDs)
	})
	if err != nil {
		return fmt.Errorf("remove %d placed files of library %q: %w", len(fileIDs), libraryID, err)
	}

	return nil
}

// checkPlacedFile rejects what would otherwise surface as a constraint error.
func checkPlacedFile(record scan.PlacedFile) error {
	file := record.File

	switch {
	case file.ID == "":
		return fmt.Errorf("empty file id: %w", domain.ErrInvalid)
	case file.LibraryID == "":
		return fmt.Errorf("empty library id: %w", domain.ErrInvalid)
	case record.ProbedAt.IsZero():
		// A zero time would be stored as year 1 and can only come from a caller that forgot it.
		return fmt.Errorf("zero probe time: %w", domain.ErrInvalid)
	}

	if err := record.Placement.Validate(); err != nil {
		return err
	}

	if _, err := library.ParseRelativePath(file.RelativePath); err != nil {
		return err
	}

	return file.Validate()
}

// placedSave is one SavePlacedFile run inside its write transaction.
type placedSave struct {
	queries *sqlcgen.Queries
	record  scan.PlacedFile
	newID   func() (string, error)
	now     time.Time
}

func (run placedSave) apply(ctx context.Context) ([]string, error) {
	file := run.record.File

	found, err := run.queries.LibraryExists(ctx, file.LibraryID)
	if err != nil {
		return nil, fmt.Errorf("read library %q: %w", file.LibraryID, err)
	}

	if !found {
		return nil, fmt.Errorf("library %q: %w", file.LibraryID, domain.ErrNotFound)
	}

	if err := requireLibraryRoot(ctx, run.queries, file.LibraryID, run.record.RootID); err != nil {
		return nil, err
	}

	formerItemID, err := run.formerItem(ctx)
	if err != nil {
		return nil, err
	}

	chain, err := upsertChain(ctx, run.queries, chainRequest{
		libraryID: file.LibraryID, placement: run.record.Placement, newID: run.newID, now: run.now,
	})
	if err != nil {
		return nil, err
	}

	file.ItemID = chain[len(chain)-1]

	if err := run.writeFile(ctx, file); err != nil {
		return nil, err
	}

	if formerItemID != "" && formerItemID != file.ItemID {
		return chain, pruneItems(ctx, run.queries, []string{formerItemID})
	}

	return chain, nil
}

// formerItem returns the item a stored file belongs to, "" for a new file.
func (run placedSave) formerItem(ctx context.Context) (string, error) {
	file := run.record.File

	owner, err := run.queries.GetFileOwner(ctx, file.ID)

	switch {
	case errors.Is(err, sql.ErrNoRows):
		return "", nil
	case err != nil:
		return "", fmt.Errorf("read stored file: %w", err)
	case owner.LibraryID != file.LibraryID:
		return "", fmt.Errorf("stored file belongs to library %q: %w", owner.LibraryID, domain.ErrInvalid)
	}

	return owner.ItemID, nil
}

func (run placedSave) writeFile(ctx context.Context, file media.File) error {
	base := fileParams(file, run.record.ProbedAt.UnixMilli())
	params := sqlcgen.UpsertPlacedFileParams{
		ID: base.ID, ItemID: base.ItemID, LibraryID: base.LibraryID, RootID: run.record.RootID, RelativePath: base.RelativePath,
		SizeBytes: base.SizeBytes, ModifiedAt: base.ModifiedAt, Container: base.Container,
		FormatName: base.FormatName, DurationMs: base.DurationMs, BitRate: base.BitRate,
		ProbedAt: base.ProbedAt, ProbeError: base.ProbeError,
		VersionLabel: run.record.Placement.VersionLabel,
		PartNumber:   partNumberValue(run.record.Placement),
	}

	if err := run.queries.UpsertPlacedFile(ctx, params); err != nil {
		if isUniqueViolation(err) {
			return fmt.Errorf("write file %q: another file is stored at this path: %w", file.RelativePath, domain.ErrConflict)
		}

		return fmt.Errorf("write file %q: %w", file.RelativePath, err)
	}

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

func moveInTransaction(
	ctx context.Context, queries *sqlcgen.Queries, request scan.MoveRequest, now time.Time,
) ([]string, error) {
	owner, err := queries.GetFileOwner(ctx, request.FileID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("read stored file: %w", err)
	}

	if err != nil || owner.LibraryID != request.LibraryID {
		return nil, fmt.Errorf("file in library %q: %w", request.LibraryID, domain.ErrNotFound)
	}

	chain, err := upsertChain(ctx, queries, chainRequest{
		libraryID: request.LibraryID, placement: request.Placement, newID: request.NewID, now: now,
	})
	if err != nil {
		return nil, err
	}

	leafID := chain[len(chain)-1]

	_, err = queries.UpdateFilePlacement(ctx, sqlcgen.UpdateFilePlacementParams{
		ItemID: leafID, VersionLabel: request.Placement.VersionLabel,
		PartNumber: partNumberValue(request.Placement), ID: request.FileID, LibraryID: request.LibraryID,
	})
	if err != nil {
		return nil, fmt.Errorf("update file placement: %w", err)
	}

	if owner.ItemID != leafID {
		return chain, pruneItems(ctx, queries, []string{owner.ItemID})
	}

	return chain, nil
}

// pruneItems deletes every listed item that owns no file and no child but extras, then walks
// up and does the same to the parents, so an emptied show disappears with its last episode.
func pruneItems(ctx context.Context, queries *sqlcgen.Queries, itemIDs []string) error {
	slices.Sort(itemIDs)

	for _, start := range slices.Compact(itemIDs) {
		for itemID := start; itemID != ""; {
			parent, err := queries.DeleteItemIfUnused(ctx, itemID)
			if errors.Is(err, sql.ErrNoRows) {
				break
			}

			if err != nil {
				return fmt.Errorf("delete unused item %q: %w", itemID, err)
			}

			itemID = parent.String
		}
	}

	return nil
}

// partNumberValue is the stored part number: 0 for a file that is not part of a multi-part
// version.
func partNumberValue(placement library.Placement) int64 {
	number, _ := placement.PartNumber.Get()

	return int64(number)
}
