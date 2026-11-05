package photometa

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"math"
)

// Field widths allowed for iloc offsets, lengths and indexes.
const (
	fieldAbsent = 0
	field32Bit  = 4
	field64Bit  = 8
	// maxItemExtents bounds the extents of one item; Exif items have one.
	maxItemExtents = 64
)

// cursor reads big-endian fields from an in-memory box; after the first short read every
// call returns zero and err stays set.
type cursor struct {
	data     []byte
	position int
	err      error
}

func newCursor(data []byte) *cursor {
	return &cursor{data: data}
}

func (reader *cursor) bytes(count int) []byte {
	if reader.err != nil || count > len(reader.data)-reader.position {
		reader.err = io.ErrUnexpectedEOF

		return nil
	}

	value := reader.data[reader.position : reader.position+count]
	reader.position += count

	return value
}

func (reader *cursor) skip(count int) { reader.bytes(count) }

func (reader *cursor) uint8() uint8 {
	if value := reader.bytes(byteSize); value != nil {
		return value[0]
	}

	return 0
}

func (reader *cursor) uint16() uint16 {
	if value := reader.bytes(shortSize); value != nil {
		return binary.BigEndian.Uint16(value)
	}

	return 0
}

func (reader *cursor) uint32() uint32 {
	if value := reader.bytes(longSize); value != nil {
		return binary.BigEndian.Uint32(value)
	}

	return 0
}

// sized reads a field of width 0, 4 or 8 bytes.
func (reader *cursor) sized(width int) uint64 {
	switch width {
	case field32Bit:
		return uint64(reader.uint32())
	case field64Bit:
		if value := reader.bytes(field64Bit); value != nil {
			return binary.BigEndian.Uint64(value)
		}
	}

	return 0
}

// ilocLayout is the header of an iloc box.
type ilocLayout struct {
	version                                           uint8
	offsetSize, lengthSize, baseOffsetSize, indexSize int
}

func validFieldWidth(width int) bool {
	return width == fieldAbsent || width == field32Bit || width == field64Bit
}

// findItemLocation returns the iloc entry of itemID (versions 0, 1 and 2).
func findItemLocation(ctx context.Context, payload []byte, itemID uint32) (itemLocation, bool, error) {
	reader := newCursor(payload)

	layout, err := readIlocLayout(reader)
	if err != nil {
		return itemLocation{}, false, err
	}

	var count uint32
	if layout.version >= ilocVersionLongID {
		count = reader.uint32()
	} else {
		count = uint32(reader.uint16())
	}

	// Each entry consumes at least its ID, data_reference_index and extent_count, and its
	// extents are capped by maxItemExtents, so a hostile count ends at a short read after
	// a bounded amount of work.
	for range count {
		if err := ctx.Err(); err != nil {
			return itemLocation{}, false, fmt.Errorf("read iloc: %w", err)
		}

		id, location, err := readLocation(reader, layout, itemID)
		if err != nil {
			return itemLocation{}, false, err
		}

		if id == itemID {
			return location, true, nil
		}
	}

	return itemLocation{}, false, nil
}

// readIlocLayout reads the version and the field widths of an iloc box.
func readIlocLayout(reader *cursor) (ilocLayout, error) {
	layout := ilocLayout{version: reader.uint8()}
	reader.skip(fullBoxHeaderSize - 1)

	sizes, indexes := reader.uint8(), reader.uint8()
	layout.offsetSize, layout.lengthSize = int(sizes>>nibbleBits), int(sizes&nibbleMask)
	layout.baseOffsetSize = int(indexes >> nibbleBits)

	if layout.version > 0 {
		layout.indexSize = int(indexes & nibbleMask)
	}

	if reader.err != nil || !validFieldWidth(layout.offsetSize) || !validFieldWidth(layout.lengthSize) ||
		!validFieldWidth(layout.baseOffsetSize) || !validFieldWidth(layout.indexSize) {
		return ilocLayout{}, fmt.Errorf("iloc header: %w", ErrCorrupt)
	}

	return layout, nil
}

// readLocation reads one iloc entry; the extents are kept only for the wanted item.
func readLocation(reader *cursor, layout ilocLayout, wanted uint32) (uint32, itemLocation, error) {
	var id uint32
	if layout.version >= ilocVersionLongID {
		id = reader.uint32()
	} else {
		id = uint32(reader.uint16())
	}

	var location itemLocation
	if layout.version > 0 {
		location.construction = reader.uint16() & constructionMask
	}

	reader.uint16() // data_reference_index
	location.baseOffset = reader.sized(layout.baseOffsetSize)
	extentCount := reader.uint16()

	if reader.err != nil || extentCount > maxItemExtents {
		return 0, itemLocation{}, fmt.Errorf("iloc entry with %d extents: %w", extentCount, ErrCorrupt)
	}

	if id != wanted {
		// Other items are skipped in one step, so their extents cost no per-extent work.
		reader.skip(int(extentCount) * (layout.indexSize + layout.offsetSize + layout.lengthSize))
	} else {
		for range extentCount {
			reader.sized(layout.indexSize)
			item := extent{offset: reader.sized(layout.offsetSize), length: reader.sized(layout.lengthSize)}
			location.extents = append(location.extents, item)
		}
	}

	if reader.err != nil {
		return 0, itemLocation{}, fmt.Errorf("iloc entry: %w", ErrCorrupt)
	}

	return id, location, nil
}

// readItem concatenates the extents of an item stored in the file or in idat, bounded by
// maxExifBytes.
func readItem(src source, meta *heifMeta, location itemLocation) ([]byte, error) {
	var container source

	switch location.construction {
	case constructionFile:
		container = src
	case constructionIDAT:
		container = source{reader: bytes.NewReader(meta.idat), size: int64(len(meta.idat))}
	default:
		return nil, fmt.Errorf("item construction method %d: %w", location.construction, ErrUnsupported)
	}

	var data []byte

	for _, item := range location.extents {
		sum := location.baseOffset + item.offset
		offset, offsetOK := toInt64(sum)
		length, lengthOK := toInt64(item.length)

		if !offsetOK || !lengthOK || sum < item.offset || length == 0 || length > maxExifBytes-int64(len(data)) {
			return nil, fmt.Errorf("item extent %d+%d: %w", sum, item.length, ErrCorrupt)
		}

		// readBytes checks the offset against the container size.
		part, err := container.readBytes(offset, length, maxExifBytes)
		if err != nil {
			return nil, err
		}

		data = append(data, part...)
	}

	return data, nil
}

// toInt64 converts an iloc field, false when it does not fit an int64.
func toInt64(value uint64) (int64, bool) {
	if value > math.MaxInt64 {
		return 0, false
	}

	return int64(value), true
}
