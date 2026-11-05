//nolint:gosec // fixture writer: sizes are small test constants
package photometa

import (
	"encoding/binary"
	"time"

	"github.com/magpeek/mivo/internal/domain/photo"
)

// cameraOptions shape the synthetic camera TIFF stream.
type cameraOptions struct {
	order     binary.ByteOrder
	magic     uint16
	thumbnail []byte // IFD1 JPEGInterchangeFormat
	ifd0Extra func(builder *tiffBuilder) []testEntry
}

func defaultCamera() cameraOptions {
	return cameraOptions{order: binary.LittleEndian, magic: tiffMagic}
}

// cameraTIFF builds IFD0 (Make, Model, Orientation 6, DateTime) with an Exif IFD, a GPS IFD,
// optional SubIFDs and an optional IFD1 thumbnail.
func cameraTIFF(options cameraOptions) []byte {
	builder := newTIFFBuilder(options.order, options.magic)

	exif := builder.addIFD([]testEntry{
		builder.rational(tagExposureTime, typeRational, 1, 250),
		builder.rational(tagFNumber, typeRational, 18, 10),
		builder.short(tagISO, 400),
		builder.ascii(tagDateTimeOriginal, "2024:06:01 14:30:15"),
		builder.rational(tagFocalLength, typeRational, 50, 1),
		builder.long(tagPixelXDimension, 6000),
		builder.short(tagPixelYDimension, 4000),
		builder.ascii(tagLensModel, "RF50mm F1.8 STM"),
	}, 0)
	gps := builder.addIFD([]testEntry{
		builder.ascii(tagGPSLatitudeRef, "N"),
		builder.rational(tagGPSLatitude, typeRational, 48, 1, 8, 1, 3000, 100),
		builder.ascii(tagGPSLongitudeRef, "W"),
		builder.rational(tagGPSLongitude, typeRational, 11, 1, 34, 1, 0, 1),
	}, 0)

	var ifd1 uint32
	if options.thumbnail != nil {
		blob := builder.addBlob(options.thumbnail)
		ifd1 = builder.addIFD([]testEntry{
			builder.long(tagJPEGOffset, blob),
			builder.long(tagJPEGLength, uint32(len(options.thumbnail))),
		}, 0)
	}

	entries := []testEntry{
		builder.ascii(tagMake, "Canon"),
		builder.ascii(tagModel, "EOS R5"),
		builder.short(tagOrientation, 6),
		builder.ascii(tagDateTime, "2024:06:02 10:00:00"),
		builder.long(tagExifIFD, exif),
		builder.long(tagGPSIFD, gps),
	}

	if options.ifd0Extra != nil {
		entries = append(entries, options.ifd0Extra(builder)...)
	}

	builder.setFirst(builder.addIFD(entries, ifd1))

	return builder.bytes()
}

// cameraInfo is what cameraTIFF yields.
func cameraInfo() photo.Info {
	return photo.Info{
		TakenAt:         time.Date(2024, 6, 1, 14, 30, 15, 0, time.UTC),
		Make:            "Canon",
		Model:           "EOS R5",
		Lens:            "RF50mm F1.8 STM",
		ExposureSeconds: 1.0 / 250,
		FNumber:         1.8,
		ISO:             400,
		FocalLengthMM:   50,
		Orientation:     6,
		GPS:             &photo.Coordinates{Latitude: 48 + 8.0/60 + 30.0/3600, Longitude: -(11 + 34.0/60)},
		Width:           6000,
		Height:          4000,
	}
}

// singleIFDTIFF builds a stream whose first IFD holds entries; CR3 CMT boxes are such streams.
func singleIFDTIFF(entries func(builder *tiffBuilder) []testEntry) []byte {
	builder := newTIFFBuilder(binary.LittleEndian, tiffMagic)
	builder.setFirst(builder.addIFD(entries(builder), 0))

	return builder.bytes()
}
