package ffmpeg

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/artwork"
	"github.com/magpeek/mivo/internal/domain/library"
	"github.com/magpeek/mivo/internal/domain/photo"
)

const (
	// maxPhotoSourceBytes bounds a photo file FFmpeg decodes directly. It is far above the
	// artwork limit because an uncompressed TIFF of a 50 MP camera is about 150 MiB; memory
	// stays bounded by -max_pixels. A larger file falls back to its embedded preview.
	maxPhotoSourceBytes = 256 << 20
	// avifExtension tells AVIF apart from the other web formats of library.ImageClassWeb.
	avifExtension = ".avif"
	// photoRenderOperation names the photo renderer's ffmpeg runs in debug logs.
	photoRenderOperation = "render photo"
)

// ErrPhotoUnsupported reports a photo that cannot be rendered: an unknown format, a format
// the ffmpeg build cannot decode without a usable embedded preview, or a corrupt file. It
// wraps domain.ErrUnsupported, which the image endpoint answers like missing artwork.
var ErrPhotoUnsupported = fmt.Errorf("photo cannot be rendered: %w", domain.ErrUnsupported)

// PhotoMetadataReader reads what the photo renderer needs from a photo file besides its
// pixels. Implemented by adapters/photometa.Reader.
type PhotoMetadataReader interface {
	// ReadInfo returns the EXIF metadata; the renderer uses its orientation.
	ReadInfo(ctx context.Context, path string) (photo.Info, error)
	// ExtractPreview returns the largest embedded JPEG preview.
	ExtractPreview(ctx context.Context, path string) ([]byte, error)
}

// PhotoRendererOptions configures NewPhotoRenderer. Decoders is the result of
// DetectPhotoDecoders; every other field is required.
type PhotoRendererOptions struct {
	FFmpegPath string
	Logger     *slog.Logger
	Metadata   PhotoMetadataReader
	Decoders   PhotoDecoders
}

// PhotoRenderer renders photo thumbnails with ffmpeg (ADR 0029): web formats and TIFF
// directly with their EXIF orientation applied, HEIC and AVIF directly when the ffmpeg build
// decodes them, RAW files and everything that fails directly through their embedded JPEG
// preview. It neither caches nor limits concurrency; the image use case does both, shared
// with artwork.
type PhotoRenderer struct {
	ffmpeg   tool
	logger   *slog.Logger
	metadata PhotoMetadataReader
	decoders PhotoDecoders
	timeout  time.Duration
}

// NewPhotoRenderer returns a renderer for options. Errors wrap domain.ErrInvalid for a
// missing option.
func NewPhotoRenderer(options PhotoRendererOptions) (*PhotoRenderer, error) {
	switch {
	case options.FFmpegPath == "":
		return nil, fmt.Errorf("new photo renderer: ffmpeg path is empty: %w", domain.ErrInvalid)
	case options.Logger == nil:
		return nil, fmt.Errorf("new photo renderer: logger is nil: %w", domain.ErrInvalid)
	case options.Metadata == nil:
		return nil, fmt.Errorf("new photo renderer: metadata reader is nil: %w", domain.ErrInvalid)
	}

	return &PhotoRenderer{
		ffmpeg:   tool{name: ffmpegToolName, executable: options.FFmpegPath},
		logger:   options.Logger,
		metadata: options.Metadata,
		decoders: options.Decoders,
		timeout:  renderTimeout,
	}, nil
}

// Render scales the photo at request.AbsolutePath (resolved and confined by the caller) to
// at most request.MaxWidth, upright. request.StreamIndex must be artwork.WholeFile. Errors
// wrap
//   - domain.ErrInvalid for a malformed request, a relative or a Windows device path,
//   - domain.ErrNotFound when the file does not exist,
//   - ErrPhotoUnsupported (domain.ErrUnsupported) when neither the file nor its preview
//     renders,
//   - domain.ErrUnavailable when ffmpeg cannot be started,
//   - context.DeadlineExceeded after the render timeout, context.Canceled on cancellation.
//
// Messages name the file by its base name only.
func (r *PhotoRenderer) Render(ctx context.Context, request artwork.RenderRequest) (artwork.Rendered, error) {
	name := filepath.Base(request.AbsolutePath)

	size, err := checkPhotoRequest(request)
	if err != nil {
		return artwork.Rendered{}, fmt.Errorf("render photo %s: %w", name, err)
	}

	renderContext, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()

	input, direct := r.directInput(name, size)
	if direct {
		rendered, directErr := r.renderFile(renderContext, request, input)
		if directErr == nil || !errors.Is(directErr, domain.ErrUnsupported) {
			return rendered, wrapPhotoError(name, directErr)
		}
	}

	rendered, err := r.renderPreview(renderContext, request)

	return rendered, wrapPhotoError(name, err)
}

