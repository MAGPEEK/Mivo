package rows

import (
	"context"
	"fmt"

	appaccess "github.com/magpeek/mivo/internal/app/access"
	applibrary "github.com/magpeek/mivo/internal/app/library"
	appwatch "github.com/magpeek/mivo/internal/app/watch"
	"github.com/magpeek/mivo/internal/domain/access"
	"github.com/magpeek/mivo/internal/domain/library"
	"github.com/magpeek/mivo/internal/domain/rows"
)

// watchScope is what the watch rows of one request share: the library they are limited to
// (empty on Home), the viewer's restriction and the row item limit.
type watchScope struct {
	libraryID   string
	restriction access.Restriction
	limit       int
}

// watchRows builds the watch rows of the kinds for the profile of ctx, leaving out empty
// ones. Without a watch store or without a selected profile there are none and no error: the
// rest of the screen does not depend on them (fail closed: no profile, no personal rows).
func (service *Service) watchRows(ctx context.Context, kinds []rows.Kind, scope watchScope) ([]Row, error) {
	if service.watch == nil {
		return nil, nil
	}

	viewer, err := appaccess.ProfileViewerFrom(ctx)
	if err != nil {
		return nil, nil //nolint:nilerr // a session without a profile simply has no watch rows
	}

	var built []Row

	for _, kind := range kinds {
		row, err := service.watchRow(ctx, kind, viewer.ProfileID, scope)
		if err != nil {
			return nil, fmt.Errorf("%s row: %w", kind, err)
		}

		built = append(built, oneRow(row)...)
	}

	return built, nil
}

// watchRow builds one watch row; it has no "See all".
func (service *Service) watchRow(ctx context.Context, kind rows.Kind, profileID string, scope watchScope) (Row, error) {
	request := WatchRowRequest{ProfileID: profileID, LibraryID: scope.libraryID, Limit: rows.WatchRowLimit(kind, scope.limit)}

	var (
		ids []string
		err error
	)

	row := Row{ID: rows.WatchRowID(kind, scope.libraryID), Kind: kind, LibraryID: scope.libraryID, CardFormat: rows.CardFormatOf(kind, "")}

	if kind == rows.KindUpNext {
		row.TitleKey = rows.TitleUpNext
		ids, err = service.watch.UpNext(ctx, request)
	} else {
		row.TitleKey = rows.TitleContinueWatching
		ids, err = service.watch.ContinueWatching(ctx, request)
	}

	if err != nil {
		return Row{}, fmt.Errorf("read ids: %w", err)
	}

	row.Items, err = service.watchItems(ctx, scope.restriction, ids)
	if err != nil {
		return Row{}, err
	}

	return row, nil
}

// watchItems reads the cards of the IDs in their order; unknown and hidden items are left
// out, since the restricted SummaryReader does not return them. Episode cards get their
// season number, show title and show poster.
func (service *Service) watchItems(ctx context.Context, restriction access.Restriction, ids []string) ([]Item, error) {
	summaries, err := service.summariesByID(ctx, restriction, ids)
	if err != nil {
		return nil, err
	}

	parents, err := service.parentsOf(ctx, restriction, summaries)
	if err != nil {
		return nil, err
	}

	items := make([]Item, 0, len(ids))

	for _, id := range ids {
		if summary, found := summaries[id]; found {
			items = append(items, watchCard(summary, parents))
		}
	}

	if err := service.decorate(ctx, items); err != nil {
		return nil, err
	}

	return items, nil
}

// parentsOf reads the seasons and shows above the episodes of the summaries, in two reads.
func (service *Service) parentsOf(ctx context.Context, restriction access.Restriction, summaries map[string]applibrary.ItemSummary) (map[string]applibrary.ItemSummary, error) {
	seasons, err := service.summariesByID(ctx, restriction, episodeParents(summaries, nil))
	if err != nil {
		return nil, err
	}

	shows, err := service.summariesByID(ctx, restriction, episodeParents(summaries, seasons))
	if err != nil {
		return nil, err
	}

	for id, show := range shows {
		seasons[id] = show
	}

	return seasons, nil
}

// episodeParents returns the parent IDs of the summaries' episodes; with seasons given, the
// parents of those seasons instead (the shows).
func episodeParents(summaries, seasons map[string]applibrary.ItemSummary) []string {
	var parentIDs []string

	if seasons != nil {
		for _, season := range seasons {
			parentIDs = append(parentIDs, season.Item.ParentID)
		}

		return parentIDs
	}

	for _, summary := range summaries {
		if summary.Item.Kind == library.ItemKindEpisode {
			parentIDs = append(parentIDs, summary.Item.ParentID)
		}
	}

	return parentIDs
}

// watchCard returns the card of an item; an episode carries its season number, its show's
// title and its show's poster.
func watchCard(summary applibrary.ItemSummary, parents map[string]applibrary.ItemSummary) Item {
	item := Item{Summary: summary}
	if summary.Item.Kind != library.ItemKindEpisode {
		return item
	}

	season := parents[summary.Item.ParentID]
	show := parents[season.Item.ParentID]

	if season.Item.Kind == library.ItemKindShow { // an episode directly below its show
		season, show = applibrary.ItemSummary{}, season
	}

	item.SeasonNumber, item.ShowTitle = season.Item.Number, show.Item.Title
	item.Summary.Poster = firstSet(show.Poster, summary.Poster)

	return item
}

// decorate adds the viewer state to the cards; without a states port they stay plain.
func (service *Service) decorate(ctx context.Context, items []Item) error {
	if service.states == nil || len(items) == 0 {
		return nil
	}

	refs := make([]appwatch.ItemRef, 0, len(items))
	for _, item := range items {
		refs = append(refs, appwatch.ItemRef{ID: item.Summary.Item.ID, Kind: item.Summary.Item.Kind})
	}

	states, err := service.states.States(ctx, refs)
	if err != nil {
		return fmt.Errorf("viewer states: %w", err)
	}

	for index := range items {
		if state, found := states[items[index].Summary.Item.ID]; found {
			items[index].ViewerState = &state
		}
	}

	return nil
}
