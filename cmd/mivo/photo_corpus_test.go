package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image/jpeg"
	"maps"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/magpeek/mivo/internal/adapters/http/apigen"
	"github.com/magpeek/mivo/internal/platform/paths"
)

const (
	// photosLibraryName is the name the test gives the photo library.
	photosLibraryName = "Photos"
	// photoTileWidth is the width the test requests photo tiles in.
	photoTileWidth = 160
	// photoWallClockLayout is how the manifest writes an EXIF capture time (no zone).
	photoWallClockLayout = "2006-01-02T15:04:05"
	// photoStateUnsupported is the manifest's thumbnail state of a photo without tile.
	photoStateUnsupported = "unsupported"
)

// photoCorpusManifest holds the photo part of testdata/library/manifest.json (M3-15g).
type photoCorpusManifest struct {
	Photos struct {
		Path   string            `json:"path"`
		Photos []corpusPhotoFile `json:"photos"`
	} `json:"photos"`
}

type corpusPhotoFile struct {
	File           string           `json:"file"`
	Format         string           `json:"format"`
	AlternateOf    string           `json:"alternateOf"`
	Optional       bool             `json:"optional"`
	Exif           *corpusPhotoExif `json:"exif"`
	TakenAtSource  string           `json:"takenAtSource"`
	ThumbnailState string           `json:"thumbnailState"`
	Renders        bool             `json:"renders"`
	DisplayWidth   int              `json:"displayWidth"`
	DisplayHeight  int              `json:"displayHeight"`
}

type corpusPhotoExif struct {
	TakenAt     string  `json:"takenAt"`
	Make        string  `json:"make"`
	Model       string  `json:"model"`
	FNumber     float64 `json:"fNumber"`
	ISO         int     `json:"iso"`
	Orientation int     `json:"orientation"`
}

// listedPhoto is a photo found by walking the album tree, keyed by its relative path.
type listedPhoto struct {
	summary apigen.ItemSummary
	detail  apigen.ItemDetail
}

// TestRunServesThePhotoCorpus is the photo walking skeleton (M3-15h): a configured photo
// library is scanned at startup without failing on the corrupt and unsupported files; the
// album tree follows the folders, every photo carries the manifest's EXIF values and
// thumbnail state, and the image endpoint serves upright tiles of the photos that render
// while the others answer 422.
func TestRunServesThePhotoCorpus(t *testing.T) {
	t.Parallel()

	corpus := copyLibraryCorpus(t)
	manifest := readPhotoManifest(t)
	root := filepath.Join(corpus.root, manifest.Photos.Path)
	environment := isolatedEnvironment(t)
	writeConfigFile(t, environment[paths.ConfigDirEnv()], fmt.Sprintf(
		"scan:\n  watch: false\n  interval: 0\nlibraries:\n  - name: %s\n    kind: photos\n    path: '%s'\n",
		photosLibraryName, root))

	server := startTestServer(t, environment, runHooks{})
	assertLibraryKindReported(t, server.baseURL, apigen.LibraryKindPhotos)

	library := awaitScannedLibraries(t, server.baseURL, 1, time.Time{})[photosLibraryName]
	if library.Scan.State != apigen.ScanStateIdle || library.Scan.ErrorCode != nil {
		t.Errorf("photo scan = %+v, want a finished scan without error", library.Scan)
	}

	photos := walkPhotoAlbums(t, server.baseURL, library.ID.String())
	want := expectedPhotos(t, root, manifest.Photos.Photos)
	assertPhotoPaths(t, photos, want)

	for _, row := range want {
		if photo, found := photos[row.File]; found {
			assertCorpusPhoto(t, server.baseURL, row, photo)
		}
	}

	server.stop(t)
}

// readPhotoManifest reads the photo part of the manifest the corpus copy came from.
func readPhotoManifest(t *testing.T) photoCorpusManifest {
	t.Helper()

	content, err := os.ReadFile(filepath.Join(libraryCorpusDirectory, libraryManifestFile))
	if err != nil {
		t.Fatalf("read the library manifest: %v", err)
	}

	var manifest photoCorpusManifest
	if err := json.Unmarshal(content, &manifest); err != nil || len(manifest.Photos.Photos) == 0 {
		t.Fatalf("library manifest has no photos (error %v); regenerate it with `go run ./scripts/gen-testmedia`", err)
	}

	return manifest
}

// expectedPhotos returns the photo items the scan must create: every generated file except
// the RAW alternates, which belong to their JPEG's item, and optional files this FFmpeg build
// could not encode.
func expectedPhotos(t *testing.T, root string, rows []corpusPhotoFile) []corpusPhotoFile {
	t.Helper()

	var want []corpusPhotoFile

	for _, row := range rows {
		_, err := os.Stat(filepath.Join(root, filepath.FromSlash(row.File)))
		if err != nil && !row.Optional {
			t.Fatalf("corpus photo %s is missing: %v", row.File, err)
		}

		if err == nil && row.AlternateOf == "" {
			want = append(want, row)
		}
	}

	return want
}

