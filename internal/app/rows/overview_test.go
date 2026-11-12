package rows_test

import (
	"slices"
	"testing"

	approws "github.com/magpeek/mivo/internal/app/rows"
	"github.com/magpeek/mivo/internal/domain/artwork"
	"github.com/magpeek/mivo/internal/domain/library"
	"github.com/magpeek/mivo/internal/domain/rows"
)

func TestLibraryRowsOfMovies(t *testing.T) {
	t.Parallel()

	got, err := newTestService(t, newFixtureCatalog()).LibraryRows(adminContext(), moviesID, 0)
	if err != nil {
		t.Fatalf("LibraryRows: %v", err)
	}

	if got.LibraryID != moviesID || got.Hint != rows.HintNone {
		t.Errorf("library %q hint %q, want %q and none", got.LibraryID, got.Hint, moviesID)
	}

	tests := []struct {
		kind      rows.Kind
		titleKey  string
		wantItems []string
		wantSort  library.ItemSort
	}{
		// Unavailable movies (m-broken, m-offline) are never listed.
		{kind: rows.KindRecentlyAdded, titleKey: rows.TitleRecentlyAdded, wantItems: []string{"m-new", "m-dateless", "m-old"}, wantSort: library.SortByDateAdded},
		// m-dateless has no release date.
		{kind: rows.KindNewReleases, titleKey: rows.TitleNewReleases, wantItems: []string{"m-new", "m-old"}, wantSort: library.SortByReleaseDate},
	}

	if len(got.Rows) != 4 {
		t.Fatalf("rows = %+v, want recentlyAdded, newReleases and two genre rows", got.Rows)
	}

	for index, test := range tests {
		row := got.Rows[index]
		if row.Kind != test.kind || row.TitleKey != test.titleKey || row.ID != rows.RowID(test.kind, moviesID) {
			t.Errorf("row %d = %s %q %q, want %s %q", index, row.Kind, row.TitleKey, row.ID, test.kind, test.titleKey)
		}

		if ids := itemIDs(row); !slices.Equal(ids, test.wantItems) {
			t.Errorf("%s items = %v, want %v", test.kind, ids, test.wantItems)
		}

		wantSeeAll := library.ListingQuery{
			Filter: library.ListingFilter{Kind: library.ItemKindMovie},
			Order:  library.ListingOrder{Sort: test.wantSort, Descending: true},
		}
		if row.CardFormat != rows.CardPoster || row.LibraryID != moviesID || !equalQuery(row.SeeAll, wantSeeAll) {
			t.Errorf("%s card %q library %q seeAll %+v", test.kind, row.CardFormat, row.LibraryID, row.SeeAll)
		}
	}

	assertGenreRows(t, got.Rows[2:], map[string][]string{"Drama": {"m-new", "m-old"}, "Comedy": {"m-new"}})
}

// assertGenreRows checks the genre rows against the items per genre, in any genre order.
func assertGenreRows(t *testing.T, genreRows []approws.Row, want map[string][]string) {
	t.Helper()

	for _, row := range genreRows {
		wantItems, found := want[row.Genre]
		if !found || row.Kind != rows.KindMoreGenre || row.TitleKey != rows.TitleMoreGenre {
			t.Errorf("unexpected genre row %+v", row)

			continue
		}

		if row.ID != rows.GenreRowID(moviesID, row.Genre) || row.TitleParams[rows.ParamGenre] != row.Genre {
			t.Errorf("genre row %q: id %q params %v", row.Genre, row.ID, row.TitleParams)
		}

		if ids := itemIDs(row); !slices.Equal(slices.Sorted(slices.Values(ids)), slices.Sorted(slices.Values(wantItems))) {
			t.Errorf("genre %q items = %v, want %v", row.Genre, ids, wantItems)
		}

		order := row.SeeAll.Order
		if order.Sort != library.SortByRandom || !order.HasSeed || order.Seed != 20261105 || !slices.Equal(row.SeeAll.Filter.Genres, []string{row.Genre}) {
			t.Errorf("genre %q seeAll = %+v, want random with the date seed and the genre", row.Genre, row.SeeAll)
		}
	}
}

