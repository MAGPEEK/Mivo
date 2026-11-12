package sqlite_test

import (
	"errors"
	"slices"
	"testing"

	applibrary "github.com/magpeek/mivo/internal/app/library"
	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/access"
	"github.com/magpeek/mivo/internal/domain/library"
	"github.com/magpeek/mivo/internal/domain/media"
	"github.com/magpeek/mivo/internal/domain/metadata"
)

func taggedTrack(path string, disc, track int, title string) musicTrack {
	return musicTrack{path: path, disc: disc, tags: media.AudioTags{
		AlbumArtist: "Daft Punk", Artist: "Daft Punk", Album: "Discovery", Title: title,
		Disc: disc, Track: track, Year: 2001,
		MusicBrainzArtistID: artistMBID, MusicBrainzAlbumID: albumMBID, MusicBrainzTrackID: trackMBID,
	}}
}

// Tracks of one album share the artist and album items; a rescan finds the same items again
// through their match keys, so their IDs are stable.
func TestSaveTrackDetailsKeepsStableItemsAndStoresDiscAndProviderIDs(t *testing.T) {
	t.Parallel()

	fixture := newMusicFixture(t)
	first := fixture.addTrack(t, taggedTrack("Daft Punk/Discovery/CD 2/01 One More Time.flac", 2, 1, "One More Time"))
	second := fixture.addTrack(t, taggedTrack("Daft Punk/Discovery/CD 2/02 Aerodynamic.flac", 2, 2, "Aerodynamic"))

	if !slices.Equal(first[:2], second[:2]) {
		t.Fatalf("chains %v and %v do not share artist and album", first, second)
	}

	fixture.files = 0
	again := fixture.addTrack(t, taggedTrack("Daft Punk/Discovery/CD 2/01 One More Time.flac", 2, 1, "One More Time"))
	assertDeepEqual(t, "chain after the rescan", again, first)

	discs := queryStrings(t, fixture.inspection, "SELECT ifnull(disc_number, 'null') FROM items WHERE kind = 'track' ORDER BY id")
	assertDeepEqual(t, "disc numbers", discs, []string{"2", "2"})

	assertDeepEqual(t, "artist IDs", fixture.providerIDs(t, first[0]), []string{"musicbrainz_artist=" + artistMBID})
	assertDeepEqual(t, "album IDs", fixture.providerIDs(t, first[1]), []string{"musicbrainz_album=" + albumMBID})
	assertDeepEqual(t, "track IDs", fixture.providerIDs(t, first[2]), []string{"musicbrainz_track=" + trackMBID})
}

// The artist ID of the tags names the track artist, so it is stored on the artist item only
// when that is the album artist: never on "Various Artists" or a featured artist.
func TestSaveTrackDetailsStoresTheArtistIDOnlyForTheAlbumArtist(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		tags media.AudioTags
		want []string
	}{
		{
			"same artist",
			media.AudioTags{Artist: "Air", Album: "Moon Safari", Title: "La Femme", MusicBrainzArtistID: artistMBID},
			[]string{"musicbrainz_artist=" + artistMBID},
		},
		{
			"compilation",
			media.AudioTags{Artist: "Air", Album: "Hits", Title: "La Femme", Compilation: true, MusicBrainzArtistID: artistMBID},
			nil,
		},
		{"featured artist", media.AudioTags{
			AlbumArtist: "Air", Artist: "Air feat. Beth Hirsch", Album: "Moon Safari", Title: "You",
			MusicBrainzArtistID: artistMBID,
		}, nil},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			fixture := newMusicFixture(t)
			chain := fixture.addTrack(t, musicTrack{path: "Air/Album/01 Track.mp3", tags: test.tags})

			assertDeepEqual(t, "artist IDs", fixture.providerIDs(t, chain[0]), test.want)
		})
	}
}

