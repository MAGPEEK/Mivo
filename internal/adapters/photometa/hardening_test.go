package photometa

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"testing"
)

func TestCorruptInputIsAnError(t *testing.T) {
	tests := []struct {
		name string
		data []byte
		want error
	}{
		{"empty", nil, ErrUnsupported},
		{"PNG", []byte("\x89PNG\r\n\x1a\n0000000000"), ErrUnsupported},
		{"MP4 brand", isoBox("ftyp", []byte("isom"), be32(0), []byte("mp41")), ErrUnsupported},
		{"tiny ftyp", []byte("\x00\x00\x00\x08ftyp"), ErrCorrupt},
		{"truncated TIFF header", []byte("II*\x00"), ErrCorrupt},
		{"IFD0 offset outside", []byte("II*\x00\xFF\x00\x00\x00"), ErrCorrupt},
		{"too many entries", manyEntriesTIFF(), ErrCorrupt},
		{"IFD past the end", truncatedIFD(), ErrCorrupt},
		{"Exif IFD outside", pointerTIFF(tagExifIFD, 0xFFFFFF), ErrCorrupt},
		{"GPS IFD outside", pointerTIFF(tagGPSIFD, 0xFFFFFF), ErrCorrupt},
		{"JPEG segment overruns", []byte{0xFF, 0xD8, 0xFF, 0xE1, 0xFF, 0xFF, 0}, ErrCorrupt},
		{"JPEG segment length < 2", []byte{0xFF, 0xD8, 0xFF, 0xE1, 0x00, 0x01, 0, 0}, ErrCorrupt},
		{"JPEG garbage marker", []byte{0xFF, 0xD8, 0x00, 0x00}, ErrCorrupt},
		{"JPEG without scan", []byte{0xFF, 0xD8, 0xFF, 0xD0}, ErrCorrupt},
		{"JPEG frame header short", shortFrameJPEG(), ErrCorrupt},
		{"JPEG EXIF without TIFF", jpegWithExif([]byte("XX"), 1, 1), ErrCorrupt},
		{"JPEG too many segments", manySegmentsJPEG(), ErrCorrupt},
		{"box past the end", append(isoBox("ftyp", []byte("heic"), be32(0)), 0, 0, 0, 0x40, 'm', 'e', 't', 'a'), ErrCorrupt},
		{"box smaller than header", append(isoBox("ftyp", []byte("heic"), be32(0)), 0, 0, 0, 4, 'm', 'e', 't', 'a'), ErrCorrupt},
		{"large box overflow", largeBoxOverflow(), ErrCorrupt},
		{"uuid box too small", append(isoBox("ftyp", []byte("heic"), be32(0)), isoBox("uuid", []byte{1})...), ErrCorrupt},
		{"HEIF bad iloc widths", corruptHEIF(func(iloc []byte) { iloc[4] = 0x33 }), ErrCorrupt},
		{"HEIF truncated iloc", corruptHEIF(func(iloc []byte) { binary.BigEndian.PutUint16(iloc[12:], 0xFFFF) }), ErrCorrupt},
		{"HEIF Exif offset past item", corruptHEIF(func([]byte) {}, withExifOffset(0xFFFF)), ErrCorrupt},
		{"HEIF Exif item outside file", corruptHEIF(func(iloc []byte) { binary.BigEndian.PutUint32(iloc[len(iloc)-8:], 0xFFFFFF) }), ErrCorrupt},
		{"HEIF zero-length extent", corruptHEIF(func(iloc []byte) { binary.BigEndian.PutUint32(iloc[len(iloc)-4:], 0) }), ErrCorrupt},
		{"HEIF construction method 2", heifConstruction(2), ErrUnsupported},
		{"CR3 without moov", isoBox("ftyp", []byte(brandCR3), be32(0)), ErrUnsupported},
		{"CR3 without Canon box", append(isoBox("ftyp", []byte(brandCR3), be32(0)), isoBox("moov")...), ErrUnsupported},
		{"CR3 broken CMT1", cr3File([]byte("II*\x00\x08\x00\x00\x00\xFF"), nil, nil, nil), ErrCorrupt},
		{"RAF header short", []byte(rafMagic + "0000"), ErrCorrupt},
		{"RAF JPEG outside", rafPointing(0xFFFF, 10), ErrCorrupt},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			info, err := readInfo(t, test.data)
			if !errors.Is(err, test.want) {
				t.Fatalf("ReadInfoFrom = %+v, %v; want %v", info, err, test.want)
			}
		})
	}
}

