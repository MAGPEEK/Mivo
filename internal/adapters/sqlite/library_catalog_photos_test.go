package sqlite_test

import (
	"errors"
	"math"
	"testing"
	"time"

	"github.com/magpeek/mivo/internal/app/scan"
	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/photo"
)

// The details of a nested photo with a RAW partner come back from PhotoDetail, and the
// photo row copies the item's library and album and the primary file's name.
func TestSavePhotoDetailsStoresTheInfo(t *testing.T) {
	t.Parallel()

	fixture := newPhotoFixture(t)
	chain, fileID := fixture.addPhotoFiles(t, "2020/Summer/IMG_1.jpg", "2020/Summer/IMG_1.CR2")

	kinds := queryStrings(t, fixture.inspection, "SELECT kind || ':' || title FROM items ORDER BY id")
	assertDeepEqual(t, "items", kinds, []string{"photo_album:2020", "photo_album:Summer", "photo:IMG 1"})

	info := photo.Info{
		TakenAt: photoTime(5), Make: "Canon", Model: "EOS R5", Lens: "RF 35mm", ExposureSeconds: 0.004,
		FNumber: 1.8, ISO: 200, FocalLengthMM: 35, Orientation: 6, Width: 8192, Height: 5464,
		GPS: &photo.Coordinates{Latitude: 47.5, Longitude: -122.25},
	}
	fixture.saveDetails(t, scan.PhotoDetails{ItemID: chain[2], PrimaryFileID: fileID, Info: info, TakenAtSource: photo.TakenAtExif, ThumbnailState: photo.ThumbnailReady})

	detail, err := fixture.reader.PhotoDetail(t.Context(), chain[2])
	if err != nil {
		t.Fatalf("PhotoDetail error = %v", err)
	}

	assertDeepEqual(t, "info", detail.Info, info)

	if detail.ThumbnailState != photo.ThumbnailReady || detail.TakenAtSource != photo.TakenAtExif {
		t.Errorf("thumbnail state, time source = %q, %q, want ready, exif", detail.ThumbnailState, detail.TakenAtSource)
	}

	row := queryStrings(t, fixture.inspection, "SELECT library_id || ' ' || parent_id || ' ' || primary_file_id || ' ' || name_key FROM photo_info")
	assertDeepEqual(t, "photo row", row, []string{photosLibraryID + " " + chain[1] + " " + fileID + " img_1.jpg"})
}

// Saving details again replaces them; a capture time from the file reads back as such, and a
// top-level photo has no album.
func TestSavePhotoDetailsAgainReplacesThem(t *testing.T) {
	t.Parallel()

	fixture := newPhotoFixture(t)
	chain, fileID := fixture.addPhotoFiles(t, "a.png")
	details := scan.PhotoDetails{
		ItemID: chain[0], PrimaryFileID: fileID, TakenAtSource: photo.TakenAtExif, ThumbnailState: photo.ThumbnailReady,
		Info: photo.Info{TakenAt: photoTime(1), GPS: &photo.Coordinates{Latitude: 1, Longitude: 2}},
	}
	fixture.saveDetails(t, details)

	modified := time.UnixMilli(fileModifiedMilli).UTC()
	details.Info, details.TakenAtSource, details.ThumbnailState = photo.Info{TakenAt: modified}, photo.TakenAtFile, photo.ThumbnailUnsupported
	fixture.saveDetails(t, details)

	detail, err := fixture.reader.PhotoDetail(t.Context(), chain[0])
	if err != nil {
		t.Fatalf("PhotoDetail error = %v", err)
	}

	assertDeepEqual(t, "info", detail.Info, photo.Info{TakenAt: time.UnixMilli(fileModifiedMilli).UTC()})

	if detail.ThumbnailState != photo.ThumbnailUnsupported || detail.TakenAtSource != photo.TakenAtFile {
		t.Errorf("thumbnail state, time source = %q, %q, want unsupported, file", detail.ThumbnailState, detail.TakenAtSource)
	}

	parents := queryStrings(t, fixture.inspection, "SELECT ifnull(parent_id, 'none') FROM photo_info")
	assertDeepEqual(t, "parent", parents, []string{"none"})
}

func TestSavePhotoDetailsRejectsInvalidDetails(t *testing.T) {
	t.Parallel()

	fixture := newPhotoFixture(t)
	chain, fileID := fixture.addPhotoFiles(t, "Album/a.jpg")
	_, otherFileID := fixture.addPhotoFiles(t, "Album/b.jpg")
	valid := readyDetails(chain[1], fileID, 1)

	cases := []struct {
		name   string
		change func(*scan.PhotoDetails)
		want   error
	}{
		{"empty item id", func(details *scan.PhotoDetails) { details.ItemID = "" }, domain.ErrInvalid},
		{"empty file id", func(details *scan.PhotoDetails) { details.PrimaryFileID = "" }, domain.ErrInvalid},
		{"unknown state", func(details *scan.PhotoDetails) { details.ThumbnailState = "pending" }, domain.ErrInvalid},
		{"missing time source", func(details *scan.PhotoDetails) { details.TakenAtSource = "" }, domain.ErrInvalid},
		{"unknown time source", func(details *scan.PhotoDetails) { details.TakenAtSource = "xmp" }, domain.ErrInvalid},
		{"zero capture time", func(details *scan.PhotoDetails) { details.Info.TakenAt = time.Time{} }, domain.ErrInvalid},
		{"invalid info", func(details *scan.PhotoDetails) { details.Info.FNumber = math.Inf(1) }, domain.ErrInvalid},
		{"album item", func(details *scan.PhotoDetails) { details.ItemID = chain[0] }, domain.ErrInvalid},
		{"file of another photo", func(details *scan.PhotoDetails) { details.PrimaryFileID = otherFileID }, domain.ErrInvalid},
		{"unknown item", func(details *scan.PhotoDetails) { details.ItemID = unknownID }, domain.ErrNotFound},
		{"unknown file", func(details *scan.PhotoDetails) { details.PrimaryFileID = unknownID }, domain.ErrNotFound},
	}

	for _, test := range cases {
		details := valid
		test.change(&details)

		if err := fixture.catalog.SavePhotoDetails(t.Context(), details); !errors.Is(err, test.want) {
			t.Errorf("%s: SavePhotoDetails error = %v, want %v", test.name, err, test.want)
		}
	}

	if rows := queryStrings(t, fixture.inspection, "SELECT item_id FROM photo_info"); len(rows) != 0 {
		t.Errorf("photo rows after rejected saves = %v, want none", rows)
	}
}

// Deleting the primary file keeps the details; the photo is then shown from its other file.
func TestDeletingThePrimaryFileKeepsTheDetails(t *testing.T) {
	t.Parallel()

	fixture := newPhotoFixture(t)
	chain, fileID := fixture.addPhotoFiles(t, "a.jpg", "a.nef")
	fixture.saveDetails(t, readyDetails(chain[0], fileID, 1))

	if err := fixture.catalog.RemovePlacedFiles(t.Context(), photosLibraryID, []string{fileID}); err != nil {
		t.Fatalf("RemovePlacedFiles error = %v", err)
	}

	primary := queryStrings(t, fixture.inspection, "SELECT ifnull(primary_file_id, 'none') FROM photo_info")
	assertDeepEqual(t, "primary file after deleting it", primary, []string{"none"})

	file, err := fixture.reader.PhotoFile(t.Context(), chain[0])
	if err != nil || file.RelativePath != "a.nef" {
		t.Errorf("PhotoFile = %+v, %v, want a.nef", file, err)
	}
}
