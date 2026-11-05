package rows_test

import (
	"testing"
	"time"

	applibrary "github.com/magpeek/mivo/internal/app/library"
	approws "github.com/magpeek/mivo/internal/app/rows"
	"github.com/magpeek/mivo/internal/domain/artwork"
	"github.com/magpeek/mivo/internal/domain/library"
)

// IDs of the fixture libraries and folders.
const (
	moviesID      = "lib-movies"
	showsID       = "lib-shows"
	emptyID       = "lib-empty"
	moviesFolder  = "folder-movies"
	offlineFolder = "folder-offline"
	showsFolder   = "folder-shows"
)

// fixedNow is the clock of every test: the genre seed is 20261105.
func fixedNow() time.Time { return time.Date(2026, time.November, 5, 20, 0, 0, 0, time.UTC) }

// day returns noon of a November 2026 day, the time items were added.
func day(number int) time.Time {
	return time.Date(2026, time.November, number, 12, 0, 0, 0, time.UTC)
}

func poster(id string) *artwork.Ref { return &artwork.Ref{ID: "poster-" + id, Tag: "t"} }

// newItem returns a catalog item of the fixture.
func newItem(id, libraryID string, kind library.ItemKind, added time.Time) fakeItem {
	return fakeItem{summary: applibrary.ItemSummary{Item: library.Item{
		ID: id, LibraryID: libraryID, Kind: kind, Title: "Title " + id, CreatedAt: added,
	}}}
}

func movie(id string, added time.Time, releaseDate string, genres ...string) fakeItem {
	item := newItem(id, moviesID, library.ItemKindMovie, added)
	item.releaseDate, item.genres, item.folderID = releaseDate, genres, moviesFolder
	item.summary.Poster = poster(id)

	return item
}

// placement is the season and show of an episode.
type placement struct{ seasonID, showID string }

// episode returns an episode of a season of a show in the shows library.
func episode(id string, place placement, added time.Time, airDate string) fakeItem {
	item := newItem(id, showsID, library.ItemKindEpisode, added)
	item.summary.Item.ParentID = place.seasonID
	item.seasonID, item.showID, item.releaseDate, item.folderID = place.seasonID, place.showID, airDate, showsFolder

	return item
}

func season(id, showID string, number int, withPoster bool) fakeItem {
	item := newItem(id, showsID, library.ItemKindSeason, day(1))
	item.summary.Item.ParentID, item.summary.Item.Number = showID, library.NumberOf(number)

	if withPoster {
		item.summary.Poster = poster(id)
	}

	return item
}

func show(id, title, releaseDate string, added time.Time) fakeItem {
	item := newItem(id, showsID, library.ItemKindShow, added)
	item.summary.Item.Title, item.summary.Poster = title, poster(id)
	item.releaseDate, item.folderID = releaseDate, showsFolder

	return item
}

func withGenres(item fakeItem, genres ...string) fakeItem {
	item.genres = genres

	return item
}

// moviesFixture: in date-added order m-new (with release date and two genres), m-dateless
// (neither), m-old (release date, Drama); m-broken could not be probed and m-offline lies in
// the offline folder, so neither is ever listed.
func moviesFixture() []fakeItem {
	broken := movie("m-broken", day(9), "2020-01-01", "Drama")
	broken.broken = true

	offline := movie("m-offline", day(8), "2021-01-01", "Drama")
	offline.folderID = offlineFolder

	return []fakeItem{
		movie("m-old", day(1), "2008-05-01", "Drama"),
		movie("m-new", day(3), "2019-12-25", "Comedy", "Drama"),
		movie("m-dateless", day(2), ""),
		broken, offline,
	}
}

// showsFixture: Show A got three episodes in two seasons on day 10; Show B got an episode on
// day 2 and another on day 12, which alone is recent (RecentEpisodeWindow); season b1 has no
// poster of its own.
func showsFixture() []fakeItem {
	return []fakeItem{
		withGenres(show("show-a", "Show A", "2010-01-01", day(10)), "Drama"),
		show("show-b", "Show B", "2015-01-01", day(2)),
		season("a1", "show-a", 1, true), season("a2", "show-a", 2, true), season("b1", "show-b", 3, false),
		episode("a1e1", placement{seasonID: "a1", showID: "show-a"}, day(10), "2010-01-01"),
		episode("a1e2", placement{seasonID: "a1", showID: "show-a"}, day(10), "2010-01-08"),
		episode("a2e1", placement{seasonID: "a2", showID: "show-a"}, day(10), "2011-01-01"),
		episode("b1e1", placement{seasonID: "b1", showID: "show-b"}, day(2), "2016-03-01"),
		episode("b1e2", placement{seasonID: "b1", showID: "show-b"}, day(12), ""),
	}
}

func newFixtureCatalog() *fakeCatalog {
	return &fakeCatalog{
		libraries: []library.ManagedLibrary{
			{ID: emptyID, Name: "Empty", Kind: library.KindMovies},
			{ID: moviesID, Name: "Movies", Kind: library.KindMovies, Folders: []library.Folder{{ID: moviesFolder}, {ID: offlineFolder}}},
			{ID: showsID, Name: "Shows", Kind: library.KindShows, Folders: []library.Folder{{ID: showsFolder}}},
		},
		items:    append(moviesFixture(), showsFixture()...),
		offline:  []string{offlineFolder},
		scanning: map[string]bool{},
	}
}

func newTestService(t *testing.T, catalog *fakeCatalog) *approws.Service {
	t.Helper()

	service, err := approws.NewService(approws.Dependencies{
		Libraries: catalog, Rows: catalog, Summaries: catalog, Catalog: catalog, Scans: catalog,
		Clock: func() time.Time { return fixedNow() },
	})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	return service
}

// itemIDs returns the IDs of a row's items in order.
func itemIDs(row approws.Row) []string {
	ids := make([]string, 0, len(row.Items))
	for _, item := range row.Items {
		ids = append(ids, item.Summary.Item.ID)
	}

	return ids
}
