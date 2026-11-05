package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"maps"
	"slices"

	"github.com/magpeek/mivo/internal/adapters/sqlite/sqlcgen"
	applibrary "github.com/magpeek/mivo/internal/app/library"
	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/artwork"
	"github.com/magpeek/mivo/internal/domain/library"
	"github.com/magpeek/mivo/internal/domain/media"
)

const (
	// maxExtrasInDetail bounds the extras of an item detail (the API's maxItems).
	maxExtrasInDetail = 100
	// maxAncestorDepth stops walking up a parent chain that a damaged database made cyclic;
	// the real chain is episode, season, show.
	maxAncestorDepth = 8
)

// readItemDetail assembles an item detail with a handful of queries, independent of the size
// of the item's library.
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

	detail := applibrary.ItemDetail{Item: item}

	steps := []func(context.Context, *sqlcgen.Queries, sqlcgen.Item, *applibrary.ItemDetail) error{
		readDetailFiles, readDetailMetadata, readDetailAncestors, readDetailSiblings,
		readDetailPlayTarget, readDetailArtwork, readDetailExtras, readDetailCounts, readDetailChapters,
	}

	for _, step := range steps {
		if err := step(ctx, queries, row, &detail); err != nil {
			return applibrary.ItemDetail{}, err
		}
	}

	return detail, nil
}

func readDetailFiles(ctx context.Context, queries *sqlcgen.Queries, row sqlcgen.Item, detail *applibrary.ItemDetail) error {
	fileRows, err := queries.ListItemFiles(ctx, row.ID)
	if err != nil {
		return fmt.Errorf("read files: %w", err)
	}

	files, err := filesWithStreams(ctx, queries, fileRows)
	if err != nil {
		return err
	}

	detail.Files = files

	for index, file := range files {
		itemFile := applibrary.ItemFile{File: file, VersionLabel: fileRows[index].VersionLabel}
		if fileRows[index].PartNumber > 0 {
			itemFile.PartNumber = library.NumberOf(int(fileRows[index].PartNumber))
		}

		detail.ItemFiles = append(detail.ItemFiles, itemFile)
	}

	return nil
}

func readDetailAncestors(ctx context.Context, queries *sqlcgen.Queries, row sqlcgen.Item, detail *applibrary.ItemDetail) error {
	var chain []applibrary.ItemRef

	for parent := row.ParentID; parent.Valid; {
		if len(chain) >= maxAncestorDepth {
			return undecodable(fmt.Sprintf("item %q: parents", row.ID), errors.New("chain of parents too long"))
		}

		parentRow, err := queries.GetItem(ctx, parent.String)
		if errors.Is(err, sql.ErrNoRows) {
			// The foreign key makes this impossible unless the database was edited.
			return undecodable(fmt.Sprintf("item %q: parent %q", row.ID, parent.String), err)
		}

		if err != nil {
			return fmt.Errorf("read parent of item %q: %w", row.ID, err)
		}

		ancestor, err := itemFromRow(parentRow)
		if err != nil {
			return err
		}

		chain = append(chain, applibrary.ItemRef{ID: ancestor.ID, Kind: ancestor.Kind, Title: ancestor.Title, Number: seasonNumber(ancestor)})
		parent = parentRow.ParentID
	}

	slices.Reverse(chain)
	detail.Ancestors = chain

	return nil
}

// seasonNumber is the number of a season; every other kind has none in a reference.
func seasonNumber(item library.Item) library.OptionalNumber {
	if item.Kind != library.ItemKindSeason {
		return library.OptionalNumber{}
	}

	return item.Number
}

// childKeyOf mirrors the order group and number of the child queries for one stored item.
func childKeyOf(row sqlcgen.Item) childKey {
	key := childKey{group: numberedChildGroup, number: row.Number.Int64, sortTitle: row.SortTitle, id: row.ID}
	if !row.Number.Valid || (row.Kind == library.ItemKindSeason.String() && row.Number.Int64 == 0) {
		key.group = trailingChildGroup
	}

	return key
}

// readDetailSiblings finds the neighbours of a season or episode by asking for one child after
// its own key in each direction.
func readDetailSiblings(ctx context.Context, queries *sqlcgen.Queries, row sqlcgen.Item, detail *applibrary.ItemDetail) error {
	if !row.ParentID.Valid || (row.Kind != library.ItemKindSeason.String() && row.Kind != library.ItemKindEpisode.String()) {
		return nil
	}

	key := childKeyOf(row)
	params := sqlcgen.ListChildrenAscendingParams{
		ParentID: row.ParentID, HasAfter: storedTrue, AfterGroup: key.group, AfterNumber: key.number,
		AfterSortTitle: key.sortTitle, AfterID: key.id, RowLimit: 1,
	}

	next, err := queries.ListChildrenAscending(ctx, params)
	if err != nil {
		return fmt.Errorf("read next sibling: %w", err)
	}

	previous, err := queries.ListChildrenDescending(ctx, sqlcgen.ListChildrenDescendingParams(params))
	if err != nil {
		return fmt.Errorf("read previous sibling: %w", err)
	}

	if len(next) > 0 {
		detail.NextID = next[0].Item.ID
	}

	if len(previous) > 0 {
		detail.PreviousID = previous[0].Item.ID
	}

	return nil
}

