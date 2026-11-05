package scan_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/magpeek/mivo/internal/app/scan"
	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/library"
	"github.com/magpeek/mivo/internal/domain/photo"
)

// photoScanBudget bounds the scan of a large folder with instant fakes; it guards against
// quadratic work per file, not against a slow machine.
const photoScanBudget = 5 * time.Second

func TestPhotoScanBuildsNestedAlbumsFromTheFolderTree(t *testing.T) {
	t.Parallel()

	cameraTime := time.Date(2024, time.July, 1, 9, 30, 0, 0, time.UTC)

	h, reader, details := newPhotoHarness(t)
	h.start(t)

	reader.script("Trip/Day 1/a.jpg", infoOutcome{info: photo.Info{
		TakenAt: cameraTime, Make: "Canon", Model: "R6", FNumber: 2.8, ISO: 200, Orientation: 6,
		Width: 6000, Height: 4000, GPS: &photo.Coordinates{Latitude: 48.1, Longitude: 11.6},
	}})

	status := h.scanPhotos(t, fileAt("Trip/Day 1/a.jpg", 1), fileAt("Trip/b.png", 2), fileAt("c.jpg", 3))

	assertState(t, status, scan.StateIdle, scan.ErrorCodeNone)
	assertCounts(t, status, scan.Counts{Added: 3})
	assertKeys(t, "albums", h.catalogV2.matchKeys(library.ItemKindPhotoAlbum), "day 1", "trip")
	assertKeys(t, "photos", h.catalogV2.matchKeys(library.ItemKindPhoto), "a jpg", "b png", "c jpg")

	if chain := h.catalogV2.chainOfFile("Trip/Day 1/a.jpg"); len(chain) != 3 {
		t.Errorf("chain of a.jpg = %v, want album, album, photo", chain)
	}

	got := detailsOf(t, h, details, "Trip/Day 1/a.jpg")
	fileID, _ := h.catalogV2.fileOf("Trip/Day 1/a.jpg")

	if got.PrimaryFileID != fileID || !got.Info.TakenAt.Equal(cameraTime) || got.Info.Model != "R6" ||
		got.Info.FNumber != 2.8 || got.Info.GPS == nil || got.ThumbnailState != photo.ThumbnailReady {
		t.Errorf("details of a.jpg = %+v, want the EXIF values, primary %s and ready", got, fileID)
	}

	if png := detailsOf(t, h, details, "Trip/b.png"); !png.Info.TakenAt.Equal(fileAt("Trip/b.png", 2).ModifiedAt) {
		t.Errorf("taken at of b.png = %v, want its modification time", png.Info.TakenAt)
	}

	if len(h.refresher.last.Files) != 3 {
		t.Errorf("refresh files = %d, want every photo", len(h.refresher.last.Files))
	}
}

func TestPhotoScanFallsBackToTheServerWallClock(t *testing.T) {
	t.Parallel()

	h, _, details := newPhotoHarness(t)
	h.photos.Zone = time.FixedZone("UTC+2", 2*60*60)
	h.start(t)

	state := fileAt("a.jpg", 1)
	h.scanPhotos(t, state)

	want := state.ModifiedAt.Add(2 * time.Hour)
	if got := detailsOf(t, h, details, "a.jpg").Info.TakenAt; !got.Equal(want) || got.Location() != time.UTC {
		t.Errorf("taken at = %v, want the wall clock %v as UTC", got, want)
	}
}

