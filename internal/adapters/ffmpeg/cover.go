package ffmpeg

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/magpeek/mivo/internal/domain"
)

const (
	// coverTimeout bounds one extraction, both tool runs together. Copying a picture takes
	// milliseconds; the rest covers a NAS disk that has to spin up first.
	coverTimeout = 30 * time.Second
	// maxCoverBytes bounds an extracted picture. Embedded covers are rarely above a few
	// megabytes; a larger one is a broken or hostile file, not artwork worth storing.
	maxCoverBytes = 16 << 20
	// findCoverOperation and copyCoverOperation name the extractor's runs in debug logs.
	findCoverOperation = "find cover"
	copyCoverOperation = "copy cover"
	// frontCoverComment is the stream comment FFmpeg sets for an ID3 APIC, FLAC PICTURE
	// or similar picture of type 3, the front cover.
	frontCoverComment = "Cover (front)"
	// videoStreams limits ffprobe's stream list to pictures and video.
	videoStreams = "v"
)

// ErrNoCover reports an audio file without an embedded JPEG or PNG picture. It wraps
// domain.ErrNotFound, so callers that know only the domain errors can tell it apart.
var ErrNoCover = fmt.Errorf("no embedded cover: %w", domain.ErrNotFound)

// coverCodecs are the picture codecs an extracted cover may have: the formats every client
// and the artwork renderer read.
func coverCodecs() []string { return []string{"mjpeg", "png"} }

// pictureSignatures are the leading bytes of the formats in coverCodecs.
func pictureSignatures() [][]byte {
	return [][]byte{{0xFF, 0xD8, 0xFF}, []byte("\x89PNG\r\n\x1a\n")}
}

// CoverExtractorOptions configures NewCoverExtractor; every field is required.
type CoverExtractorOptions struct {
	// FFmpegPath and FFprobePath are the configured tools.ffmpeg and tools.ffprobe
	// executables, resolved by the caller.
	FFmpegPath  string
	FFprobePath string
	// Logger receives one debug record with the tool's error output per failed run.
	Logger *slog.Logger
}

// CoverExtractor copies the embedded cover of an audio file (an attached picture) without
// re-encoding it.
type CoverExtractor struct {
	ffmpeg  tool
	ffprobe tool
	logger  *slog.Logger
	timeout time.Duration
	// maxBytes is maxCoverBytes; tests lower it.
	maxBytes int
}

// NewCoverExtractor returns an extractor that runs the executables of options. Errors wrap
// domain.ErrInvalid for a missing option.
func NewCoverExtractor(options CoverExtractorOptions) (*CoverExtractor, error) {
	switch {
	case options.FFmpegPath == "":
		return nil, fmt.Errorf("new cover extractor: ffmpeg path is empty: %w", domain.ErrInvalid)
	case options.FFprobePath == "":
		return nil, fmt.Errorf("new cover extractor: ffprobe path is empty: %w", domain.ErrInvalid)
	case options.Logger == nil:
		return nil, fmt.Errorf("new cover extractor: logger is nil: %w", domain.ErrInvalid)
	}

	return &CoverExtractor{
		ffmpeg:   tool{name: ffmpegToolName, executable: options.FFmpegPath},
		ffprobe:  tool{name: ffprobeName, executable: options.FFprobePath},
		logger:   options.Logger,
		timeout:  coverTimeout,
		maxBytes: maxCoverBytes,
	}, nil
}

// ExtractCover returns the bytes of the embedded JPEG or PNG cover of the file at
// absolutePath: the front cover when the file marks one, else the first picture. The caller
// has resolved the path and confined it to its library root (ADR 0022). Errors wrap
//   - ErrNoCover (and domain.ErrNotFound) when the file has no such picture,
//   - domain.ErrInvalid for a relative path or a refused Windows device path,
//   - domain.ErrUnsupported when a tool failed on the file (including a demuxer outside the
//     input whitelist), the picture exceeds 16 MiB or is no JPEG or PNG,
//   - domain.ErrUnavailable when a tool cannot be started,
//   - context.DeadlineExceeded after the timeout, context.Canceled on cancellation.
//
// Messages name the file by its base name only; tool error output goes to the debug log.
func (e *CoverExtractor) ExtractCover(ctx context.Context, absolutePath string) ([]byte, error) {
	name := filepath.Base(absolutePath)

	input, err := inputArguments(absolutePath)
	if err != nil {
		return nil, fmt.Errorf("extract cover: %w", err)
	}

	extractContext, cancel := context.WithTimeout(ctx, e.timeout)
	defer cancel()

	streamIndex, err := e.findCover(extractContext, absolutePath, input)
	if err != nil {
		return nil, fmt.Errorf("extract cover %s: %w", name, err)
	}

	data, err := e.copyCover(extractContext, absolutePath, streamIndex)
	if err != nil {
		return nil, fmt.Errorf("extract cover %s: %w", name, err)
	}

	return data, nil
}