// directInput decides whether ffmpeg decodes the file itself and how its orientation is
// applied. RAW files, HEIC and AVIF without a decoder and oversized files use the preview.
func (r *PhotoRenderer) directInput(name string, size int64) (photoInput, bool) {
	class, known := library.ClassifyImage(name)
	if !known || size > maxPhotoSourceBytes {
		return inputPreview, false
	}

	switch {
	case class == library.ImageClassHEIC:
		return inputFileWithContainerRotation, r.decoders.HEIC
	case strings.EqualFold(filepath.Ext(name), avifExtension):
		return inputFileWithContainerRotation, r.decoders.AVIF
	case class == library.ImageClassRAW:
		return inputPreview, false
	default:
		return inputFileWithEXIF, true
	}
}

// renderFile lets ffmpeg decode the photo file.
func (r *PhotoRenderer) renderFile(ctx context.Context, request artwork.RenderRequest, input photoInput) (artwork.Rendered, error) {
	job := photoJob{
		input: input, path: request.AbsolutePath, name: filepath.Base(request.AbsolutePath),
		width: request.MaxWidth, format: request.Format,
	}
	if input == inputFileWithEXIF {
		job.orientation = r.orientation(ctx, request.AbsolutePath)
	}

	return r.run(ctx, job, nil)
}

// renderPreview pipes the embedded JPEG preview to ffmpeg. A file without a readable
// preview is ErrPhotoUnsupported; the context error stays visible as such.
func (r *PhotoRenderer) renderPreview(ctx context.Context, request artwork.RenderRequest) (artwork.Rendered, error) {
	preview, err := r.metadata.ExtractPreview(ctx, request.AbsolutePath)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return artwork.Rendered{}, fmt.Errorf("extract preview: %w", ctxErr)
		}

		// The photometa error text names the absolute path; keep only that it failed.
		r.logger.LogAttrs(ctx, slog.LevelDebug, "photo has no usable preview",
			slog.String("operation", photoRenderOperation), slog.Any("error", err))

		return artwork.Rendered{}, fmt.Errorf("extract preview: %w", ErrPhotoUnsupported)
	}

	job := photoJob{
		input: inputPreview, name: filepath.Base(request.AbsolutePath),
		width: request.MaxWidth, format: request.Format,
		orientation: r.orientation(ctx, request.AbsolutePath),
	}

	return r.run(ctx, job, preview)
}

// orientation returns the EXIF orientation of the file, or unknown (shown as stored) when
// the file has none or its metadata cannot be read: a thumbnail is still better than none.
func (r *PhotoRenderer) orientation(ctx context.Context, path string) int {
	info, err := r.metadata.ReadInfo(ctx, path)
	if err != nil {
		return photo.OrientationUnknown
	}

	return info.Orientation
}

// run executes one ffmpeg render with input as its standard input.
func (r *PhotoRenderer) run(ctx context.Context, job photoJob, input []byte) (artwork.Rendered, error) {
	arguments, err := photoArguments(job)
	if err != nil {
		return artwork.Rendered{}, err
	}

	output, err := r.ffmpeg.pipedOutput(ctx, arguments, input, maxOutputBytes)
	if err != nil {
		logImageRunFailure(ctx, r.logger, imageRun{operation: photoRenderOperation, name: job.name}, err)

		return artwork.Rendered{}, asPhotoUnsupported(classifyRunError(err))
	}

	rendered, err := describeRendered(output, job.format)
	if err != nil {
		return artwork.Rendered{}, asPhotoUnsupported(err)
	}

	return rendered, nil
}

// asPhotoUnsupported marks an undecodable picture with ErrPhotoUnsupported and passes other
// errors (cancellation, a tool that cannot start) through unchanged.
func asPhotoUnsupported(err error) error {
	if errors.Is(err, domain.ErrUnsupported) && !errors.Is(err, ErrPhotoUnsupported) {
		return fmt.Errorf("%w: %w", ErrPhotoUnsupported, err)
	}

	return err
}

// checkPhotoRequest validates request and returns the size of its file. A missing file
// wraps domain.ErrNotFound.
func checkPhotoRequest(request artwork.RenderRequest) (int64, error) {
	if request.StreamIndex != artwork.WholeFile {
		return 0, fmt.Errorf("photo stream index %d is no whole file: %w", request.StreamIndex, domain.ErrInvalid)
	}

	if err := validateRenderRequest(request); err != nil {
		return 0, err
	}

	if _, err := inputArguments(request.AbsolutePath); err != nil {
		return 0, err
	}

	info, err := os.Stat(request.AbsolutePath)
	if errors.Is(err, fs.ErrNotExist) {
		return 0, fmt.Errorf("source file: %w", domain.ErrNotFound)
	}

	if err != nil {
		return 0, fmt.Errorf("source file: %w: %w", errSourceUnreadable, unwrapPathError(err))
	}

	if !info.Mode().IsRegular() {
		return 0, fmt.Errorf("source is no regular file: %w", ErrPhotoUnsupported)
	}

	return info.Size(), nil
}

// wrapPhotoError adds the file name to a failed render and passes success through.
func wrapPhotoError(name string, err error) error {
	if err == nil {
		return nil
	}

	return fmt.Errorf("render photo %s: %w", name, err)
}
