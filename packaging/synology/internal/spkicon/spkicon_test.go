package spkicon_test

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"image/png"
	"testing"

	"github.com/magpeek/mivo/packaging/synology/internal/spkicon"
)

// The expected colours are spelled out instead of taken from the package, so a drift from
// ADR 0011 (web/packages/ui-tokens/src/colors.ts) fails the test.
func coral() color.NRGBA        { return color.NRGBA{R: 0xFF, G: 0x6B, B: 0x57, A: 0xFF} }
func textOnAccent() color.NRGBA { return color.NRGBA{R: 0x3A, G: 0x0E, B: 0x07, A: 0xFF} }

func TestPackageIconsUseTheGuideNamesAndSizes(t *testing.T) {
	t.Parallel()

	want := []spkicon.Icon{
		{FileName: "PACKAGE_ICON.PNG", Size: 64},
		{FileName: "PACKAGE_ICON_256.PNG", Size: 256},
	}

	got := spkicon.PackageIcons()
	if len(got) != len(want) {
		t.Fatalf("PackageIcons() = %v, want %v", got, want)
	}

	for index := range want {
		if got[index] != want[index] {
			t.Errorf("PackageIcons()[%d] = %v, want %v", index, got[index], want[index])
		}
	}
}

func TestEncodePNGDrawsTheIconAtTheRequestedSize(t *testing.T) {
	t.Parallel()

	sizes := []int{
		spkicon.MinSize, spkicon.PackageIconSize, 100, spkicon.PackageIcon256Size, spkicon.MaxSize,
	}

	for _, size := range sizes {
		t.Run(sizeName(size), func(t *testing.T) {
			t.Parallel()

			icon := decode(t, size)
			if bounds := icon.Bounds(); bounds != image.Rect(0, 0, size, size) {
				t.Fatalf("bounds = %v, want %d x %d", bounds, size, size)
			}

			last := size - 1
			for _, corner := range []image.Point{{0, 0}, {last, 0}, {0, last}, {last, last}} {
				assertColour(t, icon, corner, color.NRGBA{})
			}

			assertColour(t, icon, image.Pt(size/2, size/2), textOnAccent())
			// Left of the triangle's flat side, inside the rounded square.
			assertColour(t, icon, image.Pt(size/5, size/2), coral())
			// Right of the triangle's tip.
			assertColour(t, icon, image.Pt(size*4/5, size/2), coral())
			// Above the triangle.
			assertColour(t, icon, image.Pt(size/2, size/6), coral())
		})
	}
}

func TestEncodePNGIsByteIdentical(t *testing.T) {
	t.Parallel()

	for _, size := range []int{spkicon.PackageIconSize, spkicon.PackageIcon256Size} {
		first := encode(t, size)
		second := encode(t, size)

		if !bytes.Equal(first, second) {
			t.Errorf("two renders of %d px differ", size)
		}
	}
}

func TestRenderIsMirroredAcrossTheHorizontalCentre(t *testing.T) {
	t.Parallel()

	icon := render(t, spkicon.PackageIcon256Size)
	size := spkicon.PackageIcon256Size

	for y := range size / 2 {
		for x := range size {
			top, bottom := icon.NRGBAAt(x, y), icon.NRGBAAt(x, size-1-y)
			if top != bottom {
				t.Fatalf("pixel (%d,%d) = %v, mirrored pixel = %v", x, y, top, bottom)
			}
		}
	}
}

func TestRenderAntiAliasesEdges(t *testing.T) {
	t.Parallel()

	icon := render(t, spkicon.PackageIconSize)

	var softEdges, blendedFill int

	for y := range spkicon.PackageIconSize {
		for x := range spkicon.PackageIconSize {
			pixel := icon.NRGBAAt(x, y)

			switch {
			case pixel.A > 0 && pixel.A < 0xFF:
				softEdges++
			case pixel.A == 0xFF && pixel != coral() && pixel != textOnAccent():
				blendedFill++
			}
		}
	}

	if softEdges == 0 {
		t.Error("no partly transparent pixel on the rounded corners")
	}

	if blendedFill == 0 {
		t.Error("no blended pixel on the triangle's edges")
	}
}

func TestRenderRejectsSizesOutsideTheRange(t *testing.T) {
	t.Parallel()

	for _, size := range []int{-1, 0, spkicon.MinSize - 1, spkicon.MaxSize + 1} {
		if _, err := spkicon.Render(size); !errors.Is(err, spkicon.ErrSizeOutOfRange) {
			t.Errorf("Render(%d) error = %v, want ErrSizeOutOfRange", size, err)
		}

		if _, err := spkicon.EncodePNG(size); !errors.Is(err, spkicon.ErrSizeOutOfRange) {
			t.Errorf("EncodePNG(%d) error = %v, want ErrSizeOutOfRange", size, err)
		}
	}
}

func render(t *testing.T, size int) *image.NRGBA {
	t.Helper()

	icon, err := spkicon.Render(size)
	if err != nil {
		t.Fatalf("Render(%d): %v", size, err)
	}

	return icon
}

func encode(t *testing.T, size int) []byte {
	t.Helper()

	data, err := spkicon.EncodePNG(size)
	if err != nil {
		t.Fatalf("EncodePNG(%d): %v", size, err)
	}

	return data
}

func decode(t *testing.T, size int) image.Image {
	t.Helper()

	icon, err := png.Decode(bytes.NewReader(encode(t, size)))
	if err != nil {
		t.Fatalf("decode %d px icon: %v", size, err)
	}

	return icon
}

func assertColour(t *testing.T, icon image.Image, point image.Point, want color.NRGBA) {
	t.Helper()

	got, ok := color.NRGBAModel.Convert(icon.At(point.X, point.Y)).(color.NRGBA)
	if !ok {
		t.Fatalf("pixel %v has no NRGBA form", point)
	}

	if got != want {
		t.Errorf("pixel %v = %v, want %v", point, got, want)
	}
}

func sizeName(size int) string {
	return image.Pt(size, size).String()
}
