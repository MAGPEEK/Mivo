// Package photo holds the pure rules for photo libraries (ADR 0029): the EXIF-derived Info
// of a photo, the capture time with its fallback, the display orientation, the sort order of
// a folder and the pairing of a RAW file with its JPEG. The EXIF parsing itself is an
// adapter; this package never touches the file system.
package photo

import (
	"fmt"
	"math"
	"time"

	"github.com/magpeek/mivo/internal/domain"
)

// Valid EXIF orientation values (1 = upright, 5-8 are rotated by a quarter turn).
const (
	// OrientationUnknown means the file carries no orientation tag; it is shown as is.
	OrientationUnknown = 0
	// OrientationMin and OrientationMax bound the EXIF orientation tag.
	OrientationMin = 1
	OrientationMax = 8
	// firstTransposedOrientation is the first orientation that swaps width and height.
	firstTransposedOrientation = 5
)

// Bounds of a GPS position in degrees.
const (
	maxLatitude  = 90
	maxLongitude = 180
)

// Coordinates is a GPS position in decimal degrees.
type Coordinates struct {
	Latitude  float64
	Longitude float64
}

// Info is the metadata of one photo as read from its EXIF block. Zero values mean "not
// present"; GPS is nil without a position.
//
// TakenAt: EXIF times carry no time zone, so the adapter must supply one consistent time
// reference, the wall-clock reading as UTC, and use the same for the modification time it
// passes to CaptureTime. Mixing references would misorder photos.
type Info struct {
	TakenAt         time.Time
	Make            string
	Model           string
	Lens            string
	ExposureSeconds float64
	ISO             int
	FocalLengthMM   float64
	Orientation     int
	GPS             *Coordinates
	Width           int
	Height          int
}

// Validate checks the ranges of the numeric fields: an orientation of 0 or 1-8, a GPS
// position inside the globe, and no negative exposure, ISO, focal length or size. The error
// wraps domain.ErrInvalid.
func (info Info) Validate() error {
	if info.Orientation != OrientationUnknown &&
		(info.Orientation < OrientationMin || info.Orientation > OrientationMax) {
		return fmt.Errorf("validate photo info: orientation %d out of range: %w", info.Orientation, domain.ErrInvalid)
	}

	if !finite(info.ExposureSeconds, info.FocalLengthMM) {
		return fmt.Errorf("validate photo info: exposure or focal length is NaN or infinite: %w", domain.ErrInvalid)
	}

	if info.ExposureSeconds < 0 || info.ISO < 0 || info.FocalLengthMM < 0 || info.Width < 0 || info.Height < 0 {
		return fmt.Errorf("validate photo info: negative value: %w", domain.ErrInvalid)
	}

	if gps := info.GPS; gps != nil && !gps.valid() {
		return fmt.Errorf("validate photo info: GPS position (%v, %v) out of range: %w",
			gps.Latitude, gps.Longitude, domain.ErrInvalid)
	}

	return nil
}

func finite(values ...float64) bool {
	for _, value := range values {
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return false
		}
	}

	return true
}

// valid is false for NaN, because every comparison with NaN is false.
func (coordinates Coordinates) valid() bool {
	return coordinates.Latitude >= -maxLatitude && coordinates.Latitude <= maxLatitude &&
		coordinates.Longitude >= -maxLongitude && coordinates.Longitude <= maxLongitude
}

// DisplaySize returns the width and height as the photo is shown after applying the
// orientation: quarter-turn orientations (5-8) swap the stored width and height.
func (info Info) DisplaySize() (width, height int) {
	if info.Orientation >= firstTransposedOrientation && info.Orientation <= OrientationMax {
		return info.Height, info.Width
	}

	return info.Width, info.Height
}

// CaptureTime is the time a photo is sorted and grouped by: the EXIF capture time, or the
// file's modification time when the file has none. Both must use the same time reference
// (wall-clock as UTC, see Info.TakenAt).
func CaptureTime(info Info, modTime time.Time) time.Time {
	if info.TakenAt.IsZero() {
		return modTime
	}

	return info.TakenAt
}
