package photometa

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"slices"
)

// format is the container layout of a photo file.
type format int

const (
	formatUnknown format = iota
	formatJPEG
	formatTIFF
	formatRAF
	formatHEIF
	formatCR3
)

const (
	// sniffBytes covers the longest magic (the 16-byte RAF signature).
	sniffBytes = 16
	// maxFtypBytes bounds the brand list read from an ISOBMFF ftyp box.
	maxFtypBytes = 256
	// brandSize is the length of an ISOBMFF brand.
	brandSize = 4
	// ftypMinorVersionSize separates the major brand from the compatible brands.
	ftypMinorVersionSize = 4
)

const (
	brandCR3 = "crx "
	boxFtyp  = "ftyp"
)

// heifBrands are the ISOBMFF brands of still images and image sequences with HEVC or AV1.
func heifBrands() []string {
	return []string{"heic", "heix", "heim", "heis", "hevc", "hevx", "mif1", "mif2", "msf1", "avif", "avis"}
}

// detectFormat sniffs the first bytes of src.
func detectFormat(src source) (format, error) {
	if src.size < 0 {
		return formatUnknown, fmt.Errorf("negative file size %d: %w", src.size, ErrCorrupt)
	}

	head := make([]byte, min(src.size, sniffBytes))
	if err := src.readAt(head, 0); err != nil {
		return formatUnknown, fmt.Errorf("sniff format: %w", err)
	}

	switch {
	case isJPEG(head):
		return formatJPEG, nil
	case isTIFFHeader(head):
		return formatTIFF, nil
	case bytes.HasPrefix(head, []byte(rafMagic)):
		return formatRAF, nil
	case len(head) >= boxHeaderSize && string(head[4:boxHeaderSize]) == boxFtyp:
		return detectISOBMFF(src, head)
	}

	return formatUnknown, ErrUnsupported
}

// detectISOBMFF tells CR3 from HEIF by the ftyp brands.
func detectISOBMFF(src source, head []byte) (format, error) {
	declared := int64(binary.BigEndian.Uint32(head))
	length := min(declared, maxFtypBytes, src.size)
	if length < boxHeaderSize+brandSize {
		return formatUnknown, fmt.Errorf("ftyp box of %d bytes: %w", declared, ErrCorrupt)
	}

	payload, err := src.readBytes(boxHeaderSize, length-boxHeaderSize, maxFtypBytes)
	if err != nil {
		return formatUnknown, fmt.Errorf("read ftyp: %w", err)
	}

	brands := []string{string(payload[:brandSize])}
	for offset := brandSize + ftypMinorVersionSize; offset+brandSize <= len(payload); offset += brandSize {
		brands = append(brands, string(payload[offset:offset+brandSize]))
	}

	if slices.Contains(brands, brandCR3) {
		return formatCR3, nil
	}

	for _, brand := range brands {
		if slices.Contains(heifBrands(), brand) {
			return formatHEIF, nil
		}
	}

	return formatUnknown, ErrUnsupported
}
