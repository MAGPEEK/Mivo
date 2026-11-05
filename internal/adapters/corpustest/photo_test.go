package corpustest_test

import (
	"errors"
	"log/slog"
	"math"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/magpeek/mivo/internal/adapters/ffmpeg"
	"github.com/magpeek/mivo/internal/adapters/photometa"
	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/artwork"
	"github.com/magpeek/mivo/internal/domain/photo"
)

const (
	// Expected photometa outcomes of the photo manifest.
	metadataRead        = "read"
	metadataUnsupported = "unsupported"
	metadataCorrupt     = "corrupt"
	// wallClockLayout is how the manifest writes an EXIF capture time (no zone).
	wallClockLayout = "2006-01-02T15:04:05"
	// exifValueTolerance absorbs the float rounding of rationals and GPS degrees.
	exifValueTolerance = 1e-9
	// thumbnailWidth is the render width of the thumbnail check.
	thumbnailWidth  = artwork.Width160
	photoFormatHEIC = "heic"
	photoFormatAVIF = "avif"
)

// photoManifest is the photos object of the library manifest (task M3-15g).
type photoManifest struct {
	Path   string     `json:"path"`
	Photos []photoRow `json:"photos"`
}

type photoRow struct {
	File           string   `json:"file"`
	Album          string   `json:"album"`
	Format         string   `json:"format"`
	AlternateOf    string   `json:"alternateOf"`
	Optional       bool     `json:"optional"`
	Metadata       string   `json:"metadata"`
	Exif           *exifRow `json:"exif"`
	TakenAtSource  string   `json:"takenAtSource"`
	ModifiedAt     string   `json:"modifiedAt"`
	ThumbnailState string   `json:"thumbnailState"`
	Renders        bool     `json:"renders"`
	DisplayWidth   int      `json:"displayWidth"`
	DisplayHeight  int      `json:"displayHeight"`
}

type exifRow struct {
	TakenAt         string  `json:"takenAt"`
	Make            string  `json:"make"`
	Model           string  `json:"model"`
	Lens            string  `json:"lens"`
	ExposureSeconds float64 `json:"exposureSeconds"`
	FNumber         float64 `json:"fNumber"`
	FocalLengthMM   float64 `json:"focalLengthMM"`
	ISO             int     `json:"iso"`
	Orientation     int     `json:"orientation"`
	GPS             *gpsRow `json:"gps"`
	Width           int     `json:"width"`
	Height          int     `json:"height"`
}

type gpsRow struct {
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
}

// photoCorpus is the generated photo tree with the build result of each file.
type photoCorpus struct {
	root    string
	photos  []photoRow
	results map[string]libraryCaseResult
	path    string
}

// requirePhotoCorpus reads the photo part of the library manifest, or skips under -short,
// without FFmpeg or without the generated corpus. Every file that is not optional must have
// been generated.
func requirePhotoCorpus(t *testing.T) photoCorpus {
	t.Helper()

	_, libraryRoot := requireMusicCorpus(t)
	manifest := readLibraryManifest(t, libraryRoot)

	if len(manifest.Photos.Photos) == 0 {
		t.Fatal("the library manifest lists no photo; regenerate it with `go run ./scripts/gen-testmedia`")
	}

	corpus := photoCorpus{
		root:    filepath.Join(libraryRoot, filepath.FromSlash(manifest.Photos.Path)),
		photos:  manifest.Photos.Photos,
		results: caseResultsByFile(manifest.Cases),
		path:    manifest.Photos.Path,
	}

	for _, row := range corpus.photos {
		if result := corpus.result(row); result.Status != statusGenerated && !row.Optional {
			t.Fatalf("photo case %s is %s: %s", result.Name, result.Status, result.Reason)
		}
	}

	return corpus
}

func (corpus photoCorpus) result(row photoRow) libraryCaseResult {
	return corpus.results[path.Join(corpus.path, row.File)]
}

