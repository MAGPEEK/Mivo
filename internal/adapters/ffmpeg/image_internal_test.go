package ffmpeg

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/artwork"
)

func TestImageArguments(t *testing.T) {
	t.Parallel()

	directory := t.TempDir()
	poster := filepath.Join(directory, "poster.jpg")
	movie := filepath.Join(directory, "100% Wolf.mkv")

	head := []string{"-hide_banner", "-nostdin", "-loglevel", "error", "-max_pixels", "64000000"}
	jpegTail := []string{
		"-map_metadata", "-1", "-vf", "scale='min(iw,320)':-2:flags=lanczos,format=yuvj420p",
		"-c:v", "mjpeg", "-q:v", "3", "-f", "image2pipe", "pipe:1",
	}

	tests := []struct {
		name    string
		request artwork.RenderRequest
		want    []string
	}{
		{
			name:    "jpeg from an image file",
			request: artwork.RenderRequest{AbsolutePath: poster, StreamIndex: artwork.WholeFile, MaxWidth: artwork.Width320, Format: artwork.FormatJPEG},
			want: slices.Concat(head, []string{
				"-format_whitelist", wantDemuxersWithImages, "-protocol_whitelist", "file", "-i", "file:" + poster,
				"-map", "0:v:0", "-frames:v", "1",
			}, jpegTail),
		},
		{
			name:    "png keeps the pixel format",
			request: artwork.RenderRequest{AbsolutePath: poster, StreamIndex: artwork.WholeFile, MaxWidth: artwork.Width1920, Format: artwork.FormatPNG},
			want: slices.Concat(head, []string{
				"-format_whitelist", wantDemuxersWithImages, "-protocol_whitelist", "file", "-i", "file:" + poster,
				"-map", "0:v:0", "-frames:v", "1", "-map_metadata", "-1",
				"-vf", "scale='min(iw,1920)':-2:flags=lanczos", "-c:v", "png", "-f", "image2pipe", "pipe:1",
			}),
		},
		{
			name:    "embedded stream of a path with a percent sign",
			request: artwork.RenderRequest{AbsolutePath: movie, StreamIndex: 2, MaxWidth: artwork.Width320, Format: artwork.FormatJPEG},
			want: slices.Concat(head, []string{
				"-format_whitelist", wantSingleFileDemuxers, "-protocol_whitelist", "file", "-i", "file:" + movie,
				"-map", "0:2", "-frames:v", "1",
			}, jpegTail),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			got, err := imageArguments(test.request)
			if err != nil {
				t.Fatalf("imageArguments: %v", err)
			}

			if !slices.Equal(got, test.want) {
				t.Errorf("arguments =\n%q\nwant\n%q", got, test.want)
			}
		})
	}
}

func TestImageArgumentsRefusals(t *testing.T) {
	t.Parallel()

	absolute := filepath.Join(t.TempDir(), "poster.jpg")
	valid := artwork.RenderRequest{AbsolutePath: absolute, StreamIndex: artwork.WholeFile, MaxWidth: artwork.Width320, Format: artwork.FormatJPEG}

	tests := []struct {
		name   string
		change func(request *artwork.RenderRequest)
		want   error
	}{
		{name: "percent in an image path", change: func(r *artwork.RenderRequest) { r.AbsolutePath = filepath.Join(filepath.Dir(absolute), "img%03d.jpg") }, want: domain.ErrUnsupported},
		{name: "relative path", change: func(r *artwork.RenderRequest) { r.AbsolutePath = "poster.jpg" }, want: domain.ErrInvalid},
		{name: "zero width", change: func(r *artwork.RenderRequest) { r.MaxWidth = 0 }, want: domain.ErrInvalid},
		{name: "stream index below whole file", change: func(r *artwork.RenderRequest) { r.StreamIndex = -2 }, want: domain.ErrInvalid},
		{name: "unknown format", change: func(r *artwork.RenderRequest) { r.Format = "gif" }, want: domain.ErrInvalid},
	}

	// Only Windows has a device namespace; elsewhere `\\.\COM1` is a relative file name,
	// already covered by the relative-path case.
	if runtime.GOOS == windowsOS {
		tests = append(tests, struct {
			name   string
			change func(request *artwork.RenderRequest)
			want   error
		}{name: "device path", change: func(r *artwork.RenderRequest) { r.AbsolutePath = `\\.\COM1` }, want: domain.ErrInvalid})
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			request := valid
			test.change(&request)

			if _, err := imageArguments(request); !errors.Is(err, test.want) {
				t.Errorf("imageArguments error = %v, want %v", err, test.want)
			}
		})
	}
}

