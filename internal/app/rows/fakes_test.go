package rows_test

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	applibrary "github.com/magpeek/mivo/internal/app/library"
	approws "github.com/magpeek/mivo/internal/app/rows"
	appscan "github.com/magpeek/mivo/internal/app/scan"
	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/artwork"
	"github.com/magpeek/mivo/internal/domain/library"
	"github.com/magpeek/mivo/internal/domain/rows"
)

// fakeItem is a catalog item of the in-memory catalog with what the rows filter and sort by.
type fakeItem struct {
	summary     applibrary.ItemSummary
	genres      []string
	releaseDate string
	folderID    string
	broken      bool
	// seasonID and showID place an episode; airDate is its release date.
	seasonID, showID string
	backdrop         bool
	summaryText      string
}

func (item fakeItem) available(offline []string) bool {
	return !item.broken && !slices.Contains(offline, item.folderID)
}

// fakeCatalog implements every port of the rows service in memory.
type fakeCatalog struct {
	libraries []library.ManagedLibrary
	items     []fakeItem
	offline   []string
	scanning  map[string]bool
	failWith  error
	// requests records the ListRowItems requests.
	requests []approws.ItemRequest
}

func (catalog *fakeCatalog) VisibleLibraries(context.Context) ([]library.ManagedLibrary, error) {
	return catalog.libraries, catalog.failWith
}

func (catalog *fakeCatalog) VisibleLibrary(_ context.Context, id string) (library.ManagedLibrary, error) {
	for _, managed := range catalog.libraries {
		if managed.ID == id {
			return managed, nil
		}
	}

	return library.ManagedLibrary{}, fmt.Errorf("library %q: %w", id, domain.ErrNotFound)
}

func (catalog *fakeCatalog) ListRowItems(_ context.Context, request approws.ItemRequest) ([]applibrary.ItemSummary, error) {
	catalog.requests = append(catalog.requests, request)

	if catalog.failWith != nil {
		return nil, catalog.failWith
	}

	matches := slices.DeleteFunc(slices.Clone(catalog.items), func(item fakeItem) bool {
		return !catalog.matches(item, request)
	})

	slices.SortFunc(matches, orderOf(request.Query.Order))

	summaries := make([]applibrary.ItemSummary, 0, len(matches))
	for _, item := range matches[:min(request.Limit, len(matches))] {
		summaries = append(summaries, item.summary)
	}

	return summaries, nil
}

func (catalog *fakeCatalog) matches(item fakeItem, request approws.ItemRequest) bool {
	filter := request.Query.Filter
	genreMatch := len(filter.Genres) == 0 || slices.ContainsFunc(item.genres, func(genre string) bool {
		return strings.EqualFold(genre, filter.Genres[0])
	})

	return item.summary.Item.LibraryID == request.LibraryID && item.summary.Item.Kind == filter.Kind &&
		item.available(filter.OfflineRootIDs) && genreMatch &&
		(request.Query.Order.Sort != library.SortByReleaseDate || item.releaseDate != "")
}

func orderOf(order library.ListingOrder) func(left, right fakeItem) int {
	return func(left, right fakeItem) int {
		if order.Sort == library.SortByReleaseDate {
			return strings.Compare(right.releaseDate, left.releaseDate)
		}

		if order.Sort == library.SortByRandom {
			// A seeded order is enough for the fake: IDs ascending for an even seed, else descending.
			if order.Seed%2 == 0 {
				return strings.Compare(left.summary.Item.ID, right.summary.Item.ID)
			}

			return strings.Compare(right.summary.Item.ID, left.summary.Item.ID)
		}

		return right.summary.Item.CreatedAt.Compare(left.summary.Item.CreatedAt)
	}
}

// episodesOf returns the available episodes of the library.
func (catalog *fakeCatalog) episodesOf(request approws.EpisodeRequest) []fakeItem {
	return slices.DeleteFunc(slices.Clone(catalog.items), func(item fakeItem) bool {
		return item.summary.Item.Kind != library.ItemKindEpisode || item.summary.Item.LibraryID != request.LibraryID ||
			!item.available(request.OfflineRootIDs)
	})
}