// findCover returns the index of the cover stream of absolutePath; input is the result of
// inputArguments for it.
func (e *CoverExtractor) findCover(ctx context.Context, absolutePath string, input []string) (int, error) {
	arguments := append([]string{
		"-v", "error", "-hide_banner", "-print_format", "json", "-show_streams", "-select_streams", videoStreams,
	}, input...)

	output, err := e.ffprobe.output(ctx, arguments)
	if err != nil {
		e.logRunFailure(ctx, findCoverOperation, absolutePath, err)

		return 0, classifyRunError(err)
	}

	return coverStreamIndex(output)
}

// copyCover writes the picture stream streamIndex of absolutePath to standard output as it
// is stored: copying keeps the original quality and needs no decoder.
func (e *CoverExtractor) copyCover(ctx context.Context, absolutePath string, streamIndex int) ([]byte, error) {
	input, err := ffmpegInputArguments(absolutePath)
	if err != nil {
		return nil, err
	}

	arguments := append([]string{"-hide_banner", "-nostdin", "-loglevel", "error"}, input...)
	arguments = append(arguments,
		"-map", streamMapping(streamIndex), "-c", "copy", "-frames:v", singleFrame,
		"-map_metadata", disabledMapping, "-f", imagePipeMuxer, standardOutput,
	)

	data, err := e.ffmpeg.boundedOutput(ctx, arguments, e.maxBytes)
	if err != nil {
		e.logRunFailure(ctx, copyCoverOperation, absolutePath, err)

		return nil, classifyRunError(err)
	}

	if err := checkPicture(data); err != nil {
		return nil, err
	}

	return data, nil
}

// coverStreamIndex picks the cover from ffprobe's -show_streams JSON: among the attached
// pictures in a format of coverCodecs the front cover, else the first one. Errors wrap
// ErrNoCover or, for unreadable output, domain.ErrUnsupported.
func coverStreamIndex(output []byte) (int, error) {
	var parsed probeOutput
	if err := json.Unmarshal(output, &parsed); err != nil {
		return 0, fmt.Errorf("decode ffprobe output: %w: %w", domain.ErrUnsupported, err)
	}

	pictures := slices.DeleteFunc(parsed.Streams, func(stream probeStream) bool {
		return stream.Disposition["attached_pic"] == 0 || !slices.Contains(coverCodecs(), stream.CodecName)
	})
	if len(pictures) == 0 {
		return 0, ErrNoCover
	}

	front := slices.IndexFunc(pictures, func(stream probeStream) bool {
		return strings.EqualFold(tag(stream.Tags, "comment"), frontCoverComment)
	})

	return pictures[max(front, 0)].Index, nil
}

// checkPicture verifies that data starts like a JPEG or PNG file, which also proves that
// the copy produced output at all. Errors wrap domain.ErrUnsupported.
func checkPicture(data []byte) error {
	for _, signature := range pictureSignatures() {
		if bytes.HasPrefix(data, signature) {
			return nil
		}
	}

	return fmt.Errorf("embedded picture is no JPEG or PNG: %w", domain.ErrUnsupported)
}

// logRunFailure writes a tool's error output of a failed run to the debug log, like
// Prober.logRunFailure; the tail never reaches the returned error.
func (e *CoverExtractor) logRunFailure(ctx context.Context, operation, path string, err error) {
	attributes := exitAttributes(err)
	if attributes == nil {
		return
	}

	attributes = append([]slog.Attr{
		slog.String("operation", operation), slog.String("file", filepath.Base(path)),
	}, attributes...)
	e.logger.LogAttrs(ctx, slog.LevelDebug, "FFmpeg failed on an audio file", attributes...)
}
