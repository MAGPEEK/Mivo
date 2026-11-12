package ffmpeg

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/media"
)

const (
	// trickplayTimeout bounds one generation; a film that needs longer is not worth the
	// NAS load (M4-12).
	trickplayTimeout = 30 * time.Minute
	// trickplayOperation names the generator's ffmpeg runs in debug logs.
	trickplayOperation = "generate trickplay"
	// trickplayTempPrefix starts the staging directory inside the destination.
	trickplayTempPrefix = ".trickplay-"
	// trickplayDirectoryMode is the permission of the destination directory.
	trickplayDirectoryMode = 0o750
	// evenPixels is the multiple the scale filter rounds a height to (-2).
	evenPixels = 2
)

// Manifest describes the sprite sheets of one video, enough for a player to find the
// thumbnail of a position: sheet = position / (Columns*Rows*Interval), then row-major.
type Manifest struct {
	// TileWidth and TileHeight are the pixel size of one thumbnail.
	TileWidth, TileHeight int
	// Columns and Rows are the thumbnails per sheet.
	Columns, Rows int
	// Interval is the nominal time between two thumbnails. With keyframes sparser than the
	// interval neighbouring tiles repeat; a player only needs the nearest tile.
	Interval time.Duration
	// SheetCount is the number of sheet-NNN.jpg files, numbered from 1.
	SheetCount int
}

// TrickplayOptions are the per-call settings of GenerateTrickplay; the zero value is right.
type TrickplayOptions struct {
	// Timeout overrides the 30 minute bound when positive.
	Timeout time.Duration
}

// TrickplayProber probes the source before generation: it decides on audio-only input and
// gives the video's aspect ratio for the manifest.
type TrickplayProber interface {
	Probe(ctx context.Context, absolutePath string) (media.Description, error)
}

// TrickplayGeneratorOptions configures NewTrickplayGenerator; every field is required.
type TrickplayGeneratorOptions struct {
	// FFmpegPath is the ffmpeg executable, the configured tools.ffmpeg value.
	FFmpegPath string
	// Prober probes the source file.
	Prober TrickplayProber
	// Logger receives one debug record with ffmpeg's error output per failed run.
	Logger *slog.Logger
}

// TrickplayGenerator writes the seek-bar preview sheets of a video with one ffmpeg pass.
type TrickplayGenerator struct {
	ffmpeg tool
	prober TrickplayProber
	logger *slog.Logger
}

// NewTrickplayGenerator returns a generator for options. Errors wrap domain.ErrInvalid for a
// missing option.
func NewTrickplayGenerator(options TrickplayGeneratorOptions) (*TrickplayGenerator, error) {
	switch {
	case options.FFmpegPath == "":
		return nil, fmt.Errorf("new trickplay generator: ffmpeg path is empty: %w", domain.ErrInvalid)
	case options.Prober == nil:
		return nil, fmt.Errorf("new trickplay generator: prober is nil: %w", domain.ErrInvalid)
	case options.Logger == nil:
		return nil, fmt.Errorf("new trickplay generator: logger is nil: %w", domain.ErrInvalid)
	}

	return &TrickplayGenerator{
		ffmpeg: tool{name: ffmpegToolName, executable: options.FFmpegPath},
		prober: options.Prober,
		logger: options.Logger,
	}, nil
}

// GenerateTrickplay writes sheet-001.jpg, sheet-002.jpg, … for the video of source into
// destinationDir, which is created when missing and must be absolute. The sheets are staged
// in a hidden directory inside destinationDir and renamed into place, so a reader never sees
// a half-written sheet and an error or cancellation leaves nothing behind. Sheets of an
// earlier run are replaced. Errors wrap
//   - domain.ErrInvalid for a source without video, a relative path or an unusable directory,
//   - domain.ErrUnsupported for input ffmpeg cannot read or that yields no frame,
//   - domain.ErrUnavailable when ffmpeg cannot be started,
//   - context.DeadlineExceeded after the timeout, context.Canceled on cancellation.
func (g *TrickplayGenerator) GenerateTrickplay(
	ctx context.Context, source, destinationDir string, options TrickplayOptions,
) (Manifest, error) {
	name := filepath.Base(source)

	description, err := g.prober.Probe(ctx, source)
	if err != nil {
		return Manifest{}, fmt.Errorf("trickplay %s: %w", name, err)
	}

	video, ok := description.PrimaryVideo()
	if !ok || video.Width <= 0 || video.Height <= 0 {
		return Manifest{}, fmt.Errorf("trickplay %s: file has no video stream: %w", name, domain.ErrInvalid)
	}

	staging, err := stageDirectory(destinationDir)
	if err != nil {
		return Manifest{}, fmt.Errorf("trickplay %s: %w", name, err)
	}
	// The staging directory is empty after a successful publish; on every error path it holds
	// the partial output, which must not stay.
	defer func() { _ = os.RemoveAll(staging) }()

	sheets, err := g.renderSheets(ctx, trickplayJob{
		SourcePath: source, VideoStream: video.Index, OutputDirectory: staging,
	}, options)
	if err != nil {
		return Manifest{}, fmt.Errorf("trickplay %s: %w", name, err)
	}

	if err := publishSheets(sheets, destinationDir); err != nil {
		return Manifest{}, fmt.Errorf("trickplay %s: %w", name, err)
	}

	return newManifest(video, len(sheets)), nil
}

