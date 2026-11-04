package ffmpeg_test

import (
	"bytes"
	"errors"
	"image"
	"image/jpeg"
	"image/png"
	"io/fs"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/magpeek/mivo/internal/adapters/ffmpeg"
	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/artwork"
)

// libraryDirectory is the generated test library (M3-17) seen from this package directory.
const libraryDirectory = "../../../testdata/library"

// Attached-picture stream index of the library clips with cover art: video, audio, cover.
const (
	libraryAudioStream = 1
	libraryCoverStream = 2
)

// renderCase is one corpus render and the pixel size it must produce.
type renderCase struct {
	name       string
	path       string
	stream     int
	width      artwork.Width
	format     artwork.Format
	wantWidth  int
	wantHeight int
	wantAlpha  bool
}

// requireFFmpeg returns the ffmpeg path or skips the test with the reason.
func requireFFmpeg(t *testing.T) string {
	t.Helper()

	if testing.Short() {
		t.Skip("skipping FFmpeg integration test in -short mode")
	}

	ffmpegPath, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("skipping FFmpeg integration test: ffmpeg is not on PATH (install FFmpeg, see docs/development.md)")
	}

	return ffmpegPath
}

func newCorpusRenderer(t *testing.T) *ffmpeg.ImageRenderer {
	t.Helper()

	renderer, err := ffmpeg.NewImageRenderer(ffmpeg.ImageRendererOptions{
		FFmpegPath: requireFFmpeg(t), Logger: slog.New(slog.DiscardHandler),
	})
	if err != nil {
		t.Fatalf("NewImageRenderer: %v", err)
	}

	return renderer
}

// The corpus stills are 320×240 (gen-testmedia stillImageCases).
func TestRenderCorpusImages(t *testing.T) {
	renderer := newCorpusRenderer(t)
	_, manifest := requireCorpus(t)

	stills := map[string]string{"image-jpeg": "jpeg", "image-png": "png", "image-webp-lossy": "webp"}
	for caseName, label := range stills {
		path := corpusPath(t, corpusCaseByName(t, manifest, caseName).File)

		runRenderCases(t, renderer, []renderCase{
			{name: label + " downscaled", path: path, stream: artwork.WholeFile, width: artwork.Width160, format: artwork.FormatJPEG, wantWidth: 160, wantHeight: 120},
			{name: label + " never upscaled", path: path, stream: artwork.WholeFile, width: artwork.Width1920, format: artwork.FormatJPEG, wantWidth: 320, wantHeight: 240},
			{name: label + " as png", path: path, stream: artwork.WholeFile, width: artwork.Width160, format: artwork.FormatPNG, wantWidth: 160, wantHeight: 120},
		})
	}
}

// The library artwork sizes come from gen-testmedia (library.go): poster 600×900, backdrop
// 1280×720, logo 800×310 RGBA; the covers embed the poster.
func TestRenderLibraryArtwork(t *testing.T) {
	renderer := newCorpusRenderer(t)
	movies := requireLibrary(t)
	bunny := filepath.Join(movies, "Big Buck Bunny (2008)")

	runRenderCases(t, renderer, []renderCase{
		{name: "poster", path: filepath.Join(bunny, "poster.jpg"), stream: artwork.WholeFile, width: artwork.Width320, format: artwork.FormatJPEG, wantWidth: 320, wantHeight: 480},
		{name: "backdrop never upscaled", path: filepath.Join(bunny, "fanart.jpg"), stream: artwork.WholeFile, width: artwork.Width1920, format: artwork.FormatJPEG, wantWidth: 1280, wantHeight: 720},
		{name: "logo keeps alpha", path: filepath.Join(bunny, "clearlogo.png"), stream: artwork.WholeFile, width: artwork.Width320, format: artwork.FormatPNG, wantWidth: 320, wantHeight: 124, wantAlpha: true},
		{name: "matroska cover attachment", path: filepath.Join(movies, "Glass Half (2015)", "Glass Half (2015).mkv"), stream: libraryCoverStream, width: artwork.Width480, format: artwork.FormatJPEG, wantWidth: 480, wantHeight: 720},
		{name: "mp4 covr", path: filepath.Join(movies, "Night of the Living Dead (1968).mp4"), stream: libraryCoverStream, width: artwork.Width160, format: artwork.FormatJPEG, wantWidth: 160, wantHeight: 240},
	})
}

// A stream index that names no picture (the clip's audio) must fail as unsupported, not
// produce an empty or garbage image.
func TestRenderRefusesAStreamThatIsNoPicture(t *testing.T) {
	renderer := newCorpusRenderer(t)
	movies := requireLibrary(t)

	_, err := renderer.Render(t.Context(), artwork.RenderRequest{
		AbsolutePath: filepath.Join(movies, "Glass Half (2015)", "Glass Half (2015).mkv"),
		StreamIndex:  libraryAudioStream, MaxWidth: artwork.Width320, Format: artwork.FormatJPEG,
	})
	if !errors.Is(err, domain.ErrUnsupported) {
		t.Fatalf("Render error = %v, want ErrUnsupported", err)
	}
}

// requireLibrary returns the absolute movies library of the test library or skips.
func requireLibrary(t *testing.T) string {
	t.Helper()

	if _, err := os.Stat(filepath.Join(libraryDirectory, "manifest.json")); errors.Is(err, fs.ErrNotExist) {
		t.Skip("skipping artwork corpus test: testdata/library/manifest.json is missing " +
			"(generate it with `go run ./scripts/gen-testmedia`)")
	}

	movies, err := filepath.Abs(filepath.Join(libraryDirectory, "movies"))
	if err != nil {
		t.Fatalf("absolute library path: %v", err)
	}

	return movies
}

func runRenderCases(t *testing.T, renderer *ffmpeg.ImageRenderer, cases []renderCase) {
	t.Helper()

	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			rendered, err := renderer.Render(t.Context(), artwork.RenderRequest{
				AbsolutePath: test.path, StreamIndex: test.stream, MaxWidth: test.width, Format: test.format,
			})
			if err != nil {
				t.Fatalf("Render: %v", err)
			}

			assertRendered(t, rendered, test)
		})
	}
}

func assertRendered(t *testing.T, rendered artwork.Rendered, test renderCase) {
	t.Helper()

	if rendered.Width != test.wantWidth || rendered.Height != test.wantHeight || rendered.Format != test.format {
		t.Errorf("rendered %d×%d %s, want %d×%d %s",
			rendered.Width, rendered.Height, rendered.Format, test.wantWidth, test.wantHeight, test.format)
	}

	decode := jpeg.Decode
	if test.format == artwork.FormatPNG {
		decode = png.Decode
	}

	picture, err := decode(bytes.NewReader(rendered.Data))
	if err != nil {
		t.Fatalf("decode output: %v", err)
	}

	if hasAlpha := isTranslucent(picture); hasAlpha != test.wantAlpha {
		t.Errorf("output translucent = %t, want %t", hasAlpha, test.wantAlpha)
	}
}

// isTranslucent reports a picture with at least one pixel that is not fully opaque.
func isTranslucent(picture image.Image) bool {
	bounds := picture.Bounds()

	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			if _, _, _, alpha := picture.At(x, y).RGBA(); alpha != 0xffff {
				return true
			}
		}
	}

	return false
}
