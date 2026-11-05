package sqlite_test

import (
	"errors"
	"slices"
	"testing"
	"time"

	approws "github.com/magpeek/mivo/internal/app/rows"
	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/library"
	"github.com/magpeek/mivo/internal/domain/rows"
)

// Episodes and seasons of the listing fixture (seedListingShows).
const (
	episodeA1     = "e0000001-0000-4000-8000-000000000000"
	episodeA2     = "e0000001-1000-4000-8000-000000000000"
	episodeB1     = "e0000002-0000-4000-8000-000000000000"
	seasonA1      = "t0000001-0000-4000-8000-000000000000"
	seasonB1      = "t0000002-0000-4000-8000-000000000000"
	seasonA2      = "t0000009-0000-4000-8000-000000000000"
	episodeA2New  = "e0000009-0000-4000-8000-000000000000"
	eightDaysInMs = int64(8 * 24 * time.Hour / time.Millisecond)
)

func (fixture catalogFixture) rowItemIDs(t *testing.T, request approws.ItemRequest) []string {
	t.Helper()

	summaries, err := fixture.reader.ListRowItems(t.Context(), request)
	if err != nil {
		t.Fatalf("ListRowItems(%+v) error = %v", request, err)
	}

	ids := make([]string, 0, len(summaries))
	for _, summary := range summaries {
		ids = append(ids, summary.Item.ID)
	}

	return ids
}

func movieRowQuery(sort library.ItemSort, descending bool) library.ListingQuery {
	return library.ListingQuery{
		Filter: library.ListingFilter{Kind: library.ItemKindMovie},
		Order:  library.ListingOrder{Sort: sort, Descending: descending},
	}
}

func TestListRowItems(t *testing.T) {
	t.Parallel()

	fixture := newListingFixture(t)

	offline := movieRowQuery(library.SortByDateAdded, true)
	offline.Filter.OfflineRootIDs = []string{listingSecondRoot}

	drama := movieRowQuery(library.SortByRandom, false)
	drama.Order.Seed, drama.Order.HasSeed, drama.Filter.Genres = 20261105, true, []string{"drama"}

	tests := []struct {
		name      string
		libraryID string
		query     library.ListingQuery
		limit     int
		want      []string
		sorted    bool
	}{
		// charlie could not be probed, so it is never in a row.
		{name: "newest first", libraryID: moviesLibraryID, query: movieRowQuery(library.SortByDateAdded, true), limit: 10, want: []string{movie1917, movieAerger, movieBravo, movieAlpha}},
		{name: "offline folder left out", libraryID: moviesLibraryID, query: offline, limit: 10, want: []string{movieAerger, movieBravo, movieAlpha}},
		{name: "release dates, limited", libraryID: moviesLibraryID, query: movieRowQuery(library.SortByReleaseDate, true), limit: 2, want: []string{movie1917, movieAlpha}},
		{name: "genre shuffled", libraryID: moviesLibraryID, query: drama, limit: 10, want: []string{movieAerger, movieAlpha}, sorted: true},
		// Show C has no episodes, so nothing of it can be played.
		{name: "shows", libraryID: showsLibraryID, query: library.ListingQuery{
			Filter: library.ListingFilter{Kind: library.ItemKindShow}, Order: library.ListingOrder{Sort: library.SortByDateAdded, Descending: true},
		}, limit: 10, want: []string{showB, showA}},
		{name: "shows without release dates", libraryID: showsLibraryID, query: library.ListingQuery{
			Filter: library.ListingFilter{Kind: library.ItemKindShow}, Order: library.ListingOrder{Sort: library.SortByReleaseDate, Descending: true},
		}, limit: 10, want: []string{}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			got := fixture.rowItemIDs(t, approws.ItemRequest{LibraryID: test.libraryID, Query: test.query, Limit: test.limit})
			if test.sorted {
				slices.Sort(got)
				slices.Sort(test.want)
			}

			if !slices.Equal(got, test.want) {
				t.Errorf("ids = %v, want %v", got, test.want)
			}
		})
	}
}

