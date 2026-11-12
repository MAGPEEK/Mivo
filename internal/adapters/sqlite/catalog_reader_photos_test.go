package sqlite_test

import (
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	applibrary "github.com/magpeek/mivo/internal/app/library"
	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/access"
	"github.com/magpeek/mivo/internal/domain/artwork"
	"github.com/magpeek/mivo/internal/domain/library"
	"github.com/magpeek/mivo/internal/domain/photo"
)

// photoPagingItems and photoPagingBudget are the size and time budget of the paging smoke
// test: a large album read page by page at the largest page size.
const (
	photoPagingItems  = 10_000
	photoPagingBudget = time.Second
)

// childNames returns the titles of a page's children, photos with a "photo:" prefix.
func childNames(page applibrary.PhotoChildPage) []string {
	names := make([]string, 0, len(page.Children))

	for _, child := range page.Children {
		if child.Photo != nil {
			names = append(names, "photo:"+child.Item.Title)
		} else {
			names = append(names, fmt.Sprintf("album:%s(%d/%d)", child.Item.Title, child.ChildCount, child.LeafCount))
		}
	}

	return names
}

// collectPhotoChildren reads every page of an album and returns the children in order and
// the number of pages.
func collectPhotoChildren(t *testing.T, fixture *photoFixture, albumID string, limit int) (names []string, pages int) {
	t.Helper()

	request := library.PageRequest{Limit: limit}

	for pages = 1; ; pages++ {
		page, err := fixture.reader.ListPhotoChildren(t.Context(), albumID, request, access.Unrestricted())
		if err != nil {
			t.Fatalf("page %d error = %v", pages, err)
		}

		names = append(names, childNames(page)...)

		if page.NextCursor == "" {
			return names, pages
		}

		request.Cursor = page.NextCursor
	}
}

// An album lists its sub-albums by sort title, then its photos by capture time and file name
// (case-insensitive), the same with every page size.
func TestListPhotoChildrenOrdersAlbumsThenPhotos(t *testing.T) {
	t.Parallel()

	fixture := newPhotoFixture(t)
	album := fixture.addPhoto(t, "Trip/late.jpg", 30)[0]
	fixture.addPhoto(t, "Trip/b.jpg", 10)
	fixture.addPhoto(t, "Trip/A.jpg", 10)
	fixture.addPhoto(t, "Trip/early.jpg", 1)
	fixture.addPhoto(t, "Trip/Zoo/z.jpg", 1)
	fixture.addPhoto(t, "Trip/Beach/x.jpg", 1)
	fixture.addPhoto(t, "Trip/Beach/y.jpg", 2)
	newest := fixture.addPhoto(t, "Trip/Beach/Deep/d.jpg", 50)[3]
	fixture.addPhoto(t, "Other/o.jpg", 59)

	// A newer photo without a thumbnail counts as a leaf but is no cover.
	unsupported, fileID := fixture.addPhotoFiles(t, "Trip/Beach/Deep/e.jpg")
	details := readyDetails(unsupported[3], fileID, 55)
	details.ThumbnailState = photo.ThumbnailUnsupported
	fixture.saveDetails(t, details)

	want := []string{"album:Beach(3/4)", "album:Zoo(1/1)", "photo:early", "photo:A", "photo:b", "photo:late"}

	first, err := fixture.reader.ListPhotoChildren(t.Context(), album, library.PageRequest{Limit: 1}, access.Unrestricted())
	if err != nil || len(first.Children) != 1 || first.Children[0].CoverPhotoID != newest {
		t.Errorf("first page = %+v, %v, want Beach with cover %s", first, err, newest)
	}

	for _, limit := range []int{1, 2, 3, 6, 100} {
		got, pages := collectPhotoChildren(t, fixture, album, limit)
		assertDeepEqual(t, fmt.Sprintf("children with limit %d", limit), got, want)

		// The reader looks one row ahead, so the last page is never empty.
		if wantPages := (len(want) + limit - 1) / limit; pages != wantPages {
			t.Errorf("limit %d: %d pages, want %d", limit, pages, wantPages)
		}
	}
}

// collectLibraryPhotos reads every page of the photo library's photo listing.
func collectLibraryPhotos(t *testing.T, fixture *photoFixture, limit int) []string {
	t.Helper()

	var names []string

	request := library.PageRequest{Limit: limit}

	for {
		page, err := fixture.reader.ListLibraryPhotos(t.Context(), photosLibraryID, request, access.Unrestricted())
		if err != nil {
			t.Fatalf("limit %d: error = %v", limit, err)
		}

		names = append(names, childNames(page)...)

		if page.NextCursor == "" {
			return names
		}

		request.Cursor = page.NextCursor
	}
}

