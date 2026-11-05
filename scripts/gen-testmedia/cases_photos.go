package main

import (
	"fmt"
	"path"
	"slices"
	"time"
)

// The photo tree (task M3-15g, ADR 0029) is declared as photo specs: library_photos.go
// derives the FFmpeg cases, the Go assemblies and the expected catalog from them, so the
// manifest and the files cannot diverge. Cameras, lenses and file names are invented; the
// only GPS position is the Eiffel Tower, a public landmark.

const (
	categoryPhotos = "photos"
	// photosLibraryPath is the photo library root below the -library directory.
	photosLibraryPath = "photos"
	// photoSourcesPath holds the HEVC streams the HEIC files are assembled from; it lies
	// outside every library root, so a scan never lists them.
	photoSourcesPath = ".photo-sources"
	heicSingleSource = photoSourcesPath + "/heic-single.hevc"
	heicGridSource   = photoSourcesPath + "/heic-grid.hevc"
)

// Photo formats as the manifest names them; each is also the file extension, except tiff
// (".tif") and jpeg (".jpg").
const (
	photoFormatJPEG = "jpeg"
	photoFormatPNG  = "png"
	photoFormatWebP = "webp"
	photoFormatGIF  = "gif"
	photoFormatTIFF = "tiff"
	photoFormatDNG  = "dng"
	photoFormatNEF  = "nef"
	photoFormatHEIC = "heic"
	photoFormatAVIF = "avif"
	photoFormatMP4  = "mp4"
)

// photoBuild says how a photo file is made.
type photoBuild int

const (
	buildJPEG photoBuild = iota
	buildPNG
	buildGIF
	buildTIFF
	buildDNG
	// buildCorruptJPEG is a JPEG with a broken Exif pointer and no scan data.
	buildCorruptJPEG
	// buildUnreadableRAW is a RAW file name over bytes no RAW reader understands.
	buildUnreadableRAW
	buildHEICSingle
	buildHEICGrid
	// The FFmpeg cases.
	buildWebP
	buildAVIF
	buildMP4
)

// Pixel sizes: every still is a 4:3 landscape; the HEIC grid has 2x2 tiles.
const (
	photoWidth      = 320
	photoHeight     = 240
	heicGridRows    = 2
	heicGridColumns = 2
)

// photoSpec is one photo file before its expectation is written out.
type photoSpec struct {
	// file is relative to the photo library root.
	file   string
	format string
	build  photoBuild
	// exif is the written EXIF block (nil: none); Width and Height are the stored size.
	exif *expectedExif
	gps  *gpsPosition
	// alternateOf is the primary file of a RAW+JPEG pair; the RAW carries the same EXIF.
	alternateOf string
	// fixedTime asks for a fixed modification time (a Go-built file without EXIF time);
	// photoSpecs fills modified from it, zero leaves the time alone.
	fixedTime bool
	modified  time.Time
	// optional marks HEIC and AVIF, which need FFmpeg encoders a build may lack.
	optional bool
}

// Folders (albums) of the tree; "Holidays 2024/Lake Shore" is nested.
const (
	albumHolidays    = "Holidays 2024"
	albumLakeShore   = albumHolidays + "/Lake Shore"
	albumRawOnly     = "Raw Only"
	albumFormats     = "Formats"
	albumPhone       = "Phone"
	albumOrientation = "Orientation"
)

// The Eiffel Tower is at 48°51'29.6"N 2°17'40.2"E, a public landmark.
const (
	eiffelLatitudeDegrees   = 48
	eiffelLatitudeMinutes   = 51
	eiffelLatitudeSeconds   = 29.6
	eiffelLongitudeDegrees  = 2
	eiffelLongitudeMinutes  = 17
	eiffelLongitudeSeconds  = 40.2
	orientationUpright      = 1
	orientationUpsideDown   = 3
	orientationClockwise    = 6
	orientationCounterClock = 8
	orientationCount        = 8
)