// A track that loses its MusicBrainz tag loses the stored track ID; the album keeps its ID,
// because other tracks may carry it. A newer value replaces the stored one.
func TestSaveTrackDetailsUpdatesTrackIDsAndKeepsSharedOnes(t *testing.T) {
	t.Parallel()

	fixture := newMusicFixture(t)
	track := taggedTrack("Daft Punk/Discovery/01 One More Time.flac", 1, 1, "One More Time")
	chain := fixture.addTrack(t, track)

	track.tags.MusicBrainzTrackID, track.tags.MusicBrainzAlbumID = "", ""
	if err := fixture.catalog.SaveTrackDetails(t.Context(), chain[2], 1, track.tags); err != nil {
		t.Fatalf("SaveTrackDetails error = %v", err)
	}

	assertDeepEqual(t, "track IDs without tag", fixture.providerIDs(t, chain[2]), []string(nil))
	assertDeepEqual(t, "album IDs without tag", fixture.providerIDs(t, chain[1]), []string{"musicbrainz_album=" + albumMBID})

	track.tags.MusicBrainzAlbumID = otherMBID
	if err := fixture.catalog.SaveTrackDetails(t.Context(), chain[2], 1, track.tags); err != nil {
		t.Fatalf("SaveTrackDetails error = %v", err)
	}

	assertDeepEqual(t, "album IDs after retag", fixture.providerIDs(t, chain[1]), []string{"musicbrainz_album=" + otherMBID})
}

func TestSaveTrackDetailsRejectsInvalidInput(t *testing.T) {
	t.Parallel()

	fixture := newMusicFixture(t)
	chain := fixture.addTrack(t, taggedTrack("Daft Punk/Discovery/01 One More Time.flac", 1, 1, "One More Time"))
	valid := taggedTrack("", 1, 1, "x").tags

	tests := []struct {
		name   string
		itemID string
		disc   int
		tags   media.AudioTags
		want   error
	}{
		{"unknown item", unknownID, 1, valid, domain.ErrNotFound},
		{"album instead of track", chain[1], 1, valid, domain.ErrInvalid},
		{"disc zero", chain[2], 0, valid, domain.ErrInvalid},
		{"malformed MusicBrainz ID", chain[2], 1, media.AudioTags{MusicBrainzTrackID: "not-a-uuid"}, domain.ErrInvalid},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			err := fixture.catalog.SaveTrackDetails(t.Context(), test.itemID, test.disc, test.tags)
			if !errors.Is(err, test.want) {
				t.Errorf("SaveTrackDetails error = %v, want %v", err, test.want)
			}
		})
	}
}

func TestSaveTrackDetailsFailsWithACancelledContext(t *testing.T) {
	t.Parallel()

	fixture := newMusicFixture(t)
	chain := fixture.addTrack(t, taggedTrack("Daft Punk/Discovery/01 One More Time.flac", 1, 1, "One More Time"))

	if err := fixture.catalog.SaveTrackDetails(cancelledContext(t), chain[2], 1, media.AudioTags{}); err == nil {
		t.Error("SaveTrackDetails with a cancelled context succeeded")
	}
}

// The metadata writer owns only the non-music providers: saving a record keeps the
// MusicBrainz IDs, and reading the record back skips them instead of failing.
func TestMetadataWriterKeepsMusicBrainzIDs(t *testing.T) {
	t.Parallel()

	fixture := newMusicFixture(t)
	chain := fixture.addTrack(t, taggedTrack("Daft Punk/Discovery/01 One More Time.flac", 1, 1, "One More Time"))
	record := metadata.Record{Title: "One More Time", ProviderIDs: []metadata.ProviderID{{Provider: metadata.ProviderTMDB, Value: "42"}}}

	if err := fixture.catalog.SaveMetadata(t.Context(), chain[2], record); err != nil {
		t.Fatalf("SaveMetadata error = %v", err)
	}

	assertDeepEqual(t, "track IDs", fixture.providerIDs(t, chain[2]), []string{"musicbrainz_track=" + trackMBID, "tmdb=42"})

	detail, err := fixture.reader.GetItem(t.Context(), chain[2], access.Unrestricted())
	if err != nil {
		t.Fatalf("GetItem error = %v", err)
	}

	assertDeepEqual(t, "provider IDs of the record", detail.Metadata.ProviderIDs, record.ProviderIDs)
}

