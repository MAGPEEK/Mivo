package media

import (
	"strconv"
	"strings"
)

// Bit depths that pixel format names encode only indirectly.
const (
	unknownBitDepth = 0
	// defaultBitDepth applies to every pixel format without a depth in its name (yuv420p,
	// nv12, rgb24, ...): FFmpeg names all formats above 8 bits per component explicitly.
	defaultBitDepth = 8
	tenBitDepth     = 10
	twelveBitDepth  = 12
	sixteenBitDepth = 16
)

// Semi-planar and packed 4:2:x formats such as p010, p210 and y210 consist of one letter, a
// subsampling digit and two depth digits.
const (
	subsampledFormatLength = 4
	subsampledDepthOffset  = 2
)

// BitDepth returns the bits per colour component of a video stream. ffprobe's
// bits_per_raw_sample wins when it is positive; it is often missing for video, so the depth
// falls back to the pixel format name (yuv420p10le → 10). 0 means unknown. Float formats
// (gbrpf32) are not recognised and count as 8 bits; no video codec Mivo plays uses them.
func BitDepth(bitsPerRawSample int, pixelFormat string) int {
	if bitsPerRawSample > 0 {
		return bitsPerRawSample
	}

	return pixelFormatBitDepth(pixelFormat)
}

func pixelFormatBitDepth(pixelFormat string) int {
	name := withoutEndianness(strings.ToLower(strings.TrimSpace(pixelFormat)))
	if name == "" {
		return unknownBitDepth
	}

	if depth, ok := irregularFormatBitDepth(name); ok {
		return depth
	}

	if depth, ok := subsampledFormatBitDepth(name); ok {
		return depth
	}

	if depth, ok := componentFormatBitDepth(name); ok {
		return depth
	}

	return defaultBitDepth
}

// withoutEndianness strips the le/be suffix FFmpeg appends to formats above 8 bits.
func withoutEndianness(name string) string {
	for _, suffix := range []string{"le", "be"} {
		if trimmed, found := strings.CutSuffix(name, suffix); found {
			return trimmed
		}
	}

	return name
}

// irregularFormatBitDepth covers packed formats whose digits count bits per pixel, not per
// component.
func irregularFormatBitDepth(name string) (int, bool) {
	switch name {
	case "xv30", "v30x", "x2rgb10", "x2bgr10":
		return tenBitDepth, true
	case "xv36":
		return twelveBitDepth, true
	case "xv48", "rgb48", "bgr48", "rgba64", "bgra64":
		return sixteenBitDepth, true
	default:
		return unknownBitDepth, false
	}
}

// subsampledFormatBitDepth reads the last two digits of p010, p216, y210 and similar names.
func subsampledFormatBitDepth(name string) (int, bool) {
	if len(name) != subsampledFormatLength || (name[0] != 'p' && name[0] != 'y') {
		return unknownBitDepth, false
	}

	if !isDigits(name[1:]) {
		return unknownBitDepth, false
	}

	return parseDepth(name[subsampledDepthOffset:])
}

// componentFormatBitDepth reads the trailing number of planar formats (yuv420p10, gbrp12,
// yuva444p16) and grey formats (gray10, ya16), where it is the depth per component. Other
// trailing numbers (nv12, rgb24, yuyv422) describe the layout and are ignored.
func componentFormatBitDepth(name string) (int, bool) {
	prefix := strings.TrimRight(name, "0123456789")
	digits := name[len(prefix):]

	if digits == "" {
		return unknownBitDepth, false
	}

	if !strings.HasSuffix(prefix, "p") && prefix != "gray" && prefix != "ya" {
		return unknownBitDepth, false
	}

	return parseDepth(digits)
}

func parseDepth(digits string) (int, bool) {
	depth, err := strconv.Atoi(digits)
	if err != nil || depth <= 0 {
		return unknownBitDepth, false
	}

	return depth, true
}

func isDigits(text string) bool {
	return text != "" && strings.Trim(text, "0123456789") == ""
}
