package sqlite_test

import (
	"testing"
	"time"

	"github.com/magpeek/mivo/internal/app/scan"
	"github.com/magpeek/mivo/internal/domain/library"
	"github.com/magpeek/mivo/internal/domain/media"
	"github.com/magpeek/mivo/internal/domain/naming"
	"github.com/magpeek/mivo/internal/domain/photo"
)

// The photos library of the photo fixture and its folder.
const (
	photosLibraryID = "dddddddd-dddd-4ddd-8ddd-dddddddddddd"
	photosRoot      = "/media/photos"
)

// photoFileSize is the size of every test photo file.
const photoFileSize = 2_000_000

// photoFixture is a placement fixture with a photos library.
type photoFixture struct {
	*placementFixture
	files int
}

func newPhotoFixture(t *testing.T) *photoFixture {
	t.Helper()

	fixture := newPlacementFixture(t)
	execute(t, fixture.inspection, `INSERT INTO libraries (id, name, kind, created_at, updated_at)
		VALUES (?1, 'Photos', 'photos', 0, 0)`, photosLibraryID)
	execute(t, fixture.inspection, `INSERT INTO library_roots (id, library_id, path, path_key, position, created_at)
		VALUES (?1, ?1, ?2, ?2, 0, 0)`, photosLibraryID, photosRoot)

	return &photoFixture{placementFixture: fixture}
}

// placedPhotoFile returns an image file of the photos library at relativePath with the next
// file ID, placed by placement.
func (fixture *photoFixture) placedPhotoFile(relativePath string, placement library.Placement) scan.PlacedFile {
	fixture.files++

	return scan.PlacedFile{
		RootID: photosLibraryID, Placement: placement, ProbedAt: time.UnixMilli(scannedMilli).UTC(),
		File: media.File{
			ID: testID('f', fixture.files), LibraryID: photosLibraryID, RelativePath: relativePath,
			Size: photoFileSize, ModifiedAt: time.UnixMilli(fileModifiedMilli).UTC(),
			Description: media.Description{Container: media.ContainerUnknown, FormatName: "image2"},
		},
	}
}

// addPhotoFiles places primary with the photo rules and saves it and the alternates (relative
// paths) as files of the same photo item, as the photo scan does. It returns the item chain
// and the ID of the primary file.
func (fixture *photoFixture) addPhotoFiles(t *testing.T, primary string, alternates ...string) (chain []string, primaryFileID string) {
	t.Helper()

	result, err := naming.Place(library.KindPhotos, naming.FileContext{Path: primary})
	if err != nil {
		t.Fatalf("Place(%s) error = %v", primary, err)
	}

	record := fixture.placedPhotoFile(primary, result.Placement)
	chain = fixture.save(t, record)

	for _, alternate := range alternates {
		fixture.save(t, fixture.placedPhotoFile(alternate, result.Placement))
	}

	return chain, record.File.ID
}

// saveDetails stores details and fails the test on an error.
func (fixture *photoFixture) saveDetails(t *testing.T, details scan.PhotoDetails) {
	t.Helper()

	if err := fixture.catalog.SavePhotoDetails(t.Context(), details); err != nil {
		t.Fatalf("SavePhotoDetails(%s) error = %v", details.ItemID, err)
	}
}

// addPhoto saves a photo at relativePath taken at the given minute of a fixed day and
// returns its item chain.
func (fixture *photoFixture) addPhoto(t *testing.T, relativePath string, minute int) []string {
	t.Helper()

	chain, fileID := fixture.addPhotoFiles(t, relativePath)
	fixture.saveDetails(t, readyDetails(chain[len(chain)-1], fileID, minute))

	return chain
}

// readyDetails returns the details of a ready photo taken at the given minute (EXIF time).
func readyDetails(itemID, fileID string, minute int) scan.PhotoDetails {
	return scan.PhotoDetails{
		ItemID: itemID, PrimaryFileID: fileID, Info: photo.Info{TakenAt: photoTime(minute)},
		TakenAtSource: photo.TakenAtExif, ThumbnailState: photo.ThumbnailReady,
	}
}

// photoTime is the given minute of 2021-05-04 (wall clock read as UTC).
func photoTime(minute int) time.Time {
	return time.Date(2021, 5, 4, 12, minute, 0, 0, time.UTC)
}
