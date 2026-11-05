package sqlite_test

import (
	"errors"
	"fmt"
	"slices"
	"testing"

	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/library"
)

// listAllIDs pages through a listing with the given limit and returns the item IDs in order;
// it also checks that every page reports the same total count.
func (fixture catalogFixture) listAllIDs(t *testing.T, libraryID string, query library.ListingQuery, limit int) ([]string, int64) {
	t.Helper()

	var (
		ids    []string
		cursor string
		total  int64 = -1
	)

	for range 100 {
		page, err := fixture.reader.ListItems(t.Context(), libraryID, library.PageRequest{Limit: limit, Cursor: cursor}, query)
		if err != nil {
			t.Fatalf("ListItems(%+v) error = %v", query, err)
		}

		if !page.Counted || (total >= 0 && page.TotalCount != total) {
			t.Fatalf("page = %+v, want a stable total count", page)
		}

		total = page.TotalCount

		for _, summary := range page.Items {
			ids = append(ids, summary.Item.ID)
		}

		if page.NextCursor == "" {
			return ids, total
		}

		cursor = page.NextCursor
	}

	t.Fatal("listing did not end")

	return nil, 0
}

func movieQuery(sort library.ItemSort, descending bool) library.ListingQuery {
	return library.ListingQuery{
		Filter: library.ListingFilter{Kind: library.ItemKindMovie},
		Order:  library.ListingOrder{Sort: sort, Descending: descending},
	}
}

func TestListItemsSortsEveryKeyWithStableTies(t *testing.T) {
	t.Parallel()

	fixture := newListingFixture(t)

	tests := []struct {
		sort           library.ItemSort
		ascending      []string
		descending     []string
		libraryID      string
		kind           library.ItemKind
		skipDescending bool
	}{
		{sort: library.SortByTitle, ascending: []string{movie1917, movieAlpha, movieBravo, movieCharlie, movieAerger}},
		{
			sort: library.SortByYear, ascending: []string{movieBravo, movieAerger, movieAlpha, movie1917, movieCharlie},
			descending: []string{movie1917, movieAlpha, movieAerger, movieBravo, movieCharlie},
		},
		{
			sort: library.SortByReleaseDate, ascending: []string{movieBravo, movieAerger, movieAlpha, movie1917, movieCharlie},
			descending: []string{movie1917, movieAlpha, movieAerger, movieBravo, movieCharlie},
		},
		{
			sort: library.SortByContentRating, ascending: []string{movieAlpha, movieAerger, movie1917, movieBravo, movieCharlie},
			descending: []string{movieBravo, movie1917, movieAerger, movieAlpha, movieCharlie},
		},
		{
			sort: library.SortByDuration, ascending: []string{movieAerger, movieAlpha, movieBravo, movie1917, movieCharlie},
			descending: []string{movie1917, movieBravo, movieAlpha, movieAerger, movieCharlie},
		},
		{sort: library.SortByDateAdded, ascending: []string{movieAlpha, movieCharlie, movieBravo, movieAerger, movie1917}},
		{
			sort: library.SortByResolution, ascending: []string{movieAerger, movie1917, movieAlpha, movieBravo, movieCharlie},
			descending: []string{movieBravo, movieAlpha, movie1917, movieAerger, movieCharlie},
		},
		{
			sort: library.SortByBitrate, ascending: []string{movieBravo, movieAerger, movieAlpha, movie1917, movieCharlie},
			descending: []string{movie1917, movieAlpha, movieAerger, movieBravo, movieCharlie},
		},
		{sort: library.SortByFileSize, ascending: []string{movieAerger, movieAlpha, movieCharlie, movieBravo, movie1917}},
		{
			sort: library.SortByFileSize, libraryID: showsLibraryID, kind: library.ItemKindShow,
			ascending: []string{showB, showA, showC}, descending: []string{showA, showB, showC},
		},
		{
			sort: library.SortByDuration, libraryID: showsLibraryID, kind: library.ItemKindShow,
			ascending: []string{showA, showB, showC}, descending: []string{showC, showB, showA},
		},
	}
	for _, test := range tests {
		libraryID, kind := cmpOr(test.libraryID, moviesLibraryID), library.ItemKind(cmpOr(string(test.kind), string(library.ItemKindMovie)))

		descending := test.descending
		if descending == nil {
			descending = reversed(test.ascending)
		}

		for direction, want := range map[bool][]string{false: test.ascending, true: descending} {
			query := movieQuery(test.sort, direction)
			query.Filter.Kind = kind

			t.Run(fmt.Sprintf("%s descending=%v %s", test.sort, direction, kind), func(t *testing.T) {
				t.Parallel()
				fixture.assertListingInEveryPageSize(t, libraryID, query, want)
			})
		}
	}
}

