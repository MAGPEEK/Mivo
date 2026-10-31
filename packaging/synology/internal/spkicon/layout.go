package spkicon

import "image/color"

// The geometry is computed on an integer sub-pixel grid instead of with floating point, so
// the output cannot differ between architectures (Go may fuse float multiply-adds on arm64).
const (
	// samplesPerAxis is the anti-aliasing supersampling per pixel and axis.
	samplesPerAxis = 4
	// samplesPerPixel is the number of samples averaged into one pixel.
	samplesPerPixel = samplesPerAxis * samplesPerAxis
	// gridUnitsPerPixel puts sample centres on odd grid coordinates: 2*i+1 for sample i.
	gridUnitsPerPixel = 2 * samplesPerAxis
	// sampleStep is the distance between neighbouring sample centres in grid units.
	sampleStep = 2
	// opaque is the alpha of a sample inside the icon.
	opaque = 0xFF

	// sixteenths is the denominator of the square's margin and corner radius.
	sixteenths = 16
	// marginSixteenths keeps the rounded square off the canvas edge (1/16 of the side).
	marginSixteenths = 1
	// cornerRadiusSixteenths is the corner radius of the rounded square (3/16 of the side).
	cornerRadiusSixteenths = 3

	// percent is the denominator of triangleHeightPercent.
	percent = 100
	// triangleHeightPercent is the play triangle's height as a share of the side.
	triangleHeightPercent = 44
	// perMille is the denominator of equilateralWidthPerMille.
	perMille = 1000
	// equilateralWidthPerMille is sqrt(3)/2: width over height of an equilateral triangle.
	equilateralWidthPerMille = 866
	// centroidFraction places the triangle's centroid, a third of its width from the flat
	// side, on the canvas centre, so it looks centred rather than shifted to the left.
	centroidFraction = 3
	// halves divides lengths into halves.
	halves = 2
)

// layout is the icon geometry of one size in sub-pixel grid units.
type layout struct {
	squareLow, squareHigh int64 // edges of the rounded square
	radius                int64 // corner radius of the rounded square
	triangleLeft          int64 // flat side of the play triangle
	triangleTip           int64 // tip of the play triangle, pointing right
	triangleHalfHeight    int64
	centre                int64
}

func newLayout(size int) layout {
	extent := int64(size) * gridUnitsPerPixel
	margin := extent * marginSixteenths / sixteenths
	height := extent * triangleHeightPercent / percent
	width := height * equilateralWidthPerMille / perMille
	centre := extent / halves
	left := centre - width/centroidFraction

	return layout{
		squareLow:          margin,
		squareHigh:         extent - margin,
		radius:             extent * cornerRadiusSixteenths / sixteenths,
		triangleLeft:       left,
		triangleTip:        left + width,
		triangleHalfHeight: height / halves,
		centre:             centre,
	}
}

// pixel supersamples the pixel at column x and row y.
func (l layout) pixel(x, y int) color.NRGBA {
	var samples coverage

	originX := int64(x) * gridUnitsPerPixel
	originY := int64(y) * gridUnitsPerPixel

	for row := range int64(samplesPerAxis) {
		for column := range int64(samplesPerAxis) {
			samples.add(l.sample(originX+column*sampleStep+1, originY+row*sampleStep+1))
		}
	}

	return samples.colour()
}

// sample returns the colour at a grid point and whether the point lies inside the icon.
func (l layout) sample(x, y int64) (color.NRGBA, bool) {
	switch {
	case !l.inRoundedSquare(x, y):
		return color.NRGBA{}, false
	case l.inTriangle(x, y):
		return color.NRGBA{R: onAccentRed, G: onAccentGreen, B: onAccentBlue, A: opaque}, true
	default:
		return color.NRGBA{R: accentRed, G: accentGreen, B: accentBlue, A: opaque}, true
	}
}

func (l layout) inRoundedSquare(x, y int64) bool {
	if x < l.squareLow || x > l.squareHigh || y < l.squareLow || y > l.squareHigh {
		return false
	}

	// Distance from the inner rectangle whose corners are the centres of the corner arcs.
	dx := outside(x, l.squareLow+l.radius, l.squareHigh-l.radius)
	dy := outside(y, l.squareLow+l.radius, l.squareHigh-l.radius)

	return dx*dx+dy*dy <= l.radius*l.radius
}

// outside returns how far value lies outside low..high, or 0 inside.
func outside(value, low, high int64) int64 {
	switch {
	case value < low:
		return low - value
	case value > high:
		return value - high
	default:
		return 0
	}
}

// inTriangle tests the right-pointing triangle: at the flat side it spans the full height,
// which shrinks linearly to zero at the tip.
func (l layout) inTriangle(x, y int64) bool {
	if x < l.triangleLeft || x > l.triangleTip {
		return false
	}

	offset := max(y-l.centre, l.centre-y)
	width := l.triangleTip - l.triangleLeft

	return offset*width <= (l.triangleTip-x)*l.triangleHalfHeight
}
