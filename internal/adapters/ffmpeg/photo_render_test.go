package ffmpeg_test

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/magpeek/mivo/internal/adapters/ffmpeg"
	"github.com/magpeek/mivo/internal/adapters/photometa"
	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/artwork"
	"github.com/magpeek/mivo/internal/domain/photo"
)

const (
	// Stored size of the rotated test JPEG; orientation 6 shows it as 32×64.
	rotatedWidth  = 64
	rotatedHeight = 32
	// exifOrientationTag is the TIFF tag of the EXIF orientation, a SHORT.
	exifOrientationTag = 0x0112
	tiffTypeShort      = 3
	// orientationRotateClockwise displays the stored picture turned a quarter clockwise.
	orientationRotateClockwise = 6
	// generatedSize is the lavfi frame size of the generated PNG, WebP and AVIF photos.
	generatedSize = "640x480"
	// redThreshold separates the red marker from the black rest after JPEG compression.
	redThreshold = 160
)

// fakeMetadata stands in for photometa: a fixed orientation and preview.
type fakeMetadata struct {
	orientation int
	preview     []byte
	previewErr  error
}

func (metadata fakeMetadata) ReadInfo(context.Context, string) (photo.Info, error) {
	return photo.Info{Orientation: metadata.orientation}, nil
}

func (metadata fakeMetadata) ExtractPreview(context.Context, string) ([]byte, error) {
	if metadata.previewErr != nil {
		return nil, metadata.previewErr
	}

	return metadata.preview, nil
}

func newPhotoRenderer(t *testing.T, metadata ffmpeg.PhotoMetadataReader, decoders ffmpeg.PhotoDecoders) *ffmpeg.PhotoRenderer {
	t.Helper()

	renderer, err := ffmpeg.NewPhotoRenderer(ffmpeg.PhotoRendererOptions{
		FFmpegPath: requireFFmpeg(t), Logger: slog.New(slog.DiscardHandler), Metadata: metadata, Decoders: decoders,
	})
	if err != nil {
		t.Fatalf("NewPhotoRenderer: %v", err)
	}

	return renderer
}

// markedJPEG encodes a black width×height picture with a red top-left quarter.
func markedJPEG(t *testing.T, width, height int) []byte {
	t.Helper()

	picture := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := range height {
		for x := range width {
			pixel := color.RGBA{A: 255}
			if x < width/2 && y < height/2 {
				pixel.R = 255
			}

			picture.SetRGBA(x, y, pixel)
		}
	}

	var encoded bytes.Buffer
	if err := jpeg.Encode(&encoded, picture, &jpeg.Options{Quality: 95}); err != nil {
		t.Fatalf("encode JPEG: %v", err)
	}

	return encoded.Bytes()
}

// withOrientation inserts an APP1 Exif segment holding only the orientation after the SOI
// marker of a JPEG.
func withOrientation(data []byte, orientation uint16) []byte {
	tiff := []byte("MM\x00\x2a\x00\x00\x00\x08")
	tiff = binary.BigEndian.AppendUint16(tiff, 1)
	tiff = binary.BigEndian.AppendUint16(tiff, exifOrientationTag)
	tiff = binary.BigEndian.AppendUint16(tiff, tiffTypeShort)
	tiff = binary.BigEndian.AppendUint32(tiff, 1)
	tiff = binary.BigEndian.AppendUint16(tiff, orientation)
	tiff = binary.BigEndian.AppendUint16(tiff, 0)
	tiff = binary.BigEndian.AppendUint32(tiff, 0)

	payload := append([]byte("Exif\x00\x00"), tiff...)
	segment := []byte{0xFF, 0xE1}
	segment = binary.BigEndian.AppendUint16(segment, uint16(len(payload)+2)) //nolint:gosec // a few bytes
	segment = append(segment, payload...)

	return bytes.Join([][]byte{data[:2], segment, data[2:]}, nil)
}

func writeFile(t *testing.T, name string, data []byte) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}

	return path
}

// generatePhoto encodes one testsrc2 frame with ffmpeg; it skips when the local build
// lacks the encoder or muxer.
func generatePhoto(t *testing.T, ffmpegPath, name string, encoding ...string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), name)
	arguments := append([]string{
		"-hide_banner", "-loglevel", "error", "-f", "lavfi", "-i", "testsrc2=size=" + generatedSize, "-frames:v", "1",
	}, encoding...)

	output, err := exec.CommandContext(t.Context(), ffmpegPath, append(arguments, path)...).CombinedOutput() //nolint:gosec // test-built arguments
	if err != nil {
		t.Skipf("local FFmpeg cannot write %s: %v: %s", name, err, output)
	}

	return path
}

