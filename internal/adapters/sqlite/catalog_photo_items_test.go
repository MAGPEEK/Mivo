package sqlite_test

import (
	"errors"
	"fmt"
	"testing"
	"time"

	applibrary "github.com/magpeek/mivo/internal/app/library"
	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/access"
	"github.com/magpeek/mivo/internal/domain/artwork"
	"github.com/magpeek/mivo/internal/domain/library"
	"github.com/magpeek/mivo/internal/domain/media"
	"github.com/magpeek/mivo/internal/domain/naming"
	"github.com/magpeek/mivo/internal/domain/photo"
)

// photoTileTag is the tag of a test photo's tile rendered from relativePath.
func photoTileTag(relativePath string) string {
	return artwork.PhotoTag(relativePath, photoFileSize, time.UnixMilli(fileModifiedMilli))
}

// addVideoPhoto saves a video at relativePath as a photo taken at the given minute and
// returns its item chain.
func (fixture *photoFixture) addVideoPhoto(t *testing.T, relativePath string, minute int) []string {
	t.Helper()

	result, err := naming.Place(library.KindPhotos, naming.FileContext{Path: relativePath})
	if err != nil {
		t.Fatalf("Place(%s) error = %v", relativePath, err)
	}

	record := fixture.placedPhotoFile(relativePath, result.Placement)
	record.File.Description = media.Description{
		Container: media.ContainerMP4, Duration: time.Minute,
		Streams: []media.Stream{{Index: 0, Kind: media.StreamKindVideo, Codec: "h264", Width: 1920, Height: 1080}},
	}

	chain := fixture.save(t, record)
	fixture.saveDetails(t, readyDetails(chain[len(chain)-1], record.File.ID, minute))

	return chain
}

// refOf returns the reference of kind, nil when refs has none.
func refOf(refs map[artwork.Kind]artwork.Ref, kind artwork.Kind) *artwork.Ref {
	ref, found := refs[kind]
	if !found {
		return nil
	}

	return &ref
}

// tileOf describes a summary's tile for comparisons: "-" for none.
func tileOf(ref *artwork.Ref) string {
	if ref == nil {
		return "-"
	}

	return ref.ID + "@" + ref.Tag
}

// summaryLines describes each summary as "kind title counts tile [takenAt state]".
func summaryLines(items []applibrary.ItemSummary) []string {
	lines := make([]string, 0, len(items))

	for _, summary := range items {
		line := fmt.Sprintf("%s %s %d/%d %s", summary.Item.Kind, summary.Item.Title, summary.ChildCount, summary.LeafCount, tileOf(summary.Poster))
		if summary.Photo != nil {
			line += fmt.Sprintf(" %s %s", summary.Photo.Info.TakenAt.Format(time.TimeOnly), summary.Photo.ThumbnailState)
		}

		lines = append(lines, line)
	}

	return lines
}

// collectChildren reads every page of an item's children through the generic ChildReader.
func collectChildren(t *testing.T, fixture *photoFixture, itemID string, limit int) []applibrary.ItemSummary {
	t.Helper()

	request := applibrary.ChildPageRequest{Page: library.PageRequest{Limit: limit}}

	var items []applibrary.ItemSummary

	for {
		page, err := fixture.reader.ListChildren(t.Context(), itemID, request, access.Unrestricted())
		if err != nil {
			t.Fatalf("ListChildren(%s, limit %d) error = %v", itemID, limit, err)
		}

		items = append(items, page.Items...)

		if page.NextCursor == "" {
			return items
		}

		request.Page.Cursor = page.NextCursor
	}
}

