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
	// gridInputLabel names the first stream group of the input, the tile grid of a HEIF or
	// AVIF photo, which FFmpeg assembles into one picture; gridOutputLabel the scaled result.
	gridInputLabel  = "[0:g:0]"
	gridOutputLabel = "[photo]"
	// maxGridAllocationBytes bounds every single allocation of a HEIF or AVIF render
	// (-max_alloc), the tile grid and the single image run alike. -max_pixels only bounds
	// each decoded tile, not the canvas the tiles are assembled on, whose size the file
	// declares (up to 65535 x 65535); this bound admits one 16-bit plane of a canvas at
	// maxDecodedPixels and refuses a forged larger one. Grid tiles are separate small items;
	// a single coded image above the bound fails over to the embedded preview.
	maxGridAllocationBytes = 2 * maxDecodedPixels
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
	// inputFileWithContainerRotation is a HEIF or AVIF file with a single coded image:
	// FFmpeg applies the rotation and mirroring properties (irot, imir) of the container
	// itself.
	inputFileWithContainerRotation
	// inputTileGrid is a HEIF or AVIF file whose image is a grid of tiles (every iPhone
	// photo): FFmpeg exposes it as a stream group and assembles it in a complex filter
	// graph, which a simple -vf on the same output cannot follow.
	inputTileGrid
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
	// The single image run of a HEIF or AVIF file is bounded too: FFmpeg may pick a tile grid
	// as its default stream and assemble the forged canvas there as well.
	if job.input == inputTileGrid || job.input == inputFileWithContainerRotation {
		arguments = append(arguments, "-max_alloc", strconv.Itoa(maxGridAllocationBytes))
	}

	arguments = slices.Concat(arguments, input, photoFilterArguments(job),
		[]string{"-frames:v", singleFrame, "-map_metadata", disabledMapping},
		imageEncoding(job.format), []string{"-f", imagePipeMuxer, standardOutput})

	return arguments, nil
}

// photoFilterArguments selects the picture and scales it. A tile grid is scaled in the
// complex graph that assembles it; a single HEIF or AVIF image is left to FFmpeg's own
// stream choice, every other input maps its first video stream.
func photoFilterArguments(job photoJob) []string {
	switch job.input {
	case inputTileGrid:
		return []string{
			"-filter_complex", gridInputLabel + photoFilter(job) + gridOutputLabel, "-map", gridOutputLabel,
		}
	case inputFileWithContainerRotation:
		return []string{"-vf", photoFilter(job)}
	case inputFileWithEXIF, inputPreview:
	}

	return []string{"-map", firstVideoStream, "-vf", photoFilter(job)}
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
	if job.input == inputFileWithContainerRotation || job.input == inputTileGrid {
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
