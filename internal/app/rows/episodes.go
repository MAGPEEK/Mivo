package rows

import (
	"context"
	"fmt"

	applibrary "github.com/magpeek/mivo/internal/app/library"
	"github.com/magpeek/mivo/internal/domain/rows"
)

// The summaries a card needs: a grouped card its item and its show, an episode card the
// episode, its season and its show.
const (
	idsPerCard    = 2
	idsPerEpisode = 3
)

// episodeCards reads the recently added episodes of a shows library as show or season cards.
func (service *Service) episodeCards(ctx context.Context, source target) ([]Item, error) {
	groups, err := service.rows.ListEpisodeGroups(ctx, EpisodeRequest{
		LibraryID: source.library.ID, OfflineRootIDs: source.offline, Limit: source.limit, Window: rows.RecentEpisodeWindow,
	})
	if err != nil {
		return nil, fmt.Errorf("read episode groups: %w", err)
	}

	cards := rows.GroupEpisodes(groups, source.limit)

	ids := make([]string, 0, idsPerCard*len(cards))
	for _, card := range cards {
		ids = append(ids, card.ItemID, card.ShowID)
	}

	summaries, err := service.summariesByID(ctx, ids)
	if err != nil {
		return nil, err
	}

	items := make([]Item, 0, len(cards))

	for _, card := range cards {
		// An item deleted since the groups were read is left out.
		summary, found := summaries[card.ItemID]
		if !found {
			continue
		}

		item := Item{Summary: summary, NewEpisodeCount: card.NewEpisodeCount}
		if card.Season {
			show := summaries[card.ShowID]
			item.ShowTitle = show.Item.Title
			item.Summary.Poster = firstSet(summary.Poster, show.Poster)
		}

		items = append(items, item)
	}

	return items, nil
}

// episodeItems reads the episode cards of "recentEpisodes": each episode with its season
// number, its show's title and its show's poster.
func (service *Service) episodeItems(ctx context.Context, refs []EpisodeRef) ([]Item, error) {
	ids := make([]string, 0, idsPerEpisode*len(refs))
	for _, ref := range refs {
		ids = append(ids, ref.EpisodeID, ref.SeasonID, ref.ShowID)
	}

	summaries, err := service.summariesByID(ctx, ids)
	if err != nil {
		return nil, err
	}

	items := make([]Item, 0, len(refs))

	for _, ref := range refs {
		episode, found := summaries[ref.EpisodeID]
		if !found {
			continue
		}

		show := summaries[ref.ShowID]
		episode.Poster = show.Poster

		items = append(items, Item{Summary: episode, SeasonNumber: summaries[ref.SeasonID].Item.Number, ShowTitle: show.Item.Title})
	}

	return items, nil
}

// summariesByID reads the summaries of the items, each ID once.
func (service *Service) summariesByID(ctx context.Context, ids []string) (map[string]applibrary.ItemSummary, error) {
	unique := make([]string, 0, len(ids))
	seen := make(map[string]bool, len(ids))

	for _, id := range ids {
		if id != "" && !seen[id] {
			seen[id] = true
			unique = append(unique, id)
		}
	}

	byID := make(map[string]applibrary.ItemSummary, len(unique))
	if len(unique) == 0 {
		return byID, nil
	}

	summaries, err := service.summaries.ItemSummaries(ctx, unique)
	if err != nil {
		return nil, fmt.Errorf("read item summaries: %w", err)
	}

	for _, summary := range summaries {
		byID[summary.Item.ID] = summary
	}

	return byID, nil
}

// firstSet returns the first reference that is set.
func firstSet[T any](first, second *T) *T {
	if first != nil {
		return first
	}

	return second
}