func renderPhoto(t *testing.T, renderer *ffmpeg.PhotoRenderer, path string, width artwork.Width) (artwork.Rendered, error) {
	t.Helper()

	return renderer.Render(t.Context(), artwork.RenderRequest{
		AbsolutePath: path, StreamIndex: artwork.WholeFile, MaxWidth: width, Format: artwork.FormatJPEG,
	})
}

func requireSize(t *testing.T, rendered artwork.Rendered, err error, want image.Point) image.Image {
	t.Helper()

	if err != nil {
		t.Fatalf("Render: %v", err)
	}

	if rendered.Width != want.X || rendered.Height != want.Y {
		t.Fatalf("rendered %d×%d, want %d×%d", rendered.Width, rendered.Height, want.X, want.Y)
	}

	decoded, err := jpeg.Decode(bytes.NewReader(rendered.Data))
	if err != nil {
		t.Fatalf("decode rendered JPEG: %v", err)
	}

	return decoded
}

// requireRedAt fails unless the pixel at (x, y) of picture is the red marker.
func requireRedAt(t *testing.T, picture image.Image, x, y int) {
	t.Helper()

	red, green, _, _ := picture.At(x, y).RGBA()
	if red>>8 < redThreshold || green>>8 > redThreshold/2 {
		t.Errorf("pixel (%d, %d) is not the red marker: r=%d g=%d", x, y, red>>8, green>>8)
	}
}

func TestRenderPhotoAppliesEXIFOrientation(t *testing.T) {
	t.Parallel()

	path := writeFile(t, "rotated.jpg", withOrientation(markedJPEG(t, rotatedWidth, rotatedHeight), orientationRotateClockwise))
	renderer := newPhotoRenderer(t, photometa.NewReader(), ffmpeg.PhotoDecoders{})

	rendered, err := renderPhoto(t, renderer, path, artwork.Width160)
	picture := requireSize(t, rendered, err, image.Pt(rotatedHeight, rotatedWidth))

	// Turned clockwise, the stored top-left quarter becomes the top-right one.
	requireRedAt(t, picture, rotatedHeight*3/4, rotatedWidth/4)
}

func TestRenderPhotoWebFormats(t *testing.T) {
	t.Parallel()

	ffmpegPath := requireFFmpeg(t)
	renderer := newPhotoRenderer(t, photometa.NewReader(), ffmpeg.PhotoDecoders{})

	formats := []struct {
		name     string
		encoding []string
	}{
		{name: "photo.png", encoding: []string{"-c:v", "png"}},
		{name: "photo.webp", encoding: []string{"-c:v", "libwebp"}},
		{name: "photo.tif", encoding: []string{"-c:v", "tiff"}},
		{name: "photo.gif", encoding: []string{"-c:v", "gif"}},
	}

	for _, format := range formats {
		t.Run(format.name, func(t *testing.T) {
			t.Parallel()

			path := generatePhoto(t, ffmpegPath, format.name, format.encoding...)

			rendered, err := renderPhoto(t, renderer, path, artwork.Width320)
			requireSize(t, rendered, err, image.Pt(320, 240))

			// Never upscaled: the widest width yields the source width.
			rendered, err = renderPhoto(t, renderer, path, artwork.Width1920)
			requireSize(t, rendered, err, image.Pt(640, 480))
		})
	}
}

func TestRenderPhotoAVIFWithDetectedDecoder(t *testing.T) {
	t.Parallel()

	ffmpegPath := requireFFmpeg(t)

	decoders, err := ffmpeg.DetectPhotoDecoders(t.Context(), ffmpegPath, slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatalf("DetectPhotoDecoders: %v", err)
	}

	if !decoders.AVIF {
		t.Skip("local FFmpeg has no software AV1 decoder")
	}

	path := generatePhoto(t, ffmpegPath, "photo.avif", "-c:v", "libaom-av1", "-still-picture", "1", "-f", "avif")
	renderer := newPhotoRenderer(t, photometa.NewReader(), decoders)

	rendered, err := renderPhoto(t, renderer, path, artwork.Width320)
	requireSize(t, rendered, err, image.Pt(320, 240))
}

func TestRenderPhotoFromPreview(t *testing.T) {
	t.Parallel()

	preview := markedJPEG(t, rotatedWidth, rotatedHeight)
	// The RAW and HEIC contents are no picture FFmpeg could decode: only the preview renders.
	junk := []byte("not a picture FFmpeg can decode")

	tests := []struct {
		name     string
		file     string
		decoders ffmpeg.PhotoDecoders
	}{
		{name: "RAW", file: "photo.nef"},
		{name: "HEIC without decoder", file: "photo.heic"},
		{name: "HEIC whose direct decode fails", file: "photo.heic", decoders: ffmpeg.PhotoDecoders{HEIC: true}},
		{name: "AVIF without decoder", file: "photo.avif"},
		{name: "corrupt JPEG", file: "photo.jpg"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			metadata := fakeMetadata{orientation: orientationRotateClockwise, preview: preview}
			renderer := newPhotoRenderer(t, metadata, test.decoders)

			rendered, err := renderPhoto(t, renderer, writeFile(t, test.file, junk), artwork.Width160)
			picture := requireSize(t, rendered, err, image.Pt(rotatedHeight, rotatedWidth))
			requireRedAt(t, picture, rotatedHeight*3/4, rotatedWidth/4)
		})
	}
}

