package photometa

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"

	"github.com/magpeek/mivo/internal/domain/photo"
)

// JPEG markers (ITU T.81 table B.1).
const (
	markerPrefix = 0xFF
	markerTEM    = 0x01
	markerSOF0   = 0xC0
	markerSOF1   = 0xC1
	markerSOF2   = 0xC2
	markerDHT    = 0xC4
	markerJPG    = 0xC8
	markerDAC    = 0xCC
	markerSOF15  = 0xCF
	markerRST0   = 0xD0
	markerRST7   = 0xD7
	markerSOI    = 0xD8
	markerEOI    = 0xD9
	markerSOS    = 0xDA
	markerAPP1   = 0xE1
)

const (
	markerSize       = 2
	segmentLenSize   = 2
	exifHeader       = "Exif\x00\x00"
	sofHeightOffset  = 1
	sofWidthOffset   = 3
	sofDimensionSize = 5
	// renderablePrecision is the sample precision of the frames common decoders accept.
	renderablePrecision = 8
	// maxJPEGSegments bounds the segments walked before the image data.
	maxJPEGSegments = 1024
)

// jpegSegment is one marker segment; offset and length describe its payload.
type jpegSegment struct {
	marker byte
	offset int64
	length int64
}

func isJPEG(head []byte) bool {
	return len(head) >= markerSize && head[0] == markerPrefix && head[1] == markerSOI
}

func isSOF(marker byte) bool {
	return marker >= markerSOF0 && marker <= markerSOF15 &&
		marker != markerDHT && marker != markerJPG && marker != markerDAC
}

// walkJPEG visits the marker segments up to the start of scan; visit returns true to stop.
func walkJPEG(ctx context.Context, src source, visit func(jpegSegment) bool) error {
	header := make([]byte, markerSize+segmentLenSize)
	if err := src.readAt(header[:markerSize], 0); err != nil || !isJPEG(header) {
		return fmt.Errorf("no JPEG start of image: %w", ErrCorrupt)
	}

	position := int64(markerSize)
	for range maxJPEGSegments {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("walk JPEG: %w", err)
		}

		segment, next, done, err := readSegment(src, position, header)
		if err != nil || done {
			return err
		}

		if segment.length >= 0 && visit(segment) {
			return nil
		}

		position = next
	}

	return fmt.Errorf("more than %d JPEG segments: %w", maxJPEGSegments, ErrCorrupt)
}

// readSegment reads the marker at position. Fill bytes and standalone markers yield a
// segment with length -1; done is true at the start of scan or the end of image.
func readSegment(src source, position int64, header []byte) (jpegSegment, int64, bool, error) {
	if err := src.readAt(header[:markerSize], position); err != nil {
		return jpegSegment{}, 0, false, fmt.Errorf("read JPEG marker: %w", err)
	}

	if header[0] != markerPrefix {
		return jpegSegment{}, 0, false, fmt.Errorf("JPEG marker at %d: %w", position, ErrCorrupt)
	}

	marker := header[1]

	switch {
	case marker == markerPrefix:
		return jpegSegment{length: -1}, position + 1, false, nil
	case marker == markerTEM || (marker >= markerRST0 && marker <= markerRST7):
		return jpegSegment{length: -1}, position + markerSize, false, nil
	case marker == markerSOS || marker == markerEOI:
		return jpegSegment{}, 0, true, nil
	}

	if err := src.readAt(header, position); err != nil {
		return jpegSegment{}, 0, false, fmt.Errorf("read JPEG segment length: %w", err)
	}

	length := int64(binary.BigEndian.Uint16(header[markerSize:]))
	if length < segmentLenSize || position+markerSize+length > src.size {
		return jpegSegment{}, 0, false, fmt.Errorf("JPEG segment of %d bytes at %d: %w", length, position, ErrCorrupt)
	}

	segment := jpegSegment{marker: marker, offset: position + markerSize + segmentLenSize, length: length - segmentLenSize}

	return segment, position + markerSize + length, false, nil
}

// jpegLayout is what the header of a JPEG tells: its EXIF TIFF block and frame size.
type jpegLayout struct {
	exif                    *source
	frameWidth, frameHeight int
}