func equalQuery(left, right library.ListingQuery) bool {
	return left.Order == right.Order && left.Filter.Kind == right.Filter.Kind &&
		slices.Equal(left.Filter.Genres, right.Filter.Genres) && len(left.Filter.OfflineRootIDs) == 0
}

func TestLibraryRowsLimit(t *testing.T) {
	t.Parallel()

	got, err := newTestService(t, newFixtureCatalog()).LibraryRows(adminContext(), moviesID, 1)
	if err != nil {
		t.Fatalf("LibraryRows: %v", err)
	}

	for _, row := range got.Rows {
		if len(row.Items) != 1 {
			t.Errorf("row %s has %d items, want 1", row.ID, len(row.Items))
		}
	}

	if ids := itemIDs(got.Rows[0]); !slices.Equal(ids, []string{"m-new"}) {
		t.Errorf("recentlyAdded = %v, want the newest", ids)
	}
}

func TestLibraryRowsOfShows(t *testing.T) {
	t.Parallel()

	got, err := newTestService(t, newFixtureCatalog()).LibraryRows(adminContext(), showsID, 0)
	if err != nil {
		t.Fatalf("LibraryRows: %v", err)
	}

	kinds := make([]rows.Kind, 0, len(got.Rows))
	for _, row := range got.Rows {
		kinds = append(kinds, row.Kind)
	}

	wantKinds := []rows.Kind{rows.KindRecentlyAdded, rows.KindNewReleases, rows.KindMoreGenre, rows.KindRecentEpisodes}
	if !slices.Equal(kinds, wantKinds) {
		t.Fatalf("kinds = %v, want %v", kinds, wantKinds)
	}

	// Show B's recent episode alone becomes a season card named after the show, with the
	// show's poster; Show A's episodes in two seasons become the show card.
	seasonCard := season("b1", "show-b", 3, false).summary
	seasonCard.Poster = poster("show-b")

	recent := got.Rows[0].Items
	want := []approws.Item{
		{Summary: seasonCard, ShowTitle: "Show B", NewEpisodeCount: 1},
		{Summary: withGenres(show("show-a", "Show A", "2010-01-01", day(10)), "Drama").summary, NewEpisodeCount: 3},
	}

	if !equalItems(recent, want) {
		t.Errorf("recentlyAdded = %+v, want %+v", recent, want)
	}

	assertRecentEpisodes(t, got.Rows[3])
}

func assertRecentEpisodes(t *testing.T, row approws.Row) {
	t.Helper()

	if ids := itemIDs(row); !slices.Equal(ids, []string{"b1e1", "a2e1", "a1e2", "a1e1"}) {
		t.Errorf("recentEpisodes = %v, want by air date, newest first", ids)
	}

	first := row.Items[0]
	if number, _ := first.SeasonNumber.Get(); number != 3 || first.ShowTitle != "Show B" || *first.Summary.Poster != *poster("show-b") {
		t.Errorf("episode card = %+v, want season 3 of Show B with its poster", first)
	}

	wantSeeAll := library.ListingQuery{
		Filter: library.ListingFilter{Kind: library.ItemKindShow},
		Order:  library.ListingOrder{Sort: library.SortByDateAdded, Descending: true},
	}
	if row.CardFormat != rows.CardEpisode || !equalQuery(row.SeeAll, wantSeeAll) {
		t.Errorf("recentEpisodes card %q seeAll %+v", row.CardFormat, row.SeeAll)
	}
}

func equalItems(got, want []approws.Item) bool {
	return slices.EqualFunc(got, want, func(left, right approws.Item) bool {
		return left.Summary.Item.ID == right.Summary.Item.ID && left.ShowTitle == right.ShowTitle &&
			left.NewEpisodeCount == right.NewEpisodeCount && left.SeasonNumber == right.SeasonNumber &&
			equalPoster(left.Summary.Poster, right.Summary.Poster)
	})
}

// equalPoster compares two optional artwork references.
func equalPoster(left, right *artwork.Ref) bool {
	return (left == nil && right == nil) || (left != nil && right != nil && *left == *right)
}