func TestPhotoScanPairsRAWWithItsJPEG(t *testing.T) {
	t.Parallel()

	h, reader, details := newPhotoHarness(t)
	h.start(t)

	status := h.scanPhotos(t, fileAt("Trip/IMG_1.CR2", 1), fileAt("Trip/IMG_1.jpg", 2))

	assertCounts(t, status, scan.Counts{Added: 2})
	assertKeys(t, "photos", h.catalogV2.matchKeys(library.ItemKindPhoto), "img 1 jpg")
	assertKeys(t, "metadata reads", reader.takeReads(), "Trip/IMG_1.jpg")

	if raw, jpeg := h.catalogV2.itemOf("Trip/IMG_1.CR2"), h.catalogV2.itemOf("Trip/IMG_1.jpg"); raw != jpeg {
		t.Errorf("items of RAW and JPEG = %s, %s, want one item", raw, jpeg)
	}

	jpegID, _ := h.catalogV2.fileOf("Trip/IMG_1.jpg")
	if got := detailsOf(t, h, details, "Trip/IMG_1.CR2"); got.PrimaryFileID != jpegID {
		t.Errorf("primary file = %s, want the JPEG %s", got.PrimaryFileID, jpegID)
	}
}

func TestPhotoScanReKeysARAWWhenItsJPEGComesAndGoes(t *testing.T) {
	t.Parallel()

	h, reader, details := newPhotoHarness(t)
	h.start(t)

	raw, jpeg := fileAt("IMG_1.CR2", 1), fileAt("IMG_1.jpg", 2)

	h.scanPhotos(t, raw)
	rawItem := h.catalogV2.itemOf("IMG_1.CR2")
	reader.takeReads()

	status := h.scanPhotos(t, raw, jpeg)

	assertCounts(t, status, scan.Counts{Added: 1, Updated: 1})
	assertKeys(t, "photos with the JPEG", h.catalogV2.matchKeys(library.ItemKindPhoto), "img 1 jpg")
	assertKeys(t, "reads with the JPEG", reader.takeReads(), "IMG_1.jpg")

	if got := h.catalogV2.itemOf("IMG_1.CR2"); got == rawItem || h.catalogV2.count(library.ItemKindPhoto) != 1 {
		t.Errorf("RAW item = %s, photos = %d, want the stale RAW item %s gone and one photo",
			got, h.catalogV2.count(library.ItemKindPhoto), rawItem)
	}

	status = h.scanPhotos(t, raw)

	assertCounts(t, status, scan.Counts{Updated: 1, Removed: 1})
	assertKeys(t, "photos without the JPEG", h.catalogV2.matchKeys(library.ItemKindPhoto), "img 1 cr2")
	assertKeys(t, "reads without the JPEG", reader.takeReads(), "IMG_1.CR2")

	rawID, _ := h.catalogV2.fileOf("IMG_1.CR2")
	if got := detailsOf(t, h, details, "IMG_1.CR2"); got.PrimaryFileID != rawID {
		t.Errorf("primary file = %s, want the RAW %s", got.PrimaryFileID, rawID)
	}
}

func TestPhotoScanKeepsCorruptAndUnreadableFiles(t *testing.T) {
	t.Parallel()

	h, reader, details := newPhotoHarness(t)
	h.start(t)

	reader.script("corrupt.jpg", infoOutcome{err: fmt.Errorf("exif: %w", domain.ErrInvalid)})
	reader.script("odd.nef", infoOutcome{err: fmt.Errorf("layout: %w", domain.ErrUnsupported)})
	reader.script("plain.png", infoOutcome{err: fmt.Errorf("format: %w", domain.ErrUnsupported)})
	// Like photometa, the error names the absolute path, which must not reach the log.
	reader.script("locked.jpg", infoOutcome{err: fmt.Errorf("open %s: permission denied", absolutePath(photosRoot, "locked.jpg"))})

	status := h.scanPhotos(t, fileAt("corrupt.jpg", 1), fileAt("odd.nef", 2), fileAt("plain.png", 3), fileAt("locked.jpg", 4))

	assertState(t, status, scan.StateIdle, scan.ErrorCodeNone)
	assertCounts(t, status, scan.Counts{Added: 4, MetadataWarnings: 3})

	if logs := h.logs.String(); strings.Contains(logs, fakeRootPrefix) {
		t.Errorf("logs contain an absolute path:\n%s", logs)
	}

	cases := []struct {
		path  string
		state photo.ThumbnailState
	}{
		{"corrupt.jpg", photo.ThumbnailReady},
		{"odd.nef", photo.ThumbnailUnsupported},
		{"plain.png", photo.ThumbnailReady},
		{"locked.jpg", photo.ThumbnailReady},
	}

	for _, testCase := range cases {
		got := detailsOf(t, h, details, testCase.path)
		modified := fileAt(testCase.path, 0).ModifiedAt

		if got.ThumbnailState != testCase.state || got.Info.TakenAt.Before(modified) {
			t.Errorf("%s: details = %+v, want state %q and the modification time", testCase.path, got, testCase.state)
		}
	}
}