// Exposure settings of the invented cameras: shutter time denominators (1/n s), f-numbers,
// focal lengths in mm and ISO speeds.
const (
	shutter60, shutter120, shutter125, shutter250, shutter1000 = 60, 120, 125, 250, 1000
	aperture1p8, aperture2p8, aperture4, aperture5p6           = 1.8, 2.8, 4, 5.6
	focal6p9, focal35, focal50, focal85                        = 6.9, 35, 50, 85
	iso64, iso100, iso200, iso400, iso800                      = 64, 100, 200, 400, 800
)

// eiffelTower is the only GPS position of the corpus.
func eiffelTower() *gpsPosition {
	return &gpsPosition{
		latitude: dms{
			degrees: eiffelLatitudeDegrees, minutes: eiffelLatitudeMinutes, seconds: eiffelLatitudeSeconds, ref: "N",
		},
		longitude: dms{
			degrees: eiffelLongitudeDegrees, minutes: eiffelLongitudeMinutes, seconds: eiffelLongitudeSeconds, ref: "E",
		},
	}
}

// photoSpecs returns every photo file in catalog order. Files marked fixedTime get distinct
// modification times, one minute apart, in that order.
func photoSpecs() []photoSpec {
	specs := []photoSpec{
		holidayPhoto(), lakeShorePortrait(),
		{file: albumLakeShore + "/DSC_0103.jpg", format: photoFormatJPEG, build: buildJPEG, exif: fennickExif()},
		{
			file: albumLakeShore + "/DSC_0103.dng", format: photoFormatDNG, build: buildDNG, exif: fennickExif(),
			alternateOf: albumLakeShore + "/DSC_0103.jpg",
		},
		{file: albumLakeShore + "/VID_0104.mp4", format: photoFormatMP4, build: buildMP4},
		loneRAW(),
		{file: albumRawOnly + "/DSC_0300.nef", format: photoFormatNEF, build: buildUnreadableRAW, fixedTime: true},
	}

	specs = slices.Concat(specs, formatSpecs(), phoneSpecs(), orientationSpecs())
	fixed := 0

	for index := range specs {
		if specs[index].fixedTime {
			specs[index].modified = fixedModifiedBase().Add(time.Duration(fixed) * time.Minute)
			fixed++
		}
	}

	return specs
}

// holidayPhoto carries every EXIF field the reader knows, including the GPS position.
func holidayPhoto() photoSpec {
	return photoSpec{
		file: albumHolidays + "/IMG_0101.jpg", format: photoFormatJPEG, build: buildJPEG, gps: eiffelTower(),
		exif: &expectedExif{
			TakenAt: "2024-07-14T09:30:00", Make: "Lumora", Model: "LX-100", Lens: "Lumora 24-70mm F2.8",
			ExposureSeconds: 1.0 / shutter250, FNumber: aperture5p6, FocalLengthMM: focal35, ISO: iso100,
			Orientation: orientationUpright, Width: photoWidth, Height: photoHeight,
		},
	}
}

// lakeShorePortrait is stored landscape and shown portrait (orientation 6).
func lakeShorePortrait() photoSpec {
	return photoSpec{
		file: albumLakeShore + "/IMG_0102.jpg", format: photoFormatJPEG, build: buildJPEG,
		exif: &expectedExif{
			TakenAt: "2024-07-14T10:05:00", Make: "Lumora", Model: "LX-100", Lens: "Lumora 24-70mm F2.8",
			ExposureSeconds: 1.0 / shutter60, FNumber: aperture2p8, FocalLengthMM: focal50, ISO: iso400,
			Orientation: orientationClockwise, Width: photoWidth, Height: photoHeight,
		},
	}
}

// fennickExif is the EXIF of the RAW+JPEG pair.
func fennickExif() *expectedExif {
	return &expectedExif{
		TakenAt: "2024-07-14T10:12:30", Make: "Fennick", Model: "F7 Mark II", Lens: "Fennick 85mm F1.8",
		ExposureSeconds: 1.0 / shutter1000, FNumber: aperture1p8, FocalLengthMM: focal85, ISO: iso200,
		Orientation: orientationUpright, Width: photoWidth, Height: photoHeight,
	}
}

