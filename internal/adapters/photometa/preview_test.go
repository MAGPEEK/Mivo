//nolint:gosec // fixture writer: sizes are small test constants
package photometa

import (
	"bytes"
	"encoding/binary"
	"errors"
	"testing"
)

// paddedJPEG is a JPEG header with padding bytes in an APP0 segment to set its size.
func paddedJPEG(padding int, frameMarker byte) []byte {
	var out bytes.Buffer
	out.Write([]byte{markerPrefix, markerSOI})
	writeSegment(&out, 0xE0, make([]byte, padding))
	writeSegment(&out, frameMarker, sofPayload(1, 1))
	writeSegment(&out, markerSOS, []byte{1, 1, 0, 0, 0x3F, 0})
	out.Write([]byte{markerPrefix, markerEOI})

	return out.Bytes()
}

func TestExtractPreview(t *testing.T) {
	thumbnail := paddedJPEG(10, markerSOF0)
	preview := paddedJPEG(2000, markerSOF2)
	lossless := paddedJPEG(9000, 0xC3)

	nef := defaultCamera()
	nef.thumbnail = thumbnail
	nef.ifd0Extra = func(b *tiffBuilder) []testEntry {
		previewAt, losslessAt := b.addBlob(preview), b.addBlob(lossless)
		rawAt := b.addIFD([]testEntry{
			b.long(tagNewSubfileType, 0), b.short(tagCompression, compressionJPEG),
			b.long(tagStripOffsets, losslessAt), b.long(tagStripCounts, uint32(len(lossless))),
		}, 0)
		previewIFD := b.addIFD([]testEntry{
			b.long(tagJPEGOffset, previewAt), b.long(tagJPEGLength, uint32(len(preview))),
		}, 0)

		return []testEntry{b.long(tagSubIFDs, rawAt, previewIFD)}
	}

	cr2 := defaultCamera()
	cr2.ifd0Extra = func(b *tiffBuilder) []testEntry {
		at := b.addBlob(preview)

		return []testEntry{
			b.short(tagCompression, compressionOldJPEG),
			b.long(tagStripOffsets, at), b.long(tagStripCounts, uint32(len(preview))),
		}
	}

	rw2 := defaultCamera()
	rw2.magic = rw2Magic
	rw2.ifd0Extra = func(*tiffBuilder) []testEntry {
		return []testEntry{{tag: tagRW2JpgFromRaw, kind: typeUndefined, count: uint32(len(preview)), data: preview}}
	}

	withThumbnail := defaultCamera()
	withThumbnail.thumbnail = thumbnail

	tests := []struct {
		name string
		data []byte
		want []byte
	}{
		{"NEF/DNG SubIFD preview beats thumbnail and lossless raw", cameraTIFF(nef), preview},
		{"CR2 IFD0 JPEG strip", cameraTIFF(cr2), preview},
		{"RW2 JpgFromRaw", cameraTIFF(rw2), preview},
		{"RAF embedded JPEG", rafFile(preview), preview},
		{"CR3 PRVW", cr3File(nil, nil, nil, preview), preview},
		{"JPEG EXIF thumbnail", jpegWithExif(cameraTIFF(withThumbnail), 1, 1), thumbnail},
		{"HEIF EXIF thumbnail", heifFile("heic", cameraTIFF(withThumbnail), 0, false), thumbnail},
		{"IFD chain loop keeps candidates", chainLoopTIFF(thumbnail), thumbnail},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := extractPreview(t, test.data)
			if err != nil {
				t.Fatalf("ExtractPreviewFrom: %v", err)
			}

			if !bytes.Equal(got, test.want) {
				t.Errorf("preview = %d bytes, want %d", len(got), len(test.want))
			}
		})
	}
}

// chainLoopTIFF has IFD0 -> IFD1 (thumbnail) -> IFD0.
func chainLoopTIFF(thumbnail []byte) []byte {
	builder := newTIFFBuilder(binary.LittleEndian, tiffMagic)
	blob := builder.addBlob(thumbnail)
	ifd1 := builder.addIFD([]testEntry{
		builder.long(tagJPEGOffset, blob), builder.long(tagJPEGLength, uint32(len(thumbnail))),
	}, 0)
	ifd0 := builder.addIFD([]testEntry{builder.ascii(tagMake, "X")}, ifd1)
	builder.setFirst(ifd0)
	// Point IFD1's next offset back at IFD0.
	binary.LittleEndian.PutUint32(builder.buf[int(ifd1)+ifdCountSize+2*ifdEntrySize:], ifd0)

	return builder.bytes()
}

func TestExtractPreviewWithoutPreview(t *testing.T) {
	outside := defaultCamera()
	outside.ifd0Extra = func(b *tiffBuilder) []testEntry {
		return []testEntry{b.long(tagJPEGOffset, 0xFFFFFF), b.long(tagJPEGLength, 100)}
	}

	notJPEG := defaultCamera()
	notJPEG.ifd0Extra = func(b *tiffBuilder) []testEntry {
		at := b.addBlob([]byte("not a jpeg at all"))

		return []testEntry{b.long(tagJPEGOffset, at), b.long(tagJPEGLength, 17)}
	}

	twoStrips := defaultCamera()
	twoStrips.ifd0Extra = func(b *tiffBuilder) []testEntry {
		at := b.addBlob(paddedJPEG(10, markerSOF0))

		return []testEntry{
			b.short(tagCompression, compressionJPEG),
			b.long(tagStripOffsets, at, at), b.long(tagStripCounts, 10, 10),
		}
	}

	tests := []struct {
		name string
		data []byte
		want error
	}{
		{"TIFF without preview", cameraTIFF(defaultCamera()), ErrNoPreview},
		{"preview outside the file", cameraTIFF(outside), ErrNoPreview},
		{"preview is not a JPEG", cameraTIFF(notJPEG), ErrNoPreview},
		{"several strips", cameraTIFF(twoStrips), ErrNoPreview},
		{"JPEG without EXIF", testJPEG(1, 1, markerSOF0), ErrNoPreview},
		{"HEIF without meta", isoBox("ftyp", []byte("heic"), be32(0)), ErrNoPreview},
		{"CR3 without preview box", append(isoBox("ftyp", []byte(brandCR3), be32(0)), isoBox("moov")...), ErrNoPreview},
		{"CR3 PRVW overruns", cr3PRVWOverrun(), ErrCorrupt},
		{"RAF JPEG outside", rafPointing(0xFFFF, 10), ErrNoPreview},
		{"RAF header short", []byte(rafMagic), ErrCorrupt},
		{"corrupt JPEG", []byte{0xFF, 0xD8, 0x00}, ErrCorrupt},
		{"corrupt TIFF", []byte("II*\x00"), ErrCorrupt},
		{"corrupt HEIF", corruptHEIF(func(iloc []byte) { iloc[4] = 0x33 }), ErrCorrupt},
		{"PNG", []byte("\x89PNG\r\n\x1a\n0000000000"), ErrUnsupported},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := extractPreview(t, test.data)
			if !errors.Is(err, test.want) {
				t.Fatalf("ExtractPreviewFrom = %d bytes, %v; want %v", len(got), err, test.want)
			}
		})
	}
}

func cr3PRVWOverrun() []byte {
	data := cr3File(nil, nil, nil, paddedJPEG(10, markerSOF0))
	at := bytes.Index(data, []byte("PRVW")) + brandSize + cr3PRVWLengthAt
	binary.BigEndian.PutUint32(data[at:], 0xFFFF)

	return data
}
