package photometa

import (
	"context"
	"encoding/binary"
	"fmt"
	"math"
)

const (
	tiffHeaderSize = 8
	// tiffMagicEnd is where the byte order mark and the magic number end.
	tiffMagicEnd = 4
	ifdCountSize = 2
	ifdEntrySize = 12
	ifdNextSize  = 4
	inlineSize   = 4
	// maxIFDEntries bounds one IFD; real files have well under 100 entries.
	maxIFDEntries = 1024
	// maxIFDsPerStream bounds the IFDs read from one TIFF stream (chain, SubIFDs, Exif, GPS).
	maxIFDsPerStream = 32
	// maxStringBytes bounds ASCII values; Make, Model, Lens and dates are short.
	maxStringBytes = 256
	// maxSubIFDs bounds the SubIFD pointers followed per IFD.
	maxSubIFDs = 8
)

// TIFF byte order marks and the magic numbers of TIFF and its RAW variants.
const (
	littleEndianMark = "II"
	bigEndianMark    = "MM"
	tiffMagic        = 42
	orfMagic         = 0x4F52 // Olympus ORF "RO"
	orfAltMagic      = 0x5352 // Olympus ORF "RS"
	rw2Magic         = 0x55   // Panasonic RW2
)

// TIFF field types (TIFF 6.0 section 2).
const (
	typeByte      = 1
	typeASCII     = 2
	typeShort     = 3
	typeLong      = 4
	typeRational  = 5
	typeSByte     = 6
	typeUndefined = 7
	typeSShort    = 8
	typeSLong     = 9
	typeSRational = 10
	typeFloat     = 11
	typeDouble    = 12
	typeIFD       = 13
)

const (
	byteSize     = 1
	shortSize    = 2
	longSize     = 4
	rationalSize = 8
)

// ifdEntry is one 12-byte directory entry; inline holds the value or its offset.
type ifdEntry struct {
	kind   uint16
	count  uint32
	inline [inlineSize]byte
}

// ifd is one image file directory: its entries by tag (first wins) and the next offset.
type ifd struct {
	entries map[uint16]ifdEntry
	next    uint32
}

// tiffStream is one TIFF structure; offsets in it are relative to src.
type tiffStream struct {
	src   source
	order binary.ByteOrder
	first uint32
	cache map[uint32]ifd
	reads int
}

// isTIFFHeader reports a TIFF byte order mark followed by a known magic number.
func isTIFFHeader(head []byte) bool {
	order, ok := byteOrderOf(head)
	if !ok || len(head) < tiffMagicEnd {
		return false
	}

	switch order.Uint16(head[2:]) {
	case tiffMagic, orfMagic, orfAltMagic, rw2Magic:
		return true
	}

	return false
}

func byteOrderOf(head []byte) (binary.ByteOrder, bool) {
	if len(head) < len(littleEndianMark) {
		return nil, false
	}

	switch string(head[:len(littleEndianMark)]) {
	case littleEndianMark:
		return binary.LittleEndian, true
	case bigEndianMark:
		return binary.BigEndian, true
	}

	return nil, false
}

// openTIFF parses the TIFF header at the start of src.
func openTIFF(src source) (*tiffStream, error) {
	head := make([]byte, tiffHeaderSize)
	if err := src.readAt(head, 0); err != nil {
		return nil, fmt.Errorf("read TIFF header: %w", err)
	}

	if !isTIFFHeader(head) {
		return nil, fmt.Errorf("no TIFF header: %w", ErrCorrupt)
	}

	order, _ := byteOrderOf(head)

	return &tiffStream{
		src:   src,
		order: order,
		first: order.Uint32(head[4:]),
		cache: map[uint32]ifd{},
	}, nil
}

// readIFD reads the directory at offset. A directory reached again through another pointer
// tag (Exif, GPS and SubIFD pointers may share an offset) comes from the cache; loops in
// the next-IFD chain are detected by the chain walk. More than maxIFDsPerStream reads are
// ErrCorrupt.
func (stream *tiffStream) readIFD(ctx context.Context, offset uint32) (ifd, error) {
	if err := ctx.Err(); err != nil {
		return ifd{}, fmt.Errorf("read IFD: %w", err)
	}

	if cached, seen := stream.cache[offset]; seen {
		return cached, nil
	}

	if stream.reads >= maxIFDsPerStream {
		return ifd{}, fmt.Errorf("more than %d IFDs: %w", maxIFDsPerStream, ErrCorrupt)
	}

	stream.reads++

	countBytes := make([]byte, ifdCountSize)
	if err := stream.src.readAt(countBytes, int64(offset)); err != nil {
		return ifd{}, fmt.Errorf("read IFD at %d: %w", offset, err)
	}

	count := int64(stream.order.Uint16(countBytes))
	if count > maxIFDEntries {
		return ifd{}, fmt.Errorf("IFD at %d has %d entries: %w", offset, count, ErrCorrupt)
	}

	body, err := stream.src.readBytes(int64(offset)+ifdCountSize, count*ifdEntrySize+ifdNextSize,
		maxIFDEntries*ifdEntrySize+ifdNextSize)
	if err != nil {
		return ifd{}, fmt.Errorf("read IFD at %d: %w", offset, err)
	}

	directory := stream.parseIFD(body, int(count))
	stream.cache[offset] = directory

	return directory, nil
}

