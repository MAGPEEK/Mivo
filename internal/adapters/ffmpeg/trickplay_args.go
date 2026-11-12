package ffmpeg

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/magpeek/mivo/internal/domain"
)

const (
	// trickplayInterval is the nominal time between two thumbnails (ADR 0030 §8).
	trickplayInterval = 10 * time.Second
	// trickplayTileWidth is the pixel width of one thumbnail; the height follows the aspect.
	trickplayTileWidth = 160
	// trickplayColumns and trickplayRows are the thumbnails per sheet: 100 per JPEG keeps a
	// sheet small enough to fetch on a slow link while a feature film needs only a few.
	trickplayColumns = 10
	trickplayRows    = 10
	// trickplayQuality is the mjpeg -q:v value (2 best, 31 worst); thumbnails tolerate 5.
	trickplayQuality = 5
	// trickplayMaxSheets bounds the output: 200 sheets cover about 55 hours at the nominal
	// interval, so only a hostile or broken file reaches it.
	trickplayMaxSheets = 200
	// trickplaySheetPattern names the sheets; the number starts at trickplayFirstSheet.
	trickplaySheetPattern = "sheet-%03d.jpg"
	trickplayFirstSheet   = 1
	// trickplaySheetGlob lists the sheets the pattern produces.
	trickplaySheetGlob = "sheet-*.jpg"
	// keyframesOnly makes the decoder skip every frame that is no keyframe, which turns the
	// pass from decoding the whole film into decoding about one frame per GOP.
	keyframesOnly = "nokey"
)

// trickplayJob is what the sheet arguments are built from.
type trickplayJob struct {
	// SourcePath is the absolute media file.
	SourcePath string
	// VideoStream is the ffprobe index of the video stream to sample.
	VideoStream int
	// OutputDirectory is the absolute directory that receives the sheets.
	OutputDirectory string
}

// trickplayArguments builds the ffmpeg arguments that write the sprite sheets of job. Errors
// wrap domain.ErrInvalid for a negative stream index, a relative path, a Windows device path
// or an output directory with "%", which FFmpeg would read as part of the name template.
func trickplayArguments(job trickplayJob) ([]string, error) {
	if job.VideoStream < 0 {
		return nil, fmt.Errorf("video stream %d is negative: %w", job.VideoStream, domain.ErrInvalid)
	}

	input, err := ffmpegInputArguments(job.SourcePath)
	if err != nil {
		return nil, err
	}

	output, err := trickplayOutput(job.OutputDirectory)
	if err != nil {
		return nil, err
	}

	arguments := []string{"-hide_banner", "-nostdin", "-loglevel", "error", "-y", "-skip_frame", keyframesOnly}
	arguments = append(arguments, input...)
	arguments = append(arguments,
		"-map", "0:"+strconv.Itoa(job.VideoStream), "-an", "-sn", "-dn", "-map_metadata", disabledMapping,
		"-vf", trickplayFilter(),
		"-frames:v", strconv.Itoa(trickplayMaxSheets),
		"-c:v", "mjpeg", "-q:v", strconv.Itoa(trickplayQuality),
		"-start_number", strconv.Itoa(trickplayFirstSheet),
		"-f", "image2", output,
	)

	return arguments, nil
}

// trickplayFilter samples one frame per interval, scales it and tiles the frames into
// sheets. eof_action=pass hands the last frame on at the end of the input; without it a clip
// shorter than one interval (one keyframe) rounds to zero frames and yields no sheet. The
// pixel format is named because mjpeg refuses the limited-range YUV that most video uses.
func trickplayFilter() string {
	return fmt.Sprintf("fps=1/%d:eof_action=pass,scale=%d:-2,tile=%dx%d,format=%s",
		int(trickplayInterval.Seconds()), trickplayTileWidth, trickplayColumns, trickplayRows, jpegPixelFormat)
}

// trickplayOutput returns the sheet name template inside directory.
func trickplayOutput(directory string) (string, error) {
	if !filepath.IsAbs(directory) {
		return "", fmt.Errorf("output directory %q is not absolute: %w", directory, domain.ErrInvalid)
	}

	if isWindowsDevicePath(directory) {
		return "", fmt.Errorf("output directory in the Windows device namespace: %w", domain.ErrInvalid)
	}

	if strings.Contains(directory, templateMarker) {
		return "", fmt.Errorf("output directory contains %q, which FFmpeg reads as a name template: %w",
			templateMarker, domain.ErrInvalid)
	}

	base := strings.TrimSuffix(filepath.ToSlash(filepath.Clean(directory)), outputSeparator)

	return fileProtocol + base + outputSeparator + trickplaySheetPattern, nil
}
