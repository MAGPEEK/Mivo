package photometa

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"testing"
	"time"

	"github.com/magpeek/mivo/internal/domain/photo"
)

// heifWithIloc builds a HEIC whose meta box has an Exif item (ID 2) and the given iloc
// payload.
func heifWithIloc(iloc []byte) []byte {
	iinf := isoBox("iinf", []byte{0, 0, 0, 0}, be16(1),
		isoBox("infe", []byte{2, 0, 0, 0}, be16(2), be16(0), []byte(itemTypeExif), []byte{0}))
	meta := isoBox("meta", []byte{0, 0, 0, 0}, iinf, isoBox("iloc", iloc))

	return append(isoBox("ftyp", []byte("heic"), be32(0)), meta...)
}

// cappedEntriesBudget is a generous wall-clock bound for parsing 65535 iloc entries: it only
// catches a quadratic regression, so a loaded CI runner under -race must not trip it.
const cappedEntriesBudget = 10 * time.Second

func TestIlocZeroWidthExtentsAreBounded(t *testing.T) {
	hostile := []byte{0, 0, 0, 0, 0x00, 0x00}
	hostile = append(hostile, be16(1)...)
	hostile = append(hostile, be16(1)...)      // item 1
	hostile = append(hostile, be16(0)...)      // data_reference_index
	hostile = append(hostile, be16(0xFFFF)...) // extent_count with zero-width fields

	if _, err := readInfo(t, heifWithIloc(hostile)); !errors.Is(err, ErrCorrupt) {
		t.Errorf("extent_count 0xFFFF = %v, want ErrCorrupt", err)
	}

	// Many entries at the extent cap, none of them the Exif item: bounded work, no Exif.
	const entries = 0xFFFF
	allowed := []byte{0, 0, 0, 0, 0x00, 0x00}
	allowed = append(allowed, be16(entries)...)

	for range entries {
		allowed = append(allowed, be16(1)...)
		allowed = append(allowed, be16(0)...)
		allowed = append(allowed, be16(maxItemExtents)...)
	}

	start := time.Now()

	info, err := readInfo(t, heifWithIloc(allowed))
	if err != nil || info != (photo.Info{}) {
		t.Errorf("capped entries = %+v, %v; want zero Info", info, err)
	}

	if elapsed := time.Since(start); elapsed > cappedEntriesBudget {
		t.Errorf("capped entries took %v", elapsed)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, _, err := findItemLocation(ctx, allowed, 2); !errors.Is(err, context.Canceled) {
		t.Errorf("cancelled iloc walk = %v, want context.Canceled", err)
	}
}

func TestSharedIFDOffsetsAreNotALoop(t *testing.T) {
	builder := newTIFFBuilder(binary.LittleEndian, tiffMagic)
	exif := builder.addIFD([]testEntry{builder.short(tagISO, 200)}, 0)
	builder.setFirst(builder.addIFD([]testEntry{
		builder.long(tagExifIFD, exif), builder.long(tagSubIFDs, exif), builder.long(tagGPSIFD, exif),
	}, 0))

	info, err := readInfo(t, builder.bytes())
	if err != nil || info.ISO != 200 {
		t.Errorf("shared offsets = %+v, %v; want ISO 200", info, err)
	}

	// An Exif pointer back at IFD0 is read from the cache, not reported as a loop.
	if _, err := readInfo(t, loopTIFF()); err != nil {
		t.Errorf("Exif pointer to IFD0 = %v, want no error", err)
	}
}

func TestTooManyIFDsIsAnError(t *testing.T) {
	builder := newTIFFBuilder(binary.LittleEndian, tiffMagic)

	var subs []uint32
	for range maxIFDsPerStream {
		subs = append(subs, builder.addIFD(nil, 0))
	}

	exif := builder.addIFD(nil, 0)
	stream := singleStream(t, builder, []testEntry{builder.long(tagExifIFD, exif)})

	for _, offset := range subs {
		if _, err := stream.readIFD(context.Background(), offset); err != nil {
			t.Fatalf("read IFD %d: %v", offset, err)
		}
	}

	if _, err := stream.readIFD(context.Background(), exif); !errors.Is(err, ErrCorrupt) {
		t.Errorf("IFD beyond the budget = %v, want ErrCorrupt", err)
	}
}

func singleStream(t *testing.T, builder *tiffBuilder, entries []testEntry) *tiffStream {
	t.Helper()
	builder.setFirst(builder.addIFD(entries, 0))

	data := builder.bytes()

	stream, err := openTIFF(source{reader: bytes.NewReader(data), size: int64(len(data))})
	if err != nil {
		t.Fatal(err)
	}

	return stream
}

func TestNegativeSizeIsCorrupt(t *testing.T) {
	ctx := context.Background()

	if _, err := ReadInfoFrom(ctx, bytes.NewReader(nil), -1); !errors.Is(err, ErrCorrupt) {
		t.Errorf("ReadInfoFrom(size -1) = %v, want ErrCorrupt", err)
	}

	if _, err := ExtractPreviewFrom(ctx, bytes.NewReader(nil), -1); !errors.Is(err, ErrCorrupt) {
		t.Errorf("ExtractPreviewFrom(size -1) = %v, want ErrCorrupt", err)
	}
}

func TestRAFWithoutExifSizeHasNoSize(t *testing.T) {
	onlyMake := singleIFDTIFF(func(b *tiffBuilder) []testEntry { return []testEntry{b.ascii(tagMake, "FUJIFILM")} })

	info, err := readInfo(t, rafFile(jpegWithExif(onlyMake, 1620, 1080)))
	if err != nil {
		t.Fatalf("ReadInfoFrom: %v", err)
	}

	assertInfo(t, info, photo.Info{Make: "FUJIFILM"})
}

// withPrecision sets the sample precision of the first frame header of a JPEG.
func withPrecision(jpeg []byte, frameMarker, precision byte) []byte {
	at := bytes.Index(jpeg, []byte{markerPrefix, frameMarker}) + markerSize + segmentLenSize
	jpeg[at] = precision

	return jpeg
}

func TestPreviewNeeds8BitHuffmanFrame(t *testing.T) {
	tests := []struct {
		name    string
		preview []byte
		wantErr error
	}{
		{"8-bit extended", paddedJPEG(10, markerSOF1), nil},
		{"12-bit extended", withPrecision(paddedJPEG(10, markerSOF1), markerSOF1, 12), ErrNoPreview},
		{"12-bit progressive", withPrecision(paddedJPEG(10, markerSOF2), markerSOF2, 12), ErrNoPreview},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := extractPreview(t, rafFile(test.preview))
			if !errors.Is(err, test.wantErr) || (err == nil && !bytes.Equal(got, test.preview)) {
				t.Errorf("ExtractPreviewFrom = %d bytes, %v; want %v", len(got), err, test.wantErr)
			}
		})
	}
}
