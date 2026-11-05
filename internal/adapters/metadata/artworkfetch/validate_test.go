package artworkfetch

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io/fs"
	"os"
	"testing"

	"github.com/magpeek/mivo/internal/domain"
)

// fixtureDir holds small synthetic pictures (4x6 JPEG/PNG, 1x1 lossless WebP, a GIF).
const fixtureDir = "../../../../testdata/providers/images"

func readFixture(t *testing.T, name string) []byte {
	t.Helper()

	data, err := fs.ReadFile(os.DirFS(fixtureDir), name)
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}

	return data
}

// testChunkLength is the size of every synthetic WebP chunk; it fits the VP8 and VP8X
// headers exactly.
const testChunkLength = 10

// webPChunk wraps the first testChunkLength bytes of data (zero padded) in a RIFF container.
func webPChunk(fourCC string, data []byte) []byte {
	chunk := make([]byte, testChunkLength)
	copy(chunk, data)

	body := make([]byte, 0, chunkDataOffset+testChunkLength)
	body = append(body, "RIFF"...)
	body = binary.LittleEndian.AppendUint32(body, uint32(4+chunkHeaderLength+testChunkLength))
	body = append(body, "WEBP"...)
	body = append(body, fourCC...)
	body = binary.LittleEndian.AppendUint32(body, uint32(testChunkLength))

	return append(body, chunk...)
}

func lossyWebP(width, height uint16) []byte {
	data := []byte{0, 0, 0, 0x9D, 0x01, 0x2A}
	data = binary.LittleEndian.AppendUint16(data, width)
	data = binary.LittleEndian.AppendUint16(data, height)

	return webPChunk("VP8 ", data)
}

// extendedWebP stores width-1 and height-1 as 24-bit little-endian values.
func extendedWebP(width, height uint32) []byte {
	data := []byte{0, 0, 0, 0}
	data = append(data, binary.LittleEndian.AppendUint32(nil, width-1)[:3]...)
	data = append(data, binary.LittleEndian.AppendUint32(nil, height-1)[:3]...)

	return webPChunk("VP8X", data)
}

func TestValidateImageAccepts(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		body          []byte
		extension     string
		width, height int
	}{
		{"jpeg", readFixture(t, "poster.jpg"), extensionJPEG, 4, 6},
		{"png", readFixture(t, "poster.png"), extensionPNG, 4, 6},
		{"lossless webp", readFixture(t, "poster.webp"), extensionWebP, 1, 1},
		{"lossy webp", lossyWebP(640, 360), extensionWebP, 640, 360},
		{"extended webp", extendedWebP(1920, 1080), extensionWebP, 1920, 1080},
		{"jpeg with zero padding", append(readFixture(t, "poster.jpg"), 0, 0), extensionJPEG, 4, 6},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			info, err := validateImage(test.body)
			if err != nil {
				t.Fatalf("validateImage: %v", err)
			}

			want := imageInfo{Extension: test.extension, Width: test.width, Height: test.height}
			if info != want {
				t.Fatalf("info = %+v, want %+v", info, want)
			}
		})
	}
}

func TestValidateImageRejects(t *testing.T) {
	t.Parallel()

	jpegBytes := readFixture(t, "poster.jpg")
	pngBytes := readFixture(t, "poster.png")
	webPBytes := readFixture(t, "poster.webp")
	oversized := append(bytes.Clone(jpegBytes), make([]byte, MaxImageBytes)...)

	tests := []struct {
		name string
		body []byte
	}{
		{"empty", nil},
		{"gif", readFixture(t, "not-an-image.gif")},
		{"html", []byte("<html><body>not a picture</body></html>")},
		{"oversized", oversized},
		{"truncated jpeg", jpegBytes[:len(jpegBytes)/2]},
		{"jpeg header only", jpegBytes[:4]},
		{"truncated png", pngBytes[:len(pngBytes)-4]},
		{"png header only", pngBytes[:16]},
		{"truncated webp", webPBytes[:len(webPBytes)-4]},
		{"webp header only", webPBytes[:14]},
		{"webp unknown chunk", webPChunk("ALPH", make([]byte, 10))},
		{"webp bad start code", webPChunk("VP8 ", make([]byte, 10))},
		{"zero width", lossyWebP(0, 10)},
		{"too wide", extendedWebP(MaxImageDimension+1, 10)},
		{"too tall", extendedWebP(10, MaxImageDimension+1)},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			if _, err := validateImage(test.body); !errors.Is(err, domain.ErrInvalid) {
				t.Fatalf("validateImage error = %v, want ErrInvalid", err)
			}
		})
	}
}
