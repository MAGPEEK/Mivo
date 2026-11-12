package ffmpeg

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/magpeek/mivo/internal/domain"
)

const (
	// subtitleTimeout bounds one extraction. FFmpeg reads the whole container to collect the
	// cues of one stream, which takes seconds for a 2 h film; the rest covers a NAS disk
	// that has to spin up first.
	subtitleTimeout = 30 * time.Second
	// maxSubtitleSourceBytes bounds an external subtitle file: real ones stay below a few
	// MiB, more is no subtitle but a hostile or mislabelled file.
	maxSubtitleSourceBytes = 8 << 20
	// subtitleOperation names the extractor's ffmpeg runs in debug logs.
	subtitleOperation = "extract subtitle"
	// subtitleDirectoryMode is the permission of the written WebVTT file and its directory.
	subtitleDirectoryMode = 0o750
)

// SubtitleExtractorOptions configures NewSubtitleExtractor; every field is required.
type SubtitleExtractorOptions struct {
	// FFmpegPath is the ffmpeg executable, the configured tools.ffmpeg value resolved by the
	// caller.
	FFmpegPath string
	// Logger receives one debug record with ffmpeg's error output per failed run.
	Logger *slog.Logger
}

// SubtitleTrack names the subtitle to extract.
type SubtitleTrack struct {
	// External marks a subtitle file next to the video. Then ExtractWebVTT's source is that
	// file, and Ordinal and Codec are ignored.
	External bool
	// Ordinal counts subtitle streams of the media file only, from 0 (ffmpeg's 0:s:<n>).
	Ordinal int
	// Codec is the stream's codec name from the probe; bitmap codecs are refused.
	Codec string
}

// SubtitleExtractor converts text subtitles to WebVTT with ffmpeg (ADR 0030 §3). It neither
// caches nor limits concurrency; the subtitle use case does both.
type SubtitleExtractor struct {
	ffmpeg  tool
	logger  *slog.Logger
	timeout time.Duration
}

// NewSubtitleExtractor returns an extractor that runs the ffmpeg executable of options.
// Errors wrap domain.ErrInvalid for a missing option.
func NewSubtitleExtractor(options SubtitleExtractorOptions) (*SubtitleExtractor, error) {
	switch {
	case options.FFmpegPath == "":
		return nil, fmt.Errorf("new subtitle extractor: ffmpeg path is empty: %w", domain.ErrInvalid)
	case options.Logger == nil:
		return nil, fmt.Errorf("new subtitle extractor: logger is nil: %w", domain.ErrInvalid)
	}

	return &SubtitleExtractor{
		ffmpeg:  tool{name: ffmpegToolName, executable: options.FFmpegPath},
		logger:  options.Logger,
		timeout: subtitleTimeout,
	}, nil
}

// ExtractWebVTT converts one text subtitle to UTF-8 WebVTT and writes it to destination,
// atomically: the file appears complete or not at all. Cue times stay relative to the
// media file. source and destination are absolute; the caller has confined source to its
// library root and chose destination (the adapter knows no cache layout). Errors wrap
//   - domain.ErrInvalid for a bitmap subtitle, a malformed request, a relative or a Windows
//     device path,
//   - domain.ErrNotFound when source does not exist,
//   - domain.ErrUnsupported for an unknown subtitle format, an external file over 8 MiB,
//     input ffmpeg cannot convert or output that is no WebVTT,
//   - domain.ErrUnavailable when ffmpeg cannot be started,
//   - context.DeadlineExceeded after 30 s, context.Canceled on cancellation (ffmpeg is
//     killed then).
//
// Messages name the file by its base name only; ffmpeg's error output goes to the debug log.
func (e *SubtitleExtractor) ExtractWebVTT(
	ctx context.Context, source string, track SubtitleTrack, destination string,
) error {
	name := filepath.Base(source)

	if !filepath.IsAbs(destination) {
		return fmt.Errorf("extract %s: destination %q is not absolute: %w", name, destination, domain.ErrInvalid)
	}

	runContext, cancel := context.WithTimeout(ctx, e.timeout)
	defer cancel()

	webVTT, err := e.convert(runContext, source, track)
	if err != nil {
		return fmt.Errorf("extract %s: %w", name, err)
	}

	if err := writeFileAtomically(destination, webVTT); err != nil {
		return fmt.Errorf("extract %s: %w", name, err)
	}

	return nil
}

