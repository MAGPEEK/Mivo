package photometa

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/photo"
)

func readInfo(t *testing.T, data []byte) (photo.Info, error) {
	t.Helper()

	return ReadInfoFrom(context.Background(), bytes.NewReader(data), int64(len(data)))
}

func extractPreview(t *testing.T, data []byte) ([]byte, error) {
	t.Helper()

	return ExtractPreviewFrom(context.Background(), bytes.NewReader(data), int64(len(data)))
}

func assertInfo(t *testing.T, got, want photo.Info) {
	t.Helper()

	gotGPS, wantGPS := got.GPS, want.GPS
	got.GPS, want.GPS = nil, nil

	if got.ExposureSeconds-want.ExposureSeconds > 1e-12 || want.ExposureSeconds-got.ExposureSeconds > 1e-12 {
		t.Errorf("exposure = %v, want %v", got.ExposureSeconds, want.ExposureSeconds)
	}

	got.ExposureSeconds, want.ExposureSeconds = 0, 0
	if got != want {
		t.Errorf("info = %+v\nwant   %+v", got, want)
	}

	switch {
	case (gotGPS == nil) != (wantGPS == nil):
		t.Errorf("GPS = %v, want %v", gotGPS, wantGPS)
	case gotGPS != nil && (math.Abs(gotGPS.Latitude-wantGPS.Latitude) > 1e-9 ||
		math.Abs(gotGPS.Longitude-wantGPS.Longitude) > 1e-9):
		t.Errorf("GPS = %+v, want %+v", *gotGPS, *wantGPS)
	}
}

func TestReadInfoFormats(t *testing.T) {
	bigEndian := defaultCamera()
	bigEndian.order = binary.BigEndian
	orf, rw2 := defaultCamera(), defaultCamera()
	orf.magic, rw2.magic = orfMagic, rw2Magic

	le := cameraTIFF(defaultCamera())
	cmt1 := singleIFDTIFF(func(b *tiffBuilder) []testEntry {
		return []testEntry{b.ascii(tagMake, "Canon"), b.ascii(tagModel, "EOS R5"), b.short(tagOrientation, 6)}
	})
	cmt2 := singleIFDTIFF(func(b *tiffBuilder) []testEntry {
		return []testEntry{
			b.rational(tagExposureTime, typeRational, 1, 250), b.short(tagISO, 400),
			b.ascii(tagDateTimeOriginal, "2024:06:01 14:30:15"), b.rational(tagFocalLength, typeRational, 50, 1),
			b.long(tagPixelXDimension, 6000), b.long(tagPixelYDimension, 4000), b.ascii(tagLensModel, "RF50mm F1.8 STM"),
		}
	})
	cmt4 := singleIFDTIFF(func(b *tiffBuilder) []testEntry {
		return []testEntry{
			b.ascii(tagGPSLatitudeRef, "N"), b.rational(tagGPSLatitude, typeRational, 48, 1, 8, 1, 3000, 100),
			b.ascii(tagGPSLongitudeRef, "W"), b.rational(tagGPSLongitude, typeRational, 11, 1, 34, 1, 0, 1),
		}
	})

	tests := []struct {
		name string
		data []byte
	}{
		{"JPEG little-endian EXIF", jpegWithExif(le, 640, 480)},
		{"JPEG big-endian EXIF", jpegWithExif(cameraTIFF(bigEndian), 640, 480)},
		{"TIFF RAW", le},
		{"ORF magic", cameraTIFF(orf)},
		{"RW2 magic", cameraTIFF(rw2)},
		{"HEIC iloc v0", heifFile("heic", le, 0, false)},
		{"HEIC iloc v1", heifFile("heic", le, 1, false)},
		{"HEIC iloc v2", heifFile("heic", le, 2, false)},
		{"HEIC idat", heifFile("heic", le, 1, true)},
		{"AVIF", heifFile("avif", le, 0, false)},
		{"compatible brand only", heifFile("abcd", le, 0, false)},
		{"CR3", cr3File(cmt1, cmt2, cmt4, testJPEG(1620, 1080, markerSOF0))},
		{"RAF", rafFile(jpegWithExif(le, 640, 480))},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := readInfo(t, test.data)
			if err != nil {
				t.Fatalf("ReadInfoFrom: %v", err)
			}

			assertInfo(t, got, cameraInfo())
		})
	}
}