func TestNewImageRendererRequiresOptions(t *testing.T) {
	t.Parallel()

	logger := slog.New(slog.DiscardHandler)

	for _, options := range []ImageRendererOptions{{Logger: logger}, {FFmpegPath: "ffmpeg"}} {
		if _, err := NewImageRenderer(options); !errors.Is(err, domain.ErrInvalid) {
			t.Errorf("NewImageRenderer(%+v) error = %v, want ErrInvalid", options, err)
		}
	}
}

// writeSource writes an image file of size bytes and returns a request for it.
func writeSource(t *testing.T, size int) artwork.RenderRequest {
	t.Helper()

	path := filepath.Join(t.TempDir(), "poster.jpg")
	if err := os.WriteFile(path, make([]byte, size), 0o600); err != nil {
		t.Fatalf("write source: %v", err)
	}

	return artwork.RenderRequest{AbsolutePath: path, StreamIndex: artwork.WholeFile, MaxWidth: artwork.Width320, Format: artwork.FormatJPEG}
}

func newTestRenderer(t *testing.T, executable string, timeout time.Duration) *ImageRenderer {
	t.Helper()

	renderer, err := NewImageRenderer(ImageRendererOptions{FFmpegPath: executable, Logger: slog.New(slog.DiscardHandler)})
	if err != nil {
		t.Fatalf("NewImageRenderer: %v", err)
	}

	if timeout > 0 {
		renderer.timeout = timeout
	}

	return renderer
}