// newManifest describes count sheets of a video; the height mirrors what scale=W:-2 picks.
func newManifest(video media.Stream, count int) Manifest {
	height := (trickplayTileWidth*video.Height + video.Width) / (video.Width * evenPixels) * evenPixels

	return Manifest{
		TileWidth: trickplayTileWidth, TileHeight: max(height, evenPixels),
		Columns: trickplayColumns, Rows: trickplayRows,
		Interval: trickplayInterval, SheetCount: count,
	}
}

// stageDirectory creates destinationDir when missing and a fresh staging directory in it.
func stageDirectory(destinationDir string) (string, error) {
	if !filepath.IsAbs(destinationDir) {
		return "", fmt.Errorf("destination %q is not absolute: %w", destinationDir, domain.ErrInvalid)
	}

	if err := os.MkdirAll(destinationDir, trickplayDirectoryMode); err != nil {
		return "", fmt.Errorf("create destination: %w: %w", domain.ErrInvalid, unwrapPathError(err))
	}

	staging, err := os.MkdirTemp(destinationDir, trickplayTempPrefix)
	if err != nil {
		return "", fmt.Errorf("create staging directory: %w: %w", domain.ErrInvalid, unwrapPathError(err))
	}

	return staging, nil
}

// renderSheets runs ffmpeg into the staging directory and returns the sheet files it wrote
// in name order.
func (g *TrickplayGenerator) renderSheets(
	ctx context.Context, job trickplayJob, options TrickplayOptions,
) ([]string, error) {
	arguments, err := trickplayArguments(job)
	if err != nil {
		return nil, err
	}

	timeout := trickplayTimeout
	if options.Timeout > 0 {
		timeout = options.Timeout
	}

	runContext, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// ffmpeg writes to the files, not to standard output; the bounded read only keeps a
	// misbehaving process from filling memory.
	if _, err := g.ffmpeg.output(runContext, arguments); err != nil {
		logImageRunFailure(ctx, g.logger,
			imageRun{operation: trickplayOperation, name: filepath.Base(job.SourcePath)}, err)

		return nil, classifyRunError(err)
	}

	sheets, err := filepath.Glob(filepath.Join(job.OutputDirectory, trickplaySheetGlob))
	if err != nil {
		return nil, fmt.Errorf("list sheets: %w", err)
	}

	if len(sheets) == 0 {
		return nil, fmt.Errorf("ffmpeg wrote no sheet: %w", domain.ErrUnsupported)
	}

	slices.Sort(sheets)

	return sheets, nil
}

// publishSheets moves the staged sheets into destinationDir, replacing earlier ones, and
// removes what it already moved when a rename fails.
func publishSheets(sheets []string, destinationDir string) error {
	if err := removeSheets(destinationDir); err != nil {
		return err
	}

	for _, sheet := range sheets {
		if err := os.Rename(sheet, filepath.Join(destinationDir, filepath.Base(sheet))); err != nil {
			cleanupErr := removeSheets(destinationDir)

			return errors.Join(fmt.Errorf("publish sheet: %w", unwrapPathError(err)), cleanupErr)
		}
	}

	return nil
}

// removeSheets deletes the sheet files in directory.
func removeSheets(directory string) error {
	existing, err := filepath.Glob(filepath.Join(directory, trickplaySheetGlob))
	if err != nil {
		return fmt.Errorf("list sheets: %w", err)
	}

	for _, sheet := range existing {
		if err := os.Remove(sheet); err != nil {
			return fmt.Errorf("remove old sheet: %w", unwrapPathError(err))
		}
	}

	return nil
}