// walkPhotoAlbums lists the top-level albums and walks their children, returning every
// photo by its path below the library folder (album titles are the folder names).
func walkPhotoAlbums(t *testing.T, baseURL, libraryID string) map[string]listedPhoto {
	t.Helper()

	walk := albumWalk{baseURL: baseURL, photos: map[string]listedPhoto{}}

	for _, album := range listItemsOfKind(t, baseURL, libraryID, apigen.ItemKindPhotoAlbum) {
		if album.Kind != apigen.ItemKindPhotoAlbum {
			t.Fatalf("top-level item %q is a %s, want photo_album", album.Title, album.Kind)
		}

		walk.album(t, album, album.Title)
	}

	return walk.photos
}

// albumWalk collects the photos of an album tree by path.
type albumWalk struct {
	baseURL string
	photos  map[string]listedPhoto
}

// album adds the photos of album and its sub-albums. The children must come sub-albums
// first, then photos by capture time.
func (walk albumWalk) album(t *testing.T, album apigen.ItemSummary, albumPath string) {
	t.Helper()

	children := listChildren(t, walk.baseURL, album.ID.String())
	if album.ChildCount == nil || int(*album.ChildCount) != len(children) {
		t.Errorf("album %s childCount = %v, want %d", albumPath, album.ChildCount, len(children))
	}

	var lastTaken time.Time

	for index, child := range children {
		if child.Kind == apigen.ItemKindPhotoAlbum {
			if index > 0 && children[index-1].Kind != apigen.ItemKindPhotoAlbum {
				t.Errorf("album %s lists sub-album %q after a photo", albumPath, child.Title)
			}

			walk.album(t, child, path.Join(albumPath, child.Title))

			continue
		}

		if child.TakenAt == nil || child.TakenAt.Before(lastTaken) {
			t.Errorf("album %s photo %q takenAt = %v, want ascending after %v", albumPath, child.Title, child.TakenAt, lastTaken)
		} else {
			lastTaken = *child.TakenAt
		}

		walk.photo(t, child, albumPath)
	}
}

// photo reads the detail of a photo and records it under its album path and file name.
func (walk albumWalk) photo(t *testing.T, photo apigen.ItemSummary, albumPath string) {
	t.Helper()

	var detail apigen.ItemDetail

	getJSON(t, walk.baseURL+"/api/v1/items/"+photo.ID.String(), &detail)

	if len(detail.Files) == 0 {
		t.Fatalf("photo %q in %s has no files", photo.Title, albumPath)
	}

	walk.photos[path.Join(albumPath, detail.Files[0].FileName)] = listedPhoto{summary: photo, detail: detail}
}

func assertPhotoPaths(t *testing.T, photos map[string]listedPhoto, want []corpusPhotoFile) {
	t.Helper()

	wantPaths := make([]string, 0, len(want))
	for _, row := range want {
		wantPaths = append(wantPaths, row.File)
	}

	slices.Sort(wantPaths)

	if got := slices.Sorted(maps.Keys(photos)); !slices.Equal(got, wantPaths) {
		t.Errorf("photos in the album tree = %v, want %v", got, wantPaths)
	}
}

// assertCorpusPhoto compares one photo with its manifest row: the card, the detail with its
// EXIF block and files, and the tile from the image endpoint.
func assertCorpusPhoto(t *testing.T, baseURL string, row corpusPhotoFile, photo listedPhoto) {
	t.Helper()

	summary, detail := photo.summary, photo.detail
	if summary.ThumbnailState == nil || string(*summary.ThumbnailState) != row.ThumbnailState ||
		detail.ThumbnailState == nil || string(*detail.ThumbnailState) != row.ThumbnailState {
		t.Errorf("%s thumbnailState = %v / %v, want %s", row.File, summary.ThumbnailState, detail.ThumbnailState, row.ThumbnailState)
	}

	assertPhotoBlock(t, row, detail.Photo)
	assertPhotoFiles(t, row, detail)

	isVideo := row.Format == "mp4"
	if (summary.Media != nil) != isVideo || (detail.PlayItemID != nil) != isVideo {
		t.Errorf("%s media = %+v, playItemId = %v; want both only for a video", row.File, summary.Media, detail.PlayItemID)
	}

	assertPhotoTile(t, baseURL, row, summary.Square)
}