// assertListingInEveryPageSize pages through the listing with several limits; every page
// size must give the same order and total.
func (fixture catalogFixture) assertListingInEveryPageSize(t *testing.T, libraryID string, query library.ListingQuery, want []string) {
	t.Helper()

	for _, limit := range []int{1, 2, library.MaxPageLimit} {
		got, total := fixture.listAllIDs(t, libraryID, query, limit)
		if !slices.Equal(got, want) || total != int64(len(want)) {
			t.Errorf("limit %d: ids = %v (total %d), want %v", limit, got, total, want)
		}
	}
}

func cmpOr(value, fallback string) string {
	if value == "" {
		return fallback
	}

	return value
}

func reversed(ids []string) []string {
	result := slices.Clone(ids)
	slices.Reverse(result)

	return result
}

func TestListItemsShufflesStablyPerSeed(t *testing.T) {
	t.Parallel()

	fixture := newListingFixture(t)
	seeded := func(seed int64) library.ListingQuery {
		query := movieQuery(library.SortByRandom, true)
		query.Order.Seed, query.Order.HasSeed = seed, true

		return query
	}

	first, _ := fixture.listAllIDs(t, moviesLibraryID, seeded(42), library.MaxPageLimit)
	paged, _ := fixture.listAllIDs(t, moviesLibraryID, seeded(42), 2)

	if !slices.Equal(first, paged) || len(first) != 5 {
		t.Fatalf("seed 42: one page %v, paged %v; want the same five items", first, paged)
	}

	sorted := slices.Sorted(slices.Values(first))
	if !slices.Equal(sorted, []string{movieAlpha, movieBravo, movieCharlie, movieAerger, movie1917}) {
		t.Errorf("shuffled ids = %v, want every movie once", first)
	}

	orders := map[string]bool{}

	for seed := range int64(20) {
		ids, _ := fixture.listAllIDs(t, moviesLibraryID, seeded(seed), library.MaxPageLimit)
		orders[joinIDs(ids)] = true
	}

	if len(orders) < 2 {
		t.Errorf("20 seeds gave %d orders, want different orders", len(orders))
	}

	// Without a seed in the request the cursor carries it.
	page, err := fixture.reader.ListItems(t.Context(), moviesLibraryID, library.PageRequest{Limit: 2}, seeded(42))
	if err != nil {
		t.Fatalf("ListItems error = %v", err)
	}

	unseeded := movieQuery(library.SortByRandom, false)

	next, err := fixture.reader.ListItems(t.Context(), moviesLibraryID, library.PageRequest{Limit: 2, Cursor: page.NextCursor}, unseeded)
	if err != nil || len(next.Items) != 2 || next.Items[0].Item.ID != first[2] {
		t.Errorf("unseeded second page = %+v, %v; want it to continue the seed-42 order", next, err)
	}
}

func joinIDs(ids []string) string {
	text := ""
	for _, id := range ids {
		text += id[:8]
	}

	return text
}