func TestPhotoScanStoresAnUnresolvableImageAsFailed(t *testing.T) {
	t.Parallel()

	h, reader, details := newPhotoHarness(t)
	h.resolver.errs["gone.jpg"] = fmt.Errorf("stat: %w", domain.ErrNotFound)
	h.start(t)

	status := h.scanPhotos(t, fileAt("gone.jpg", 1))

	assertCounts(t, status, scan.Counts{Failed: 1})
	assertKeys(t, "reads", reader.takeReads())

	if got := detailsOf(t, h, details, "gone.jpg"); got.ThumbnailState != photo.ThumbnailUnsupported {
		t.Errorf("thumbnail state = %q, want unsupported", got.ThumbnailState)
	}
}

func TestPhotoScanProbesVideosAsPhotoItems(t *testing.T) {
	t.Parallel()

	h, reader, details := newPhotoHarness(t)
	h.prober.script(absolutePath(photosRoot, "Trip/broken.mov"), probeOutcome{err: errors.New("moov atom not found")})
	h.prober.script(absolutePath(photosRoot, "Trip/voice.m4v"), probeOutcome{description: audioDescription()})
	h.start(t)

	status := h.scanPhotos(t, fileAt("Trip/clip.mp4", 1), fileAt("Trip/broken.mov", 2), fileAt("Trip/voice.m4v", 3))

	assertCounts(t, status, scan.Counts{Added: 1, Failed: 1, Skipped: 1})
	assertKeys(t, "photos", h.catalogV2.matchKeys(library.ItemKindPhoto), "broken mov", "clip mp4")
	assertKeys(t, "metadata reads", reader.takeReads())

	clip := detailsOf(t, h, details, "Trip/clip.mp4")
	if clip.Info.Width != 1920 || clip.Info.Height != 1080 || clip.ThumbnailState != photo.ThumbnailReady ||
		!clip.Info.TakenAt.Equal(fileAt("Trip/clip.mp4", 1).ModifiedAt) {
		t.Errorf("details of the clip = %+v, want the stream size, the modification time and ready", clip)
	}

	if broken := detailsOf(t, h, details, "Trip/broken.mov"); broken.ThumbnailState != photo.ThumbnailUnsupported {
		t.Errorf("thumbnail state of the broken video = %q, want unsupported", broken.ThumbnailState)
	}
}

func TestPhotoRescanSkipsUnchangedFiles(t *testing.T) {
	t.Parallel()

	h, reader, details := newPhotoHarness(t)
	h.start(t)

	files := []library.FileState{fileAt("a.jpg", 1), fileAt("b.jpg", 2), fileAt("clip.mp4", 3)}
	h.scanPhotos(t, files...)
	reader.takeReads()

	writes := details.writeCount()
	probes := len(h.prober.probedPaths())

	files[1] = fileAt("b.jpg", 20)
	status := h.scanPhotos(t, files...)

	assertCounts(t, status, scan.Counts{Updated: 1, Unchanged: 2})
	assertKeys(t, "reads", reader.takeReads(), "b.jpg")

	if got := len(h.prober.probedPaths()); got != probes {
		t.Errorf("probes = %d, want %d: the unchanged video is not probed again", got, probes)
	}

	if got := details.writeCount(); got != writes+1 {
		t.Errorf("detail writes = %d, want %d", got, writes+1)
	}
}