// newOrderingAlbum stores one album whose tracks exercise every rule of library.CompareTracks.
func newOrderingAlbum(t *testing.T, fixture *musicFixture) string {
	t.Helper()

	tracks := []musicTrack{
		{path: "Band/Album/CD 2/01 Second Disc.flac", disc: 2},
		{path: "Band/Album/CD 1/02 Two.flac", disc: 1},
		{path: "Band/Album/CD 1/bravo.flac", disc: 1},
		{path: "Band/Album/CD 1/01 One.flac", disc: 1},
		{path: "Band/Album/CD 1/Alpha.flac", disc: 1},
	}

	var albumID string
	for _, track := range tracks {
		albumID = fixture.addTrack(t, track)[1]
	}

	return albumID
}

// Tracks are ordered by disc, then track number (unnumbered last), then file name, on one
// page and across pages.
func TestListAlbumTracksOrdersByDiscTrackAndFileName(t *testing.T) {
	t.Parallel()

	fixture := newMusicFixture(t)
	albumID := newOrderingAlbum(t, &fixture)
	want := []string{"One", "Two", "Alpha", "bravo", "Second Disc"}

	for _, limit := range []int{100, 2, 1} {
		titles, pages := collectPages(t, limit, func(page library.PageRequest) (applibrary.ItemPage, error) {
			return fixture.reader.ListAlbumTracks(t.Context(), albumID, page, access.Unrestricted())
		})

		assertDeepEqual(t, "track titles", titles, want)

		if wantPages := (len(want) + limit - 1) / limit; pages != wantPages {
			t.Errorf("limit %d: pages = %d, want %d", limit, pages, wantPages)
		}
	}
}

func TestListAlbumTracksReturnsTrackSummaries(t *testing.T) {
	t.Parallel()

	fixture := newMusicFixture(t)
	albumID := newOrderingAlbum(t, &fixture)

	page, err := fixture.reader.ListAlbumTracks(t.Context(), albumID, library.PageRequest{Limit: 1}, access.Unrestricted())
	if err != nil {
		t.Fatalf("ListAlbumTracks error = %v", err)
	}

	summary := page.Items[0]
	if summary.Item.Kind != library.ItemKindTrack || summary.PrimaryFile.RelativePath != "Band/Album/CD 1/01 One.flac" {
		t.Errorf("first track = %+v, want the track of 01 One.flac with its file", summary)
	}
}

func TestListAlbumTracksRejectsBadRequests(t *testing.T) {
	t.Parallel()

	fixture := newMusicFixture(t)
	albumID := newOrderingAlbum(t, &fixture)
	other := fixture.addTrack(t, musicTrack{path: "Band/Other/01 X.flac"})

	first, err := fixture.reader.ListAlbumTracks(t.Context(), albumID, library.PageRequest{Limit: 2}, access.Unrestricted())
	if err != nil || first.NextCursor == "" {
		t.Fatalf("first page = %+v, %v, want a next cursor", first, err)
	}

	tests := []struct {
		name   string
		itemID string
		page   library.PageRequest
		want   error
	}{
		{"unknown album", unknownID, library.PageRequest{}, domain.ErrNotFound},
		{"artist instead of album", other[0], library.PageRequest{}, domain.ErrInvalid},
		{"cursor of another album", other[1], library.PageRequest{Cursor: first.NextCursor}, domain.ErrInvalid},
		{"malformed cursor", albumID, library.PageRequest{Cursor: "%%%"}, domain.ErrInvalid},
		{"album listing cursor", albumID, library.PageRequest{Cursor: "c29tZQ"}, domain.ErrInvalid},
		{"limit too large", albumID, library.PageRequest{Limit: library.MaxPageLimit + 1}, domain.ErrInvalid},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			if _, err := fixture.reader.ListAlbumTracks(t.Context(), test.itemID, test.page, access.Unrestricted()); !errors.Is(err, test.want) {
				t.Errorf("ListAlbumTracks error = %v, want %v", err, test.want)
			}
		})
	}
}

// newDiscography stores two artists with albums in an order that differs from title order.
func newDiscography(t *testing.T, fixture *musicFixture) (artistID string) {
	t.Helper()

	for _, path := range []string{
		"Air/Talkie Walkie/01 Venus.flac",
		"Air/Moon Safari/01 La Femme.flac",
		"Air/The Virgin Suicides/01 Playground Love.flac",
		"Air/Moon Safari/02 Sexy Boy.flac",
		"Beck/Odelay/01 Devils Haircut.flac",
	} {
		chain := fixture.addTrack(t, musicTrack{path: path})
		if artistID == "" {
			artistID = chain[0]
		}
	}

	return artistID
}