// readDetailPlayTarget picks the item Play starts: a leaf plays itself, a season its first
// episode, a show the first episode of its first regular season that has one.
func readDetailPlayTarget(ctx context.Context, queries *sqlcgen.Queries, row sqlcgen.Item, detail *applibrary.ItemDetail) error {
	switch row.Kind {
	case library.ItemKindShow.String():
		first, err := queries.FirstEpisodeOfShow(ctx, sql.NullString{String: row.ID, Valid: true})
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("read first episode: %w", err)
		}

		detail.PlayItemID = first
	case library.ItemKindSeason.String():
		episodes, err := queries.ListChildrenAscending(ctx, sqlcgen.ListChildrenAscendingParams{
			ParentID: sql.NullString{String: row.ID, Valid: true}, RowLimit: 1,
		})
		if err != nil {
			return fmt.Errorf("read first episode: %w", err)
		}

		if len(episodes) > 0 {
			detail.PlayItemID = episodes[0].Item.ID
		}
	default:
		detail.PlayItemID = row.ID
	}

	return nil
}

func readDetailArtwork(ctx context.Context, queries *sqlcgen.Queries, row sqlcgen.Item, detail *applibrary.ItemDetail) error {
	itemIDs := []string{row.ID}
	// Only a season falls back to its show's poster.
	if row.ParentID.Valid && row.Kind == library.ItemKindSeason.String() {
		itemIDs = append(itemIDs, row.ParentID.String)
	}

	byItem, err := selectedArtworkByItem(ctx, queries, itemIDs)
	if err != nil {
		return err
	}

	detail.Artwork = withSeasonFallback(detail.Item.Kind, byItem[row.ID], byItem[row.ParentID.String])
	if detail.Artwork == nil {
		detail.Artwork = map[artwork.Kind]artwork.Ref{}
	}

	return nil
}

func readDetailExtras(ctx context.Context, queries *sqlcgen.Queries, row sqlcgen.Item, detail *applibrary.ItemDetail) error {
	extraRows, err := queries.ListExtras(ctx, sqlcgen.ListExtrasParams{
		ParentID: sql.NullString{String: row.ID, Valid: true}, RowLimit: maxExtrasInDetail,
	})
	if err != nil {
		return fmt.Errorf("read extras: %w", err)
	}

	listed := make([]listedItem, 0, len(extraRows))
	for _, extra := range extraRows {
		listed = append(listed, listedItem{item: extra})
	}

	detail.Extras, err = summariesOf(ctx, queries, listed, sqlcgen.Item{})

	return err
}

func readDetailCounts(ctx context.Context, queries *sqlcgen.Queries, row sqlcgen.Item, detail *applibrary.ItemDetail) error {
	counts, err := queries.GetItemCounts(ctx, row.ID)
	if err != nil {
		return fmt.Errorf("read counts: %w", err)
	}

	detail.ChildCount, detail.LeafCount = int(counts.ChildCount), int(counts.LeafCount)

	return nil
}

// readDetailChapters reads the chapters of an audiobook across its part files. Other kinds
// get none yet: their chapters need a per-file view (milestone 4, see ItemChapters).
func readDetailChapters(ctx context.Context, queries *sqlcgen.Queries, row sqlcgen.Item, detail *applibrary.ItemDetail) error {
	if row.Kind != library.ItemKindAudiobook.String() {
		return nil
	}

	chapters, err := readItemChapters(ctx, queries, row.ID)
	if err != nil {
		return fmt.Errorf("read chapters: %w", err)
	}

	detail.Chapters = chapters

	return nil
}

// primaryFilesByItem returns the primary file with streams of each given item that has one.
func primaryFilesByItem(ctx context.Context, queries *sqlcgen.Queries, itemIDs []string) (map[string]media.File, error) {
	rows, err := queries.ListPrimaryFiles(ctx, itemIDs)
	if err != nil {
		return nil, fmt.Errorf("read primary files: %w", err)
	}

	fileRows := make([]sqlcgen.MediaFile, 0, len(rows))
	for _, row := range rows {
		fileRows = append(fileRows, row.MediaFile)
	}

	files, err := filesWithStreams(ctx, queries, fileRows)
	if err != nil {
		return nil, err
	}

	byItem := make(map[string]media.File, len(files))
	for _, file := range files {
		byItem[file.ItemID] = file
	}

	return byItem, nil
}

// selectedArtworkByItem returns the selected artwork of the given items, by item and kind.
func selectedArtworkByItem(ctx context.Context, queries *sqlcgen.Queries, itemIDs []string) (map[string]map[artwork.Kind]artwork.Ref, error) {
	rows, err := queries.ListSelectedArtwork(ctx, itemIDs)
	if err != nil {
		return nil, fmt.Errorf("read artwork: %w", err)
	}

	byItem := map[string]map[artwork.Kind]artwork.Ref{}

	for _, row := range rows {
		kind, err := artwork.ParseKind(row.Kind)
		if err != nil {
			return nil, undecodable(fmt.Sprintf("artwork %q: stored kind", row.ID), err)
		}

		if byItem[row.ItemID] == nil {
			byItem[row.ItemID] = map[artwork.Kind]artwork.Ref{}
		}

		byItem[row.ItemID][kind] = artwork.Ref{ID: row.ID, Tag: row.Tag}
	}

	return byItem, nil
}

// withSeasonFallback returns own, plus the poster of the show for a season that has none.
func withSeasonFallback(kind library.ItemKind, own, show map[artwork.Kind]artwork.Ref) map[artwork.Kind]artwork.Ref {
	_, hasOwn := own[artwork.KindPoster]
	showPoster, hasShow := show[artwork.KindPoster]

	if kind != library.ItemKindSeason || hasOwn || !hasShow {
		return own
	}

	merged := make(map[artwork.Kind]artwork.Ref, len(own)+1)
	maps.Copy(merged, own)
	merged[artwork.KindPoster] = showPoster

	return merged
}