// The library listing holds every photo of the library across albums, by capture time, and
// rejects foreign cursors.
func TestListLibraryPhotosListsEveryPhotoByCaptureTime(t *testing.T) {
	t.Parallel()

	fixture := newPhotoFixture(t)
	album := fixture.addPhoto(t, "Trip/c.jpg", 3)[0]
	fixture.addPhoto(t, "Trip/Beach/b.jpg", 2)
	fixture.addPhoto(t, "root.jpg", 4)
	fixture.addPhoto(t, "Other/a.jpg", 2)

	want := []string{"photo:a", "photo:b", "photo:c", "photo:root"}

	for _, limit := range []int{1, 3, 10} {
		got := collectLibraryPhotos(t, fixture, limit)
		assertDeepEqual(t, fmt.Sprintf("photos with limit %d", limit), got, want)
	}

	albumPage, err := fixture.reader.ListPhotoChildren(t.Context(), album, library.PageRequest{Limit: 1}, access.Unrestricted())
	if err != nil {
		t.Fatalf("album page error = %v", err)
	}

	for _, request := range []library.PageRequest{{Cursor: albumPage.NextCursor}, {Limit: -1}} {
		if _, err := fixture.reader.ListLibraryPhotos(t.Context(), photosLibraryID, request, access.Unrestricted()); !errors.Is(err, domain.ErrInvalid) {
			t.Errorf("request %+v: error = %v, want ErrInvalid", request, err)
		}
	}

	// A well-formed cursor of this library's scope, but for the album group.
	scope := base64.RawURLEncoding.EncodeToString([]byte(
		strings.Join([]string{"photos", "library:" + photosLibraryID, "album", album, "", "trip"}, "\x00")))
	if _, err := fixture.reader.ListLibraryPhotos(t.Context(), photosLibraryID, library.PageRequest{Cursor: scope}, access.Unrestricted()); !errors.Is(err, domain.ErrInvalid) {
		t.Errorf("album-group cursor: error = %v, want ErrInvalid", err)
	}

	empty, err := fixture.reader.ListLibraryPhotos(t.Context(), unknownID, library.PageRequest{}, access.Unrestricted())
	if err != nil || len(empty.Children) != 0 {
		t.Errorf("unknown library = %+v, %v, want an empty page", empty, err)
	}
}

func TestListPhotoChildrenRejectsBadRequests(t *testing.T) {
	t.Parallel()

	fixture := newPhotoFixture(t)
	chain := fixture.addPhoto(t, "Trip/a.jpg", 1)
	fixture.addPhoto(t, "Trip/b.jpg", 2)
	other := fixture.addPhoto(t, "Other/c.jpg", 3)[0]

	first, err := fixture.reader.ListPhotoChildren(t.Context(), chain[0], library.PageRequest{Limit: 1}, access.Unrestricted())
	if err != nil || first.NextCursor == "" {
		t.Fatalf("first page = %v, %v, want a cursor", first, err)
	}

	cases := []struct {
		name    string
		albumID string
		page    library.PageRequest
		want    error
	}{
		{"unknown album", unknownID, library.PageRequest{}, domain.ErrNotFound},
		{"photo instead of album", chain[1], library.PageRequest{}, domain.ErrInvalid},
		{"cursor of another album", other, library.PageRequest{Cursor: first.NextCursor}, domain.ErrInvalid},
		{"garbage cursor", chain[0], library.PageRequest{Cursor: "%%%"}, domain.ErrInvalid},
		{"limit too large", chain[0], library.PageRequest{Limit: library.MaxPageLimit + 1}, domain.ErrInvalid},
	}

	for _, test := range cases {
		if _, err := fixture.reader.ListPhotoChildren(t.Context(), test.albumID, test.page, access.Unrestricted()); !errors.Is(err, test.want) {
			t.Errorf("%s: error = %v, want %v", test.name, err, test.want)
		}
	}
}

func TestPhotoDetailErrors(t *testing.T) {
	t.Parallel()

	fixture := newPhotoFixture(t)
	chain := fixture.addPhoto(t, "Trip/a.jpg", 1)
	bare, _ := fixture.addPhotoFiles(t, "Trip/no-info.jpg")

	cases := []struct {
		name   string
		itemID string
		want   error
	}{
		{"unknown item", unknownID, domain.ErrNotFound},
		{"album", chain[0], domain.ErrInvalid},
		{"photo without info", bare[1], domain.ErrNotFound},
	}

	for _, test := range cases {
		if _, err := fixture.reader.PhotoDetail(t.Context(), test.itemID, access.Unrestricted()); !errors.Is(err, test.want) {
			t.Errorf("%s: error = %v, want %v", test.name, err, test.want)
		}
	}
}

