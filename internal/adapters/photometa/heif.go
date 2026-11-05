package photometa

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"

	"github.com/magpeek/mivo/internal/domain/photo"
)

const (
	// maxMetaBytes bounds the HEIF meta box read into memory (grid images list many items).
	maxMetaBytes = 4 << 20
	// maxExifBytes bounds an Exif item; EXIF blocks are at most a few hundred KiB.
	maxExifBytes = 1 << 20
	// exifOffsetFieldSize is the exif_tiff_header_offset before an Exif item's data.
	exifOffsetFieldSize = 4
	itemTypeExif        = "Exif"
	constructionFile    = 0
	constructionIDAT    = 1
	constructionMask    = 0x0F
	nibbleBits          = 4
	nibbleMask          = 0x0F
	infeVersionShortID  = 2
	infeVersionLongID   = 3
	ilocVersionLongID   = 2
)

// extent is one byte range of an item.
type extent struct {
	offset uint64
	length uint64
}

// itemLocation is an item's iloc entry.
type itemLocation struct {
	construction uint16
	baseOffset   uint64
	extents      []extent
}

// heifMeta is the in-memory meta box with the ranges of its children.
type heifMeta struct {
	data       []byte
	iinf, iloc []byte
	idat       []byte
}

func readHEIFInfo(ctx context.Context, src source) (photo.Info, error) {
	exif, err := heifExif(ctx, src)
	if err != nil || exif == nil {
		return photo.Info{}, err
	}

	stream, err := openTIFF(*exif)
	if err != nil {
		return photo.Info{}, fmt.Errorf("HEIF Exif item: %w", err)
	}

	var builder infoBuilder
	if err := readTIFFInfo(ctx, stream, &builder); err != nil {
		return photo.Info{}, fmt.Errorf("HEIF Exif item: %w", err)
	}

	return builder.build(), nil
}

// extractHEIFPreview returns the EXIF thumbnail; HEIF's own thumbnails are HEVC/AV1
// items that FFmpeg decodes itself.
func extractHEIFPreview(ctx context.Context, src source) ([]byte, error) {
	exif, err := heifExif(ctx, src)
	if err != nil {
		return nil, err
	}

	if exif == nil {
		return nil, ErrNoPreview
	}

	return previewFromTIFF(ctx, *exif)
}

// heifExif returns the TIFF block of the first Exif item, nil when the file has none.
func heifExif(ctx context.Context, src source) (*source, error) {
	meta, err := readHEIFMeta(ctx, src)
	if err != nil || meta == nil {
		return nil, err
	}

	itemID, found := exifItemID(ctx, meta)
	if !found {
		return nil, nil
	}

	location, found, err := findItemLocation(ctx, meta.iloc, itemID)
	if err != nil || !found {
		return nil, err
	}

	data, err := readItem(src, meta, location)
	if err != nil {
		return nil, fmt.Errorf("read Exif item: %w", err)
	}

	return exifPayload(data)
}

// readHEIFMeta loads the top-level meta box and locates iinf, iloc and idat in it.
func readHEIFMeta(ctx context.Context, src source) (*heifMeta, error) {
	metaBox, found, err := findBox(ctx, src, src.whole(), boxQuery{kind: "meta"})
	if err != nil || !found {
		return nil, err
	}

	data, err := src.readBytes(metaBox.payloadStart, metaBox.payloadEnd-metaBox.payloadStart, maxMetaBytes)
	if err != nil {
		return nil, fmt.Errorf("read meta box: %w", err)
	}

	meta := &heifMeta{data: data}
	memory := source{reader: bytes.NewReader(data), size: int64(len(data))}

	err = walkBoxes(ctx, memory, span{start: fullBoxHeaderSize, end: memory.size}, func(child box) bool {
		payload := data[child.payloadStart:child.payloadEnd]
		switch child.kind {
		case "iinf":
			meta.iinf = payload
		case "iloc":
			meta.iloc = payload
		case "idat":
			meta.idat = payload
		}

		return false
	})
	if err != nil {
		return nil, fmt.Errorf("walk meta box: %w", err)
	}

	return meta, nil
}

// exifItemID returns the ID of the first item of type Exif in iinf.
func exifItemID(ctx context.Context, meta *heifMeta) (uint32, bool) {
	payload := newCursor(meta.iinf)
	version := payload.uint8()
	payload.skip(fullBoxHeaderSize - 1)

	if version == 0 {
		payload.uint16()
	} else {
		payload.uint32()
	}

	if payload.err != nil {
		return 0, false
	}

	memory := source{reader: bytes.NewReader(meta.iinf), size: int64(len(meta.iinf))}

	var (
		itemID uint32
		found  bool
	)

	_ = walkBoxes(ctx, memory, span{start: int64(payload.position), end: memory.size}, func(child box) bool {
		if child.kind != "infe" {
			return false
		}

		itemID, found = parseInfe(meta.iinf[child.payloadStart:child.payloadEnd])

		return found
	})

	return itemID, found
}

// parseInfe returns the item ID of an item info entry of type Exif (versions 2 and 3).
func parseInfe(payload []byte) (uint32, bool) {
	entry := newCursor(payload)
	version := entry.uint8()
	entry.skip(fullBoxHeaderSize - 1)

	var itemID uint32

	switch version {
	case infeVersionShortID:
		itemID = uint32(entry.uint16())
	case infeVersionLongID:
		itemID = entry.uint32()
	default:
		return 0, false
	}

	entry.uint16() // item_protection_index
	itemType := entry.bytes(brandSize)

	return itemID, entry.err == nil && string(itemType) == itemTypeExif
}

// exifPayload strips the exif_tiff_header_offset prefix of an Exif item.
func exifPayload(data []byte) (*source, error) {
	if len(data) < exifOffsetFieldSize {
		return nil, fmt.Errorf("exif item of %d bytes: %w", len(data), ErrCorrupt)
	}

	start := uint64(exifOffsetFieldSize) + uint64(binary.BigEndian.Uint32(data))
	if start > uint64(len(data)) {
		return nil, fmt.Errorf("exif TIFF offset %d past %d bytes: %w", start, len(data), ErrCorrupt)
	}

	tiff := data[start:]

	return &source{reader: bytes.NewReader(tiff), size: int64(len(tiff))}, nil
}