// generated returns the files that exist; optional files a build could not encode are left out.
func (corpus photoCorpus) generated() []photoRow {
	return slices.DeleteFunc(slices.Clone(corpus.photos), func(row photoRow) bool {
		return corpus.result(row).Status != statusGenerated
	})
}

func (corpus photoCorpus) file(row photoRow) string {
	return filepath.Join(corpus.root, filepath.FromSlash(row.File))
}

// TestCorpusPhotoMetadataIsReadable reads every photo with photometa like a scan does: the
// outcome, the EXIF values, the capture time source with its fallback, the thumbnail state
// and, for files without EXIF time, the modification time the generator set.
func TestCorpusPhotoMetadataIsReadable(t *testing.T) {
	t.Parallel()

	corpus := requirePhotoCorpus(t)
	reader := photometa.NewReader()

	for _, row := range corpus.generated() {
		t.Run(row.File, func(t *testing.T) {
			t.Parallel()

			file := corpus.file(row)
			info, err := reader.ReadInfo(t.Context(), file)

			assertMetadataOutcome(t, row.Metadata, err)
			assertExif(t, row.Exif, info)

			source := photo.TakenAtFile
			if err == nil {
				source = photo.SourceOf(info)
			}

			if source.String() != row.TakenAtSource {
				t.Errorf("capture time source %s, want %s", source, row.TakenAtSource)
			}

			state := photo.ThumbnailStateFor(path.Base(row.File), errors.Is(err, domain.ErrUnsupported))
			if state.String() != row.ThumbnailState {
				t.Errorf("thumbnail state %s, want %s", state, row.ThumbnailState)
			}

			assertModifiedAt(t, file, row.ModifiedAt)
		})
	}
}

func assertMetadataOutcome(t *testing.T, want string, err error) {
	t.Helper()

	got := metadataRead

	switch {
	case errors.Is(err, photometa.ErrUnsupported):
		got = metadataUnsupported
	case errors.Is(err, photometa.ErrCorrupt):
		got = metadataCorrupt
	case err != nil:
		t.Fatalf("ReadInfo: %v", err)
	}

	if got != want {
		t.Errorf("ReadInfo outcome %s (%v), want %s", got, err, want)
	}
}

// assertExif compares the Info with the manifest; a nil row expects a zero Info.
func assertExif(t *testing.T, want *exifRow, info photo.Info) {
	t.Helper()

	if want == nil {
		want = &exifRow{}
	}

	takenAt := ""
	if !info.TakenAt.IsZero() {
		takenAt = info.TakenAt.Format(wallClockLayout)

		if info.TakenAt.Location() != time.UTC {
			t.Errorf("TakenAt %v is not the wall clock as UTC", info.TakenAt)
		}
	}

	got := exifRow{
		TakenAt: takenAt, Make: info.Make, Model: info.Model, Lens: info.Lens, ISO: info.ISO,
		Orientation: info.Orientation, Width: info.Width, Height: info.Height,
	}
	wanted := *want
	wanted.ExposureSeconds, wanted.FNumber, wanted.FocalLengthMM, wanted.GPS = 0, 0, 0, nil

	if got != wanted {
		t.Errorf("Info = %+v, want %+v", got, wanted)
	}

	for _, value := range []struct {
		name      string
		got, want float64
	}{
		{"exposure", info.ExposureSeconds, want.ExposureSeconds},
		{"f-number", info.FNumber, want.FNumber},
		{"focal length", info.FocalLengthMM, want.FocalLengthMM},
	} {
		if math.Abs(value.got-value.want) > exifValueTolerance {
			t.Errorf("%s %v, want %v", value.name, value.got, value.want)
		}
	}

	assertGPS(t, want.GPS, info.GPS)
}

