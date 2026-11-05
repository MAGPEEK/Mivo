package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"math"
	"slices"
)

// A minimal TIFF writer for the photo corpus (task M3-15g): EXIF blocks of JPEG and HEIC
// files, a plain RGB TIFF and DNG stand-ins. It writes what photometa reads (TIFF IFDs) and
// nothing it does not need; it is test-fixture code, not a product encoder.

// TIFF field types (TIFF 6.0 section 2).
const (
	tiffByte      = 1
	tiffASCII     = 2
	tiffShort     = 3
	tiffLong      = 4
	tiffRational  = 5
	tiffUndefined = 7
)

const (
	// tiffHeaderSize is the byte order mark, the magic 42 and the offset of IFD0.
	tiffHeaderSize = 8
	tiffMagic      = 42
	// tiffEntrySize is one IFD entry: tag, type, count and a 4-byte value or offset.
	tiffEntrySize = 12
	// tiffInlineSize is the largest value stored in the entry itself.
	tiffInlineSize = 4
	// tiffCountSize and tiffNextSize frame the entries of a directory.
	tiffCountSize = 2
	tiffNextSize  = 4
	// rationalDenominator is the denominator of values that are no unit fraction.
	rationalDenominator = 100
)

// tiffRef points a LONG field at a directory or a blob of the same file; -1 means unused.
type tiffRef struct {
	directory int
	blob      int
}

// tiffField is one IFD entry. data is the encoded value; ref, when set, makes the field a
// single LONG whose value is the offset of the referenced directory or blob.
type tiffField struct {
	tag   uint16
	kind  uint16
	count uint32
	data  []byte
	ref   *tiffRef
}

// tiffFile is a TIFF stream: directories[0] is IFD0, the others are reached through
// reference fields (Exif, GPS, SubIFDs). Blobs (strips, JPEG previews) follow the directories.
type tiffFile struct {
	order       binary.AppendByteOrder
	directories [][]tiffField
	blobs       [][]byte
}

// addDirectory appends a directory and returns its index for references.
func (file *tiffFile) addDirectory(fields ...tiffField) int {
	file.directories = append(file.directories, fields)

	return len(file.directories) - 1
}

// addBlob appends a blob and returns its index for references.
func (file *tiffFile) addBlob(data []byte) int {
	file.blobs = append(file.blobs, data)

	return len(file.blobs) - 1
}

// encode lays out the header, the directories with their out-of-line values and the blobs,
// each starting on an even offset as TIFF requires.
func (file *tiffFile) encode() ([]byte, error) {
	directoryOffsets := make([]uint32, len(file.directories))
	position := tiffHeaderSize

	for index, fields := range file.directories {
		directoryOffsets[index] = uint32(position)
		position += even(directorySize(fields))
	}

	blobOffsets := make([]uint32, len(file.blobs))
	for index, blob := range file.blobs {
		blobOffsets[index] = uint32(position)
		position += even(len(blob))
	}

	if position > math.MaxUint32 {
		return nil, fmt.Errorf("TIFF stream of %d bytes exceeds 4 GiB", position)
	}

	out := bytes.NewBuffer(make([]byte, 0, position))
	out.Write(file.byteOrderMark())
	out.Write(file.order.AppendUint16(nil, tiffMagic))
	out.Write(file.order.AppendUint32(nil, tiffHeaderSize))

	for index, fields := range file.directories {
		resolved := file.resolve(fields, directoryOffsets, blobOffsets)
		out.Write(file.encodeDirectory(resolved, directoryOffsets[index]))
	}

	for _, blob := range file.blobs {
		out.Write(padEven(blob))
	}

	return out.Bytes(), nil
}

func (file *tiffFile) byteOrderMark() []byte {
	if file.order == binary.AppendByteOrder(binary.LittleEndian) {
		return []byte("II")
	}

	return []byte("MM")
}

// resolve replaces reference fields by LONG offsets and sorts the fields by tag.
func (file *tiffFile) resolve(fields []tiffField, directoryOffsets, blobOffsets []uint32) []tiffField {
	resolved := slices.Clone(fields)

	for index, field := range resolved {
		if field.ref == nil {
			continue
		}

		var offset uint32
		if field.ref.directory >= 0 {
			offset = directoryOffsets[field.ref.directory]
		} else {
			offset = blobOffsets[field.ref.blob]
		}

		resolved[index] = tiffField{tag: field.tag, kind: tiffLong, count: 1, data: file.order.AppendUint32(nil, offset)}
	}

	slices.SortFunc(resolved, func(left, right tiffField) int { return int(left.tag) - int(right.tag) })

	return resolved
}

