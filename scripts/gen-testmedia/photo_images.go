package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	"image/color/palette"
	"image/draw"
	"image/gif"
	"image/jpeg"
	"image/png"
	"slices"
)

// Pixel files of the photo corpus, encoded in Go: FFmpeg writes no EXIF (ADR 0021), and the
// image bytes only need to decode, so the standard library encoders are enough.

const (
	// photoJPEGQuality keeps the fixtures small but clean.
	photoJPEGQuality = 85
	// maxChannel is the full intensity of an 8-bit colour channel.
	maxChannel = 255
	// markerDivisor sizes the red corner marker: a quarter of each side. A thumbnail shows
	// it top left only when the orientation was applied.
	markerDivisor = 4
	// gifFrames is the frame count of the animated GIF; gifDelay its frame delay (1/100 s).
	gifFrames = 3
	gifDelay  = 50
	// TIFF values of the uncompressed RGB file and the DNG stand-in.
	rgbSamples             = 3
	bitsPerSample          = 8
	compressionNone        = 1
	compressionJPEG        = 7
	photometricRGB         = 2
	photometricYCbCr       = 6
	photometricCFA         = 32803
	planarChunky           = 1
	reducedResolution      = 1
	fullResolution         = 0
	tagDNGVersion          = 0xC612
	tagUniqueCameraModel   = 0xC614
	tagCFARepeatPatternDim = 0x828D
	tagCFAPattern          = 0x828E
	cfaSide                = 2
	// jpegMarkerAPP1 introduces the EXIF segment; jpegSegmentLengthSize is its length field.
	jpegMarkerAPP1        = 0xE1
	jpegSegmentLengthSize = 2
	maxJPEGSegmentPayload = 0xFFFF - jpegSegmentLengthSize
	jpegMarkerSOF0        = 0xC0
	jpegMarkerPrefix      = 0xFF
	jpegHeaderSize        = 4
)

// dngVersion is DNGVersion 1.4.0.0.
func dngVersion() []byte { return []byte{1, 4, 0, 0} }

// cfaPattern is an RGGB Bayer pattern (0 red, 1 green, 2 blue).
func cfaPattern() []byte { return []byte{0, 1, 1, 2} }

// testPattern is a colour gradient with a red block in the top-left corner, so a rendered
// thumbnail shows whether the orientation was applied. shift varies the gradient per frame.
func testPattern(width, height, shift int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, width, height))

	for y := range height {
		for x := range width {
			img.SetRGBA(x, y, color.RGBA{
				R: uint8((x + shift) * maxChannel / width), G: uint8(y * maxChannel / height), //nolint:gosec // 0-255
				B: uint8(maxChannel - (x+y)*maxChannel/(width+height)), A: maxChannel, //nolint:gosec // 0-255
			})
		}
	}

	marker := image.Rect(0, 0, width/markerDivisor, height/markerDivisor)
	draw.Draw(img, marker, image.NewUniform(color.RGBA{R: maxChannel, A: maxChannel}), image.Point{}, draw.Src)

	return img
}

func encodeJPEG(img image.Image) ([]byte, error) {
	var out bytes.Buffer
	if err := jpeg.Encode(&out, img, &jpeg.Options{Quality: photoJPEGQuality}); err != nil {
		return nil, fmt.Errorf("encode JPEG: %w", err)
	}

	return out.Bytes(), nil
}

func encodePNG(img image.Image) ([]byte, error) {
	var out bytes.Buffer
	if err := png.Encode(&out, img); err != nil {
		return nil, fmt.Errorf("encode PNG: %w", err)
	}

	return out.Bytes(), nil
}

