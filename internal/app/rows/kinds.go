package rows

import (
	"context"
	"fmt"

	applibrary "github.com/magpeek/mivo/internal/app/library"
	"github.com/magpeek/mivo/internal/domain/library"
	"github.com/magpeek/mivo/internal/domain/rows"
)

// recentlyAdded builds the "recentlyAdded" row with the given title: in a shows library the
// recently added episodes grouped into show or season cards (HO-05), else the newest
// top-level items. The row is returned even without items; the caller judges the library by
// it.
func (service *Service) recentlyAdded(ctx context.Context, source target, titleKey string, params map[string]string) (Row, error) {
	query := listedQuery(source, library.SortByDateAdded)
	row := newRow(source, rows.KindRecentlyAdded, titleKey, query)
	row.TitleParams = params

	var err error

	if rows.GroupsEpisodes(source.library.Kind) {
		row.Items, err = service.episodeCards(ctx, source)
	} else {
		row.Items, err = service.topLevelItems(ctx, source, query)
	}

	if err != nil {
		return Row{}, fmt.Errorf("recently added: %w", err)
	}

	return row, nil
}

// newReleases builds the "newReleases" row: the items with a release date, newest first.
func (service *Service) newReleases(ctx context.Context, source target) ([]Row, error) {
	query := listedQuery(source, library.SortByReleaseDate)
	row := newRow(source, rows.KindNewReleases, rows.TitleNewReleases, query)

	var err error

	row.Items, err = service.topLevelItems(ctx, source, query)
	if err != nil {
		return nil, fmt.Errorf("new releases: %w", err)
	}

	return oneRow(row), nil
}

// genreRows builds the "moreGenre" rows of today's genres (rows.PickGenres). Each lists the
// genre's items shuffled with the date seed, and its "See all" repeats that order.
func (service *Service) genreRows(ctx context.Context, source target) ([]Row, error) {
	facets, err := service.catalog.ListFacets(ctx, source.library.ID, applibrary.FacetRequest{Fields: []library.FacetField{library.FacetGenre}})
	if err != nil {
		return nil, fmt.Errorf("count genres: %w", err)
	}

	seed := rows.DateSeed(service.clock())

	var built []Row

	for _, genre := range rows.PickGenres(genreCounts(facets), seed) {
		query := listedQuery(source, library.SortByRandom)
		query.Order = library.ListingOrder{Sort: library.SortByRandom, Seed: seed, HasSeed: true} // a shuffle has no direction
		query.Filter.Genres = []string{genre}

		row := newRow(source, rows.KindMoreGenre, rows.TitleMoreGenre, query)
		row.ID, row.Genre, row.TitleParams = rows.GenreRowID(source.library.ID, genre), genre, map[string]string{rows.ParamGenre: genre}

		row.Items, err = service.topLevelItems(ctx, source, query)
		if err != nil {
			return nil, fmt.Errorf("genre %q: %w", genre, err)
		}

		built = append(built, oneRow(row)...)
	}

	return built, nil
}

// genreCounts returns the values of the genre facet.
func genreCounts(facets []applibrary.Facet) []rows.GenreCount {
	var counts []rows.GenreCount

	for _, facet := range facets {
		if facet.Field != library.FacetGenre {
			continue
		}

		for _, value := range facet.Values {
			counts = append(counts, rows.GenreCount{Genre: value.Value, Count: value.Count})
		}
	}

	return counts
}

// recentEpisodes builds the "recentEpisodes" row: episodes by air date, newest first, each
// with its season number, its show's title and its show's poster. The Browse pivot lists no
// episodes, so "See all" lists the shows by date added.
func (service *Service) recentEpisodes(ctx context.Context, source target) ([]Row, error) {
	query := listedQuery(source, library.SortByDateAdded)
	row := newRow(source, rows.KindRecentEpisodes, rows.TitleRecentEpisodes, query)

	refs, err := service.rows.ListRecentEpisodes(ctx, EpisodeRequest{LibraryID: source.library.ID, OfflineRootIDs: source.offline, Limit: source.limit})
	if err != nil {
		return nil, fmt.Errorf("recent episodes: %w", err)
	}

	row.Items, err = service.episodeItems(ctx, refs)
	if err != nil {
		return nil, fmt.Errorf("recent episodes: %w", err)
	}

	return oneRow(row), nil
}
