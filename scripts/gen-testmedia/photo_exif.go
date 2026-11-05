package main

import (
	"encoding/binary"
	"fmt"
	"time"
)

// IFD0 tags of the EXIF blocks, TIFF files and DNG stand-ins the generator writes.
const (
	tagNewSubfileType      = 0x00FE
	tagImageWidth          = 0x0100
	tagImageLength         = 0x0101
	tagBitsPerSample       = 0x0102
	tagCompression         = 0x0103
	tagPhotometric         = 0x0106
	tagMake                = 0x010F
	tagModel               = 0x0110
	tagStripOffsets        = 0x0111
	tagOrientation         = 0x0112
	tagSamplesPerPixel     = 0x0115
	tagRowsPerStrip        = 0x0116
	tagStripByteCounts     = 0x0117
	tagPlanarConfiguration = 0x011C
	tagDateTime            = 0x0132
	tagSubIFDs             = 0x014A
	tagExifIFD             = 0x8769
	tagGPSIFD              = 0x8825
)

// Exif IFD tags (EXIF 2.32).
const (
	tagExposureTime      = 0x829A
	tagFNumber           = 0x829D
	tagISOSpeed          = 0x8827
	tagExifVersion       = 0x9000
	tagDateTimeOriginal  = 0x9003
	tagDateTimeDigitized = 0x9004
	tagFocalLength       = 0x920A
	tagPixelXDimension   = 0xA002
	tagPixelYDimension   = 0xA003
	tagLensModel         = 0xA434
)

// GPS IFD tags.
const (
	tagGPSVersionID    = 0x0000
	tagGPSLatitudeRef  = 0x0001
	tagGPSLatitude     = 0x0002
	tagGPSLongitudeRef = 0x0003
	tagGPSLongitude    = 0x0004
)

const (
	// exifTimeLayout is the EXIF date format; it has no time zone.
	exifTimeLayout = "2006:01:02 15:04:05"
	// wallClockLayout is how the manifest writes an EXIF time: the wall clock without a zone,
	// the reading photo.Info.TakenAt holds as UTC (M3-15b).
	wallClockLayout = "2006-01-02T15:04:05"
	// exifVersion is EXIF 2.32.
	exifVersion = "0232"
	// minutesPerDegree and secondsPerDegree convert degrees, minutes and seconds.
	minutesPerDegree = 60
	secondsPerDegree = 3600
)

// gpsVersion is GPSVersionID 2.3.0.0.
func gpsVersion() []byte { return []byte{2, 3, 0, 0} }

// jpegExifHeader starts the APP1 payload of an EXIF block.
func jpegExifHeader() []byte { return []byte("Exif\x00\x00") }

// expectedExif is what photometa ReadInfo returns for a photo; the generator writes exactly
// these values (Width and Height also as PixelXDimension/PixelYDimension). Zero values are
// absent tags.
type expectedExif struct {
	// TakenAt is the capture time as wall clock without a zone, e.g. "2024-07-14T09:30:00".
	TakenAt         string       `json:"takenAt,omitempty"`
	Make            string       `json:"make,omitempty"`
	Model           string       `json:"model,omitempty"`
	Lens            string       `json:"lens,omitempty"`
	ExposureSeconds float64      `json:"exposureSeconds,omitempty"`
	FNumber         float64      `json:"fNumber,omitempty"`
	FocalLengthMM   float64      `json:"focalLengthMM,omitempty"`
	ISO             int          `json:"iso,omitempty"`
	Orientation     int          `json:"orientation,omitempty"`
	GPS             *expectedGPS `json:"gps,omitempty"`
	Width           int          `json:"width,omitempty"`
	Height          int          `json:"height,omitempty"`
}

// expectedGPS is a position in decimal degrees as photometa computes it from the written
// degrees, minutes and seconds.
type expectedGPS struct {
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
}

// dms is one GPS coordinate as EXIF stores it; ref is "N", "S", "E" or "W".
type dms struct {
	degrees float64
	minutes float64
	seconds float64
	ref     string
}

// decimal is the signed decimal value, computed as photometa does.
func (coordinate dms) decimal() float64 {
	value := coordinate.degrees + coordinate.minutes/minutesPerDegree + coordinate.seconds/secondsPerDegree
	if coordinate.ref == "S" || coordinate.ref == "W" {
		return -value
	}

	return value
}

// gpsPosition is the written position of a photo.
type gpsPosition struct {
	latitude  dms
	longitude dms
}

func (position gpsPosition) expected() *expectedGPS {
	return &expectedGPS{Latitude: position.latitude.decimal(), Longitude: position.longitude.decimal()}
}

