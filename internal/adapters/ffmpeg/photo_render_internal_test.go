package ffmpeg

import (
	"bytes"
	"context"
	"image"
	"image/jpeg"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"testing"

	"github.com/magpeek/mivo/internal/domain/artwork"
	"github.com/magpeek/mivo/internal/domain/photo"
)

// previewWidth and previewHeight are the size of the stub preview the fake echoes back.
const (
	previewWidth  = 32
	previewHeight = 16
)

// stubPreviewMetadata returns no orientation and a fixed preview.
type stubPreviewMetadata struct{ preview []byte }

func (stubPreviewMetadata) ReadInfo(context.Context, string) (photo.Info, error) {
	return photo.Info{}, nil
}

func (metadata stubPreviewMetadata) ExtractPreview(context.Context, string) ([]byte, error) {
	return metadata.preview, nil
}

// TestRenderHEIFBoundsEveryDirectRun runs a HEIF photo whose direct renders fail: the tile
// grid run, then the single image run, then the preview, and every run that decodes the
// file itself carries the allocation bound, since FFmpeg may assemble a forged grid canvas
// on the single image run too.
func TestRenderHEIFBoundsEveryDirectRun(t *testing.T) {
	var preview bytes.Buffer
	if err := jpeg.Encode(&preview, image.NewGray(image.Rect(0, 0, previewWidth, previewHeight)), nil); err != nil {
		t.Fatalf("encode preview: %v", err)
	}

	argumentsPath := filepath.Join(t.TempDir(), "arguments.jsonl")
	t.Setenv(fakeArgumentsVariable, argumentsPath)

	photoPath := filepath.Join(t.TempDir(), "photo.heic")
	if err := os.WriteFile(photoPath, []byte("not a HEIF"), 0o600); err != nil {
		t.Fatalf("write photo: %v", err)
	}

	renderer := &PhotoRenderer{
		ffmpeg: fakeTool(t, fakeModeFailFileInput), logger: slog.New(slog.DiscardHandler),
		metadata: stubPreviewMetadata{preview: preview.Bytes()}, decoders: PhotoDecoders{HEIC: true},
		timeout: renderTimeout,
	}

	rendered, err := renderer.Render(t.Context(), artwork.RenderRequest{
		AbsolutePath: photoPath, StreamIndex: artwork.WholeFile, MaxWidth: artwork.Width320, Format: artwork.FormatJPEG,
	})
	if err != nil || rendered.Width != previewWidth {
		t.Fatalf("Render = %d wide, %v; want the %d wide preview", rendered.Width, err, previewWidth)
	}

	calls := readRecordedCalls(t, argumentsPath)
	if len(calls) != 3 {
		t.Fatalf("ffmpeg ran %d times, want grid, single image and preview: %q", len(calls), calls)
	}

	allocation := strconv.Itoa(maxGridAllocationBytes)
	runs := []struct {
		name          string
		arguments     []string
		wantComplex   bool
		wantAllocated bool
	}{
		{"tile grid", calls[0], true, true},
		{"single image", calls[1], false, true},
		{"preview", calls[2], false, false},
	}

	for _, run := range runs {
		if got := slices.Contains(run.arguments, "-filter_complex"); got != run.wantComplex {
			t.Errorf("%s run: -filter_complex present = %v, want %v", run.name, got, run.wantComplex)
		}

		if got := argumentValue(run.arguments, "-max_alloc") == allocation; got != run.wantAllocated {
			t.Errorf("%s run: -max_alloc %s present = %v, want %v", run.name, allocation, got, run.wantAllocated)
		}
	}
}