// PhotoFile and PhotoTags use the primary file of the details, never the RAW alternate that
// sorts first; without details they take the first file by path.
func TestPhotoSourceUsesThePrimaryFile(t *testing.T) {
	t.Parallel()

	fixture := newPhotoFixture(t)
	chain, fileID := fixture.addPhotoFiles(t, "Trip/a.jpg", "Trip/a.arw")
	fixture.saveDetails(t, readyDetails(chain[1], fileID, 1))

	bare, _ := fixture.addPhotoFiles(t, "c.jpg", "c.arw")
	single := fixture.addPhoto(t, "b.png", 2)[0]

	file, err := fixture.reader.PhotoFile(t.Context(), chain[1])
	if err != nil {
		t.Fatalf("PhotoFile error = %v", err)
	}

	if file.LibraryRoot != photosRoot || file.RelativePath != "Trip/a.jpg" || file.ItemID != chain[1] || file.Size != photoFileSize {
		t.Errorf("PhotoFile = %+v, want Trip/a.jpg in %s", file, photosRoot)
	}

	tags, err := fixture.reader.PhotoTags(t.Context())
	if err != nil {
		t.Fatalf("PhotoTags error = %v", err)
	}

	modified := time.UnixMilli(fileModifiedMilli).UTC()
	want := map[string]string{
		chain[1]: artwork.PhotoTag("Trip/a.jpg", photoFileSize, modified),
		bare[0]:  artwork.PhotoTag("c.arw", photoFileSize, modified),
		single:   artwork.PhotoTag("b.png", photoFileSize, modified),
	}
	assertDeepEqual(t, "photo tags", tags, want)

	for _, itemID := range []string{unknownID, chain[0]} {
		if _, err := fixture.reader.PhotoFile(t.Context(), itemID); !errors.Is(err, domain.ErrNotFound) {
			t.Errorf("PhotoFile(%s) error = %v, want ErrNotFound", itemID, err)
		}
	}
}

// Paging through a 10 000-photo album at the largest page size stays under a second.
func TestListPhotoChildrenPagesALargeAlbumQuickly(t *testing.T) {
	t.Parallel()

	fixture := newPhotoFixture(t)
	album := fixture.addPhoto(t, "Big/first.jpg", 0)[0]
	insertManyPhotos(t, fixture, album, photoPagingItems)

	start := time.Now()
	names, pages := collectPhotoChildren(t, fixture, album, library.MaxPageLimit)
	elapsed := time.Since(start)

	t.Logf("listed %d photos in %d pages in %v (race detector %t, coverage %q)",
		len(names), pages, elapsed, raceDetectorEnabled, testing.CoverMode())

	unique := map[string]bool{}
	for _, name := range names {
		unique[name] = true
	}

	if len(names) != photoPagingItems+1 || len(unique) != len(names) {
		t.Errorf("listed %d photos (%d distinct), want %d", len(names), len(unique), photoPagingItems+1)
	}

	if elapsed > photoPagingBudget && !raceDetectorEnabled && testing.CoverMode() == "" {
		t.Errorf("paging took %v, want under %v", elapsed, photoPagingBudget)
	}
}

// insertManyPhotos stores count photos with their info below albumID directly, which is much
// faster than one SavePhoto transaction per photo. Every tenth photo shares a capture time.
func insertManyPhotos(t *testing.T, fixture *photoFixture, albumID string, count int) {
	t.Helper()

	transaction, err := fixture.inspection.BeginTx(t.Context(), nil)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}

	defer func() { _ = transaction.Rollback() }()

	for number := range count {
		itemID, name := testID('p', number), fmt.Sprintf("img_%05d.jpg", number)
		statements := []string{
			`INSERT INTO items (id, library_id, parent_id, kind, match_key, title, sort_title, created_at, updated_at)
				VALUES (?1, ?2, ?3, 'photo', ?4, ?4, ?4, 0, 0)`,
			`INSERT INTO photo_info (item_id, library_id, parent_id, name_key, taken_at, taken_at_source, make, model, lens,
				exposure, f_number, iso, focal_mm, orientation, width, height, thumbnail_state)
				VALUES (?1, ?2, ?3, ?4, ?5, 'exif', '', '', '', 0, 0, 0, 0, 0, 0, 0, 'ready')`,
		}

		for _, statement := range statements {
			if _, err := transaction.ExecContext(t.Context(), statement,
				itemID, photosLibraryID, albumID, name, photoTime(number/10).UnixMilli()); err != nil {
				t.Fatalf("insert photo %d: %v", number, err)
			}
		}
	}

	if err := transaction.Commit(); err != nil {
		t.Fatalf("commit: %v", err)
	}
}