func TestListItemsFilters(t *testing.T) {
	t.Parallel()

	fixture := newListingFixture(t)

	tests := []struct {
		name      string
		libraryID string
		filter    library.ListingFilter
		want      []string
	}{
		{name: "no filter, no kind", filter: library.ListingFilter{Kind: ""}, want: []string{movie1917, movieAlpha, movieBravo, movieCharlie, movieAerger}},
		{name: "genre case-insensitive", filter: library.ListingFilter{Genres: []string{"drama"}}, want: []string{movieAlpha, movieAerger}},
		{name: "genres or", filter: library.ListingFilter{Genres: []string{"Drama", "War"}}, want: []string{movie1917, movieAlpha, movieAerger}},
		{name: "year", filter: library.ListingFilter{Years: []int{1999}}, want: []string{movieBravo}},
		{name: "decade", filter: library.ListingFilter{Decades: []int{2000}}, want: []string{movieAlpha, movieAerger}},
		{name: "content rating", filter: library.ListingFilter{ContentRatings: []string{"PG"}}, want: []string{movieAlpha, movieAerger}},
		{name: "resolution 4k", filter: library.ListingFilter{Resolutions: []library.Resolution{library.Resolution4K}}, want: []string{movieBravo}},
		{name: "resolution sd or 720p", filter: library.ListingFilter{Resolutions: []library.Resolution{library.ResolutionSD, library.Resolution720p}}, want: []string{movie1917, movieAerger}},
		{name: "resolution ignores attached pictures", filter: library.ListingFilter{Resolutions: []library.Resolution{library.Resolution1080p}}, want: []string{movieAlpha}},
		{name: "video codec of the primary file", filter: library.ListingFilter{VideoCodecs: []string{"hevc", "vp9"}}, want: []string{movie1917, movieBravo}},
		{name: "audio codec of any stream", filter: library.ListingFilter{AudioCodecs: []string{"truehd"}}, want: []string{movie1917}},
		{name: "container", filter: library.ListingFilter{Containers: []string{"mkv"}}, want: []string{movie1917, movieAlpha, movieCharlie}},
		{name: "audio language", filter: library.ListingFilter{AudioLanguages: []string{"ger"}}, want: []string{movieBravo}},
		{name: "subtitle language", filter: library.ListingFilter{SubtitleLanguages: []string{"ger"}}, want: []string{movieAlpha}},
		{name: "quick all", filter: library.ListingFilter{Quick: library.QuickAll}, want: []string{movie1917, movieAlpha, movieBravo, movieCharlie, movieAerger}},
		{name: "quick hdr", filter: library.ListingFilter{Quick: library.QuickHDR}, want: []string{movieBravo}},
		{name: "quick uhd", filter: library.ListingFilter{Quick: library.QuickUHD}, want: []string{movieBravo}},
		{name: "quick unavailable", filter: library.ListingFilter{Quick: library.QuickUnavailable}, want: []string{movieCharlie}},
		{name: "quick unavailable offline folder", filter: library.ListingFilter{Quick: library.QuickUnavailable, OfflineRootIDs: []string{listingSecondRoot}}, want: []string{movie1917, movieCharlie}},
		{name: "quick duplicates", filter: library.ListingFilter{Quick: library.QuickDuplicates}, want: []string{movieBravo}},
		{name: "quick dolby vision", filter: library.ListingFilter{Quick: library.QuickDolbyVision}},
		{name: "quick atmos", filter: library.ListingFilter{Quick: library.QuickAtmos}},
		{name: "filters combine with and", filter: library.ListingFilter{Containers: []string{"mkv"}, Genres: []string{"Drama"}}, want: []string{movieAlpha}},
		{name: "show by its episodes hdr", libraryID: showsLibraryID, filter: library.ListingFilter{Kind: library.ItemKindShow, Quick: library.QuickHDR}, want: []string{showA}},
		{name: "show by its episodes resolution", libraryID: showsLibraryID, filter: library.ListingFilter{Kind: library.ItemKindShow, Resolutions: []library.Resolution{library.Resolution720p}}, want: []string{showB}},
		{name: "show by its episodes codec", libraryID: showsLibraryID, filter: library.ListingFilter{Kind: library.ItemKindShow, VideoCodecs: []string{"h264"}}, want: []string{showA, showB}},
		{name: "show year", libraryID: showsLibraryID, filter: library.ListingFilter{Kind: library.ItemKindShow, Years: []int{2020}}, want: []string{showC}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			filter := test.filter
			if filter.Kind == "" && test.name != "no filter, no kind" {
				filter.Kind = library.ItemKindMovie
			}

			got, total := fixture.listAllIDs(t, cmpOr(test.libraryID, moviesLibraryID), library.ListingQuery{Filter: filter}, 2)
			if !slices.Equal(got, test.want) || total != int64(len(test.want)) {
				t.Errorf("ids = %v (total %d), want %v", got, total, test.want)
			}
		})
	}
}