// TestListRowItemsTiesAndAlbums checks that the tie breakers follow the direction of the
// sort, as the "See all" listing does, and that albums are judged by their tracks.
func TestListRowItemsTiesAndAlbums(t *testing.T) {
	t.Parallel()

	fixture := newListingFixture(t)
	seedListingMusic(t, fixture)

	// Zulu was added together with 1917; descending, its sort title comes first.
	const movieZulu = "m0000009-0000-4000-8000-000000000000"
	insertListingItem(t, fixture.inspection, listingItem{
		id: movieZulu, libraryID: moviesLibraryID, kind: "movie", title: "Zulu", sortTitle: "zulu", createdAt: 5000,
	})
	insertListingFile(t, fixture.inspection, listingFile{
		id: "f0000009-0000-4000-8000-000000000000", itemID: movieZulu, libraryID: moviesLibraryID,
		relativePath: "Zulu.mkv", container: "mkv", sizeBytes: 1,
	})

	got := fixture.rowItemIDs(t, approws.ItemRequest{LibraryID: moviesLibraryID, Query: movieRowQuery(library.SortByDateAdded, true), Limit: 2})
	if want := []string{movieZulu, movie1917}; !slices.Equal(got, want) {
		t.Errorf("ties = %v, want %v", got, want)
	}

	execute(t, fixture.inspection, `UPDATE media_files SET probe_error = 'probe_failed' WHERE item_id IN
		(SELECT id FROM items WHERE parent_id = ?)`, albumX)

	albums := fixture.rowItemIDs(t, approws.ItemRequest{LibraryID: listingMusicLibraryID, Query: library.ListingQuery{
		Filter: library.ListingFilter{Kind: library.ItemKindAlbum}, Order: library.ListingOrder{Sort: library.SortByTitle},
	}, Limit: 10})
	if want := []string{albumY, albumZ}; !slices.Equal(albums, want) {
		t.Errorf("albums = %v, want %v without Xylo, whose only track cannot be played", albums, want)
	}
}

// IDs of the photos fixture.
const (
	rowPhotosLibraryID = "eeeeeeee-eeee-4eee-8eee-eeeeeeeeeeee"
	photosOffline      = "eeeeeeee-0000-4eee-8eee-eeeeeeeeeeee"
	albumPlain         = "p0000001-0000-4000-8000-000000000000"
	albumOffline       = "p0000002-0000-4000-8000-000000000000"
	albumBroken        = "p0000003-0000-4000-8000-000000000000"
	albumOuter         = "p0000004-0000-4000-8000-000000000000"
	albumInner         = "p0000005-0000-4000-8000-000000000000"
	looseBroken        = "p0000006-0000-4000-8000-000000000000"
)

// rowPhoto is one photo below parentID (empty for a loose photo at the top).
type rowPhoto struct {
	id, parentID, rootID, probeError string
}

