package ffmpeg

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image/jpeg"
	"image/png"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/artwork"
)

const (
	// renderTimeout bounds one render. Decoding and scaling one picture takes well under a
	// second; the rest covers a NAS disk that has to spin up first.
	renderTimeout = 15 * time.Second
	// maxImageSourceBytes bounds an image file before FFmpeg starts: artwork beyond it is
	// no poster but a photo or a hostile file. Embedded pictures are bounded by
	// maxDecodedPixels instead, because their media file is large by nature.
	maxImageSourceBytes = 32 << 20
	// renderOperation names the renderer's ffmpeg runs in debug logs.
	renderOperation = "render image"
)

// ImageRendererOptions configures NewImageRenderer; every field is required.
type ImageRendererOptions struct {
	// FFmpegPath is the ffmpeg executable, the configured tools.ffmpeg value resolved by the
	// caller.
	FFmpegPath string
	// Logger receives one debug record with ffmpeg's error output per failed run.
	Logger *slog.Logger
}

// ImageRenderer turns artwork into small JPEG or PNG pictures with ffmpeg (ADR 0027 §8). It
// neither caches nor limits concurrency; the image use case does both.
type ImageRenderer struct {
	ffmpeg  tool
	logger  *slog.Logger
	timeout time.Duration
}

// NewImageRenderer returns a renderer that runs the ffmpeg executable of options. Errors
// wrap domain.ErrInvalid for a missing option.
func NewImageRenderer(options ImageRendererOptions) (*ImageRenderer, error) {
	switch {
	case options.FFmpegPath == "":
		return nil, fmt.Errorf("new image renderer: ffmpeg path is empty: %w", domain.ErrInvalid)
	case options.Logger == nil:
		return nil, fmt.Errorf("new image renderer: logger is nil: %w", domain.ErrInvalid)
	}

	return &ImageRenderer{
		ffmpeg:  tool{name: ffmpegToolName, executable: options.FFmpegPath},
		logger:  options.Logger,
		timeout: renderTimeout,
	}, nil
}

// Render scales the picture of request to at most request.MaxWidth (never up) and encodes
// it in request.Format. The caller has resolved the path and confined it to its root.
// Errors wrap
//   - domain.ErrInvalid for a malformed request, a relative or a Windows device path,
//   - domain.ErrNotFound when the file does not exist,
//   - domain.ErrUnsupported for an image file over 32 MiB or with "%" in its path, input
//     ffmpeg cannot decode or that exceeds the pixel limit, and output over 8 MiB,
//   - domain.ErrUnavailable when ffmpeg cannot be started,
//   - context.DeadlineExceeded after the render timeout, context.Canceled on cancellation.
//
// Messages name the file by its base name only; ffmpeg's error output goes to the debug log.
func (r *ImageRenderer) Render(ctx context.Context, request artwork.RenderRequest) (artwork.Rendered, error) {
	name := filepath.Base(request.AbsolutePath)

	arguments, err := imageArguments(request)
	if err != nil {
		return artwork.Rendered{}, fmt.Errorf("render %s: %w", name, err)
	}

	if err := checkSourceSize(request); err != nil {
		return artwork.Rendered{}, fmt.Errorf("render %s: %w", name, err)
	}

	renderContext, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()

	output, err := r.ffmpeg.output(renderContext, arguments)
	if err != nil {
		r.logRunFailure(ctx, name, err)

		return artwork.Rendered{}, fmt.Errorf("render %s: %w", name, classifyRunError(err))
	}

	rendered, err := describeRendered(output, request.Format)
	if err != nil {
		return artwork.Rendered{}, fmt.Errorf("render %s: %w", name, err)
	}

	return rendered, nil
}

// errSourceUnreadable reports a source whose file information cannot be read.
var errSourceUnreadable = errors.New("source file is unreadable")

// checkSourceSize refuses an image file over maxImageSourceBytes and reports a missing file
// before ffmpeg starts. The file may change between this check and ffmpeg opening it; that
// race is accepted because a swapped-in file is still bounded by -max_pixels, the output
// cap and the render timeout, so the check only spares ffmpeg the obvious cases.
func checkSourceSize(request artwork.RenderRequest) error {
	info, err := os.Stat(request.AbsolutePath)
	if errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("source file: %w", domain.ErrNotFound)
	}

	if err != nil {
		// The *fs.PathError text holds the absolute path; keep only its cause.
		var pathError *fs.PathError
		if errors.As(err, &pathError) {
			err = pathError.Err
		}

		return fmt.Errorf("source file: %w: %w", errSourceUnreadable, err)
	}

	if !info.Mode().IsRegular() {
		return fmt.Errorf("source is no regular file: %w", domain.ErrUnsupported)
	}

	if request.StreamIndex == artwork.WholeFile && info.Size() > maxImageSourceBytes {
		return fmt.Errorf("image file of %d bytes exceeds %d bytes: %w",
			info.Size(), maxImageSourceBytes, domain.ErrUnsupported)
	}

	return nil
}

// describeRendered reads the pixel size of ffmpeg's output, which also proves that ffmpeg
// wrote one complete header of the requested format. Errors wrap domain.ErrUnsupported.
func describeRendered(output []byte, format artwork.Format) (artwork.Rendered, error) {
	decodeConfig := jpeg.DecodeConfig
	if format == artwork.FormatPNG {
		decodeConfig = png.DecodeConfig
	}

	config, err := decodeConfig(bytes.NewReader(output))
	if err != nil {
		return artwork.Rendered{}, fmt.Errorf("decode %s output: %w: %w", format, domain.ErrUnsupported, err)
	}

	return artwork.Rendered{Data: output, Width: config.Width, Height: config.Height, Format: format}, nil
}

// logRunFailure writes ffmpeg's error output of a failed run to the debug log, like
// Prober.logRunFailure; the tail never reaches the returned error.
func (r *ImageRenderer) logRunFailure(ctx context.Context, name string, err error) {
	attributes := exitAttributes(err)
	if attributes == nil {
		return
	}

	attributes = append([]slog.Attr{
		slog.String("operation", renderOperation), slog.String("file", name),
	}, attributes...)
	r.logger.LogAttrs(ctx, slog.LevelDebug, "ffmpeg failed on an image", attributes...)
}
