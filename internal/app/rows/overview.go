package rows

import (
	"context"
	"fmt"

	appscan "github.com/magpeek/mivo/internal/app/scan"
	"github.com/magpeek/mivo/internal/domain/library"
	"github.com/magpeek/mivo/internal/domain/rows"
)

// target is a library whose rows are built, with what every row of it shares.
type target struct {
	library library.ManagedLibrary
	// offline are the folders that cannot be reached now; their items are left out.
	offline []string
	limit   int
}

// LibraryRows returns the rows of a library's Overview pivot in display order (see
// rows.OverviewKinds), leaving out rows without items, and the hint. A limit of 0 means
// rows.DefaultItemLimit; one outside 1 to MaxItemLimit wraps domain.ErrInvalid, an unknown
// library domain.ErrNotFound.
func (service *Service) LibraryRows(ctx context.Context, libraryID string, limit int) (LibraryRows, error) {
	limit, err := itemLimit(limit)
	if err != nil {
		return LibraryRows{}, fmt.Errorf("library rows of %q: %w", libraryID, err)
	}

	managed, err := service.libraries.VisibleLibrary(ctx, libraryID)
	if err != nil {
		return LibraryRows{}, fmt.Errorf("library rows of %q: %w", libraryID, err)
	}

	built, state, err := service.overview(ctx, service.targetOf(ctx, managed, limit))
	if err != nil {
		return LibraryRows{}, fmt.Errorf("library rows of %q: %w", libraryID, err)
	}

	return LibraryRows{LibraryID: libraryID, Rows: built, Hint: rows.LibraryHint(state)}, nil
}

func (service *Service) targetOf(ctx context.Context, managed library.ManagedLibrary, limit int) target {
	return target{library: managed, offline: service.catalog.OfflineFolders(ctx, managed.Folders), limit: limit}
}

// overview builds the Overview rows. A library without recently added items has no items at
// all, so it gets no further rows; a row its kind should have but that stays empty marks the
// library as lacking metadata.
func (service *Service) overview(ctx context.Context, source target) ([]Row, rows.LibraryState, error) {
	recent, err := service.recentlyAdded(ctx, source, rows.TitleRecentlyAdded, nil)
	if err != nil {
		return nil, rows.LibraryState{}, err
	}

	state := service.stateOf(source, recent)
	if !state.HasItems {
		return nil, state, nil
	}

	built := []Row{recent}

	for _, kind := range rows.OverviewKinds(source.library.Kind)[1:] {
		more, err := service.rowsOfKind(ctx, source, kind)
		if err != nil {
			return nil, rows.LibraryState{}, err
		}

		state.MissingMetadata = state.MissingMetadata || len(more) == 0
		built = append(built, more...)
	}

	return built, state, nil
}

// stateOf tells whether the library has items, judged by its "recentlyAdded" row, and
// whether it is being scanned.
func (service *Service) stateOf(source target, recent Row) rows.LibraryState {
	return rows.LibraryState{
		HasItems: len(recent.Items) > 0,
		Scanning: service.scans.Status(source.library.ID).State == appscan.StateRunning,
	}
}

// rowsOfKind builds the rows of one kind after "recentlyAdded"; none when they would be
// empty.
func (service *Service) rowsOfKind(ctx context.Context, source target, kind rows.Kind) ([]Row, error) {
	switch kind {
	case rows.KindNewReleases:
		return service.newReleases(ctx, source)
	case rows.KindMoreGenre:
		return service.genreRows(ctx, source)
	case rows.KindRecentEpisodes:
		return service.recentEpisodes(ctx, source)
	case rows.KindRecentlyAdded:
		return nil, nil
	default:
		return nil, nil
	}
}

// newRow returns a row of the library without items.
func newRow(source target, kind rows.Kind, titleKey string, seeAll library.ListingQuery) Row {
	return Row{
		ID: rows.RowID(kind, source.library.ID), Kind: kind, TitleKey: titleKey, LibraryID: source.library.ID,
		CardFormat: rows.CardFormatOf(kind, source.library.Kind), SeeAll: seeAll,
	}
}

// listedQuery returns the query of the library's listed kind in the given order.
func listedQuery(source target, sort library.ItemSort) library.ListingQuery {
	return library.ListingQuery{
		Filter: library.ListingFilter{Kind: rows.ListedItemKind(source.library.Kind)},
		Order:  library.ListingOrder{Sort: sort, Descending: true},
	}
}

// topLevelItems reads the items of a top-level row for the query, leaving out offline items.
func (service *Service) topLevelItems(ctx context.Context, source target, query library.ListingQuery) ([]Item, error) {
	query.Filter.OfflineRootIDs = source.offline

	summaries, err := service.rows.ListRowItems(ctx, ItemRequest{LibraryID: source.library.ID, Query: query, Limit: source.limit})
	if err != nil {
		return nil, fmt.Errorf("read row items: %w", err)
	}

	items := make([]Item, 0, len(summaries))
	for _, summary := range summaries {
		items = append(items, Item{Summary: summary})
	}

	return items, nil
}

// oneRow returns the row in a list, or none when it has no items.
func oneRow(row Row) []Row {
	if len(row.Items) == 0 {
		return nil
	}

	return []Row{row}
}
