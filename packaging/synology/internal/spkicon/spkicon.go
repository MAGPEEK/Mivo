// Package spkicon renders the Package Center icons of the Synology package at build time, so
// the repository holds no binary assets and the icons carry no product name (ADR 0026, task
// SPK-08).
//
// The icon is a placeholder until the project has a logo: a coral rounded square with a
// centred play triangle. When a logo exists, it replaces this generator.
package spkicon

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
)

// Pixel sizes of the Package Center icons. The DSM 7.2.2 Package Developer Guide ("Package
// Structure") asks for 64 x 64 for PACKAGE_ICON.PNG on DSM 7.0 or above and 256 x 256 for
// PACKAGE_ICON_256.PNG.
const (
	// PackageIconSize is the side of PACKAGE_ICON.PNG in pixels.
	PackageIconSize = 64
	// PackageIcon256Size is the side of PACKAGE_ICON_256.PNG in pixels.
	PackageIcon256Size = 256
	// MinSize is the smallest side Render accepts.
	MinSize = 16
	// MaxSize is the largest side Render accepts.
	MaxSize = 1024
)

// File names of the icons in the SPK, as the guide spells them (case sensitive).
const (
	// PackageIconFile is the small Package Center icon.
	PackageIconFile = "PACKAGE_ICON.PNG"
	// PackageIcon256File is the large Package Center icon.
	PackageIcon256File = "PACKAGE_ICON_256.PNG"
)

// ErrSizeOutOfRange reports an icon side outside MinSize..MaxSize.
var ErrSizeOutOfRange = errors.New("icon size out of range")

// Colours of the coral theme (ADR 0011). The web app's source of truth is
// web/packages/ui-tokens/src/colors.ts (`accent` and `onAccent`); keep both in sync. They are
// split into channels so no narrowing conversion is needed.
const (
	// accent is coral #FF6B57.
	accentRed, accentGreen, accentBlue uint8 = 0xFF, 0x6B, 0x57
	// onAccent is #3A0E07, the colour of text and icons on the accent.
	onAccentRed, onAccentGreen, onAccentBlue uint8 = 0x3A, 0x0E, 0x07
)

// Icon names one icon file of the SPK and its pixel size.
type Icon struct {
	// FileName is the name of the file in the SPK root.
	FileName string
	// Size is the side of the square image in pixels.
	Size int
}

// PackageIcons lists the icons the staging step (SPK-14) writes into the package.
func PackageIcons() []Icon {
	return []Icon{
		{FileName: PackageIconFile, Size: PackageIconSize},
		{FileName: PackageIcon256File, Size: PackageIcon256Size},
	}
}

// Render draws the icon as a size x size image with a transparent background.
func Render(size int) (*image.NRGBA, error) {
	if size < MinSize || size > MaxSize {
		return nil, fmt.Errorf("render icon of %d px (allowed %d to %d): %w",
			size, MinSize, MaxSize, ErrSizeOutOfRange)
	}

	geometry := newLayout(size)
	canvas := image.NewNRGBA(image.Rect(0, 0, size, size))

	for y := range size {
		for x := range size {
			canvas.SetNRGBA(x, y, geometry.pixel(x, y))
		}
	}

	return canvas, nil
}

// EncodePNG renders the icon and encodes it as PNG. The same size always yields the same
// bytes with the same Go version.
func EncodePNG(size int) ([]byte, error) {
	canvas, err := Render(size)
	if err != nil {
		return nil, err
	}

	var buffer bytes.Buffer

	encoder := png.Encoder{CompressionLevel: png.BestCompression}
	if err := encoder.Encode(&buffer, canvas); err != nil {
		return nil, fmt.Errorf("encode %d px icon as PNG: %w", size, err)
	}

	return buffer.Bytes(), nil
}

// coverage averages the sub-pixel samples of one pixel. Every sample inside the icon is
// opaque, so the colour is the mean of the covered samples and the alpha their share.
type coverage struct {
	covered, red, green, blue uint32
}

func (c *coverage) add(sample color.NRGBA, inside bool) {
	if !inside {
		return
	}

	c.covered++
	c.red += uint32(sample.R)
	c.green += uint32(sample.G)
	c.blue += uint32(sample.B)
}

func (c *coverage) colour() color.NRGBA {
	if c.covered == 0 {
		return color.NRGBA{}
	}

	return color.NRGBA{
		R: roundedMean(c.red, c.covered),
		G: roundedMean(c.green, c.covered),
		B: roundedMean(c.blue, c.covered),
		A: roundedMean(c.covered*math.MaxUint8, samplesPerPixel),
	}
}

// roundedMean divides sum by count, rounding half up, and clamps the result to a channel.
func roundedMean(sum, count uint32) uint8 {
	mean := (sum + count/2) / count
	if mean > math.MaxUint8 {
		return math.MaxUint8
	}

	return uint8(mean)
}