// scanJPEG finds the first EXIF APP1 block and the first frame header.
func scanJPEG(ctx context.Context, src source) (jpegLayout, error) {
	var (
		layout  jpegLayout
		readErr error
	)

	err := walkJPEG(ctx, src, func(segment jpegSegment) bool {
		switch {
		case segment.marker == markerAPP1 && layout.exif == nil:
			layout.exif, readErr = exifSection(src, segment)
		case isSOF(segment.marker):
			layout.frameWidth, layout.frameHeight, readErr = frameSize(src, segment)

			return true
		}

		return readErr != nil
	})
	if err != nil {
		return jpegLayout{}, err
	}

	return layout, readErr
}

// exifSection returns the TIFF block of an APP1 segment, nil for non-EXIF APP1 (XMP).
func exifSection(src source, segment jpegSegment) (*source, error) {
	if segment.length < int64(len(exifHeader)) {
		return nil, nil
	}

	signature := make([]byte, len(exifHeader))
	if err := src.readAt(signature, segment.offset); err != nil {
		return nil, err
	}

	if !bytes.Equal(signature, []byte(exifHeader)) {
		return nil, nil
	}

	section, err := src.section(segment.offset+int64(len(exifHeader)), segment.length-int64(len(exifHeader)))
	if err != nil {
		return nil, err
	}

	return &section, nil
}

func frameSize(src source, segment jpegSegment) (width, height int, err error) {
	if segment.length < sofDimensionSize {
		return 0, 0, fmt.Errorf("JPEG frame header of %d bytes: %w", segment.length, ErrCorrupt)
	}

	header := make([]byte, sofDimensionSize)
	if err := src.readAt(header, segment.offset); err != nil {
		return 0, 0, err
	}

	return int(binary.BigEndian.Uint16(header[sofWidthOffset:])), int(binary.BigEndian.Uint16(header[sofHeightOffset:])), nil
}

// readJPEGInfo reads the EXIF block of a JPEG; the frame size backs up a missing Exif size.
func readJPEGInfo(ctx context.Context, src source) (photo.Info, error) {
	layout, err := scanJPEG(ctx, src)
	if err != nil {
		return photo.Info{}, fmt.Errorf("scan JPEG: %w", err)
	}

	builder := infoBuilder{frameWidth: layout.frameWidth, frameHeight: layout.frameHeight}
	if err := readLayoutExif(ctx, layout, &builder); err != nil {
		return photo.Info{}, err
	}

	return builder.build(), nil
}

// readLayoutExif applies the EXIF block of a scanned JPEG, if it has one.
func readLayoutExif(ctx context.Context, layout jpegLayout, builder *infoBuilder) error {
	if layout.exif == nil {
		return nil
	}

	stream, err := openTIFF(*layout.exif)
	if err != nil {
		return fmt.Errorf("JPEG EXIF: %w", err)
	}

	if err := readTIFFInfo(ctx, stream, builder); err != nil {
		return fmt.Errorf("JPEG EXIF: %w", err)
	}

	return nil
}

// extractJPEGPreview returns the EXIF thumbnail of a JPEG.
func extractJPEGPreview(ctx context.Context, src source) ([]byte, error) {
	layout, err := scanJPEG(ctx, src)
	if err != nil {
		return nil, fmt.Errorf("scan JPEG: %w", err)
	}

	if layout.exif == nil {
		return nil, ErrNoPreview
	}

	return previewFromTIFF(ctx, *layout.exif)
}

// isRenderableJPEG reports a JPEG whose first frame is baseline, extended or progressive
// Huffman with 8-bit samples. That rejects the lossless JPEG (SOF3) raw data of CR2 and DNG
// files and 12-bit extended or progressive frames that common decoders refuse.
func isRenderableJPEG(ctx context.Context, src source) bool {
	renderable := false

	err := walkJPEG(ctx, src, func(segment jpegSegment) bool {
		if !isSOF(segment.marker) {
			return false
		}

		huffman := segment.marker == markerSOF0 || segment.marker == markerSOF1 || segment.marker == markerSOF2
		precision := make([]byte, 1)
		renderable = huffman && segment.length > 0 &&
			src.readAt(precision, segment.offset) == nil && precision[0] == renderablePrecision

		return true
	})

	return err == nil && renderable
}
