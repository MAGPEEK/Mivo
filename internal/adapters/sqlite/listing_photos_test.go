package sqlite_test

import (
	"slices"
	"testing"

	"github.com/magpeek/mivo/internal/domain/access"
	"github.com/magpeek/mivo/internal/domain/library"
)

const (
	listingPhotosLibraryID = "dddddddd-dddd-4ddd-8ddd-dddddddddddd"
	photoAlbumTrip         = "e0000001-0000-4000-8000-000000000000"
	photoAlbumBroken       = "e0000002-0000-4000-8000-000000000000"
)

// seedListingPhotos adds a photos library: album Trip with a readable JPEG and album Broken
// whose only photo failed to probe.
func seedListingPhotos(t *testing.T, fixture catalogFixture) {
	t.Helper()

	execute(t, fixture.inspection, `INSERT INTO libraries (id, name, kind, created_at, updated_at) VALUES (?1, 'Photos', 'photos', 0, 0)`, listingPhotosLibraryID)
	execute(t, fixture.inspection, `INSERT INTO library_roots (id, library_id, path, path_key, position, created_at)
		VALUES (?1, ?1, '/media/photos', '/media/photos', 0, 0)`, listingPhotosLibraryID)

	albums := []struct{ id, title, photoID, probeError string }{
		{photoAlbumTrip, "Trip", "f0000001-0000-4000-8000-000000000000", ""},
		{photoAlbumBroken, "Broken", "f0000002-0000-4000-8000-000000000000", "unsupported"},
	}

	for _, album := range albums {
		insertListingItem(t, fixture.inspection, listingItem{
			id: album.id, libraryID: listingPhotosLibraryID, kind: "photo_album", title: album.title, sortTitle: album.title,
		})
		insertListingItem(t, fixture.inspection, listingItem{
			id: album.photoID, libraryID: listingPhotosLibraryID, parentID: album.id, kind: "photo", title: "IMG", sortTitle: "img",
		})
		insertListingFile(t, fixture.inspection, listingFile{
			id: "h" + album.photoID[1:], itemID: album.photoID, libraryID: listingPhotosLibraryID,
			relativePath: album.title + "/img.jpg", container: "unknown", sizeBytes: 10, probeError: album.probeError,
		})
	}
}

// A photo album is decided by its photos, as a music album by its tracks: an album with a
// readable photo is available, an album whose photos all failed is not.
func TestListPhotoAlbumsDecidedByTheirPhotos(t *testing.T) {
	t.Parallel()

	fixture := newCatalogFixture(t)
	seedListingPhotos(t, fixture)

	tests := []struct {
		name   string
		filter library.ListingFilter
		want   []string
	}{
		{name: "every album", filter: library.ListingFilter{Kind: library.ItemKindPhotoAlbum}, want: []string{photoAlbumBroken, photoAlbumTrip}},
		{
			name: "unavailable albums", filter: library.ListingFilter{Kind: library.ItemKindPhotoAlbum, Quick: library.QuickUnavailable},
			want: []string{photoAlbumBroken},
		},
	}

	for _, test := range tests {
		query := library.ListingQuery{Restriction: access.Unrestricted(), Filter: test.filter, Order: library.ListingOrder{Sort: library.SortByTitle}}

		got, total := fixture.listAllIDs(t, listingPhotosLibraryID, query, 2)
		if !slices.Equal(got, test.want) || total != int64(len(test.want)) {
			t.Errorf("%s: ids = %v (total %d), want %v", test.name, got, total, test.want)
		}
	}
}