func TestListItemsRejectsForeignCursors(t *testing.T) {
	t.Parallel()

	fixture := newListingFixture(t)

	title, err := fixture.reader.ListItems(t.Context(), moviesLibraryID, library.PageRequest{Limit: 1}, movieQuery(library.SortByTitle, false))
	if err != nil {
		t.Fatalf("ListItems error = %v", err)
	}

	random := movieQuery(library.SortByRandom, false)
	random.Order.Seed, random.Order.HasSeed = 1, true

	shuffled, err := fixture.reader.ListItems(t.Context(), moviesLibraryID, library.PageRequest{Limit: 1}, random)
	if err != nil {
		t.Fatalf("ListItems error = %v", err)
	}

	otherSeed := random
	otherSeed.Order.Seed = 2

	tests := []struct {
		name   string
		cursor string
		query  library.ListingQuery
	}{
		{name: "title cursor for year", cursor: title.NextCursor, query: movieQuery(library.SortByYear, false)},
		{name: "ascending cursor for descending", cursor: title.NextCursor, query: movieQuery(library.SortByTitle, true)},
		{name: "other seed", cursor: shuffled.NextCursor, query: otherSeed},
		{name: "not base64", cursor: "!!", query: movieQuery(library.SortByTitle, false)},
		{name: "legacy cursor", cursor: "aWQAdGl0bGU", query: movieQuery(library.SortByTitle, false)},
		{name: "bad value type", cursor: rawListingCursor("list\x00year\x00asc\x000\x000\x00x\x001\x00id\x00t"), query: movieQuery(library.SortByYear, false)},
		{name: "bad integer", cursor: rawListingCursor("list\x00year\x00asc\x000\x000\x00i\x00x\x00id\x00t"), query: movieQuery(library.SortByYear, false)},
		{name: "negative seed", cursor: rawListingCursor("list\x00random\x00asc\x00-1\x000\x00i\x005\x00id\x00t"), query: movieQuery(library.SortByRandom, false)},
		{name: "seed above int32", cursor: rawListingCursor("list\x00random\x00asc\x002147483648\x000\x00i\x005\x00id\x00t"), query: movieQuery(library.SortByRandom, false)},
		{name: "bad seed", cursor: rawListingCursor("list\x00year\x00asc\x00x\x001\x00n\x00\x00id\x00t"), query: movieQuery(library.SortByYear, false)},
	}
	for _, test := range tests {
		_, err := fixture.reader.ListItems(t.Context(), moviesLibraryID, library.PageRequest{Cursor: test.cursor}, test.query)
		if !errors.Is(err, domain.ErrInvalid) {
			t.Errorf("%s: error = %v, want domain.ErrInvalid", test.name, err)
		}
	}

	_, err = fixture.reader.ListItems(t.Context(), unknownID, library.PageRequest{}, library.ListingQuery{})
	if !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("unknown library: error = %v, want domain.ErrNotFound", err)
	}
}

func TestListItemsContinuesATextCursor(t *testing.T) {
	t.Parallel()

	fixture := newListingFixture(t)
	query := movieQuery(library.SortByContentRating, false)

	first, err := fixture.reader.ListItems(t.Context(), moviesLibraryID, library.PageRequest{Limit: 1}, query)
	if err != nil {
		t.Fatalf("ListItems error = %v", err)
	}

	// The cursor carries the text value "PG"; a hand-made one with a "s" value resumes too.
	cursor := rawListingCursor("list\x00contentRating\x00asc\x000\x000\x00s\x00PG\x00" + movieAlpha + "\x00alpha")
	if first.NextCursor != cursor {
		t.Errorf("cursor = %q, want %q", first.NextCursor, cursor)
	}
}