// encodeGIF writes an animated GIF of gifFrames shifted patterns.
func encodeGIF(width, height int) ([]byte, error) {
	animation := &gif.GIF{}

	for frame := range gifFrames {
		paletted := image.NewPaletted(image.Rect(0, 0, width, height), palette.Plan9)
		draw.FloydSteinberg.Draw(paletted, paletted.Rect, testPattern(width, height, frame*width/gifFrames), image.Point{})
		animation.Image = append(animation.Image, paletted)
		animation.Delay = append(animation.Delay, gifDelay)
	}

	var out bytes.Buffer
	if err := gif.EncodeAll(&out, animation); err != nil {
		return nil, fmt.Errorf("encode GIF: %w", err)
	}

	return out.Bytes(), nil
}

// insertExif puts an APP1 EXIF segment right after the SOI marker of a JPEG.
func insertExif(jpegData, tiffStream []byte) ([]byte, error) {
	payload := slices.Concat(jpegExifHeader(), tiffStream)
	if len(payload) > maxJPEGSegmentPayload {
		return nil, fmt.Errorf("EXIF block of %d bytes does not fit a JPEG segment", len(payload))
	}

	if len(jpegData) < jpegSegmentLengthSize || jpegData[0] != jpegMarkerPrefix {
		return nil, fmt.Errorf("not a JPEG stream")
	}

	segment := []byte{jpegMarkerPrefix, jpegMarkerAPP1}
	segment = binary.BigEndian.AppendUint16(segment, uint16(len(payload)+jpegSegmentLengthSize)) //nolint:gosec // checked

	return slices.Concat(jpegData[:jpegSegmentLengthSize], segment, payload, jpegData[jpegSegmentLengthSize:]), nil
}

// jpegWithExif encodes the pattern as JPEG and adds the EXIF block when exif is set.
func jpegWithExif(width, height int, exif *expectedExif, gps *gpsPosition) ([]byte, error) {
	data, err := encodeJPEG(testPattern(width, height, 0))
	if err != nil || exif == nil {
		return data, err
	}

	block, err := exifBlock(*exif, gps)
	if err != nil {
		return nil, err
	}

	return insertExif(data, block)
}

// corruptJPEG is a JPEG whose Exif IFD pointer lies far outside the file and whose frame
// header and scan are missing: photometa reports the metadata as corrupt and no decoder finds
// a picture (without the frame header FFmpeg cannot even guess one).
func corruptJPEG(width, height int) ([]byte, error) {
	data, err := encodeJPEG(testPattern(width, height, 0))
	if err != nil {
		return nil, err
	}

	file := newTIFFFile(binary.BigEndian)
	file.directories[0] = []tiffField{file.longField(tagExifIFD, outOfRangeOffset)}

	block, err := file.encode()
	if err != nil {
		return nil, err
	}

	withExif, err := insertExif(data, block)
	if err != nil {
		return nil, err
	}

	return truncateBeforeFrame(withExif)
}

// outOfRangeOffset points past every fixture file.
const outOfRangeOffset = 0x7FFF0000

// truncateBeforeFrame keeps the segments before the baseline frame header (SOF0, which the
// standard library encoder writes) and ends the stream there with EOI.
func truncateBeforeFrame(data []byte) ([]byte, error) {
	position := jpegSegmentLengthSize

	for position+jpegHeaderSize <= len(data) && data[position] == jpegMarkerPrefix {
		if data[position+1] == jpegMarkerSOF0 {
			return slices.Concat(data[:position], []byte{jpegMarkerPrefix, jpegMarkerEOI}), nil
		}

		position += jpegSegmentLengthSize + int(binary.BigEndian.Uint16(data[position+jpegSegmentLengthSize:]))
	}

	return nil, fmt.Errorf("no baseline frame header in the JPEG stream")
}

// jpegMarkerEOI ends a JPEG stream.
const jpegMarkerEOI = 0xD9