func TestRenderPhotoUnsupported(t *testing.T) {
	t.Parallel()

	junk := []byte("not a picture")

	tests := []struct {
		name     string
		file     string
		metadata fakeMetadata
	}{
		{name: "RAW without preview", file: "photo.cr2", metadata: fakeMetadata{previewErr: photometa.ErrNoPreview}},
		{name: "unknown extension", file: "photo.psd", metadata: fakeMetadata{previewErr: photometa.ErrUnsupported}},
		{name: "corrupt preview", file: "photo.arw", metadata: fakeMetadata{preview: junk}},
		{name: "corrupt JPEG without preview", file: "photo.jpg", metadata: fakeMetadata{previewErr: photometa.ErrCorrupt}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			renderer := newPhotoRenderer(t, test.metadata, ffmpeg.PhotoDecoders{})

			_, err := renderPhoto(t, renderer, writeFile(t, test.file, junk), artwork.Width160)
			if !errors.Is(err, ffmpeg.ErrPhotoUnsupported) || !errors.Is(err, domain.ErrUnsupported) {
				t.Errorf("Render = %v, want ErrPhotoUnsupported", err)
			}
		})
	}
}

func TestRenderPhotoRequestErrors(t *testing.T) {
	t.Parallel()

	renderer := newPhotoRenderer(t, fakeMetadata{}, ffmpeg.PhotoDecoders{})
	directory := t.TempDir()

	tests := []struct {
		name    string
		request artwork.RenderRequest
		want    error
	}{
		{"missing file", artwork.RenderRequest{
			AbsolutePath: filepath.Join(directory, "gone.jpg"), StreamIndex: artwork.WholeFile,
			MaxWidth: artwork.Width160, Format: artwork.FormatJPEG,
		}, domain.ErrNotFound},
		{"directory", artwork.RenderRequest{
			AbsolutePath: directory, StreamIndex: artwork.WholeFile, MaxWidth: artwork.Width160, Format: artwork.FormatJPEG,
		}, domain.ErrUnsupported},
		{"stream index", artwork.RenderRequest{
			AbsolutePath: filepath.Join(directory, "a.jpg"), StreamIndex: 0, MaxWidth: artwork.Width160, Format: artwork.FormatJPEG,
		}, domain.ErrInvalid},
		{"relative path", artwork.RenderRequest{
			AbsolutePath: "a.jpg", StreamIndex: artwork.WholeFile, MaxWidth: artwork.Width160, Format: artwork.FormatJPEG,
		}, domain.ErrInvalid},
		{"format", artwork.RenderRequest{
			AbsolutePath: filepath.Join(directory, "a.jpg"), StreamIndex: artwork.WholeFile, MaxWidth: artwork.Width160,
		}, domain.ErrInvalid},
	}

	for _, test := range tests {
		if _, err := renderer.Render(t.Context(), test.request); !errors.Is(err, test.want) {
			t.Errorf("%s: err = %v, want %v", test.name, err, test.want)
		}
	}
}

func TestRenderPhotoCancelled(t *testing.T) {
	t.Parallel()

	renderer := newPhotoRenderer(t, fakeMetadata{previewErr: context.Canceled}, ffmpeg.PhotoDecoders{})
	path := writeFile(t, "photo.nef", []byte("raw"))

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	_, err := renderer.Render(ctx, artwork.RenderRequest{
		AbsolutePath: path, StreamIndex: artwork.WholeFile, MaxWidth: artwork.Width160, Format: artwork.FormatJPEG,
	})
	if !errors.Is(err, context.Canceled) || errors.Is(err, domain.ErrUnsupported) {
		t.Errorf("cancelled render = %v, want context.Canceled only", err)
	}
}

func TestNewPhotoRendererValidatesOptions(t *testing.T) {
	t.Parallel()

	logger := slog.New(slog.DiscardHandler)
	options := []ffmpeg.PhotoRendererOptions{
		{Logger: logger, Metadata: fakeMetadata{}},
		{FFmpegPath: "ffmpeg", Metadata: fakeMetadata{}},
		{FFmpegPath: "ffmpeg", Logger: logger},
	}

	for index, option := range options {
		if _, err := ffmpeg.NewPhotoRenderer(option); !errors.Is(err, domain.ErrInvalid) {
			t.Errorf("options %d: err = %v, want ErrInvalid", index, err)
		}
	}
}

// Compile-time check that photometa satisfies the renderer's port.
var _ ffmpeg.PhotoMetadataReader = (*photometa.Reader)(nil)