// convert runs ffmpeg for track and returns the WebVTT it printed.
func (e *SubtitleExtractor) convert(ctx context.Context, source string, track SubtitleTrack) ([]byte, error) {
	var (
		arguments []string
		input     []byte
		err       error
	)

	if track.External {
		arguments, input, err = externalSubtitleJob(source)
	} else {
		arguments, err = embeddedSubtitleJob(source, track)
	}

	if err != nil {
		return nil, err
	}

	output, err := e.ffmpeg.pipedOutput(ctx, arguments, input, maxOutputBytes)
	if err != nil {
		logImageRunFailure(ctx, e.logger, imageRun{operation: subtitleOperation, name: filepath.Base(source)}, err)

		return nil, classifyRunError(err)
	}

	if !bytes.HasPrefix(output, []byte(webVTTSignature)) {
		return nil, fmt.Errorf("ffmpeg output is no WebVTT: %w", domain.ErrUnsupported)
	}

	return output, nil
}

// embeddedSubtitleJob builds the arguments for a stream of the media file at path.
func embeddedSubtitleJob(path string, track SubtitleTrack) ([]string, error) {
	arguments, err := embeddedSubtitleArguments(path, track.Ordinal, track.Codec)
	if err != nil {
		return nil, err
	}

	if _, err := os.Stat(path); errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("media file: %w", domain.ErrNotFound)
	}

	return arguments, nil
}

// externalSubtitleJob reads the subtitle file at path and builds the arguments that convert
// it from standard input.
func externalSubtitleJob(path string) ([]string, []byte, error) {
	if !filepath.IsAbs(path) {
		return nil, nil, fmt.Errorf("path %q is not absolute: %w", path, domain.ErrInvalid)
	}

	if isWindowsDevicePath(path) {
		return nil, nil, fmt.Errorf("path in the Windows device namespace: %w", domain.ErrInvalid)
	}

	format, err := externalSubtitleFormat(path)
	if err != nil {
		return nil, nil, err
	}

	data, err := readBounded(path, maxSubtitleSourceBytes)
	if err != nil {
		return nil, nil, err
	}

	return externalSubtitleArguments(format, subtitleCharset(data)), data, nil
}

// readBounded reads a regular file of at most limit bytes through an os.Root on its
// directory, so a symbolic link in the file name cannot lead out of it.
func readBounded(path string, limit int64) ([]byte, error) {
	root, err := os.OpenRoot(filepath.Dir(path))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("subtitle file: %w", domain.ErrNotFound)
	}

	if err != nil {
		return nil, fmt.Errorf("open subtitle directory: %w", unwrapPathError(err))
	}

	defer func() { _ = root.Close() }()

	file, err := root.Open(filepath.Base(path))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("subtitle file: %w", domain.ErrNotFound)
	}

	if err != nil {
		return nil, fmt.Errorf("open subtitle file: %w: %w", errSourceUnreadable, unwrapPathError(err))
	}

	defer func() { _ = file.Close() }()

	// One byte more than the limit tells an oversized file from one of exactly limit bytes.
	data, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil {
		return nil, fmt.Errorf("read subtitle file: %w: %w", errSourceUnreadable, unwrapPathError(err))
	}

	if int64(len(data)) > limit {
		return nil, fmt.Errorf("subtitle file exceeds %d bytes: %w", limit, domain.ErrUnsupported)
	}

	return data, nil
}

// writeFileAtomically writes data to path through a temporary file in the same directory and
// a rename, so a reader never sees a partial file and a crash leaves no half-written one
// under the final name.
func writeFileAtomically(path string, data []byte) error {
	directory := filepath.Dir(path)

	if err := os.MkdirAll(directory, subtitleDirectoryMode); err != nil {
		return fmt.Errorf("create %s: %w", filepath.Base(directory), unwrapPathError(err))
	}

	temporary, err := os.CreateTemp(directory, filepath.Base(path)+".*.tmp")
	if err != nil {
		return fmt.Errorf("create temporary file: %w", unwrapPathError(err))
	}

	_, writeErr := temporary.Write(data)
	closeErr := temporary.Close()

	if err := errors.Join(writeErr, closeErr); err != nil {
		_ = os.Remove(temporary.Name())

		return fmt.Errorf("write %s: %w", filepath.Base(path), unwrapPathError(err))
	}

	if err := os.Rename(temporary.Name(), path); err != nil {
		_ = os.Remove(temporary.Name())

		return fmt.Errorf("publish %s: %w", filepath.Base(path), unwrapPathError(err))
	}

	return nil
}