func (catalog *fakeCatalog) ListEpisodeGroups(_ context.Context, request approws.EpisodeRequest) ([]rows.EpisodeGroup, error) {
	newest := map[string]time.Time{}

	for _, episode := range catalog.episodesOf(request) {
		if added := episode.summary.Item.CreatedAt; added.After(newest[episode.showID]) {
			newest[episode.showID] = added
		}
	}

	shows := make([]string, 0, len(newest))
	for show := range newest {
		shows = append(shows, show)
	}

	slices.SortFunc(shows, func(left, right string) int { return newest[right].Compare(newest[left]) })
	shows = shows[:min(request.Limit, len(shows))]

	groups := map[string]*rows.EpisodeGroup{}

	for _, episode := range catalog.episodesOf(request) {
		added := episode.summary.Item.CreatedAt
		if !slices.Contains(shows, episode.showID) || added.Before(newest[episode.showID].Add(-request.Window)) {
			continue
		}

		group, found := groups[episode.seasonID]
		if !found {
			group = &rows.EpisodeGroup{ShowID: episode.showID, SeasonID: episode.seasonID}
			groups[episode.seasonID] = group
		}

		group.EpisodeCount++
		if added.After(group.NewestAdded) {
			group.NewestAdded = added
		}
	}

	result := make([]rows.EpisodeGroup, 0, len(groups))
	for _, group := range groups {
		result = append(result, *group)
	}

	return result, nil
}

func (catalog *fakeCatalog) ListRecentEpisodes(_ context.Context, request approws.EpisodeRequest) ([]approws.EpisodeRef, error) {
	episodes := slices.DeleteFunc(catalog.episodesOf(request), func(item fakeItem) bool { return item.releaseDate == "" })
	slices.SortFunc(episodes, func(left, right fakeItem) int { return strings.Compare(right.releaseDate, left.releaseDate) })

	refs := make([]approws.EpisodeRef, 0, len(episodes))
	for _, episode := range episodes[:min(request.Limit, len(episodes))] {
		refs = append(refs, approws.EpisodeRef{EpisodeID: episode.summary.Item.ID, SeasonID: episode.seasonID, ShowID: episode.showID})
	}

	return refs, nil
}

func (catalog *fakeCatalog) ItemSummaries(_ context.Context, ids []string) ([]applibrary.ItemSummary, error) {
	var summaries []applibrary.ItemSummary

	for _, id := range ids {
		if item, found := catalog.item(id); found {
			summaries = append(summaries, item.summary)
		}
	}

	return summaries, nil
}

func (catalog *fakeCatalog) item(id string) (fakeItem, bool) {
	index := slices.IndexFunc(catalog.items, func(item fakeItem) bool { return item.summary.Item.ID == id })
	if index < 0 {
		return fakeItem{}, false
	}

	return catalog.items[index], true
}

func (catalog *fakeCatalog) ListFacets(_ context.Context, libraryID string, _ applibrary.FacetRequest) ([]applibrary.Facet, error) {
	counts := map[string]int64{}

	for _, item := range catalog.items {
		if item.summary.Item.LibraryID == libraryID && item.summary.Item.ParentID == "" {
			for _, genre := range item.genres {
				counts[genre]++
			}
		}
	}

	facet := applibrary.Facet{Field: library.FacetGenre}
	for genre, count := range counts {
		facet.Values = append(facet.Values, applibrary.FacetValue{Value: genre, Count: count})
	}

	slices.SortFunc(facet.Values, func(left, right applibrary.FacetValue) int { return cmp.Compare(left.Value, right.Value) })

	return []applibrary.Facet{facet}, nil
}

func (catalog *fakeCatalog) GetItem(_ context.Context, id string) (applibrary.ItemDetail, error) {
	item, found := catalog.item(id)
	if !found {
		return applibrary.ItemDetail{}, fmt.Errorf("item %q: %w", id, domain.ErrNotFound)
	}

	detail := applibrary.ItemDetail{Item: item.summary.Item, Artwork: map[artwork.Kind]artwork.Ref{}}
	detail.Metadata.Summary = item.summaryText

	if item.backdrop {
		detail.Artwork[artwork.KindBackdrop] = artwork.Ref{ID: "backdrop-" + id, Tag: "t"}
	}

	return detail, nil
}

func (catalog *fakeCatalog) OfflineFolders(_ context.Context, folders []library.Folder) []string {
	var offline []string

	for _, folder := range folders {
		if slices.Contains(catalog.offline, folder.ID) {
			offline = append(offline, folder.ID)
		}
	}

	return offline
}

func (catalog *fakeCatalog) Status(libraryID string) appscan.Status {
	if catalog.scanning[libraryID] {
		return appscan.Status{State: appscan.StateRunning}
	}

	return appscan.Status{State: appscan.StateIdle}
}
