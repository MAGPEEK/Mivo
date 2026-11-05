package photometa

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/magpeek/mivo/internal/domain/photo"
)

// IFD0 tags (TIFF 6.0).
const (
	tagNewSubfileType = 0x00FE
	tagImageWidth     = 0x0100
	tagImageLength    = 0x0101
	tagCompression    = 0x0103
	tagMake           = 0x010F
	tagModel          = 0x0110
	tagStripOffsets   = 0x0111
	tagOrientation    = 0x0112
	tagStripCounts    = 0x0117
	tagDateTime       = 0x0132
	tagSubIFDs        = 0x014A
	tagJPEGOffset     = 0x0201
	tagJPEGLength     = 0x0202
	tagExifIFD        = 0x8769
	tagGPSIFD         = 0x8825
	// tagRW2JpgFromRaw is the Panasonic RW2 IFD0 tag holding a complete JPEG preview.
	tagRW2JpgFromRaw = 0x002E
)

// Exif IFD tags (EXIF 2.32).
const (
	tagExposureTime      = 0x829A
	tagISO               = 0x8827
	tagDateTimeOriginal  = 0x9003
	tagDateTimeDigitized = 0x9004
	tagFocalLength       = 0x920A
	tagPixelXDimension   = 0xA002
	tagPixelYDimension   = 0xA003
	tagLensModel         = 0xA434
)

// GPS IFD tags.
const (
	tagGPSLatitudeRef  = 0x0001
	tagGPSLatitude     = 0x0002
	tagGPSLongitudeRef = 0x0003
	tagGPSLongitude    = 0x0004
)

const (
	// exifTimeLayout is the EXIF date format; it carries no time zone (see photo.Info).
	exifTimeLayout = "2006:01:02 15:04:05"
	minutesPerDeg  = 60
	secondsPerDeg  = 3600
	maxLatitude    = 90
	maxLongitude   = 180
	// fullResolutionSubfile is the NewSubfileType of the main image (not a reduced preview).
	fullResolutionSubfile = 0
	// dmsParts are degrees, minutes and seconds of a GPS coordinate.
	dmsDegrees = 0
	dmsMinutes = 1
	dmsSeconds = 2
)

// infoBuilder collects tags from several IFDs and resolves their precedence in build.
type infoBuilder struct {
	info                    photo.Info
	original, digitized     time.Time
	modified                time.Time
	pixelWidth, pixelHeight int
	frameWidth, frameHeight int
	imageWidth, imageHeight int
	imageArea               int64
}

// readTIFFInfo reads IFD0 and the Exif and GPS IFDs it points to.
func readTIFFInfo(ctx context.Context, stream *tiffStream, builder *infoBuilder) error {
	ifd0, err := stream.readIFD(ctx, stream.first)
	if err != nil {
		return fmt.Errorf("IFD0: %w", err)
	}

	builder.applyIFD0(stream, ifd0)
	builder.applySubIFDSizes(ctx, stream, ifd0)

	if offset, ok := stream.uint(ifd0, tagExifIFD); ok && offset != 0 {
		exif, err := stream.readIFD(ctx, offset)
		if err != nil {
			return fmt.Errorf("exif IFD: %w", err)
		}

		builder.applyExif(stream, exif)
	}

	if offset, ok := stream.uint(ifd0, tagGPSIFD); ok && offset != 0 {
		gps, err := stream.readIFD(ctx, offset)
		if err != nil {
			return fmt.Errorf("GPS IFD: %w", err)
		}

		builder.applyGPS(stream, gps)
	}

	return nil
}

// readTIFFFileInfo reads a TIFF-based RAW file.
func readTIFFFileInfo(ctx context.Context, src source) (photo.Info, error) {
	stream, err := openTIFF(src)
	if err != nil {
		return photo.Info{}, err
	}

	var builder infoBuilder
	if err := readTIFFInfo(ctx, stream, &builder); err != nil {
		return photo.Info{}, err
	}

	return builder.build(), nil
}

func (builder *infoBuilder) applyIFD0(stream *tiffStream, directory ifd) {
	builder.info.Make = stream.ascii(directory, tagMake)
	builder.info.Model = stream.ascii(directory, tagModel)
	builder.modified = parseEXIFTime(stream.ascii(directory, tagDateTime))

	if orientation, ok := stream.uint(directory, tagOrientation); ok &&
		orientation >= photo.OrientationMin && orientation <= photo.OrientationMax {
		builder.info.Orientation = int(orientation)
	}

	builder.considerImageSize(stream, directory)
}

// applySubIFDSizes considers the SubIFDs, where DNG, NEF and ARW keep the full-size image.
// Their errors only cost the size, so they are ignored.
func (builder *infoBuilder) applySubIFDSizes(ctx context.Context, stream *tiffStream, directory ifd) {
	for _, offset := range stream.uints(directory, tagSubIFDs, maxSubIFDs) {
		sub, err := stream.readIFD(ctx, offset)
		if err != nil {
			continue
		}

		builder.considerImageSize(stream, sub)
	}
}

