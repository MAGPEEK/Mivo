package media_test

import (
	"testing"

	"github.com/magpeek/mivo/internal/domain/media"
)

func TestBitDepthPrefersTheReportedValue(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name             string
		bitsPerRawSample int
		pixelFormat      string
		want             int
	}{
		{name: "reported 10 bit", bitsPerRawSample: 10, pixelFormat: "yuv420p10le", want: 10},
		{name: "reported wins over pixel format", bitsPerRawSample: 12, pixelFormat: "yuv420p", want: 12},
		{name: "reported without pixel format", bitsPerRawSample: 8, pixelFormat: "", want: 8},
		{name: "missing falls back", bitsPerRawSample: 0, pixelFormat: "yuv420p10le", want: 10},
		{name: "negative falls back", bitsPerRawSample: -1, pixelFormat: "yuv420p", want: 8},
		{name: "nothing known", bitsPerRawSample: 0, pixelFormat: "", want: 0},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			got := media.BitDepth(test.bitsPerRawSample, test.pixelFormat)
			if got != test.want {
				t.Errorf("BitDepth(%d, %q) = %d, want %d",
					test.bitsPerRawSample, test.pixelFormat, got, test.want)
			}
		})
	}
}

func TestBitDepthFromPixelFormat(t *testing.T) {
	t.Parallel()

	tests := []struct {
		pixelFormat string
		want        int
	}{
		// Planar YUV and RGB: the number after "p" is the depth.
		{pixelFormat: "yuv420p", want: 8},
		{pixelFormat: "yuvj420p", want: 8},
		{pixelFormat: "yuv444p", want: 8},
		{pixelFormat: "yuv420p9le", want: 9},
		{pixelFormat: "yuv420p10le", want: 10},
		{pixelFormat: "yuv420p10be", want: 10},
		{pixelFormat: "yuv422p10le", want: 10},
		{pixelFormat: "yuv420p12le", want: 12},
		{pixelFormat: "yuv444p16le", want: 16},
		{pixelFormat: "yuva420p10le", want: 10},
		{pixelFormat: "gbrp10le", want: 10},
		{pixelFormat: "gbrap12le", want: 12},
		{pixelFormat: "YUV420P10LE", want: 10},
		// Grey formats.
		{pixelFormat: "gray", want: 8},
		{pixelFormat: "gray10le", want: 10},
		{pixelFormat: "gray12be", want: 12},
		{pixelFormat: "ya8", want: 8},
		{pixelFormat: "ya16le", want: 16},
		// Semi-planar and packed formats: the digits encode subsampling and depth.
		{pixelFormat: "nv12", want: 8},
		{pixelFormat: "nv21", want: 8},
		{pixelFormat: "p010le", want: 10},
		{pixelFormat: "p016le", want: 16},
		{pixelFormat: "p210le", want: 10},
		{pixelFormat: "p412le", want: 12},
		{pixelFormat: "y210le", want: 10},
		{pixelFormat: "y212le", want: 12},
		{pixelFormat: "xv30le", want: 10},
		{pixelFormat: "xv36le", want: 12},
		{pixelFormat: "x2rgb10le", want: 10},
		{pixelFormat: "rgb48le", want: 16},
		{pixelFormat: "rgba64be", want: 16},
		{pixelFormat: "rgb24", want: 8},
		{pixelFormat: "bgr0", want: 8},
		{pixelFormat: "rgba", want: 8},
		{pixelFormat: "pal8", want: 8},
		{pixelFormat: "yuyv422", want: 8},
		// Depth digits out of range fall back to the default.
		{pixelFormat: "gray0", want: 8},
		{pixelFormat: "yuv420p99999999999999999999", want: 8},
		// Unknown input.
		{pixelFormat: "", want: 0},
		{pixelFormat: "le", want: 0},
		{pixelFormat: "  ", want: 0},
	}

	for _, test := range tests {
		t.Run(test.pixelFormat, func(t *testing.T) {
			t.Parallel()

			if got := media.BitDepth(0, test.pixelFormat); got != test.want {
				t.Errorf("BitDepth(0, %q) = %d, want %d", test.pixelFormat, got, test.want)
			}
		})
	}
}