func TestImageRendererFailures(t *testing.T) {
	tests := []struct {
		name    string
		tool    func(t *testing.T) string
		request func(t *testing.T) artwork.RenderRequest
		timeout time.Duration
		want    error
	}{
		{
			name:    "missing tool",
			tool:    func(t *testing.T) string { return filepath.Join(t.TempDir(), "no-such-ffmpeg") },
			request: func(t *testing.T) artwork.RenderRequest { return writeSource(t, 1) },
			want:    domain.ErrUnavailable,
		},
		{
			name:    "timeout",
			tool:    func(t *testing.T) string { return fakeTool(t, fakeModeHang).executable },
			request: func(t *testing.T) artwork.RenderRequest { return writeSource(t, 1) },
			timeout: 200 * time.Millisecond,
			want:    context.DeadlineExceeded,
		},
		{
			name:    "oversized output",
			tool:    func(t *testing.T) string { return fakeTool(t, fakeModeFloodStdout).executable },
			request: func(t *testing.T) artwork.RenderRequest { return writeSource(t, 1) },
			want:    domain.ErrUnsupported,
		},
		{
			name:    "undecodable input",
			tool:    func(t *testing.T) string { return fakeTool(t, fakeModeFail).executable },
			request: func(t *testing.T) artwork.RenderRequest { return writeSource(t, 1) },
			want:    domain.ErrUnsupported,
		},
		{
			name: "output is no picture",
			tool: func(t *testing.T) string {
				fake, _ := fakeResponder(t, map[string]string{fakeDefaultResponse: "not a jpeg"})

				return fake.executable
			},
			request: func(t *testing.T) artwork.RenderRequest { return writeSource(t, 1) },
			want:    domain.ErrUnsupported,
		},
		{
			name:    "oversized source is refused before the tool starts",
			tool:    func(*testing.T) string { return "ffmpeg-that-is-never-started" },
			request: func(t *testing.T) artwork.RenderRequest { return writeSource(t, maxImageSourceBytes+1) },
			want:    domain.ErrUnsupported,
		},
		{
			name: "missing source",
			tool: func(*testing.T) string { return "ffmpeg-that-is-never-started" },
			request: func(t *testing.T) artwork.RenderRequest {
				request := writeSource(t, 1)
				request.AbsolutePath += ".gone"

				return request
			},
			want: domain.ErrNotFound,
		},
		{
			// A stat error other than "not exist": ENOTDIR on unix, an invalid name on
			// Windows, where a path through a file reports "not found" instead.
			name: "unreadable source path",
			tool: func(*testing.T) string { return "ffmpeg-that-is-never-started" },
			request: func(t *testing.T) artwork.RenderRequest {
				request := writeSource(t, 1)
				if runtime.GOOS == windowsOS {
					request.AbsolutePath = filepath.Join(filepath.Dir(request.AbsolutePath), "bad<name>.jpg")
				} else {
					request.AbsolutePath = filepath.Join(request.AbsolutePath, "x.jpg")
				}

				return request
			},
			want: errSourceUnreadable,
		},
		{
			name: "directory as source",
			tool: func(*testing.T) string { return "ffmpeg-that-is-never-started" },
			request: func(t *testing.T) artwork.RenderRequest {
				request := writeSource(t, 1)
				request.AbsolutePath = filepath.Dir(request.AbsolutePath)

				return request
			},
			want: domain.ErrUnsupported,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := test.request(t)
			renderer := newTestRenderer(t, test.tool(t), test.timeout)

			_, err := renderer.Render(t.Context(), request)
			if !errors.Is(err, test.want) {
				t.Fatalf("Render error = %v, want %v", err, test.want)
			}

			assertErrorNamesBaseOnly(t, err, request.AbsolutePath)
		})
	}
}

// assertErrorNamesBaseOnly checks that an error never reveals the directory of the source.
func assertErrorNamesBaseOnly(t *testing.T, err error, path string) {
	t.Helper()

	if strings.Contains(err.Error(), filepath.Dir(path)) {
		t.Errorf("error %q reveals the directory of %s", err, path)
	}
}

func TestImageRendererLargeEmbeddedSourceIsNotSizeChecked(t *testing.T) {
	request := writeSource(t, maxImageSourceBytes+1)
	request.StreamIndex = 2

	renderer := newTestRenderer(t, fakeTool(t, fakeModeFail).executable, 0)

	// The tool runs (and fails like on an undecodable file): the size limit is for images.
	_, err := renderer.Render(t.Context(), request)

	var exit *exitError
	if !errors.As(err, &exit) {
		t.Fatalf("Render error = %v, want the tool's exit error", err)
	}
}

func TestImageRendererReportsCancellation(t *testing.T) {
	request := writeSource(t, 1)
	renderer := newTestRenderer(t, fakeTool(t, fakeModeHang).executable, 0)

	cancelled, cancelNow := context.WithCancel(t.Context())
	defer cancelNow()

	time.AfterFunc(200*time.Millisecond, cancelNow)

	if _, err := renderer.Render(cancelled, request); !errors.Is(err, context.Canceled) {
		t.Fatalf("Render error = %v, want context.Canceled", err)
	}
}

func TestImageRendererPassesArguments(t *testing.T) {
	fake, calls := fakeResponder(t, map[string]string{fakeDefaultResponse: "x"})
	request := writeSource(t, 1)

	_, _ = newTestRenderer(t, fake.executable, 0).Render(t.Context(), request)

	want, err := imageArguments(request)
	if err != nil {
		t.Fatalf("imageArguments: %v", err)
	}

	if recorded := calls(); len(recorded) != 1 || !slices.Equal(recorded[0], want) {
		t.Errorf("arguments = %q, want %q", recorded, want)
	}
}