func TestMissingTagsAreZeroValues(t *testing.T) {
	onlyMake := singleIFDTIFF(func(b *tiffBuilder) []testEntry {
		return []testEntry{b.ascii(tagMake, "  Nikon \x00garbage"), b.long(tagExifIFD, 0)}
	})

	tests := []struct {
		name string
		data []byte
		want photo.Info
	}{
		{"JPEG without EXIF", testJPEG(320, 200, markerSOF2), photo.Info{Width: 320, Height: 200}},
		{"TIFF with only Make", onlyMake, photo.Info{Make: "Nikon"}},
		{"HEIF without meta", isoBox("ftyp", []byte("heic"), be32(0)), photo.Info{}},
		{"JPEG with XMP only", jpegWithXMPOnly(), photo.Info{Width: 16, Height: 8}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := readInfo(t, test.data)
			if err != nil {
				t.Fatalf("ReadInfoFrom: %v", err)
			}

			assertInfo(t, got, test.want)
		})
	}
}

func jpegWithXMPOnly() []byte {
	var out bytes.Buffer
	out.Write([]byte{markerPrefix, markerSOI})
	writeSegment(&out, markerAPP1, []byte("Exi"))
	writeSegment(&out, markerSOF1, sofPayload(16, 8))
	out.Write([]byte{markerPrefix, markerEOI})

	return out.Bytes()
}

func TestTagEdgeCases(t *testing.T) {
	tests := []struct {
		name    string
		entries func(b *tiffBuilder) []testEntry
		want    photo.Info
	}{
		{"DateTime fallback", func(b *tiffBuilder) []testEntry {
			return []testEntry{b.ascii(tagDateTime, "2020:01:02 03:04:05")}
		}, photo.Info{TakenAt: time.Date(2020, 1, 2, 3, 4, 5, 0, time.UTC)}},
		{"blank and malformed dates", func(b *tiffBuilder) []testEntry {
			return []testEntry{b.ascii(tagDateTime, "0000:00:00 00:00:00")}
		}, photo.Info{}},
		{"short date", func(b *tiffBuilder) []testEntry {
			return []testEntry{b.ascii(tagDateTime, "2020")}
		}, photo.Info{}},
		{"orientation out of range", func(b *tiffBuilder) []testEntry {
			return []testEntry{b.short(tagOrientation, 9)}
		}, photo.Info{}},
		{"wrong types ignored", func(*tiffBuilder) []testEntry {
			return []testEntry{
				{tag: tagMake, kind: typeShort, count: 1, data: []byte{1, 0}},
				{tag: tagOrientation, kind: typeASCII, count: 2, data: []byte("6\x00")},
				{tag: tagImageWidth, kind: 99, count: 1, data: []byte{1, 0}},
			}
		}, photo.Info{}},
		{"value offset outside the file", func(*tiffBuilder) []testEntry {
			return []testEntry{{tag: tagModel, kind: typeASCII, count: 100, data: []byte{0xFF, 0xFF, 0, 0}}}
		}, photo.Info{}},
		{"BYTE orientation and huge width", func(b *tiffBuilder) []testEntry {
			return []testEntry{
				{tag: tagOrientation, kind: typeByte, count: 1, data: []byte{3}},
				b.long(tagImageWidth, math.MaxUint32), b.long(tagImageLength, 10),
			}
		}, photo.Info{Orientation: 3}},
		{"reduced IFD0 size ignored", func(b *tiffBuilder) []testEntry {
			return []testEntry{b.long(tagNewSubfileType, 1), b.long(tagImageWidth, 160), b.long(tagImageLength, 120)}
		}, photo.Info{}},
		{"full IFD0 size used", func(b *tiffBuilder) []testEntry {
			return []testEntry{b.long(tagImageWidth, 5472), b.short(tagImageLength, 3648)}
		}, photo.Info{Width: 5472, Height: 3648}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := readInfo(t, singleIFDTIFF(test.entries))
			if err != nil {
				t.Fatalf("ReadInfoFrom: %v", err)
			}

			assertInfo(t, got, test.want)
		})
	}
}