// considerImageSize keeps the largest full-resolution image size (NewSubfileType absent or
// 0), which skips the reduced previews RAW files keep in IFD0.
func (builder *infoBuilder) considerImageSize(stream *tiffStream, directory ifd) {
	if subfile, ok := stream.uint(directory, tagNewSubfileType); ok && subfile != fullResolutionSubfile {
		return
	}

	width, widthOK := intTag(stream, directory, tagImageWidth)
	height, heightOK := intTag(stream, directory, tagImageLength)

	if !widthOK || !heightOK {
		return
	}

	if area := int64(width) * int64(height); area > builder.imageArea {
		builder.imageArea, builder.imageWidth, builder.imageHeight = area, width, height
	}
}

func (builder *infoBuilder) applyExif(stream *tiffStream, directory ifd) {
	builder.original = parseEXIFTime(stream.ascii(directory, tagDateTimeOriginal))
	builder.digitized = parseEXIFTime(stream.ascii(directory, tagDateTimeDigitized))
	builder.info.Lens = stream.ascii(directory, tagLensModel)

	if exposure, ok := stream.rational(directory, tagExposureTime, 0); ok && exposure >= 0 {
		builder.info.ExposureSeconds = exposure
	}

	if focal, ok := stream.rational(directory, tagFocalLength, 0); ok && focal >= 0 {
		builder.info.FocalLengthMM = focal
	}

	if iso, ok := intTag(stream, directory, tagISO); ok {
		builder.info.ISO = iso
	}

	builder.pixelWidth, _ = intTag(stream, directory, tagPixelXDimension)
	builder.pixelHeight, _ = intTag(stream, directory, tagPixelYDimension)
}

// applyGPS sets the position only when both coordinates are present and on the globe.
// Exposing it is a policy of the API layer (BACKLOG m3-15a), not of this reader.
func (builder *infoBuilder) applyGPS(stream *tiffStream, directory ifd) {
	latitude, latitudeOK := coordinate(stream, directory, tagGPSLatitude, tagGPSLatitudeRef)
	longitude, longitudeOK := coordinate(stream, directory, tagGPSLongitude, tagGPSLongitudeRef)

	if !latitudeOK || !longitudeOK ||
		latitude < -maxLatitude || latitude > maxLatitude ||
		longitude < -maxLongitude || longitude > maxLongitude {
		return
	}

	builder.info.GPS = &photo.Coordinates{Latitude: latitude, Longitude: longitude}
}

// coordinate converts degrees, minutes and seconds to signed decimal degrees; the
// reference "S" or "W" makes it negative.
func coordinate(stream *tiffStream, directory ifd, valueTag, refTag uint16) (float64, bool) {
	degrees, degreesOK := stream.rational(directory, valueTag, dmsDegrees)
	minutes, minutesOK := stream.rational(directory, valueTag, dmsMinutes)
	seconds, secondsOK := stream.rational(directory, valueTag, dmsSeconds)

	if !degreesOK || !minutesOK || !secondsOK || degrees < 0 || minutes < 0 || seconds < 0 {
		return 0, false
	}

	value := degrees + minutes/minutesPerDeg + seconds/secondsPerDeg

	switch stream.ascii(directory, refTag) {
	case "S", "W":
		value = -value
	}

	return value, true
}

// build resolves precedence: DateTimeOriginal, then DateTimeDigitized, then IFD0 DateTime;
// Exif pixel size, then the JPEG frame, then the largest full-resolution IFD size.
func (builder *infoBuilder) build() photo.Info {
	info := builder.info

	for _, taken := range []time.Time{builder.original, builder.digitized, builder.modified} {
		if !taken.IsZero() {
			info.TakenAt = taken

			break
		}
	}

	switch {
	case builder.pixelWidth > 0 && builder.pixelHeight > 0:
		info.Width, info.Height = builder.pixelWidth, builder.pixelHeight
	case builder.frameWidth > 0 && builder.frameHeight > 0:
		info.Width, info.Height = builder.frameWidth, builder.frameHeight
	default:
		info.Width, info.Height = builder.imageWidth, builder.imageHeight
	}

	return info
}

func intTag(stream *tiffStream, directory ifd, tag uint16) (int, bool) {
	value, ok := stream.uint(directory, tag)
	if !ok {
		return 0, false
	}

	return toInt(value)
}

// parseEXIFTime reads an EXIF date as wall-clock UTC; zero for absent, blank
// ("0000:00:00 00:00:00") or malformed dates.
func parseEXIFTime(value string) time.Time {
	if len(value) < len(exifTimeLayout) {
		return time.Time{}
	}

	parsed, err := time.ParseInLocation(exifTimeLayout, value[:len(exifTimeLayout)], time.UTC)
	if err != nil {
		return time.Time{}
	}

	return parsed
}

func trimSpaceValid(value string) string {
	return strings.TrimSpace(strings.ToValidUTF8(value, ""))
}
