package artwork

import (
	"fmt"
	"strconv"

	"github.com/magpeek/mivo/internal/domain"
)

// Width is one of the fixed image widths the image endpoint serves (UX LB-44). A fixed set
// keeps the cache small and lets every client share the same rendered files.
type Width int

// Allowed widths in pixels.
const (
	Width160  Width = 160
	Width320  Width = 320
	Width480  Width = 480
	Width640  Width = 640
	Width960  Width = 960
	Width1280 Width = 1280
	Width1920 Width = 1920
)

// Widths returns the allowed widths in ascending order.
func Widths() []Width {
	return []Width{Width160, Width320, Width480, Width640, Width960, Width1280, Width1920}
}

// ParseWidth accepts the decimal text of an allowed width in its canonical form ("320", not
// "0320" or "+320"), so one width has one cache entry. The error wraps domain.ErrInvalid.
func ParseWidth(text string) (Width, error) {
	for _, width := range Widths() {
		if width.String() == text {
			return width, nil
		}
	}

	return 0, fmt.Errorf("parse image width %q: not an allowed width: %w", text, domain.ErrInvalid)
}

// Pixels returns the width in pixels.
func (width Width) Pixels() int {
	return int(width)
}

// String returns the decimal text of the width.
func (width Width) String() string {
	return strconv.Itoa(int(width))
}

// Format is the encoding of a rendered picture.
type Format string

// Output formats. The text values appear in cache file names.
const (
	FormatJPEG Format = "jpeg"
	FormatPNG  Format = "png"
)

// OutputFormat returns the format an artwork kind is rendered in: PNG for logos to keep
// their transparency, JPEG for everything else (ADR 0027 §8).
func OutputFormat(kind Kind) Format {
	if kind == KindLogo {
		return FormatPNG
	}

	return FormatJPEG
}

// ContentType returns the media type of the format for HTTP responses.
func (format Format) ContentType() string {
	if format == FormatPNG {
		return "image/png"
	}

	return "image/jpeg"
}

// Extension returns the file extension of the format without a dot.
func (format Format) Extension() string {
	if format == FormatPNG {
		return "png"
	}

	return "jpg"
}

// String returns the text value of the format.
func (format Format) String() string {
	return string(format)
}