func TestExifAndGPSEdgeCases(t *testing.T) {
	tests := []struct {
		name string
		exif func(b *tiffBuilder) []testEntry
		gps  func(b *tiffBuilder) []testEntry
		want photo.Info
	}{
		{"digitized fallback and SRATIONAL", func(b *tiffBuilder) []testEntry {
			return []testEntry{
				b.ascii(tagDateTimeDigitized, "2021:05:06 07:08:09"),
				b.rational(tagExposureTime, typeSRational, 1, 2),
				b.rational(tagFocalLength, typeSRational, 0xFFFFFFFF, 1), // -1 mm is dropped
				b.long(tagISO, 3200),
			}
		}, nil, photo.Info{TakenAt: time.Date(2021, 5, 6, 7, 8, 9, 0, time.UTC), ExposureSeconds: 0.5, ISO: 3200}},
		{"zero denominator", func(b *tiffBuilder) []testEntry {
			return []testEntry{b.rational(tagExposureTime, typeRational, 1, 0)}
		}, nil, photo.Info{}},
		{"south east", nil, func(b *tiffBuilder) []testEntry {
			return []testEntry{
				b.ascii(tagGPSLatitudeRef, "S"), b.rational(tagGPSLatitude, typeRational, 33, 1, 52, 1, 0, 1),
				b.ascii(tagGPSLongitudeRef, "E"), b.rational(tagGPSLongitude, typeRational, 151, 1, 12, 1, 36, 1),
			}
		}, photo.Info{GPS: &photo.Coordinates{Latitude: -(33 + 52.0/60), Longitude: 151 + 12.0/60 + 36.0/3600}}},
		{"latitude off the globe", nil, func(b *tiffBuilder) []testEntry {
			return []testEntry{
				b.rational(tagGPSLatitude, typeRational, 91, 1, 0, 1, 0, 1),
				b.rational(tagGPSLongitude, typeRational, 10, 1, 0, 1, 0, 1),
			}
		}, photo.Info{}},
		{"longitude missing", nil, func(b *tiffBuilder) []testEntry {
			return []testEntry{b.rational(tagGPSLatitude, typeRational, 10, 1, 0, 1, 0, 1)}
		}, photo.Info{}},
		{"negative minutes", nil, func(b *tiffBuilder) []testEntry {
			return []testEntry{
				b.rational(tagGPSLatitude, typeSRational, 10, 1, 0xFFFFFFFF, 1, 0, 1),
				b.rational(tagGPSLongitude, typeRational, 10, 1, 0, 1, 0, 1),
			}
		}, photo.Info{}},
		{"two of three parts", nil, func(b *tiffBuilder) []testEntry {
			return []testEntry{
				b.rational(tagGPSLatitude, typeRational, 10, 1, 0, 1),
				b.rational(tagGPSLongitude, typeRational, 10, 1, 0, 1, 0, 1),
			}
		}, photo.Info{}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := readInfo(t, exifGPSTIFF(test.exif, test.gps))
			if err != nil {
				t.Fatalf("ReadInfoFrom: %v", err)
			}

			assertInfo(t, got, test.want)
		})
	}
}

// exifGPSTIFF builds IFD0 pointing at the given Exif and GPS IFDs (nil = absent).
func exifGPSTIFF(exif, gps func(b *tiffBuilder) []testEntry) []byte {
	builder := newTIFFBuilder(binary.BigEndian, tiffMagic)

	var entries []testEntry
	if exif != nil {
		entries = append(entries, builder.long(tagExifIFD, builder.addIFD(exif(builder), 0)))
	}

	if gps != nil {
		entries = append(entries, builder.long(tagGPSIFD, builder.addIFD(gps(builder), 0)))
	}

	builder.setFirst(builder.addIFD(entries, 0))

	return builder.bytes()
}

func TestReaderOnDisk(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "photo.jpg")
	thumbnail := testJPEG(160, 120, markerSOF0)

	options := defaultCamera()
	options.thumbnail = thumbnail
	if err := os.WriteFile(path, jpegWithExif(cameraTIFF(options), 640, 480), 0o600); err != nil {
		t.Fatal(err)
	}

	reader := NewReader()

	info, err := reader.ReadInfo(context.Background(), path)
	if err != nil {
		t.Fatalf("ReadInfo: %v", err)
	}

	assertInfo(t, info, cameraInfo())

	preview, err := reader.ExtractPreview(context.Background(), path)
	if err != nil || !bytes.Equal(preview, thumbnail) {
		t.Fatalf("ExtractPreview = %d bytes, %v; want the thumbnail", len(preview), err)
	}

	missing := filepath.Join(directory, "missing.jpg")
	if _, err := reader.ReadInfo(context.Background(), missing); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("ReadInfo(missing) = %v, want ErrNotExist", err)
	}

	if _, err := reader.ExtractPreview(context.Background(), missing); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("ExtractPreview(missing) = %v, want ErrNotExist", err)
	}
}

func TestErrorsWrapDomainErrors(t *testing.T) {
	if !errors.Is(ErrUnsupported, domain.ErrUnsupported) || !errors.Is(ErrCorrupt, domain.ErrInvalid) {
		t.Error("ErrUnsupported/ErrCorrupt must wrap the domain errors")
	}
}

func TestCancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	inputs := map[string][]byte{
		"JPEG": jpegWithExif(cameraTIFF(defaultCamera()), 1, 1),
		"TIFF": cameraTIFF(defaultCamera()),
		"HEIF": heifFile("heic", cameraTIFF(defaultCamera()), 0, false),
	}

	for name, data := range inputs {
		if _, err := ReadInfoFrom(ctx, bytes.NewReader(data), int64(len(data))); !errors.Is(err, context.Canceled) {
			t.Errorf("%s: ReadInfoFrom = %v, want context.Canceled", name, err)
		}

		if _, err := ExtractPreviewFrom(ctx, bytes.NewReader(data), int64(len(data))); err == nil {
			t.Errorf("%s: ExtractPreviewFrom succeeded on a cancelled context", name)
		}
	}
}
