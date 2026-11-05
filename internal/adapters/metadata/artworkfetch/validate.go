package artworkfetch

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"io"
	"net/http"

	"github.com/magpeek/mivo/internal/domain"
)

// Limits of an accepted picture (ADR 0031 §11).
const (
	// MaxImageBytes is the largest accepted picture.
	MaxImageBytes = 15 << 20
	// MaxImageDimension bounds width and height so a small file cannot expand into a
	// huge bitmap when it is rendered later.
	MaxImageDimension = 16384
)

// Sniffed content types (http.DetectContentType) that are accepted.
const (
	contentTypeJPEG = "image/jpeg"
	contentTypePNG  = "image/png"
	contentTypeWebP = "image/webp"
)

// File name extensions of the stored pictures.
const (
	extensionJPEG = "jpg"
	extensionPNG  = "png"
	extensionWebP = "webp"
)

// storedExtensions lists every extension a stored picture can have.
func storedExtensions() []string {
	return []string{extensionJPEG, extensionPNG, extensionWebP}
}

// End markers of complete JPEG and PNG files.
const (
	jpegEndOfImage = "\xFF\xD9"
	pngEndChunk    = "IEND\xAE\x42\x60\x82"
)

// imageInfo describes an accepted picture.
type imageInfo struct {
	Extension string
	Width     int
	Height    int
}

// validateImage accepts JPEG, PNG and WebP pictures by their bytes, never by a header the
// server sent. Everything else, oversized, truncated or implausibly large pictures fail
// with domain.ErrInvalid.
func validateImage(body []byte) (imageInfo, error) {
	if len(body) > MaxImageBytes {
		return imageInfo{}, fmt.Errorf("picture of %d bytes exceeds %d: %w", len(body), MaxImageBytes, domain.ErrInvalid)
	}

	info, err := decodeHeader(body)
	if err != nil {
		return imageInfo{}, err
	}

	if info.Width <= 0 || info.Height <= 0 || info.Width > MaxImageDimension || info.Height > MaxImageDimension {
		return imageInfo{}, fmt.Errorf("picture dimensions %dx%d out of range: %w", info.Width, info.Height, domain.ErrInvalid)
	}

	return info, nil
}

func decodeHeader(body []byte) (imageInfo, error) {
	switch contentType := http.DetectContentType(body); contentType {
	case contentTypeJPEG:
		return decodeStandard(body, extensionJPEG, jpeg.DecodeConfig, jpegEndOfImage)
	case contentTypePNG:
		return decodeStandard(body, extensionPNG, png.DecodeConfig, pngEndChunk)
	case contentTypeWebP:
		return decodeWebP(body)
	default:
		return imageInfo{}, fmt.Errorf("content type %q is not an accepted picture: %w", contentType, domain.ErrInvalid)
	}
}

// decodeStandard reads the dimensions with the standard library decoder and requires the
// format's end marker, because a decode-config check alone passes a cut-off file.
func decodeStandard(
	body []byte, extension string, decodeConfig func(r io.Reader) (image.Config, error), trailer string,
) (imageInfo, error) {
	config, err := decodeConfig(bytes.NewReader(body))
	if err != nil {
		return imageInfo{}, fmt.Errorf("read %s header: %w: %w", extension, err, domain.ErrInvalid)
	}

	if !bytes.HasSuffix(bytes.TrimRight(body, "\x00"), []byte(trailer)) {
		return imageInfo{}, fmt.Errorf("%s picture is truncated: %w", extension, domain.ErrInvalid)
	}

	return imageInfo{Extension: extension, Width: config.Width, Height: config.Height}, nil
}

// WebP layout (RIFF container, https://developers.google.com/speed/webp/docs/riff_container).
const (
	riffHeaderLength  = 12
	riffSizeOffset    = 4
	riffSizeAdjust    = 8
	chunkHeaderLength = 8
	chunkDataOffset   = riffHeaderLength + chunkHeaderLength
	vp8HeaderLength   = 10
	vp8LHeaderLength  = 5
	vp8XHeaderLength  = 10
	vp8LSignature     = 0x2F
	dimension14Mask   = 0x3FFF
	dimension14Bits   = 14
)

// vp8StartCode is the key frame start code of a lossy WebP bitstream.
const vp8StartCode = "\x9D\x01\x2A"

// decodeWebP reads the dimensions from the first chunk and checks that the RIFF container
// is complete; the standard library has no WebP decoder.
func decodeWebP(body []byte) (imageInfo, error) {
	if len(body) < chunkDataOffset {
		return imageInfo{}, fmt.Errorf("webp picture is truncated: %w", domain.ErrInvalid)
	}

	riffSize := int64(binary.LittleEndian.Uint32(body[riffSizeOffset:]))
	if int64(len(body)) < riffSize+riffSizeAdjust {
		return imageInfo{}, fmt.Errorf("webp picture is truncated: %w", domain.ErrInvalid)
	}

	width, height, err := webPDimensions(string(body[riffHeaderLength:riffHeaderLength+4]), body[chunkDataOffset:])
	if err != nil {
		return imageInfo{}, err
	}

	return imageInfo{Extension: extensionWebP, Width: width, Height: height}, nil
}

func webPDimensions(chunk string, data []byte) (width, height int, err error) {
	switch {
	case chunk == "VP8 " && len(data) >= vp8HeaderLength && string(data[3:6]) == vp8StartCode:
		width = int(binary.LittleEndian.Uint16(data[6:]) & dimension14Mask)
		height = int(binary.LittleEndian.Uint16(data[8:]) & dimension14Mask)
	case chunk == "VP8L" && len(data) >= vp8LHeaderLength && data[0] == vp8LSignature:
		bits := binary.LittleEndian.Uint32(data[1:])
		width = int(bits&dimension14Mask) + 1
		height = int((bits>>dimension14Bits)&dimension14Mask) + 1
	case chunk == "VP8X" && len(data) >= vp8XHeaderLength:
		width = int(uint24(data[4:])) + 1
		height = int(uint24(data[7:])) + 1
	default:
		return 0, 0, fmt.Errorf("unreadable webp chunk %q: %w", chunk, domain.ErrInvalid)
	}

	return width, height, nil
}

func uint24(data []byte) uint32 {
	return uint32(data[0]) | uint32(data[1])<<8 | uint32(data[2])<<16
}