// encodeDirectory writes the entries at offset, then their out-of-line values.
func (file *tiffFile) encodeDirectory(fields []tiffField, offset uint32) []byte {
	valueOffset := offset + uint32(tiffCountSize+tiffEntrySize*len(fields)+tiffNextSize) //nolint:gosec // few fields

	var entries, values []byte

	entries = file.order.AppendUint16(entries, uint16(len(fields))) //nolint:gosec // few fields

	for _, field := range fields {
		entries = file.order.AppendUint16(entries, field.tag)
		entries = file.order.AppendUint16(entries, field.kind)
		entries = file.order.AppendUint32(entries, field.count)

		if len(field.data) <= tiffInlineSize {
			entries = append(entries, make([]byte, tiffInlineSize)...)
			copy(entries[len(entries)-tiffInlineSize:], field.data)

			continue
		}

		entries = file.order.AppendUint32(entries, valueOffset+uint32(len(values))) //nolint:gosec // small values
		values = append(values, padEven(field.data)...)
	}

	entries = file.order.AppendUint32(entries, 0)

	return padEven(append(entries, values...))
}

// directorySize is the encoded size of a directory with its out-of-line values.
func directorySize(fields []tiffField) int {
	size := tiffCountSize + tiffEntrySize*len(fields) + tiffNextSize

	for _, field := range fields {
		if field.ref == nil && len(field.data) > tiffInlineSize {
			size += even(len(field.data))
		}
	}

	return size
}

func even(size int) int { return size + size%2 }

func padEven(data []byte) []byte {
	if len(data)%2 == 0 {
		return data
	}

	return append(slices.Clone(data), 0)
}

// Field constructors; the byte order is the file's.

func asciiField(tag uint16, text string) tiffField {
	data := append([]byte(text), 0)

	return tiffField{tag: tag, kind: tiffASCII, count: uint32(len(data)), data: data} //nolint:gosec // short text
}

func (file *tiffFile) shortField(tag uint16, values ...uint16) tiffField {
	var data []byte
	for _, value := range values {
		data = file.order.AppendUint16(data, value)
	}

	return tiffField{tag: tag, kind: tiffShort, count: uint32(len(values)), data: data} //nolint:gosec // few values
}

func (file *tiffFile) longField(tag uint16, values ...uint32) tiffField {
	var data []byte
	for _, value := range values {
		data = file.order.AppendUint32(data, value)
	}

	return tiffField{tag: tag, kind: tiffLong, count: uint32(len(values)), data: data} //nolint:gosec // few values
}

func (file *tiffFile) rationalField(tag uint16, values ...float64) tiffField {
	var data []byte

	for _, value := range values {
		numerator, denominator := rationalOf(value)
		data = file.order.AppendUint32(data, numerator)
		data = file.order.AppendUint32(data, denominator)
	}

	return tiffField{tag: tag, kind: tiffRational, count: uint32(len(values)), data: data} //nolint:gosec // few values
}

func bytesField(tag, kind uint16, data []byte) tiffField {
	return tiffField{tag: tag, kind: kind, count: uint32(len(data)), data: data} //nolint:gosec // short data
}

func directoryRef(tag uint16, directory int) tiffField {
	return tiffField{tag: tag, kind: tiffLong, count: 1, ref: &tiffRef{directory: directory, blob: -1}}
}

func blobRef(tag uint16, blob int) tiffField {
	return tiffField{tag: tag, kind: tiffLong, count: 1, ref: &tiffRef{directory: -1, blob: blob}}
}

// rationalOf writes a positive value below one as the unit fraction 1/n it is (exposure
// times), anything else in hundredths. Dividing the parts again gives the same float64.
func rationalOf(value float64) (numerator, denominator uint32) {
	if value > 0 && value < 1 {
		if reciprocal := math.Round(1 / value); 1/reciprocal == value {
			return 1, uint32(reciprocal)
		}
	}

	return uint32(math.Round(value * rationalDenominator)), rationalDenominator
}
