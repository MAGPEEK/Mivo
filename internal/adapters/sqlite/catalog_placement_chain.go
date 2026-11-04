package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/magpeek/mivo/internal/adapters/sqlite/sqlcgen"
	"github.com/magpeek/mivo/internal/app/scan"
	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/library"
)

// chainRequest is the placement chain to upsert inside a write transaction.
type chainRequest struct {
	libraryID string
	placement library.Placement
	newID     func() (string, error)
	now       time.Time
}

// upsertChain finds or creates the items of the placement level by level, top-level item
// first, and returns their IDs. Existing items are left as they are.
func upsertChain(ctx context.Context, queries *sqlcgen.Queries, request chainRequest) ([]string, error) {
	if request.newID == nil {
		return nil, fmt.Errorf("no item id generator: %w", domain.ErrInvalid)
	}

	chain := make([]string, 0, len(request.placement.Levels))
	parentID := ""

	for _, level := range request.placement.Levels {
		itemID, err := upsertLevel(ctx, queries, request, levelAt{parentID: parentID, level: level})
		if err != nil {
			return nil, fmt.Errorf("%s %q: %w", level.Kind, level.MatchKey, err)
		}

		chain = append(chain, itemID)
		parentID = itemID
	}

	return chain, nil
}

// levelAt is a placement level together with the ID of the item above it ("" at the top).
type levelAt struct {
	parentID string
	level    library.Level
}

func upsertLevel(
	ctx context.Context, queries *sqlcgen.Queries, request chainRequest, at levelAt,
) (string, error) {
	parentID, level := at.parentID, at.level

	itemID, err := queries.FindChainItem(ctx, sqlcgen.FindChainItemParams{
		LibraryID: request.libraryID, ParentID: sql.NullString{String: parentID, Valid: true}, Kind: level.Kind.String(), MatchKey: level.MatchKey,
	})
	if err == nil {
		return itemID, nil
	}

	if !errors.Is(err, sql.ErrNoRows) {
		return "", fmt.Errorf("find item: %w", err)
	}

	itemID, err = request.newID()
	if err != nil {
		return "", fmt.Errorf("create item id: %w", err)
	}

	if err := queries.InsertChainItem(ctx, newChainItem(itemID, parentID, request, level)); err != nil {
		return "", fmt.Errorf("insert item: %w", err)
	}

	return itemID, nil
}

func newChainItem(itemID, parentID string, request chainRequest, level library.Level) sqlcgen.InsertChainItemParams {
	sortTitle := library.SortTitle(level.Title)
	if sortTitle == "" {
		sortTitle = level.Title
	}

	created := request.now.UnixMilli()
	year, _ := level.Year.Get()
	params := sqlcgen.InsertChainItemParams{
		ID: itemID, LibraryID: request.libraryID, ParentID: sql.NullString{String: parentID, Valid: parentID != ""},
		Kind: level.Kind.String(), MatchKey: level.MatchKey, Title: level.Title, SortTitle: sortTitle,
		Year: int64(year), Number: nullInt(level.Number), AbsoluteNumber: nullInt(level.AbsoluteNumber),
		ExtraType: sql.NullString{String: level.ExtraType.String(), Valid: level.ExtraType != ""},
		CreatedAt: created, UpdatedAt: created,
	}

	// The schema wants number_end > number; a "range" of one episode is no range.
	start, _ := level.Number.Get()
	if end, ok := level.NumberEnd.Get(); ok && end > start {
		params.NumberEnd = sql.NullInt64{Int64: int64(end), Valid: true}
	}

	return params
}

func nullInt(number library.OptionalNumber) sql.NullInt64 {
	value, ok := number.Get()

	return sql.NullInt64{Int64: int64(value), Valid: ok}
}

// chainIndex finds the ancestors of an item in the items of one library.
type chainIndex map[string]sqlcgen.ListLibraryChainItemsRow

func newChainIndex(items []sqlcgen.ListLibraryChainItemsRow) chainIndex {
	index := make(chainIndex, len(items))
	for _, item := range items {
		index[item.ID] = item
	}

	return index
}

// chainOf returns the IDs and the placement levels (kind and match key) from the top-level
// item down to the item.
func (index chainIndex) chainOf(itemID string) (ids []string, levels []library.Level, err error) {
	for current, steps := itemID, 0; current != ""; steps++ {
		item, ok := index[current]
		if !ok || steps > len(index) {
			return nil, nil, undecodable(fmt.Sprintf("item %q", itemID), errors.New("broken parent chain"))
		}

		kind, err := library.ParseItemKind(item.Kind)
		if err != nil {
			return nil, nil, undecodable(fmt.Sprintf("item %q: stored kind", item.ID), err)
		}

		ids = append([]string{item.ID}, ids...)
		levels = append([]library.Level{{Kind: kind, MatchKey: item.MatchKey}}, levels...)
		current = item.ParentID.String
	}

	return ids, levels, nil
}

func placedStates(files []sqlcgen.ListPlacedFilesRow, index chainIndex) ([]scan.PlacedFileState, error) {
	states := make([]scan.PlacedFileState, 0, len(files))

	for _, file := range files {
		ids, levels, err := index.chainOf(file.ItemID)
		if err != nil {
			return nil, fmt.Errorf("file %q: %w", file.RelativePath, err)
		}

		placement := library.Placement{Levels: levels, VersionLabel: file.VersionLabel}
		if file.PartNumber > 0 {
			placement.PartNumber = library.NumberOf(int(file.PartNumber))
		}

		states = append(states, scan.PlacedFileState{
			ID:           file.ID,
			PlacementKey: placement.Key(),
			ItemIDs:      ids,
			State: library.FileState{
				RelativePath: file.RelativePath,
				Class:        library.FileClassVideo,
				Size:         file.SizeBytes,
				ModifiedAt:   time.UnixMilli(file.ModifiedAt).UTC(),
			},
		})
	}

	return states, nil
}
