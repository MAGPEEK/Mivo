package sqlite_test

import (
	"fmt"
	"slices"
	"testing"

	"github.com/magpeek/mivo/internal/domain/library"
)

const (
	listingMusicLibraryID = "cccccccc-cccc-4ccc-8ccc-cccccccccccc"
	artistA               = "a0000001-0000-4000-8000-000000000000"
	artistB               = "a0000002-0000-4000-8000-000000000000"
	albumZ                = "b0000001-0000-4000-8000-000000000000"
	albumY                = "b0000002-0000-4000-8000-000000000000"
	albumX                = "b0000003-0000-4000-8000-000000000000"
)

// seedListingMusic adds a music library: artist A with albums Zeta (two flac tracks) and Yell
// (one mp3 track), artist B with album Xylo (one flac track).
func seedListingMusic(t *testing.T, fixture catalogFixture) {
	t.Helper()

	execute(t, fixture.inspection, `INSERT INTO libraries (id, name, kind, created_at, updated_at) VALUES (?1, 'Music', 'music', 0, 0)`, listingMusicLibraryID)
	execute(t, fixture.inspection, `INSERT INTO library_roots (id, library_id, path, path_key, position, created_at)
		VALUES (?1, ?1, '/media/music', '/media/music', 0, 0)`, listingMusicLibraryID)

	albums := []struct {
		id, artist, title string
		tracks            []listingFile
	}{
		{albumZ, artistA, "Zeta", []listingFile{
			{container: "flac", sizeBytes: 30, streams: []listingStream{audio("flac", "eng")}},
			{container: "flac", sizeBytes: 40, streams: []listingStream{audio("flac", "eng")}},
		}},
		{albumY, artistA, "Yell", []listingFile{{container: "mp3", sizeBytes: 5, streams: []listingStream{audio("mp3", "eng")}}}},
		{albumX, artistB, "Xylo", []listingFile{{container: "flac", sizeBytes: 50, streams: []listingStream{audio("flac", "ger")}}}},
	}

	for _, artist := range []struct{ id, title string }{{artistA, "Artist A"}, {artistB, "Artist B"}} {
		insertListingItem(t, fixture.inspection, listingItem{id: artist.id, libraryID: listingMusicLibraryID, kind: "artist", title: artist.title, sortTitle: artist.title})
	}

	for albumIndex, album := range albums {
		insertListingItem(t, fixture.inspection, listingItem{id: album.id, libraryID: listingMusicLibraryID, parentID: album.artist, kind: "album", title: album.title, sortTitle: album.title})

		for trackIndex, file := range album.tracks {
			trackID := fmt.Sprintf("c%07d-%d000-4000-8000-000000000000", albumIndex+1, trackIndex)
			insertListingItem(t, fixture.inspection, listingItem{
				id: trackID, libraryID: listingMusicLibraryID, parentID: album.id, kind: "track",
				title: "Track", sortTitle: "track", number: trackIndex + 1,
			})

			file.id, file.itemID, file.libraryID = "h"+trackID[1:], trackID, listingMusicLibraryID
			file.relativePath = fmt.Sprintf("%s/%d.%s", album.title, trackIndex+1, file.container)
			insertListingFile(t, fixture.inspection, file)
		}
	}
}

func TestListItemsOfAMusicLibrary(t *testing.T) {
	t.Parallel()

	fixture := newCatalogFixture(t)
	seedListingMusic(t, fixture)

	tests := []struct {
		name  string
		query library.ListingQuery
		want  []string
	}{
		{name: "artists", query: musicQuery(library.ItemKindArtist, library.SortByTitle), want: []string{artistA, artistB}},
		{name: "albums below their artists", query: musicQuery(library.ItemKindAlbum, library.SortByTitle), want: []string{albumX, albumY, albumZ}},
		{name: "albums by size of their tracks", query: musicQuery(library.ItemKindAlbum, library.SortByFileSize), want: []string{albumY, albumX, albumZ}},
		{name: "artists by size of their tracks", query: musicQuery(library.ItemKindArtist, library.SortByFileSize), want: []string{artistB, artistA}},
		{name: "artists by track codec", query: withAudioCodec(musicQuery(library.ItemKindArtist, library.SortByTitle), "mp3"), want: []string{artistA}},
		{name: "albums by track codec", query: withAudioCodec(musicQuery(library.ItemKindAlbum, library.SortByTitle), "flac"), want: []string{albumX, albumZ}},
	}
	for _, test := range tests {
		got, total := fixture.listAllIDs(t, listingMusicLibraryID, test.query, 2)
		if !slices.Equal(got, test.want) || total != int64(len(test.want)) {
			t.Errorf("%s: ids = %v (total %d), want %v", test.name, got, total, test.want)
		}
	}
}

func musicQuery(kind library.ItemKind, sort library.ItemSort) library.ListingQuery {
	return library.ListingQuery{Filter: library.ListingFilter{Kind: kind}, Order: library.ListingOrder{Sort: sort}}
}

func withAudioCodec(query library.ListingQuery, codec string) library.ListingQuery {
	query.Filter.AudioCodecs = []string{codec}

	return query
}