// The children of a photo album come in album order with the photo details and the tiles: a
// still photo is its own tile (the JPEG of a RAW pair), a photo that cannot be rendered and
// a video have none, and a sub-album shows its newest still photo.
func TestListChildrenOfAPhotoAlbum(t *testing.T) {
	t.Parallel()

	fixture := newPhotoFixture(t)
	trip, jpegID := fixture.addPhotoFiles(t, "Trip/pair.jpg", "Trip/pair.arw")
	fixture.saveDetails(t, readyDetails(trip[1], jpegID, 5))

	broken, brokenID := fixture.addPhotoFiles(t, "Trip/broken.jpg")
	details := readyDetails(broken[1], brokenID, 7)
	details.ThumbnailState = photo.ThumbnailUnsupported
	fixture.saveDetails(t, details)

	video := fixture.addVideoPhoto(t, "Trip/clip.mp4", 9)
	beach := fixture.addPhoto(t, "Trip/Beach/sand.jpg", 1)
	fixture.addVideoPhoto(t, "Trip/Beach/wave.mp4", 2)

	want := []string{
		"photo_album Beach 2/2 " + beach[2] + "@" + photoTileTag("Trip/Beach/sand.jpg"),
		"photo pair 0/0 " + trip[1] + "@" + photoTileTag("Trip/pair.jpg") + " 12:05:00 ready",
		"photo broken 0/0 - 12:07:00 unsupported",
		"photo clip 0/0 - 12:09:00 ready",
	}

	for _, limit := range []int{1, 3, 100} {
		got := collectChildren(t, fixture, trip[0], limit)
		assertDeepEqual(t, fmt.Sprintf("children with limit %d", limit), summaryLines(got), want)

		if file := got[1].PrimaryFile.RelativePath; file != "Trip/pair.jpg" {
			t.Errorf("primary file of the pair = %s, want the JPEG", file)
		}

		if got[3].Item.ID != video[1] || got[3].PrimaryFile.Description.Container != media.ContainerMP4 {
			t.Errorf("video child = %+v, want %s with its MP4 file", got[3], video[1])
		}
	}
}

// A photo album lists its children ascending only, and a cursor of other children is
// refused for it, as a photo cursor is for a show.
func TestListChildrenOfAPhotoAlbumRejectsForeignRequests(t *testing.T) {
	t.Parallel()

	fixture := newPhotoFixture(t)
	album := fixture.addPhoto(t, "Trip/a.jpg", 1)[0]
	fixture.addPhoto(t, "Trip/b.jpg", 2)
	show := fixture.save(t, placedFile(photosLibraryID, testID('f', 90), "Show/S01E01.mkv", episodePlacement("show", 1, 1)))[0]
	fixture.save(t, placedFile(photosLibraryID, testID('f', 91), "Show/S02E01.mkv", episodePlacement("show", 2, 1)))

	photoPage, err := fixture.reader.ListChildren(t.Context(), album, applibrary.ChildPageRequest{Page: library.PageRequest{Limit: 1}}, access.Unrestricted())
	if err != nil || photoPage.NextCursor == "" {
		t.Fatalf("first photo page = %+v, %v", photoPage, err)
	}

	showPage, err := fixture.reader.ListChildren(t.Context(), show, applibrary.ChildPageRequest{Page: library.PageRequest{Limit: 1}}, access.Unrestricted())
	if err != nil || showPage.NextCursor == "" {
		t.Fatalf("show page error = %v", err)
	}

	requests := map[string]struct {
		itemID  string
		request applibrary.ChildPageRequest
	}{
		"descending album":         {album, applibrary.ChildPageRequest{Descending: true}},
		"photo cursor on a show":   {show, applibrary.ChildPageRequest{Page: library.PageRequest{Cursor: photoPage.NextCursor}}},
		"child cursor on an album": {album, applibrary.ChildPageRequest{Page: library.PageRequest{Cursor: showPage.NextCursor}}},
	}

	for name, test := range requests {
		if _, err := fixture.reader.ListChildren(t.Context(), test.itemID, test.request, access.Unrestricted()); !errors.Is(err, domain.ErrInvalid) {
			t.Errorf("%s: error = %v, want ErrInvalid", name, err)
		}
	}
}