func TestPhotoScanRetriesDetailsThatFailedAfterTheFileWasSaved(t *testing.T) {
	t.Parallel()

	h, reader, details := newPhotoHarness(t)
	h.start(t)

	details.setErr(fmt.Errorf("disk full: %w", domain.ErrUnavailable))
	status := h.scanPhotos(t, fileAt("a.jpg", 1))

	assertState(t, status, scan.StateFailed, scan.ErrorCodeCatalogFailed)
	reader.takeReads()

	if got := h.refresher.last.ItemIDs; len(got) != 1 {
		t.Errorf("refreshed items after the early end = %v, want the saved photo", got)
	}

	details.setErr(nil)
	status = h.scanPhotos(t, fileAt("a.jpg", 1))

	assertState(t, status, scan.StateIdle, scan.ErrorCodeNone)
	assertCounts(t, status, scan.Counts{Updated: 1})
	assertKeys(t, "reads", reader.takeReads(), "a.jpg")
	detailsOf(t, h, details, "a.jpg")

	status = h.scanPhotos(t, fileAt("a.jpg", 1))
	assertCounts(t, status, scan.Counts{Unchanged: 1})
}

func TestPhotoScanOfALargeFolderIsBoundedAndFast(t *testing.T) {
	t.Parallel()

	const fileCount = 2000

	h, reader, details := newPhotoHarness(t)
	h.start(t)

	files := make([]library.FileState, 0, fileCount)
	for index := range fileCount {
		files = append(files, fileAt(fmt.Sprintf("Big/IMG_%04d.jpg", index), int64(index+1)))
	}

	started := time.Now()
	status := h.scanPhotos(t, files...)

	if elapsed := time.Since(started); elapsed > photoScanBudget {
		t.Errorf("scan of %d photos took %v, want under %v", fileCount, elapsed, photoScanBudget)
	}

	assertCounts(t, status, scan.Counts{Added: fileCount})

	if got := details.writeCount(); got != fileCount {
		t.Errorf("detail writes = %d, want %d", got, fileCount)
	}

	if got := reader.maxConcurrent(); got > scan.MaxConcurrentPhotoReads {
		t.Errorf("concurrent reads = %d, want at most %d", got, scan.MaxConcurrentPhotoReads)
	}
}

func TestPhotoScanCancelStopsTheReads(t *testing.T) {
	t.Parallel()

	h, reader, details := newPhotoHarness(t)
	reader.gate, reader.entered = make(chan struct{}), make(chan struct{})
	h.start(t)

	h.walker.set(photosRoot, walkResult{files: []library.FileState{fileAt("a.jpg", 1), fileAt("b.jpg", 2)}})

	if _, err := h.service.Start(context.Background(), photosID); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	receive(t, reader.entered, "the first read")

	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	if err := h.service.Cancel(ctx, photosID); err != nil {
		t.Fatalf("Cancel() error = %v", err)
	}

	assertState(t, h.service.Status(photosID), scan.StateFailed, scan.ErrorCodeCanceled)

	if got := details.writeCount(); got != 0 {
		t.Errorf("detail writes = %d, want none after the cancel", got)
	}
}

func TestPhotoScanWithoutPortsFails(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.libraries.set(oneFolder(photosID, "Photos", photosRoot, library.KindPhotos))
	h.start(t)

	status := h.scanPhotos(t, fileAt("a.jpg", 1))

	assertState(t, status, scan.StateFailed, scan.ErrorCodeFailed)
}

func TestPhotoScanRemovesDeletedPhotosAndEmptyAlbums(t *testing.T) {
	t.Parallel()

	h, _, _ := newPhotoHarness(t)
	h.start(t)

	h.scanPhotos(t, fileAt("Trip/a.jpg", 1), fileAt("Home/b.jpg", 2))
	status := h.scanPhotos(t, fileAt("Home/b.jpg", 2))

	assertCounts(t, status, scan.Counts{Removed: 1, Unchanged: 1})
	assertKeys(t, "albums", h.catalogV2.matchKeys(library.ItemKindPhotoAlbum), "home")
}