func loopTIFF() []byte {
	builder := newTIFFBuilder(binary.LittleEndian, tiffMagic)
	// IFD0 is written at offset 8 and points its Exif IFD back at itself.
	builder.setFirst(builder.addIFD([]testEntry{builder.long(tagExifIFD, tiffHeaderSize)}, 0))

	return builder.bytes()
}

func manyEntriesTIFF() []byte {
	data := []byte("II*\x00\x08\x00\x00\x00")

	return binary.LittleEndian.AppendUint16(data, maxIFDEntries+1)
}

func truncatedIFD() []byte {
	data := []byte("MM\x00*\x00\x00\x00\x08")

	return append(binary.BigEndian.AppendUint16(data, 3), make([]byte, 10)...)
}

func pointerTIFF(tag uint16, offset uint32) []byte {
	return singleIFDTIFF(func(b *tiffBuilder) []testEntry { return []testEntry{b.long(tag, offset)} })
}

func shortFrameJPEG() []byte {
	var out bytes.Buffer
	out.Write([]byte{markerPrefix, markerSOI})
	writeSegment(&out, markerSOF0, []byte{8, 0})
	out.Write([]byte{markerPrefix, markerEOI})

	return out.Bytes()
}

func manySegmentsJPEG() []byte {
	var out bytes.Buffer
	out.Write([]byte{markerPrefix, markerSOI})

	for range maxJPEGSegments + 1 {
		out.Write([]byte{markerPrefix, markerRST0})
	}

	return out.Bytes()
}

func largeBoxOverflow() []byte {
	data := isoBox("ftyp", []byte("heic"), be32(0))
	data = append(data, be32(boxSizeLarge)...)
	data = append(data, "meta"...)

	return binary.BigEndian.AppendUint64(data, 1<<63)
}

type heifOption func(item []byte) []byte

func withExifOffset(offset uint32) heifOption {
	return func(item []byte) []byte {
		binary.BigEndian.PutUint32(item, offset)

		return item
	}
}

// corruptHEIF builds a version-0 HEIC and lets mutate patch the iloc payload in place.
func corruptHEIF(mutate func(iloc []byte), options ...heifOption) []byte {
	data := heifFile("heic", cameraTIFF(defaultCamera()), 0, false)
	start := bytes.Index(data, []byte("iloc")) + brandSize
	length := int(binary.BigEndian.Uint32(data[start-boxHeaderSize:])) - boxHeaderSize
	mutate(data[start : start+length])

	itemStart := bytes.Index(data, []byte("mdat")) + brandSize
	for _, option := range options {
		option(data[itemStart:])
	}

	return data
}

func heifConstruction(method uint16) []byte {
	data := heifFile("heic", cameraTIFF(defaultCamera()), 1, false)
	start := bytes.Index(data, []byte("iloc")) + brandSize
	// Second item (the Exif item): header 8, item 1 entry of 16 bytes, then id (2) + method.
	binary.BigEndian.PutUint16(data[start+8+16+2:], method)

	return data
}

func rafPointing(offset, length uint32) []byte {
	data := rafFile(nil)
	binary.BigEndian.PutUint32(data[rafJPEGOffsetAt:], offset)
	binary.BigEndian.PutUint32(data[rafJPEGLengthAt:], length)

	return data
}

// shortReader returns fewer bytes than asked without an error.
type shortReader struct{}

func (shortReader) ReadAt(buf []byte, _ int64) (int, error) {
	return len(buf) / 2, nil
}

func TestShortReadIsAnError(t *testing.T) {
	_, err := ReadInfoFrom(context.Background(), shortReader{}, 1000)
	if !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Errorf("short read = %v, want ErrUnexpectedEOF", err)
	}
}

func TestSourceBounds(t *testing.T) {
	src := source{reader: bytes.NewReader(make([]byte, 10)), size: 10}

	if _, err := src.section(5, 6); !errors.Is(err, ErrCorrupt) {
		t.Errorf("section past the end = %v", err)
	}

	if _, err := src.readBytes(0, 11, 10); !errors.Is(err, ErrCorrupt) {
		t.Errorf("readBytes above limit = %v", err)
	}

	if err := src.readAt(make([]byte, 1), -1); !errors.Is(err, ErrCorrupt) {
		t.Errorf("negative offset = %v", err)
	}
}