func assertGPS(t *testing.T, want *gpsRow, got *photo.Coordinates) {
	t.Helper()

	switch {
	case want == nil && got == nil:
	case want == nil || got == nil:
		t.Errorf("GPS %+v, want %+v", got, want)
	case math.Abs(got.Latitude-want.Latitude) > exifValueTolerance ||
		math.Abs(got.Longitude-want.Longitude) > exifValueTolerance:
		t.Errorf("GPS %+v, want %+v", *got, *want)
	}
}

// assertModifiedAt checks the modification time the scan falls back to.
func assertModifiedAt(t *testing.T, file, want string) {
	t.Helper()

	if want == "" {
		return
	}

	expected, err := time.Parse(time.RFC3339, want)
	if err != nil {
		t.Fatalf("manifest modifiedAt %q: %v", want, err)
	}

	stat, err := os.Stat(file)
	if err != nil {
		t.Fatal(err)
	}

	if !stat.ModTime().Equal(expected) {
		t.Errorf("modification time %v, want %v", stat.ModTime().UTC(), expected)
	}
}

// TestCorpusPhotosGroupIntoTheManifestPairs lists every album folder like a scan and pairs
// the files with photo.GroupFiles: each RAW joins the primary the manifest names, every
// other file is a primary of its own.
func TestCorpusPhotosGroupIntoTheManifestPairs(t *testing.T) {
	t.Parallel()

	corpus := requirePhotoCorpus(t)
	byAlbum := map[string][]photoRow{}

	for _, row := range corpus.generated() {
		byAlbum[row.Album] = append(byAlbum[row.Album], row)
	}

	for album, rows := range byAlbum {
		entries, err := os.ReadDir(filepath.Join(corpus.root, filepath.FromSlash(album)))
		if err != nil {
			t.Fatal(err)
		}

		var names []string

		for _, entry := range entries {
			if !entry.IsDir() {
				names = append(names, entry.Name())
			}
		}

		got := map[string][]string{}
		for _, group := range photo.GroupFiles(names) {
			got[group.Primary] = group.Alternates
		}

		if want := wantedGroups(rows); !mapsEqual(got, want) {
			t.Errorf("album %s groups %v, want %v", album, got, want)
		}
	}
}

// wantedGroups maps the base name of each primary to the base names of its alternates.
func wantedGroups(rows []photoRow) map[string][]string {
	groups := map[string][]string{}

	for _, row := range rows {
		if _, listed := groups[path.Base(row.File)]; row.AlternateOf == "" && !listed {
			groups[path.Base(row.File)] = nil
		}
	}

	for _, row := range rows {
		if row.AlternateOf != "" {
			primary := path.Base(row.AlternateOf)
			groups[primary] = append(groups[primary], path.Base(row.File))
		}
	}

	return groups
}

func mapsEqual(got, want map[string][]string) bool {
	if len(got) != len(want) {
		return false
	}

	for key, values := range want {
		gotValues, ok := got[key]
		if !ok || !slices.Equal(gotValues, values) {
			return false
		}
	}

	return true
}

// TestCorpusPhotoThumbnailsRender renders every photo with the ffmpeg photo renderer:
// stills render upright (portrait exactly when the manifest's display size is), the corrupt
// file, the unreadable RAW and the video are unsupported. HEIC and AVIF are skipped when the
// ffmpeg build cannot decode them (they would fall back to a preview they do not have).
func TestCorpusPhotoThumbnailsRender(t *testing.T) {
	t.Parallel()

	corpus := requirePhotoCorpus(t)
	renderer, decoders := newPhotoRenderer(t)
	rendering := photoRendering{renderer: renderer, decoders: decoders}

	for _, row := range corpus.generated() {
		t.Run(row.File, func(t *testing.T) {
			t.Parallel()
			rendering.assertThumbnail(t, corpus.file(row), row)
		})
	}
}

// photoRendering is the renderer under test and the decoders of its ffmpeg build.
type photoRendering struct {
	renderer *ffmpeg.PhotoRenderer
	decoders ffmpeg.PhotoDecoders
}

