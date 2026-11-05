package ffmpeg

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/artwork"
	"github.com/magpeek/mivo/internal/domain/photo"
)

const (
	// noAutorotateOption keeps FFmpeg from applying rotation side data itself, so the EXIF
	// orientation is applied exactly once, by orientationFilter, whatever the FFmpeg version
	// does with JPEG EXIF data.
	noAutorotateOption = "-noautorotate"
	// previewDemuxer reads the embedded JPEG preview of a RAW or HEIF file from the pipe.
	previewDemuxer = "jpeg_pipe"
	// pipeProtocol and standardInput let FFmpeg read the preview from its standard input.
	pipeProtocol  = "pipe"
	standardInput = "pipe:0"
	// maxOrientationFilters is the longest filter chain an orientation needs (upside down).
	maxOrientationFilters = 2
)

// EXIF orientations (photo.Info.Orientation) that need a transform; 1 and 0 are upright.
const (
	orientationMirrored         = 2
	orientationUpsideDown       = 3
	orientationFlipped          = 4
	orientationTransposed       = 5
	orientationClockwise        = 6
	orientationTransverse       = 7
	orientationCounterClockwise = 8
)

// photoInput says where a photo render reads its picture from.
type photoInput int

const (
	// inputFileWithEXIF is an image file whose EXIF orientation Mivo applies.
	inputFileWithEXIF photoInput = iota
	// inputFileWithContainerRotation is a HEIF or AVIF file: FFmpeg applies the rotation
	// and mirroring properties (irot, imir) of the container itself.
	inputFileWithContainerRotation
	// inputPreview is an embedded JPEG preview piped to standard input.
	inputPreview
)

// photoJob describes one photo render. Path is only used for the file inputs; name is the
// base name of the photo file for the debug log.
type photoJob struct {
	input       photoInput
	path        string
	name        string
	width       artwork.Width
	format      artwork.Format
	orientation int
}

// photoArguments builds the ffmpeg arguments of job, keeping the -max_pixels guard and the
// input whitelists of the artwork renderer. Errors wrap domain.ErrInvalid for a malformed
// job or path and domain.ErrUnsupported for a file path with "%" (see imageArguments).
func photoArguments(job photoJob) ([]string, error) {
	request := artwork.RenderRequest{
		AbsolutePath: job.path, StreamIndex: artwork.WholeFile, MaxWidth: job.width, Format: job.format,
	}
	if err := validateRenderRequest(request); err != nil {
		return nil, err
	}

	input, err := photoInputArguments(job)
	if err != nil {
		return nil, err
	}

	arguments := []string{
		"-hide_banner", "-nostdin", "-loglevel", "error",
		"-max_pixels", strconv.Itoa(maxDecodedPixels),
	}
	arguments = append(arguments, input...)

	// A HEIF grid image is a stream group that FFmpeg assembles only when it picks the
	// video stream itself (the largest picture); mapping 0:v:0 would render one tile.
	if job.input != inputFileWithContainerRotation {
		arguments = append(arguments, "-map", firstVideoStream)
	}

	arguments = append(arguments,
		"-frames:v", singleFrame, "-map_metadata", disabledMapping,
		"-vf", photoFilter(job),
	)
	arguments = append(arguments, imageEncoding(job.format)...)

	return slices.Concat(arguments, []string{"-f", imagePipeMuxer, standardOutput}), nil
}

// photoInputArguments returns the input part of the arguments of job.
func photoInputArguments(job photoJob) ([]string, error) {
	if job.input == inputPreview {
		return []string{
			noAutorotateOption,
			formatWhitelistOption, previewDemuxer,
			protocolWhitelistOption, pipeProtocol,
			"-f", previewDemuxer, "-i", standardInput,
		}, nil
	}

	if strings.Contains(job.path, imageSequenceMarker) {
		return nil, fmt.Errorf("image path contains %q: %w", imageSequenceMarker, domain.ErrUnsupported)
	}

	input, err := ffmpegInputArguments(job.path)
	if err != nil {
		return nil, err
	}

	if job.input == inputFileWithEXIF {
		input = slices.Insert(input, 0, noAutorotateOption)
	}

	return input, nil
}

// photoFilter orients the picture before scaling, so the width limit applies to the width
// the viewer sees.
func photoFilter(job photoJob) string {
	scale := imageFilter(job.width, job.format)
	if job.input == inputFileWithContainerRotation {
		return scale
	}

	return strings.Join(append(orientationFilters(job.orientation), scale), ",")
}

// orientationFilters returns the FFmpeg filters that turn a picture stored with the EXIF
// orientation into an upright one. transpose=0 swaps the axes (rotate counter-clockwise and
// flip vertically), 1 rotates clockwise, 2 counter-clockwise, 3 swaps along the other
// diagonal. Unknown values leave the picture as stored.
func orientationFilters(orientation int) []string {
	filters := make([]string, 0, maxOrientationFilters)

	switch orientation {
	case orientationMirrored:
		filters = append(filters, "hflip")
	case orientationUpsideDown:
		filters = append(filters, "hflip", "vflip")
	case orientationFlipped:
		filters = append(filters, "vflip")
	case orientationTransposed:
		filters = append(filters, "transpose=0")
	case orientationClockwise:
		filters = append(filters, "transpose=1")
	case orientationTransverse:
		filters = append(filters, "transpose=3")
	case orientationCounterClockwise:
		filters = append(filters, "transpose=2")
	case photo.OrientationUnknown, photo.OrientationMin:
	}

	return filters
}