// rgbTIFF is an uncompressed little-endian RGB TIFF carrying exif in IFD0 and its Exif IFD.
func rgbTIFF(width, height int, exif expectedExif) ([]byte, error) {
	img := testPattern(width, height, 0)
	pixels := make([]byte, 0, width*height*rgbSamples)

	for y := range height {
		for x := range width {
			offset := img.PixOffset(x, y)
			pixels = append(pixels, img.Pix[offset:offset+rgbSamples]...)
		}
	}

	file := newTIFFFile(binary.LittleEndian)

	fields, err := file.exifFields(exif, nil)
	if err != nil {
		return nil, err
	}

	strip := file.addBlob(pixels)
	file.directories[0] = append(fields,
		file.longField(tagImageWidth, uint32(width)), file.longField(tagImageLength, uint32(height)), //nolint:gosec // fixture sizes
		file.shortField(tagBitsPerSample, bitsPerSample, bitsPerSample, bitsPerSample),
		file.shortField(tagCompression, compressionNone), file.shortField(tagPhotometric, photometricRGB),
		blobRef(tagStripOffsets, strip), file.shortField(tagSamplesPerPixel, rgbSamples),
		file.longField(tagRowsPerStrip, uint32(height)), file.longField(tagStripByteCounts, uint32(len(pixels))), //nolint:gosec // fixture sizes
		file.shortField(tagPlanarConfiguration, planarChunky),
	)

	return file.encode()
}

// dngStandIn is a minimal little-endian DNG: IFD0 is the JPEG preview (a reduced-resolution
// image, as in camera DNGs) with the camera tags and the Exif IFD, its SubIFD a tiny 8-bit
// CFA "raw" image. Real DNGs hold more tags; photometa and the photo renderer only need the
// preview, the camera tags and the Exif values.
func dngStandIn(width, height int, exif expectedExif, gps *gpsPosition) ([]byte, error) {
	preview, err := encodeJPEG(testPattern(width, height, 0))
	if err != nil {
		return nil, err
	}

	file := newTIFFFile(binary.LittleEndian)

	fields, err := file.exifFields(exif, gps)
	if err != nil {
		return nil, err
	}

	rawWidth, rawHeight := width/markerDivisor, height/markerDivisor
	raw := file.addBlob(make([]byte, rawWidth*rawHeight))
	rawIFD := file.addDirectory(
		file.longField(tagNewSubfileType, fullResolution),
		file.longField(tagImageWidth, uint32(rawWidth)), file.longField(tagImageLength, uint32(rawHeight)), //nolint:gosec // fixture sizes
		file.shortField(tagBitsPerSample, bitsPerSample), file.shortField(tagCompression, compressionNone),
		file.shortField(tagPhotometric, photometricCFA), blobRef(tagStripOffsets, raw),
		file.shortField(tagSamplesPerPixel, 1), file.longField(tagRowsPerStrip, uint32(rawHeight)), //nolint:gosec // fixture sizes
		file.longField(tagStripByteCounts, uint32(rawWidth*rawHeight)), //nolint:gosec // fixture sizes
		file.shortField(tagCFARepeatPatternDim, cfaSide, cfaSide), bytesField(tagCFAPattern, tiffByte, cfaPattern()),
	)

	previewBlob := file.addBlob(preview)
	file.directories[0] = append(fields,
		file.longField(tagNewSubfileType, reducedResolution),
		file.longField(tagImageWidth, uint32(width)), file.longField(tagImageLength, uint32(height)), //nolint:gosec // fixture sizes
		file.shortField(tagBitsPerSample, bitsPerSample, bitsPerSample, bitsPerSample),
		file.shortField(tagCompression, compressionJPEG), file.shortField(tagPhotometric, photometricYCbCr),
		blobRef(tagStripOffsets, previewBlob), file.shortField(tagSamplesPerPixel, rgbSamples),
		file.longField(tagRowsPerStrip, uint32(height)), file.longField(tagStripByteCounts, uint32(len(preview))), //nolint:gosec // fixture sizes
		directoryRef(tagSubIFDs, rawIFD),
		bytesField(tagDNGVersion, tiffByte, dngVersion()),
		asciiField(tagUniqueCameraModel, exif.Make+" "+exif.Model),
	)

	return file.encode()
}
