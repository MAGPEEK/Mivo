package media

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/magpeek/mivo/internal/domain"
)

// displayPrecision rounds frame rates to three decimal places, enough to tell 23.976 from 24
// and 29.97 from 30.
const displayPrecision = 1000

const frameRateSeparator = "/"

// FrameRate is a frame rate as an exact fraction, because the common NTSC rates
// (24000/1001, 30000/1001) have no exact decimal form. The fraction is always stored
// reduced, so equal rates compare equal with ==. The zero value means unknown, which is
// what ffprobe reports as "0/0".
type FrameRate struct {
	numerator   int
	denominator int
}

// NewFrameRate returns the reduced fraction numerator/denominator. A zero or negative part
// yields the unknown rate.
func NewFrameRate(numerator, denominator int) FrameRate {
	if numerator <= 0 || denominator <= 0 {
		return FrameRate{}
	}

	divisor := greatestCommonDivisor(numerator, denominator)

	return FrameRate{numerator: numerator / divisor, denominator: denominator / divisor}
}

// ParseFrameRate reads the "numerator/denominator" notation of ffprobe (r_frame_rate,
// avg_frame_rate) or a plain integer. Empty text, a zero numerator and ffprobe's "0/0" yield
// the unknown rate. Malformed text, negative numbers and a zero denominator with a non-zero
// numerator wrap domain.ErrInvalid.
func ParseFrameRate(text string) (FrameRate, error) {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return FrameRate{}, nil
	}

	numeratorText, denominatorText, hasDenominator := strings.Cut(trimmed, frameRateSeparator)
	if !hasDenominator {
		denominatorText = "1"
	}

	numerator, numeratorErr := parseNonNegative(numeratorText)
	denominator, denominatorErr := parseNonNegative(denominatorText)

	if numeratorErr != nil || denominatorErr != nil || (denominator == 0 && numerator != 0) {
		return FrameRate{}, fmt.Errorf("parse frame rate %q: %w", text, domain.ErrInvalid)
	}

	return NewFrameRate(numerator, denominator), nil
}

// Numerator returns the numerator of the reduced fraction, 0 when unknown.
func (rate FrameRate) Numerator() int {
	return rate.numerator
}

// Denominator returns the denominator of the reduced fraction, 0 when unknown.
func (rate FrameRate) Denominator() int {
	return rate.denominator
}

// IsKnown reports whether the rate carries a value.
func (rate FrameRate) IsKnown() bool {
	return rate.denominator > 0
}

// FramesPerSecond returns the exact quotient, 0 when unknown.
func (rate FrameRate) FramesPerSecond() float64 {
	if !rate.IsKnown() {
		return 0
	}

	return float64(rate.numerator) / float64(rate.denominator)
}

// RoundedFramesPerSecond returns the rate rounded half away from zero to three decimal
// places (24000/1001 → 23.976, 30000/1001 → 29.97) for display and for storage as a number.
func (rate FrameRate) RoundedFramesPerSecond() float64 {
	return math.Round(rate.FramesPerSecond()*displayPrecision) / displayPrecision
}

// String returns the ffprobe notation "numerator/denominator", or "" when unknown, so that
// ParseFrameRate(rate.String()) == rate.
func (rate FrameRate) String() string {
	if !rate.IsKnown() {
		return ""
	}

	return strconv.Itoa(rate.numerator) + frameRateSeparator + strconv.Itoa(rate.denominator)
}

// parseNonNegative accepts decimal digits only, so signs, exponents and fractions fail.
func parseNonNegative(text string) (int, error) {
	if !isDigits(text) {
		return 0, fmt.Errorf("not a non-negative integer: %q", text)
	}

	value, err := strconv.Atoi(text)
	if err != nil {
		return 0, fmt.Errorf("parse %q: %w", text, err)
	}

	return value, nil
}

func greatestCommonDivisor(first, second int) int {
	for second != 0 {
		first, second = second, first%second
	}

	return first
}