// seedRowPhotos adds a photos library: Plain holds an available photo, Offline only a photo
// in an offline folder, Broken only an unprobeable one; Outer holds no photo itself, only the
// album Inner with an available photo; a loose unprobeable photo lies at the top.
func seedRowPhotos(t *testing.T, fixture catalogFixture) {
	t.Helper()

	execute(t, fixture.inspection, `INSERT INTO libraries (id, name, kind, created_at, updated_at) VALUES (?1, 'Photos', 'photos', 0, 0)`, rowPhotosLibraryID)

	for position, rootID := range []string{rowPhotosLibraryID, photosOffline} {
		execute(t, fixture.inspection, `INSERT INTO library_roots (id, library_id, path, path_key, position, created_at)
			VALUES (?1, ?2, ?3, ?3, ?4, 0)`, rootID, rowPhotosLibraryID, "/media/photos-"+rootID, position)
	}

	albums := []struct{ id, parentID string }{{albumPlain, ""}, {albumOffline, ""}, {albumBroken, ""}, {albumOuter, ""}, {albumInner, albumOuter}}
	for index, album := range albums {
		insertListingItem(t, fixture.inspection, listingItem{
			id: album.id, libraryID: rowPhotosLibraryID, parentID: album.parentID, kind: "photo_album",
			title: album.id, sortTitle: album.id, createdAt: int64(index),
		})
	}

	photos := []rowPhoto{
		{id: "q0000001-0000-4000-8000-000000000000", parentID: albumPlain},
		{id: "q0000002-0000-4000-8000-000000000000", parentID: albumOffline, rootID: photosOffline},
		{id: "q0000003-0000-4000-8000-000000000000", parentID: albumBroken, probeError: "probe_failed"},
		{id: "q0000005-0000-4000-8000-000000000000", parentID: albumInner},
		{id: looseBroken, probeError: "probe_failed"},
	}

	for _, photo := range photos {
		insertListingItem(t, fixture.inspection, listingItem{
			id: photo.id, libraryID: rowPhotosLibraryID, parentID: photo.parentID, kind: "photo", title: photo.id, sortTitle: photo.id,
		})
		insertListingFile(t, fixture.inspection, listingFile{
			id: "h" + photo.id[1:], itemID: photo.id, libraryID: rowPhotosLibraryID, rootID: photo.rootID,
			relativePath: photo.id + ".jpg", container: "jpeg", sizeBytes: 1, probeError: photo.probeError,
		})
	}
}

// TestListRowItemsJudgesPhotoAlbumsByTheirPhotos checks that a photo album is available
// through a playable photo anywhere below it, and not through an offline or unprobeable one.
func TestListRowItemsJudgesPhotoAlbumsByTheirPhotos(t *testing.T) {
	t.Parallel()

	fixture := newListingFixture(t)
	seedRowPhotos(t, fixture)

	got := fixture.rowItemIDs(t, approws.ItemRequest{LibraryID: rowPhotosLibraryID, Query: library.ListingQuery{
		Filter: library.ListingFilter{Kind: library.ItemKindPhotoAlbum, OfflineRootIDs: []string{photosOffline}},
		Order:  library.ListingOrder{Sort: library.SortByTitle},
	}, Limit: 10})

	if want := []string{albumPlain, albumOuter}; !slices.Equal(got, want) {
		t.Errorf("photo albums = %v, want %v (Offline, Broken and the loose broken photo left out)", got, want)
	}
}

func TestListRowItemsUnknownLibrary(t *testing.T) {
	t.Parallel()

	fixture := newListingFixture(t)

	_, err := fixture.reader.ListRowItems(t.Context(), approws.ItemRequest{LibraryID: "unknown", Query: movieRowQuery(library.SortByDateAdded, true), Limit: 1})
	if !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("error = %v, want ErrNotFound", err)
	}

	_, err = fixture.reader.ListEpisodeGroups(t.Context(), approws.EpisodeRequest{LibraryID: "unknown", Limit: 1})
	if !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("episode groups error = %v, want ErrNotFound", err)
	}
}

// seedRecentEpisodes adds season 2 of Show A with an episode added eight days after the
// others, gives the fixture episodes air dates and breaks the probe of Show A's second
// episode.
func seedRecentEpisodes(t *testing.T, fixture catalogFixture) {
	t.Helper()

	insertListingItem(t, fixture.inspection, listingItem{
		id: seasonA2, libraryID: showsLibraryID, parentID: showA, kind: "season", title: "Season 2", sortTitle: "season 2", number: 2,
	})
	insertListingItem(t, fixture.inspection, listingItem{
		id: episodeA2New, libraryID: showsLibraryID, parentID: seasonA2, kind: "episode", title: "New", sortTitle: "new",
		number: 1, createdAt: eightDaysInMs, releaseDate: "2022-02-02",
	})
	insertListingFile(t, fixture.inspection, listingFile{
		id: "g0000009-0000-4000-8000-000000000000", itemID: episodeA2New, libraryID: showsLibraryID,
		relativePath: "Show A/Season 2/e1.mkv", container: "mkv", sizeBytes: 1,
	})

	execute(t, fixture.inspection, `UPDATE items SET release_date = '2020-01-01' WHERE id = ?`, episodeA1)
	execute(t, fixture.inspection, `UPDATE items SET release_date = '2023-01-01' WHERE id = ?`, episodeA2)
	execute(t, fixture.inspection, `UPDATE media_files SET probe_error = 'probe_failed' WHERE item_id = ?`, episodeA2)
	execute(t, fixture.inspection, `UPDATE items SET release_date = '2021-01-01' WHERE id = ?`, episodeB1)
}