// Albums are ordered by sort title ("The" ignored), on one page and across pages.
func TestListArtistAlbumsOrdersBySortTitleAcrossPages(t *testing.T) {
	t.Parallel()

	fixture := newMusicFixture(t)
	artistID := newDiscography(t, &fixture)
	want := []string{"Moon Safari", "Talkie Walkie", "The Virgin Suicides"}

	for _, limit := range []int{100, 2, 1} {
		titles, _ := collectPages(t, limit, func(page library.PageRequest) (applibrary.ItemPage, error) {
			return fixture.reader.ListArtistAlbums(t.Context(), artistID, page, access.Unrestricted())
		})

		assertDeepEqual(t, "album titles", titles, want)
	}

	page, err := fixture.reader.ListArtistAlbums(t.Context(), artistID, library.PageRequest{Limit: 1}, access.Unrestricted())
	if err != nil {
		t.Fatalf("ListArtistAlbums error = %v", err)
	}

	if got := page.Items[0].ChildCount; got != 2 {
		t.Errorf("track count of Moon Safari = %d, want 2", got)
	}
}

func TestListLibraryAlbumsPagesOverEveryArtist(t *testing.T) {
	t.Parallel()

	fixture := newMusicFixture(t)
	newDiscography(t, &fixture)
	want := []string{"Moon Safari", "Odelay", "Talkie Walkie", "The Virgin Suicides"}

	for _, limit := range []int{100, 3, 1} {
		titles, _ := collectPages(t, limit, func(page library.PageRequest) (applibrary.ItemPage, error) {
			return fixture.reader.ListLibraryAlbums(t.Context(), musicLibraryID, page, access.Unrestricted())
		})

		assertDeepEqual(t, "album titles", titles, want)
	}

	page, err := fixture.reader.ListLibraryAlbums(t.Context(), moviesLibraryID, library.PageRequest{}, access.Unrestricted())
	if err != nil || len(page.Items) != 0 {
		t.Errorf("albums of the movies library = %+v, %v, want an empty page", page, err)
	}
}

func TestAlbumListingsRejectBadRequests(t *testing.T) {
	t.Parallel()

	fixture := newMusicFixture(t)
	artistID := newDiscography(t, &fixture)
	ctx := t.Context()

	albumOf := func(itemID string, page library.PageRequest) error {
		_, err := fixture.reader.ListArtistAlbums(ctx, itemID, page, access.Unrestricted())

		return err
	}
	albumsIn := func(libraryID string, page library.PageRequest) error {
		_, err := fixture.reader.ListLibraryAlbums(ctx, libraryID, page, access.Unrestricted())

		return err
	}

	tests := []struct {
		name string
		call func(string, library.PageRequest) error
		id   string
		page library.PageRequest
		want error
	}{
		{"unknown artist", albumOf, unknownID, library.PageRequest{}, domain.ErrNotFound},
		{"album instead of artist", albumOf, albumIDOf(t, fixture), library.PageRequest{}, domain.ErrInvalid},
		{"artist with malformed cursor", albumOf, artistID, library.PageRequest{Cursor: "%%%"}, domain.ErrInvalid},
		{"artist with limit too large", albumOf, artistID, library.PageRequest{Limit: library.MaxPageLimit + 1}, domain.ErrInvalid},
		{"unknown library", albumsIn, unknownID, library.PageRequest{}, domain.ErrNotFound},
		{"library with malformed cursor", albumsIn, musicLibraryID, library.PageRequest{Cursor: "%%%"}, domain.ErrInvalid},
		{"library with limit too large", albumsIn, musicLibraryID, library.PageRequest{Limit: -1}, domain.ErrInvalid},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			if err := test.call(test.id, test.page); !errors.Is(err, test.want) {
				t.Errorf("error = %v, want %v", err, test.want)
			}
		})
	}
}

func albumIDOf(t *testing.T, fixture musicFixture) string {
	t.Helper()

	ids := queryStrings(t, fixture.inspection, "SELECT id FROM items WHERE kind = 'album' ORDER BY id LIMIT 1")

	return ids[0]
}