// exifTime converts a manifest wall-clock time to the EXIF format.
func exifTime(wallClock string) (string, error) {
	parsed, err := time.Parse(wallClockLayout, wallClock)
	if err != nil {
		return "", fmt.Errorf("capture time %q: %w", wallClock, err)
	}

	return parsed.Format(exifTimeLayout), nil
}

// newTIFFFile returns a stream with IFD0 reserved, so sub-directories can be added before
// IFD0's fields (which reference them) are known.
func newTIFFFile(order binary.AppendByteOrder) *tiffFile {
	return &tiffFile{order: order, directories: [][]tiffField{nil}}
}

// exifFields adds the Exif and GPS directories of exif to file and returns the IFD0 fields
// that describe the camera and reference them.
func (file *tiffFile) exifFields(exif expectedExif, gps *gpsPosition) ([]tiffField, error) {
	ifd0 := []tiffField{}

	if exif.Make != "" {
		ifd0 = append(ifd0, asciiField(tagMake, exif.Make))
	}

	if exif.Model != "" {
		ifd0 = append(ifd0, asciiField(tagModel, exif.Model))
	}

	if exif.Orientation > 0 {
		ifd0 = append(ifd0, file.shortField(tagOrientation, uint16(exif.Orientation))) //nolint:gosec // 1-8
	}

	sub, err := file.exifDirectory(exif)
	if err != nil {
		return nil, err
	}

	if exif.TakenAt != "" {
		taken, _ := exifTime(exif.TakenAt) // exifDirectory checked it
		ifd0 = append(ifd0, asciiField(tagDateTime, taken))
	}

	ifd0 = append(ifd0, directoryRef(tagExifIFD, file.addDirectory(sub...)))

	if gps != nil {
		ifd0 = append(ifd0, directoryRef(tagGPSIFD, file.addDirectory(file.gpsDirectory(*gps)...)))
	}

	return ifd0, nil
}

// exifDirectory returns the Exif IFD fields of exif.
func (file *tiffFile) exifDirectory(exif expectedExif) ([]tiffField, error) {
	fields := []tiffField{bytesField(tagExifVersion, tiffUndefined, []byte(exifVersion))}

	if exif.TakenAt != "" {
		taken, err := exifTime(exif.TakenAt)
		if err != nil {
			return nil, err
		}

		fields = append(fields, asciiField(tagDateTimeOriginal, taken), asciiField(tagDateTimeDigitized, taken))
	}

	for _, rational := range []struct {
		tag   uint16
		value float64
	}{
		{tagExposureTime, exif.ExposureSeconds}, {tagFNumber, exif.FNumber}, {tagFocalLength, exif.FocalLengthMM},
	} {
		if rational.value > 0 {
			fields = append(fields, file.rationalField(rational.tag, rational.value))
		}
	}

	if exif.ISO > 0 {
		fields = append(fields, file.shortField(tagISOSpeed, uint16(exif.ISO))) //nolint:gosec // camera ISO values
	}

	if exif.Lens != "" {
		fields = append(fields, asciiField(tagLensModel, exif.Lens))
	}

	if exif.Width > 0 && exif.Height > 0 {
		fields = append(fields,
			file.longField(tagPixelXDimension, uint32(exif.Width)),  //nolint:gosec // fixture sizes
			file.longField(tagPixelYDimension, uint32(exif.Height))) //nolint:gosec // fixture sizes
	}

	return fields, nil
}

// gpsDirectory returns the GPS IFD fields of position.
func (file *tiffFile) gpsDirectory(position gpsPosition) []tiffField {
	return []tiffField{
		bytesField(tagGPSVersionID, tiffByte, gpsVersion()),
		asciiField(tagGPSLatitudeRef, position.latitude.ref),
		file.rationalField(tagGPSLatitude, position.latitude.degrees, position.latitude.minutes, position.latitude.seconds),
		asciiField(tagGPSLongitudeRef, position.longitude.ref),
		file.rationalField(tagGPSLongitude,
			position.longitude.degrees, position.longitude.minutes, position.longitude.seconds),
	}
}

// exifBlock is the big-endian TIFF stream of an EXIF block (JPEG APP1, HEIF Exif item).
func exifBlock(exif expectedExif, gps *gpsPosition) ([]byte, error) {
	file := newTIFFFile(binary.BigEndian)

	fields, err := file.exifFields(exif, gps)
	if err != nil {
		return nil, err
	}

	file.directories[0] = fields

	return file.encode()
}