// assertThumbnail renders one photo and checks the outcome against the manifest.
func (rendering photoRendering) assertThumbnail(t *testing.T, file string, row photoRow) {
	t.Helper()

	renderer, decoders := rendering.renderer, rendering.decoders

	if (row.Format == photoFormatHEIC && !decoders.HEIC) || (row.Format == photoFormatAVIF && !decoders.AVIF) {
		t.Skipf("ffmpeg cannot decode %s", row.Format)
	}

	rendered, err := renderer.Render(t.Context(), artwork.RenderRequest{
		AbsolutePath: file, StreamIndex: artwork.WholeFile,
		MaxWidth: thumbnailWidth, Format: artwork.FormatJPEG,
	})

	if !row.Renders {
		if !errors.Is(err, ffmpeg.ErrPhotoUnsupported) {
			t.Errorf("Render error %v, want ErrPhotoUnsupported", err)
		}

		return
	}

	if reason, known := knownRenderFailures()[row.File]; known && err != nil {
		t.Skipf("known render failure: %s (%v)", reason, err)
	}

	if err != nil {
		t.Fatalf("Render: %v", err)
	}

	portrait := row.DisplayHeight > row.DisplayWidth
	if rendered.Width > thumbnailWidth.Pixels() || (rendered.Height > rendered.Width) != portrait {
		t.Errorf("thumbnail %dx%d, want at most %d wide and the aspect of %dx%d",
			rendered.Width, rendered.Height, thumbnailWidth.Pixels(), row.DisplayWidth, row.DisplayHeight)
	}
}

// knownRenderFailures are corpus files the renderer cannot show yet, with the reason. The
// test still renders them, so a fix makes the check strict without touching this list;
// remove an entry once its fix is in.
func knownRenderFailures() map[string]string {
	return map[string]string{
		// FFmpeg assembles a HEIF grid in a complex filtergraph of its own, and a -vf on the
		// same output is rejected ("Simple and complex filtering cannot be used together").
		"Phone/IMG_1002.heic": "HEIF grid images cannot be scaled with -vf (BACKLOG m3-15g findings)",
	}
}

func newPhotoRenderer(t *testing.T) (*ffmpeg.PhotoRenderer, ffmpeg.PhotoDecoders) {
	t.Helper()

	ffmpegPath, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Fatalf("ffmpeg disappeared from PATH: %v", err)
	}

	logger := slog.New(slog.DiscardHandler)

	decoders, err := ffmpeg.DetectPhotoDecoders(t.Context(), ffmpegPath, logger)
	if err != nil {
		t.Fatalf("DetectPhotoDecoders: %v", err)
	}

	renderer, err := ffmpeg.NewPhotoRenderer(ffmpeg.PhotoRendererOptions{
		FFmpegPath: ffmpegPath, Logger: logger, Metadata: photometa.NewReader(), Decoders: decoders,
	})
	if err != nil {
		t.Fatalf("NewPhotoRenderer: %v", err)
	}

	return renderer, decoders
}

// TestCorpusRAWPreviewsAreRenderableJPEGs extracts the embedded preview of every readable
// RAW file, the path the renderer takes for RAW files.
func TestCorpusRAWPreviewsAreRenderableJPEGs(t *testing.T) {
	t.Parallel()

	corpus := requirePhotoCorpus(t)
	reader := photometa.NewReader()

	for _, row := range corpus.generated() {
		if row.Format != "dng" {
			continue
		}

		preview, err := reader.ExtractPreview(t.Context(), corpus.file(row))
		if err != nil {
			t.Errorf("%s: ExtractPreview: %v", row.File, err)

			continue
		}

		if !slices.Equal(preview[:len(jpegSignature())], jpegSignature()) {
			t.Errorf("%s: the preview is no JPEG", row.File)
		}
	}
}
