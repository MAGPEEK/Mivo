//nolint:gosec // fixture writer: sizes are small test constants
package photometa

import (
	"bytes"
	"encoding/binary"
)

// testEntry is one IFD entry for the fixture builder; data is the encoded value.
type testEntry struct {
	tag   uint16
	kind  uint16
	count uint32
	data  []byte
}

// tiffBuilder lays out a synthetic TIFF stream. Child IFDs are added first so their offsets
// are known when IFD0 is written; setFirst points the header at IFD0.
type tiffBuilder struct {
	order binary.ByteOrder
	buf   []byte
}

func newTIFFBuilder(order binary.ByteOrder, magic uint16) *tiffBuilder {
	builder := &tiffBuilder{order: order, buf: make([]byte, tiffHeaderSize)}
	if order == binary.ByteOrder(binary.LittleEndian) {
		copy(builder.buf, littleEndianMark)
	} else {
		copy(builder.buf, bigEndianMark)
	}

	order.PutUint16(builder.buf[2:], magic)

	return builder
}

func (builder *tiffBuilder) align() {
	if len(builder.buf)%2 == 1 {
		builder.buf = append(builder.buf, 0)
	}
}

// addBlob appends raw data and returns its offset.
func (builder *tiffBuilder) addBlob(data []byte) uint32 {
	builder.align()
	offset := uint32(len(builder.buf))
	builder.buf = append(builder.buf, data...)

	return offset
}

// addIFD appends a directory (with out-of-line values after it) and returns its offset.
func (builder *tiffBuilder) addIFD(entries []testEntry, next uint32) uint32 {
	builder.align()
	offset := uint32(len(builder.buf))
	body := make([]byte, ifdCountSize+len(entries)*ifdEntrySize+ifdNextSize)
	builder.order.PutUint16(body, uint16(len(entries)))

	builder.buf = append(builder.buf, body...)
	for index, entry := range entries {
		at := int(offset) + ifdCountSize + index*ifdEntrySize
		builder.order.PutUint16(builder.buf[at:], entry.tag)
		builder.order.PutUint16(builder.buf[at+2:], entry.kind)
		builder.order.PutUint32(builder.buf[at+4:], entry.count)

		if len(entry.data) <= inlineSize {
			copy(builder.buf[at+8:], entry.data)

			continue
		}

		dataOffset := builder.addBlob(entry.data)
		builder.order.PutUint32(builder.buf[at+8:], dataOffset)
	}

	builder.order.PutUint32(builder.buf[int(offset)+ifdCountSize+len(entries)*ifdEntrySize:], next)

	return offset
}

func (builder *tiffBuilder) setFirst(offset uint32) {
	builder.order.PutUint32(builder.buf[4:], offset)
}

func (builder *tiffBuilder) bytes() []byte {
	return builder.buf
}

func (builder *tiffBuilder) ascii(tag uint16, value string) testEntry {
	data := append([]byte(value), 0)

	return testEntry{tag: tag, kind: typeASCII, count: uint32(len(data)), data: data}
}

func (builder *tiffBuilder) short(tag uint16, value uint16) testEntry {
	data := make([]byte, shortSize)
	builder.order.PutUint16(data, value)

	return testEntry{tag: tag, kind: typeShort, count: 1, data: data}
}

func (builder *tiffBuilder) long(tag uint16, values ...uint32) testEntry {
	data := make([]byte, longSize*len(values))
	for index, value := range values {
		builder.order.PutUint32(data[index*longSize:], value)
	}

	return testEntry{tag: tag, kind: typeLong, count: uint32(len(values)), data: data}
}

func (builder *tiffBuilder) rational(tag uint16, kind uint16, pairs ...uint32) testEntry {
	data := make([]byte, longSize*len(pairs))
	for index, value := range pairs {
		builder.order.PutUint32(data[index*longSize:], value)
	}

	return testEntry{tag: tag, kind: kind, count: uint32(len(pairs) / 2), data: data}
}

// testJPEG is a minimal baseline JPEG header: SOI, SOF0 with the size, SOS, data, EOI.
func testJPEG(width, height uint16, marker byte) []byte {
	var out bytes.Buffer
	out.Write([]byte{markerPrefix, markerSOI})
	writeSegment(&out, marker, sofPayload(width, height))
	writeSegment(&out, markerSOS, []byte{1, 1, 0, 0, 0x3F, 0})
	out.Write([]byte{0x12, 0x34, markerPrefix, markerEOI})

	return out.Bytes()
}

func sofPayload(width, height uint16) []byte {
	payload := []byte{8, 0, 0, 0, 0, 1, 1, 0x11, 0}
	binary.BigEndian.PutUint16(payload[sofHeightOffset:], height)
	binary.BigEndian.PutUint16(payload[sofWidthOffset:], width)

	return payload
}

func writeSegment(out *bytes.Buffer, marker byte, payload []byte) {
	out.Write([]byte{markerPrefix, marker})
	_ = binary.Write(out, binary.BigEndian, uint16(len(payload)+segmentLenSize))
	out.Write(payload)
}