// assertPhotoBlock compares the EXIF block; a file without EXIF still has a capture time.
func assertPhotoBlock(t *testing.T, row corpusPhotoFile, block *apigen.ItemPhoto) {
	t.Helper()

	if block == nil || string(block.TakenAtSource) != row.TakenAtSource || block.TakenAt.IsZero() {
		t.Errorf("%s photo = %+v, want takenAtSource %s and a capture time", row.File, block, row.TakenAtSource)

		return
	}

	exif := row.Exif
	if exif == nil || exif.TakenAt == "" {
		return
	}

	// The size comes from the EXIF block; a file without one has none in the API yet.
	if block.Width == nil || block.Height == nil || int(*block.Width) != row.DisplayWidth || int(*block.Height) != row.DisplayHeight {
		t.Errorf("%s size = %v x %v, want %d x %d as displayed", row.File, valueOf(block.Width), valueOf(block.Height),
			row.DisplayWidth, row.DisplayHeight)
	}

	got := fmt.Sprintf("%s %v %v %v %v %v", block.TakenAt.Format(photoWallClockLayout),
		valueOf(block.Make), valueOf(block.Model), valueOf(block.FNumber), valueOf(block.Iso), valueOf(block.Orientation))
	want := fmt.Sprintf("%s %v %v %v %v %v", exif.TakenAt,
		zeroAsAbsent(exif.Make), zeroAsAbsent(exif.Model), zeroAsAbsent(exif.FNumber), zeroAsAbsent(exif.ISO),
		zeroAsAbsent(exif.Orientation))

	if got != want || block.TakenAt.Location() != time.UTC {
		t.Errorf("%s EXIF = %s (%s), want %s in UTC", row.File, got, block.TakenAt.Location(), want)
	}
}

// valueOf dereferences an optional value for comparison; absent is nil.
func valueOf[T any](value *T) any {
	if value == nil {
		return nil
	}

	return *value
}

// zeroAsAbsent returns nil for the zero value, which the manifest writes for a value the
// file does not carry and the API omits.
func zeroAsAbsent[T comparable](value T) any {
	var zero T
	if value == zero {
		return nil
	}

	return value
}

// assertPhotoFiles expects the photo's file first and its RAW alternate after it.
func assertPhotoFiles(t *testing.T, row corpusPhotoFile, detail apigen.ItemDetail) {
	t.Helper()

	want := []string{path.Base(row.File)}
	if strings.HasSuffix(row.File, "/DSC_0103.jpg") {
		want = append(want, "DSC_0103.dng")
	}

	got := make([]string, 0, len(detail.Files))
	for _, file := range detail.Files {
		got = append(got, file.FileName)
	}

	if !slices.Equal(got, want) {
		t.Errorf("%s files = %v, want %v", row.File, got, want)
	}
}

// assertPhotoTile fetches the tile: a photo that renders comes back as JPEG at most
// photoTileWidth wide, upright (portrait exactly when its displayed size is); one that does
// not render either has no tile or answers 422. Optional HEIC and AVIF photos depend on the
// FFmpeg build and may answer 422.
func assertPhotoTile(t *testing.T, baseURL string, row corpusPhotoFile, square *apigen.ImageRef) {
	t.Helper()

	if row.ThumbnailState == photoStateUnsupported || row.Format == "mp4" {
		if square != nil {
			t.Errorf("%s has square %+v, want none", row.File, square)
		}

		return
	}

	if square == nil {
		t.Errorf("%s has no square", row.File)

		return
	}

	query := url.Values{"width": {fmt.Sprint(photoTileWidth)}, "tag": {square.Tag}}
	image := fetchImage(t, baseURL+"/api/v1/images/"+square.ID.String()+"?"+query.Encode())

	switch {
	case image.status == http.StatusUnprocessableEntity && (!row.Renders || row.Optional):
	case !row.Renders:
		t.Errorf("%s tile = %d, want 422 for a photo that does not render", row.File, image.status)
	default:
		assertUprightTile(t, row, image)
	}
}

func assertUprightTile(t *testing.T, row corpusPhotoFile, image fetchedImage) {
	t.Helper()

	if image.status != http.StatusOK || image.header.Get("Content-Type") != "image/jpeg" {
		t.Errorf("%s tile = %d %q, want 200 image/jpeg: %.200s", row.File, image.status, image.header.Get("Content-Type"), image.body)

		return
	}

	decoded, err := jpeg.DecodeConfig(bytes.NewReader(image.body))
	portrait := row.DisplayHeight > row.DisplayWidth

	if err != nil || decoded.Width == 0 || decoded.Width > photoTileWidth || (decoded.Height > decoded.Width) != portrait {
		t.Errorf("%s tile = %dx%d (%v), want at most %d wide, portrait %v", row.File, decoded.Width, decoded.Height, err, photoTileWidth, portrait)
	}
}
