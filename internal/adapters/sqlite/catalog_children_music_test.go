package sqlite_test

import (
	"encoding/base64"
	"errors"
	"slices"
	"strings"
	"testing"

	applibrary "github.com/magpeek/mivo/internal/app/library"
	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/library"
)

// The children of an album are its tracks in track order (disc first), in both directions
// and across pages (task M3-13g).
func TestListChildrenOfAnAlbumFollowsTrackOrder(t *testing.T) {
	t.Parallel()

	fixture := newMusicFixture(t)
	albumID := newOrderingAlbum(t, &fixture)
	ascending := []string{"One", "Two", "Alpha", "bravo", "Second Disc"}

	for _, descending := range []bool{false, true} {
		want := slices.Clone(ascending)
		if descending {
			slices.Reverse(want)
		}

		for _, limit := range []int{100, 2, 1} {
			titles, _ := collectPages(t, limit, func(page library.PageRequest) (applibrary.ItemPage, error) {
				return fixture.reader.ListChildren(t.Context(), albumID, applibrary.ChildPageRequest{Page: page, Descending: descending})
			})

			assertDeepEqual(t, "album children", titles, want)
		}
	}
}

// A track carries its stored disc; the children of an artist are its albums.
func TestListChildrenReadsDiscNumbersAndArtistAlbums(t *testing.T) {
	t.Parallel()

	fixture := newMusicFixture(t)
	albumID := newOrderingAlbum(t, &fixture)

	page, err := fixture.reader.ListChildren(t.Context(), albumID, applibrary.ChildPageRequest{})
	if err != nil {
		t.Fatalf("ListChildren(album) error = %v", err)
	}

	last := page.Items[len(page.Items)-1].Item
	if disc, ok := last.DiscNumber.Get(); !ok || disc != 2 {
		t.Errorf("disc of %q = %v, want 2", last.Title, last.DiscNumber)
	}

	artistID := newDiscography(t, &fixture)

	albums, err := fixture.reader.ListChildren(t.Context(), artistID, applibrary.ChildPageRequest{})
	if err != nil {
		t.Fatalf("ListChildren(artist) error = %v", err)
	}

	assertDeepEqual(t, "artist children", pageTitles(albums), []string{"Moon Safari", "Talkie Walkie", "The Virgin Suicides"})
}

// A cursor names one listing: track cursors are refused for other parents and directions,
// child cursors for an album, and neither works as a cursor of an album listing.
func TestChildrenCursorsStayInTheirListing(t *testing.T) {
	t.Parallel()

	fixture := newMusicFixture(t)
	albumID := newOrderingAlbum(t, &fixture)
	artistID := newDiscography(t, &fixture)

	trackPage, err := fixture.reader.ListChildren(t.Context(), albumID, applibrary.ChildPageRequest{Page: library.PageRequest{Limit: 1}})
	if err != nil || trackPage.NextCursor == "" {
		t.Fatalf("track page = %+v, %v; want a next cursor", trackPage, err)
	}

	albumPage, err := fixture.reader.ListChildren(t.Context(), artistID, applibrary.ChildPageRequest{Page: library.PageRequest{Limit: 1}})
	if err != nil || albumPage.NextCursor == "" {
		t.Fatalf("album page = %+v, %v; want a next cursor", albumPage, err)
	}

	// Well-formed cursors of the other listing, scoped to the item they are used on.
	child := opaqueCursor("children", "asc", albumID, "0", "1", unknownID, "title")
	track := opaqueCursor("tracks", "asc", artistID, "1", "-1", unknownID, "file.flac")

	tests := []struct {
		name   string
		itemID string
		page   library.PageRequest
		down   bool
	}{
		{name: "track cursor reversed", itemID: albumID, page: library.PageRequest{Cursor: trackPage.NextCursor}, down: true},
		{name: "track cursor of another album", itemID: artistID, page: library.PageRequest{Cursor: trackPage.NextCursor}},
		{name: "child cursor on an album", itemID: albumID, page: library.PageRequest{Cursor: child}},
		{name: "track cursor on an artist", itemID: artistID, page: library.PageRequest{Cursor: track}},
	}

	for _, test := range tests {
		request := applibrary.ChildPageRequest{Page: test.page, Descending: test.down}
		if _, err := fixture.reader.ListChildren(t.Context(), test.itemID, request); !errors.Is(err, domain.ErrInvalid) {
			t.Errorf("%s: ListChildren error = %v, want domain.ErrInvalid", test.name, err)
		}
	}

	_, err = fixture.reader.ListLibraryAlbums(t.Context(), musicLibraryID, library.PageRequest{Cursor: trackPage.NextCursor})
	if !errors.Is(err, domain.ErrInvalid) {
		t.Errorf("ListLibraryAlbums with a track cursor error = %v, want domain.ErrInvalid", err)
	}
}

// opaqueCursor builds a cursor the way the reader encodes them (NUL-separated, base64url).
func opaqueCursor(fields ...string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(strings.Join(fields, "\x00")))
}