func TestListEpisodeGroups(t *testing.T) {
	t.Parallel()

	fixture := newListingFixture(t)
	seedRecentEpisodes(t, fixture)

	got, err := fixture.reader.ListEpisodeGroups(t.Context(), approws.EpisodeRequest{
		LibraryID: showsLibraryID, Limit: 10, Window: rows.RecentEpisodeWindow,
	})
	if err != nil {
		t.Fatalf("ListEpisodeGroups: %v", err)
	}

	slices.SortFunc(got, func(left, right rows.EpisodeGroup) int { return right.NewestAdded.Compare(left.NewestAdded) })

	// Show A's episodes of season 1 are more than a week older than its new one, and its
	// second episode cannot be played anyway; Show B's only episode is its whole batch.
	want := []rows.EpisodeGroup{
		{ShowID: showA, SeasonID: seasonA2, EpisodeCount: 1, NewestAdded: time.UnixMilli(eightDaysInMs).UTC()},
		{ShowID: showB, SeasonID: seasonB1, EpisodeCount: 1, NewestAdded: time.UnixMilli(0).UTC()},
	}
	if !slices.Equal(got, want) {
		t.Errorf("groups = %+v, want %+v", got, want)
	}

	limited, err := fixture.reader.ListEpisodeGroups(t.Context(), approws.EpisodeRequest{
		LibraryID: showsLibraryID, Limit: 1, Window: 10 * rows.RecentEpisodeWindow,
	})
	if err != nil {
		t.Fatalf("ListEpisodeGroups(limit 1): %v", err)
	}

	// A wide window counts season 1 of Show A as well; the limit keeps only Show A.
	if len(limited) != 2 || limited[0].ShowID != showA || limited[1].ShowID != showA {
		t.Errorf("limited groups = %+v, want the two seasons of Show A", limited)
	}

	if !slices.ContainsFunc(limited, func(group rows.EpisodeGroup) bool { return group.SeasonID == seasonA1 && group.EpisodeCount == 1 }) {
		t.Errorf("limited groups = %+v, want season 1 of Show A with its playable episode", limited)
	}
}

func TestListRecentEpisodes(t *testing.T) {
	t.Parallel()

	fixture := newListingFixture(t)
	seedRecentEpisodes(t, fixture)

	got, err := fixture.reader.ListRecentEpisodes(t.Context(), approws.EpisodeRequest{
		LibraryID: showsLibraryID, OfflineRootIDs: []string{"another-folder"}, Limit: 10,
	})
	if err != nil {
		t.Fatalf("ListRecentEpisodes: %v", err)
	}

	// The episode aired 2023 cannot be played.
	want := []approws.EpisodeRef{
		{EpisodeID: episodeA2New, SeasonID: seasonA2, ShowID: showA},
		{EpisodeID: episodeB1, SeasonID: seasonB1, ShowID: showB},
		{EpisodeID: episodeA1, SeasonID: seasonA1, ShowID: showA},
	}
	if !slices.Equal(got, want) {
		t.Errorf("recent episodes = %+v, want %+v", got, want)
	}

	offline, err := fixture.reader.ListRecentEpisodes(t.Context(), approws.EpisodeRequest{
		LibraryID: showsLibraryID, OfflineRootIDs: []string{showsLibraryID}, Limit: 10,
	})
	if err != nil || len(offline) != 0 {
		t.Errorf("recent episodes in an offline folder = %+v, %v; want none", offline, err)
	}
}