func (stream *tiffStream) parseIFD(body []byte, count int) ifd {
	directory := ifd{entries: make(map[uint16]ifdEntry, count)}

	for index := range count {
		raw := body[index*ifdEntrySize : (index+1)*ifdEntrySize]
		tag := stream.order.Uint16(raw)

		if _, duplicate := directory.entries[tag]; duplicate {
			continue
		}

		entry := ifdEntry{kind: stream.order.Uint16(raw[2:]), count: stream.order.Uint32(raw[4:])}
		copy(entry.inline[:], raw[8:])
		directory.entries[tag] = entry
	}

	directory.next = stream.order.Uint32(body[count*ifdEntrySize:])

	return directory
}

// typeSize is the byte size of one value of a TIFF field type, 0 for unknown types.
func typeSize(kind uint16) int64 {
	switch kind {
	case typeByte, typeASCII, typeSByte, typeUndefined:
		return byteSize
	case typeShort, typeSShort:
		return shortSize
	case typeLong, typeSLong, typeFloat, typeIFD:
		return longSize
	case typeRational, typeSRational, typeDouble:
		return rationalSize
	}

	return 0
}

// valueLocation returns where the value of entry lives and its total byte size.
func (stream *tiffStream) valueLocation(entry ifdEntry) (offset, size int64, inline bool) {
	size = typeSize(entry.kind) * int64(entry.count)
	if size <= inlineSize {
		return 0, size, true
	}

	return int64(stream.order.Uint32(entry.inline[:])), size, false
}

// readValue returns at most limit bytes of the value of entry; false when unreadable.
func (stream *tiffStream) readValue(entry ifdEntry, limit int64) ([]byte, bool) {
	offset, size, inline := stream.valueLocation(entry)
	length := min(size, limit)

	if inline {
		return entry.inline[:length], true
	}

	value, err := stream.src.readBytes(offset, length, limit)

	return value, err == nil
}

// ascii returns the string value of tag, empty when absent or not ASCII.
func (stream *tiffStream) ascii(directory ifd, tag uint16) string {
	entry, ok := directory.entries[tag]
	if !ok || entry.kind != typeASCII {
		return ""
	}

	value, ok := stream.readValue(entry, maxStringBytes)
	if !ok {
		return ""
	}

	return cleanString(value)
}

// uints returns up to limit unsigned values of tag (BYTE, SHORT, LONG or IFD).
func (stream *tiffStream) uints(directory ifd, tag uint16, limit int) []uint32 {
	entry, ok := directory.entries[tag]
	if !ok {
		return nil
	}

	size := typeSize(entry.kind)
	switch entry.kind {
	case typeByte, typeShort, typeLong, typeIFD:
	default:
		return nil
	}

	count := min(int64(entry.count), int64(limit))
	value, ok := stream.readValue(entry, count*size)
	if !ok {
		return nil
	}

	values := make([]uint32, 0, count)
	for index := int64(0); index < count; index++ {
		values = append(values, stream.decodeUint(value[index*size:], entry.kind))
	}

	return values
}

func (stream *tiffStream) decodeUint(value []byte, kind uint16) uint32 {
	switch kind {
	case typeByte:
		return uint32(value[0])
	case typeShort:
		return uint32(stream.order.Uint16(value))
	}

	return stream.order.Uint32(value)
}

// uint returns the first unsigned value of tag.
func (stream *tiffStream) uint(directory ifd, tag uint16) (uint32, bool) {
	values := stream.uints(directory, tag, 1)
	if len(values) == 0 {
		return 0, false
	}

	return values[0], true
}

// rational returns the value at index of a RATIONAL or SRATIONAL tag; false when absent
// or the denominator is zero.
func (stream *tiffStream) rational(directory ifd, tag uint16, index int) (float64, bool) {
	entry, ok := directory.entries[tag]
	if !ok || (entry.kind != typeRational && entry.kind != typeSRational) || int64(entry.count) <= int64(index) {
		return 0, false
	}

	value, ok := stream.readValue(entry, int64(index+1)*rationalSize)
	if !ok {
		return 0, false
	}

	raw := value[index*rationalSize:]
	numerator, denominator := stream.order.Uint32(raw), stream.order.Uint32(raw[longSize:])

	if denominator == 0 {
		return 0, false
	}

	if entry.kind == typeSRational {
		return float64(int32(numerator)) / float64(int32(denominator)), true //nolint:gosec // reinterpreting SRATIONAL bits is the intent
	}

	return float64(numerator) / float64(denominator), true
}

// cleanString trims NUL padding and blanks and drops invalid UTF-8.
func cleanString(value []byte) string {
	end := len(value)
	for index, char := range value {
		if char == 0 {
			end = index

			break
		}
	}

	return trimSpaceValid(string(value[:end]))
}

// toInt converts a value read from the file to int, false when it would not fit a 32-bit
// int (the 386 build).
func toInt(value uint32) (int, bool) {
	if value > math.MaxInt32 {
		return 0, false
	}

	return int(value), true
}