// jpegWithExif wraps a TIFF stream in an APP1 segment ahead of the frame header.
func jpegWithExif(tiff []byte, width, height uint16) []byte {
	var out bytes.Buffer
	out.Write([]byte{markerPrefix, markerSOI})
	writeSegment(&out, 0xE0, []byte("JFIF\x00\x01\x02"))
	writeSegment(&out, markerAPP1, []byte("http://ns.adobe.com/xap/1.0/\x00<x/>"))
	writeSegment(&out, markerAPP1, append([]byte(exifHeader), tiff...))
	out.Write([]byte{markerPrefix, markerPrefix}) // fill byte
	writeSegment(&out, markerDHT, []byte{0})
	writeSegment(&out, markerSOF0, sofPayload(width, height))
	writeSegment(&out, markerSOS, []byte{1, 1, 0, 0, 0x3F, 0})
	out.Write([]byte{0x12, markerPrefix, markerEOI})

	return out.Bytes()
}

// isoBox encodes an ISOBMFF box.
func isoBox(kind string, payload ...[]byte) []byte {
	body := bytes.Join(payload, nil)
	out := make([]byte, boxHeaderSize, boxHeaderSize+len(body))
	binary.BigEndian.PutUint32(out, uint32(boxHeaderSize+len(body)))
	copy(out[4:], kind)

	return append(out, body...)
}

func uuidBox(uuid string, payload ...[]byte) []byte {
	return isoBox(boxTypeUUID, append([][]byte{[]byte(uuid)}, payload...)...)
}

func be16(value uint16) []byte { return binary.BigEndian.AppendUint16(nil, value) }
func be32(value uint32) []byte { return binary.BigEndian.AppendUint32(nil, value) }

// heifFile builds ftyp + meta (iinf with an image and an Exif item, iloc) + mdat holding the
// Exif item. ilocVersion selects the iloc layout; inIDAT stores the item in idat instead.
func heifFile(brand string, exifTIFF []byte, ilocVersion uint8, inIDAT bool) []byte {
	ftyp := isoBox("ftyp", []byte(brand), be32(0), []byte("mif1"))
	item := append(append(be32(6), []byte(exifHeader)...), exifTIFF...)

	build := func(itemOffset uint32) []byte {
		iinf := isoBox("iinf", []byte{0, 0, 0, 0}, be16(2),
			isoBox("infe", []byte{2, 0, 0, 0}, be16(1), be16(0), []byte("hvc1"), []byte{0}),
			isoBox("infe", []byte{2, 0, 0, 0}, be16(2), be16(0), []byte(itemTypeExif), []byte{0}))
		children := [][]byte{{0, 0, 0, 0}, isoBox("hdlr", make([]byte, 24)), iinf, ilocBox(ilocVersion, inIDAT, itemOffset, uint32(len(item)))}

		if inIDAT {
			children = append(children, isoBox("idat", item))
		}

		return isoBox("meta", children...)
	}

	meta := build(0)
	if inIDAT {
		return append(ftyp, meta...)
	}

	offset := uint32(len(ftyp) + len(meta) + boxHeaderSize)

	return bytes.Join([][]byte{ftyp, build(offset), isoBox("mdat", item)}, nil)
}

// ilocBox lists item 1 (unused) and item 2 (the Exif item) with 4-byte offsets and lengths.
func ilocBox(version uint8, inIDAT bool, offset, length uint32) []byte {
	payload := []byte{version, 0, 0, 0, 0x44, 0x00}
	id := func(value uint32) []byte {
		if version >= ilocVersionLongID {
			return be32(value)
		}

		return be16(uint16(value))
	}

	if version >= ilocVersionLongID {
		payload = append(payload, be32(2)...)
	} else {
		payload = append(payload, be16(2)...)
	}

	method := uint16(constructionFile)
	if inIDAT {
		method = constructionIDAT
		offset = 0
	}

	for _, itemID := range []uint32{1, 2} {
		payload = append(payload, id(itemID)...)
		if version > 0 {
			payload = append(payload, be16(method)...)
		}

		payload = append(payload, be16(0)...) // data_reference_index
		payload = append(payload, be16(1)...) // extent_count
		payload = append(payload, be32(offset)...)
		payload = append(payload, be32(length)...)
	}

	return isoBox("iloc", payload)
}

// cr3File builds ftyp(crx) + moov{Canon uuid{CMT1, CMT2, CMT4}} + preview uuid{PRVW}.
func cr3File(cmt1, cmt2, cmt4, preview []byte) []byte {
	ftyp := isoBox("ftyp", []byte(brandCR3), be32(1), []byte(brandCR3), []byte("isom"))
	canon := uuidBox(cr3CanonUUID, isoBox("CNCV", []byte("CanonCR3_001/00.09.00/00.00.00")),
		isoBox("CMT1", cmt1), isoBox("CMT2", cmt2), isoBox("CMT4", cmt4))
	moov := isoBox("moov", canon)

	prvwPayload := bytes.Join([][]byte{be32(0), be16(1), be16(1620), be16(1080), be16(1), be32(uint32(len(preview))), preview}, nil)
	previewBox := uuidBox(cr3PreviewUUID, make([]byte, cr3PreviewSkip), isoBox("PRVW", prvwPayload))

	return bytes.Join([][]byte{ftyp, moov, previewBox, isoBox("mdat", []byte{0, 0})}, nil)
}

// rafFile builds a RAF header pointing at an embedded JPEG.
func rafFile(embedded []byte) []byte {
	header := make([]byte, 100)
	copy(header, rafMagic)
	copy(header[16:], "0201FF129502")
	binary.BigEndian.PutUint32(header[rafJPEGOffsetAt:], uint32(len(header)))
	binary.BigEndian.PutUint32(header[rafJPEGLengthAt:], uint32(len(embedded)))

	return append(header, embedded...)
}