// The detail of a photo carries its details and tile, the JPEG of a pair first and nothing
// to play; a video plays and has no tile; an album counts its photos with the sub-albums.
func TestGetItemOfPhotosAndAlbums(t *testing.T) {
	t.Parallel()

	fixture := newPhotoFixture(t)
	pair, jpegID := fixture.addPhotoFiles(t, "Trip/pair.jpg", "Trip/pair.arw")
	fixture.saveDetails(t, readyDetails(pair[1], jpegID, 5))

	video := fixture.addVideoPhoto(t, "Trip/clip.mp4", 9)
	fixture.addPhoto(t, "Trip/Beach/sand.jpg", 1)

	detail, err := fixture.reader.GetItem(t.Context(), pair[1], access.Unrestricted())
	if err != nil {
		t.Fatalf("GetItem(photo) error = %v", err)
	}

	if detail.Photo == nil || !detail.Photo.Info.TakenAt.Equal(photoTime(5)) || detail.PlayItemID != "" ||
		len(detail.Files) != 2 || detail.Files[0].ID != jpegID || detail.ItemFiles[0].File.ID != jpegID ||
		tileOf(refOf(detail.Artwork, artwork.KindPoster)) != pair[1]+"@"+photoTileTag("Trip/pair.jpg") {
		t.Errorf("photo detail = %+v, want details, the JPEG first, its tile and nothing to play", detail)
	}

	clip, err := fixture.reader.GetItem(t.Context(), video[1], access.Unrestricted())
	if _, hasTile := clip.Artwork[artwork.KindPoster]; err != nil || clip.Photo == nil || clip.PlayItemID != video[1] || hasTile {
		t.Errorf("video detail = %+v, %v, want details, play and no tile", clip, err)
	}

	album, err := fixture.reader.GetItem(t.Context(), pair[0], access.Unrestricted())
	if err != nil || album.Photo != nil || album.PlayItemID != "" || album.ChildCount != 3 || album.LeafCount != 3 ||
		album.Artwork[artwork.KindPoster].ID != pair[1] {
		t.Errorf("album detail = %+v, %v, want 3 children, 3 photos, the pair as tile", album, err)
	}
}

// A photos library lists its top-level albums under "photo_album", without the loose photos
// of the library folder, and every photo under "photo", with details and tiles.
func TestListItemsOfAPhotosLibrary(t *testing.T) {
	t.Parallel()

	fixture := newPhotoFixture(t)
	nested := fixture.addPhoto(t, "Trip/Beach/sand.jpg", 1)
	loose := fixture.addPhoto(t, "loose.jpg", 3)[0]
	cat := fixture.addPhoto(t, "Home/cat.jpg", 2)[1]

	albums, err := fixture.reader.ListItems(t.Context(), photosLibraryID, library.PageRequest{Limit: 10},
		library.ListingQuery{Restriction: access.Unrestricted(), Filter: library.ListingFilter{Kind: library.ItemKindPhotoAlbum}})
	if err != nil {
		t.Fatalf("ListItems(photo_album) error = %v", err)
	}

	assertDeepEqual(t, "albums", summaryLines(albums.Items), []string{
		"photo_album Home 1/1 " + cat + "@" + photoTileTag("Home/cat.jpg"),
		"photo_album Trip 1/1 " + nested[2] + "@" + photoTileTag("Trip/Beach/sand.jpg"),
	})

	photos, err := fixture.reader.ListItems(t.Context(), photosLibraryID, library.PageRequest{Limit: 10},
		library.ListingQuery{Restriction: access.Unrestricted(), Filter: library.ListingFilter{Kind: library.ItemKindPhoto}})
	if err != nil {
		t.Fatalf("ListItems(photo) error = %v", err)
	}

	assertDeepEqual(t, "photos", summaryLines(photos.Items), []string{
		"photo cat 0/0 " + cat + "@" + photoTileTag("Home/cat.jpg") + " 12:02:00 ready",
		"photo loose 0/0 " + loose + "@" + photoTileTag("loose.jpg") + " 12:03:00 ready",
		"photo sand 0/0 " + nested[2] + "@" + photoTileTag("Trip/Beach/sand.jpg") + " 12:01:00 ready",
	})
}