// loneRAW is a DNG without a JPEG partner, shown through its preview turned by
// orientation 8.
func loneRAW() photoSpec {
	return photoSpec{
		file: albumRawOnly + "/DSC_0201.dng", format: photoFormatDNG, build: buildDNG,
		exif: &expectedExif{
			TakenAt: "2023-11-02T16:45:10", Make: "Fennick", Model: "F7 Mark II", Lens: "Fennick 35mm F2",
			ExposureSeconds: 1.0 / shutter125, FNumber: aperture4, FocalLengthMM: focal35, ISO: iso800,
			Orientation: orientationCounterClock, Width: photoWidth, Height: photoHeight,
		},
	}
}

// formatSpecs are the formats without a camera: no EXIF (the capture time falls back to the
// modification time), a scanner TIFF with EXIF, and a corrupt JPEG.
func formatSpecs() []photoSpec {
	return []photoSpec{
		{file: albumFormats + "/plain.png", format: photoFormatPNG, build: buildPNG, fixedTime: true},
		{file: albumFormats + "/sticker.webp", format: photoFormatWebP, build: buildWebP},
		{file: albumFormats + "/loop.gif", format: photoFormatGIF, build: buildGIF, fixedTime: true},
		{
			file: albumFormats + "/scan.tif", format: photoFormatTIFF, build: buildTIFF,
			exif: &expectedExif{
				TakenAt: "2022-05-20T08:00:00", Make: "Papyra", Model: "ScanDesk 4",
				Orientation: orientationUpsideDown, Width: photoWidth, Height: photoHeight,
			},
		},
		{file: albumFormats + "/no-exif.jpg", format: photoFormatJPEG, build: buildJPEG, fixedTime: true},
		{file: albumFormats + "/broken.jpg", format: photoFormatJPEG, build: buildCorruptJPEG, fixedTime: true},
	}
}

// phoneSpecs are the optional HEIF-family files: a single HEIC picture and a 2x2 HEIC grid
// with EXIF items, and an AVIF without EXIF.
func phoneSpecs() []photoSpec {
	phone := func(taken string) *expectedExif {
		return &expectedExif{
			TakenAt: taken, Make: "Pomelo", Model: "Pomelo 15 Pro", Lens: "Pomelo back camera",
			ExposureSeconds: 1.0 / shutter120, FNumber: aperture1p8, FocalLengthMM: focal6p9, ISO: iso64,
			Orientation: orientationUpright, Width: photoWidth, Height: photoHeight,
		}
	}

	return []photoSpec{
		{
			file: albumPhone + "/IMG_1001.heic", format: photoFormatHEIC, build: buildHEICSingle,
			exif: phone("2024-08-01T18:20:00"), optional: true,
		},
		{
			file: albumPhone + "/IMG_1002.heic", format: photoFormatHEIC, build: buildHEICGrid,
			exif: phone("2024-08-01T18:21:00"), optional: true,
		},
		{file: albumPhone + "/IMG_1003.avif", format: photoFormatAVIF, build: buildAVIF, optional: true},
	}
}

// orientationSpecs are one landscape JPEG per EXIF orientation 1-8, a minute apart.
func orientationSpecs() []photoSpec {
	specs := make([]photoSpec, 0, orientationCount)

	for orientation := orientationUpright; orientation <= orientationCount; orientation++ {
		specs = append(specs, photoSpec{
			file:   fmt.Sprintf("%s/orientation-%d.jpg", albumOrientation, orientation),
			format: photoFormatJPEG, build: buildJPEG,
			exif: &expectedExif{
				TakenAt: fmt.Sprintf("2024-03-01T12:%02d:00", orientation), Make: "Lumora", Model: "LX-100",
				Orientation: orientation, Width: photoWidth, Height: photoHeight,
			},
		})
	}

	return specs
}

// fixedModifiedBase is the modification time of the first file without an EXIF time.
func fixedModifiedBase() time.Time {
	const year, hour = 2021, 12

	return time.Date(year, time.June, 1, hour, 0, 0, 0, time.UTC)
}

// album is the folder of the photo, relative to the library root.
func (spec photoSpec) album() string { return path.Dir(spec.file) }
