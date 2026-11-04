package ffmpeg

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/artwork"
)

const (
	// maxDecodedPixels bounds the picture size the decoder accepts (-max_pixels), so a
	// hostile image header cannot make FFmpeg allocate gigabytes: 64 MP covers 8K artwork.
	maxDecodedPixels = 64_000_000
	// jpegQuality is the mjpeg -q:v value: FFmpeg's scale runs from 2 (best) to 31; 3 keeps
	// posters free of visible artefacts at about a third of the size of 2.
	jpegQuality = 3
	// scaleFlags selects the resampling filter; lanczos keeps downscaled text sharp.
	scaleFlags = "lanczos"
	// jpegPixelFormat is the full-range 4:2:0 format every JPEG decoder reads. Naming it
	// keeps FFmpeg from negotiating limited-range YUV, which mjpeg refuses as non-standard.
	jpegPixelFormat = "yuvj420p"
	// imagePipeMuxer writes the encoded picture as is, without a container.
	imagePipeMuxer = "image2pipe"
	// standardOutput is FFmpeg's URL for its standard output.
	standardOutput = "pipe:1"
	// firstVideoStream selects the picture of an image file.
	firstVideoStream = "0:v:0"
	// singleFrame limits the output to one picture (animated GIF and WebP have many).
	singleFrame = "1"
)

// imageArguments builds the ffmpeg arguments that render request to standard output. Errors
// wrap domain.ErrInvalid for a malformed request, a relative path or a Windows device path,
// and domain.ErrUnsupported for an image file whose path contains "%": without image2 in the
// whitelist (ADR 0022) FFmpeg could only guess the demuxer of such a file.
func imageArguments(request artwork.RenderRequest) ([]string, error) {
	if err := validateRenderRequest(request); err != nil {
		return nil, err
	}

	if request.StreamIndex == artwork.WholeFile && strings.Contains(request.AbsolutePath, imageSequenceMarker) {
		return nil, fmt.Errorf("image path contains %q: %w", imageSequenceMarker, domain.ErrUnsupported)
	}

	input, err := ffmpegInputArguments(request.AbsolutePath)
	if err != nil {
		return nil, err
	}

	arguments := []string{
		"-hide_banner", "-nostdin", "-loglevel", "error",
		"-max_pixels", strconv.Itoa(maxDecodedPixels),
	}
	arguments = append(arguments, input...)
	arguments = append(arguments,
		"-map", streamMapping(request.StreamIndex), "-frames:v", singleFrame,
		"-map_metadata", disabledMapping,
		"-vf", imageFilter(request.MaxWidth, request.Format),
	)
	arguments = append(arguments, imageEncoding(request.Format)...)

	return slices.Concat(arguments, []string{"-f", imagePipeMuxer, standardOutput}), nil
}

// validateRenderRequest checks the parts of request the arguments are built from.
func validateRenderRequest(request artwork.RenderRequest) error {
	switch {
	case request.MaxWidth <= 0:
		return fmt.Errorf("image width %d is not positive: %w", request.MaxWidth, domain.ErrInvalid)
	case request.StreamIndex < artwork.WholeFile:
		return fmt.Errorf("stream index %d is negative: %w", request.StreamIndex, domain.ErrInvalid)
	case request.Format != artwork.FormatJPEG && request.Format != artwork.FormatPNG:
		return fmt.Errorf("image format %q is unknown: %w", request.Format, domain.ErrInvalid)
	}

	return nil
}

// streamMapping selects the attached-picture stream of a media file or the picture of an
// image file.
func streamMapping(streamIndex int) string {
	if streamIndex == artwork.WholeFile {
		return firstVideoStream
	}

	return "0:" + strconv.Itoa(streamIndex)
}

// imageFilter scales to at most maxWidth, never up, keeping the aspect ratio with an even
// height. The quotes are filtergraph syntax that protects the comma of min(); no shell sees
// them. PNG keeps whatever pixel format the encoder can take, alpha included.
func imageFilter(maxWidth artwork.Width, format artwork.Format) string {
	filter := "scale='min(iw," + maxWidth.String() + ")':-2:flags=" + scaleFlags
	if format == artwork.FormatJPEG {
		filter += ",format=" + jpegPixelFormat
	}

	return filter
}

// imageEncoding returns the encoder arguments of format.
func imageEncoding(format artwork.Format) []string {
	if format == artwork.FormatPNG {
		return []string{"-c:v", "png"}
	}

	return []string{"-c:v", "mjpeg", "-q:v", strconv.Itoa(jpegQuality)}
}
