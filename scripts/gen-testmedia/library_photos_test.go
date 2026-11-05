package main

import (
	"bytes"
	"errors"
	"math"
	"os"
	"path"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/magpeek/mivo/internal/adapters/photometa"
	"github.com/magpeek/mivo/internal/domain/photo"
)

// readBack runs photometa over data and maps the outcome to the manifest words.
func readBack(t *testing.T, data []byte) (photo.Info, string) {
	t.Helper()

	info, err := photometa.ReadInfoFrom(t.Context(), bytes.NewReader(data), int64(len(data)))

	switch {
	case err == nil:
		return info, metadataRead
	case errors.Is(err, photometa.ErrUnsupported):
		return info, metadataUnsupported
	case errors.Is(err, photometa.ErrCorrupt):
		return info, metadataCorrupt
	default:
		t.Fatalf("ReadInfoFrom: %v", err)

		return info, ""
	}
}

// infoAsExif converts an Info to the manifest shape; a zero Info is nil.
func infoAsExif(info photo.Info) *expectedExif {
	if info == (photo.Info{}) {
		return nil
	}

	exif := &expectedExif{
		Make: info.Make, Model: info.Model, Lens: info.Lens, ExposureSeconds: info.ExposureSeconds,
		FNumber: info.FNumber, FocalLengthMM: info.FocalLengthMM, ISO: info.ISO, Orientation: info.Orientation,
		Width: info.Width, Height: info.Height,
	}

	if !info.TakenAt.IsZero() {
		exif.TakenAt = info.TakenAt.Format(wallClockLayout)
	}

	if info.GPS != nil {
		exif.GPS = &expectedGPS{Latitude: info.GPS.Latitude, Longitude: info.GPS.Longitude}
	}

	return exif
}

// heicSources builds the fake HEVC source of a HEIC spec.
func heicSources(spec photoSpec) [][]byte {
	pictures := map[photoBuild]int{buildHEICSingle: 1, buildHEICGrid: heicGridRows * heicGridColumns}[spec.build]
	if pictures == 0 {
		return nil
	}

	return [][]byte{fakeHEVCStream(pictures)}
}

// TestGoBuiltPhotosReadBackAsTheManifestSays builds every Go-written photo (HEIC from fake
// HEVC streams) and reads it with photometa: the outcome and the EXIF values must be the
// manifest's, exactly (rationals and GPS degrees are computed as photometa does).
func TestGoBuiltPhotosReadBackAsTheManifestSays(t *testing.T) {
	t.Parallel()

	for _, spec := range photoSpecs() {
		build, _ := spec.builder()
		if build == nil {
			continue
		}

		t.Run(spec.file, func(t *testing.T) {
			t.Parallel()

			data, err := build(heicSources(spec))
			if err != nil {
				t.Fatalf("build: %v", err)
			}

			checkReadBack(t, data, spec.expected())
		})
	}
}

func checkReadBack(t *testing.T, data []byte, want expectedPhoto) {
	t.Helper()

	info, outcome := readBack(t, data)
	if outcome != want.Metadata {
		t.Fatalf("outcome %s, want %s", outcome, want.Metadata)
	}

	if got := infoAsExif(info); !equalExif(got, want.Exif) {
		t.Errorf("Info %+v (GPS %+v), want %+v", got, gpsOf(got), want.Exif)
	}

	if source := photo.SourceOf(info).String(); outcome == metadataRead && source != want.TakenAtSource {
		t.Errorf("capture time source %s, want %s", source, want.TakenAtSource)
	}
}

func equalExif(got, want *expectedExif) bool {
	if got == nil || want == nil {
		return got == want
	}

	left, right := *got, *want
	left.GPS, right.GPS = nil, nil

	return left == right && equalGPS(got.GPS, want.GPS)
}

func equalGPS(got, want *expectedGPS) bool {
	if got == nil || want == nil {
		return got == want
	}

	return *got == *want
}

func gpsOf(exif *expectedExif) *expectedGPS {
	if exif == nil {
		return nil
	}

	return exif.GPS
}

func TestDNGStandInsCarryARenderablePreview(t *testing.T) {
	t.Parallel()

	data, err := dngStandIn(photoWidth, photoHeight, *fennickExif(), nil)
	if err != nil {
		t.Fatal(err)
	}

	preview, err := photometa.ExtractPreviewFrom(t.Context(), bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("ExtractPreviewFrom: %v", err)
	}

	if !bytes.HasPrefix(preview, []byte{jpegMarkerPrefix, 0xD8}) {
		t.Error("the preview is no JPEG")
	}
}

func TestEiffelTowerDecimalDegrees(t *testing.T) {
	t.Parallel()

	got := eiffelTower().expected()

	const tolerance = 1e-6
	if math.Abs(got.Latitude-48.858222) > tolerance || math.Abs(got.Longitude-2.294500) > tolerance {
		t.Errorf("Eiffel Tower at %+v", *got)
	}

	south := dms{degrees: 1, ref: "S"}
	if south.decimal() != -1 {
		t.Errorf("1 degree south = %v, want -1", south.decimal())
	}
}

func TestRationalOfKeepsTheFloat(t *testing.T) {
	t.Parallel()

	for _, value := range []float64{1.0 / 250, 1.0 / 60, 2.8, 35, 6.9, 29.6, 0.3} {
		numerator, denominator := rationalOf(value)
		if float64(numerator)/float64(denominator) != value {
			t.Errorf("%v written as %d/%d", value, numerator, denominator)
		}
	}
}

func TestExifBlockRejectsAMalformedCaptureTime(t *testing.T) {
	t.Parallel()

	if _, err := exifBlock(expectedExif{TakenAt: "14.07.2024"}, nil); err == nil {
		t.Error("exifBlock accepted a malformed capture time")
	}
}

func TestJPEGHelpersRejectBadInput(t *testing.T) {
	t.Parallel()

	if _, err := insertExif([]byte("no jpeg"), nil); err == nil {
		t.Error("insertExif accepted a stream that is no JPEG")
	}

	if _, err := insertExif([]byte{jpegMarkerPrefix, 0xD8}, make([]byte, maxJPEGSegmentPayload)); err == nil {
		t.Error("insertExif accepted an EXIF block larger than a segment")
	}

	if _, err := truncateBeforeFrame([]byte{jpegMarkerPrefix, 0xD8, jpegMarkerPrefix, jpegMarkerEOI, 0, 0}); err == nil {
		t.Error("truncateBeforeFrame accepted a stream without frame header")
	}
}

func TestParseHEVC(t *testing.T) {
	t.Parallel()

	stream, err := parseHEVC(fakeHEVCStream(heicGridRows * heicGridColumns))
	if err != nil || len(stream.pictures) != heicGridRows*heicGridColumns {
		t.Fatalf("parseHEVC = %d pictures, %v", len(stream.pictures), err)
	}

	withoutSlices := fakeHEVCStream(0)
	if _, err := parseHEVC(withoutSlices); !errors.Is(err, errNoHEVCPicture) {
		t.Errorf("stream without slices: %v, want errNoHEVCPicture", err)
	}

	if _, err := parseHEVC([]byte("fake media")); err == nil {
		t.Error("parseHEVC accepted a stream without parameter sets")
	}

	// A second slice without first_slice_segment_in_pic_flag belongs to the same picture.
	continued := slices.Concat(fakeHEVCStream(1), []byte{0, 0, 1, fakeIDRSlice << nalTypeShift, 1, 0, 1})
	if stream, err := parseHEVC(continued); err != nil || len(stream.pictures) != 1 {
		t.Errorf("continued slice: %d pictures, %v", len(stream.pictures), err)
	}
}

func TestEncodeHEIFRejectsMismatchedGrids(t *testing.T) {
	t.Parallel()

	stream, err := parseHEVC(fakeHEVCStream(1))
	if err != nil {
		t.Fatal(err)
	}

	if _, err := encodeHEIF(heifImage{stream: stream, rows: heicGridRows, columns: heicGridColumns}); err == nil {
		t.Error("encodeHEIF accepted one picture for a 2x2 grid")
	}

	stream.sps = stream.sps[:nalHeaderSize+1]
	if _, err := encodeHEIF(heifImage{stream: stream, rows: 1, columns: 1}); err == nil {
		t.Error("encodeHEIF accepted an SPS without profile")
	}
}

func TestRemoveEmulationPrevention(t *testing.T) {
	t.Parallel()

	got := removeEmulationPrevention([]byte{1, 0, 0, 3, 1, 0, 0, 3, 0, 3})
	if want := []byte{1, 0, 0, 1, 0, 0, 0, 3}; !bytes.Equal(got, want) {
		t.Errorf("RBSP % x, want % x", got, want)
	}
}

// TestPhotoCatalogIsConsistent checks the catalog against the ADR 0029 rules it exercises.
func TestPhotoCatalogIsConsistent(t *testing.T) {
	t.Parallel()

	catalog := photoCatalog()
	files := map[string]expectedPhoto{}
	withGPS := 0

	for _, photo := range catalog.Photos {
		if _, seen := files[photo.File]; seen {
			t.Errorf("%s is listed twice", photo.File)
		}

		files[photo.File] = photo

		if checkCatalogPhoto(t, photo) {
			withGPS++
		}
	}

	if withGPS != 1 {
		t.Errorf("%d photos carry GPS, want exactly one", withGPS)
	}

	for _, photo := range catalog.Photos {
		primary, ok := files[photo.AlternateOf]
		if photo.AlternateOf != "" && (!ok || primary.Album != photo.Album || primary.AlternateOf != "") {
			t.Errorf("%s: alternate of %q, which is no primary in its album", photo.File, photo.AlternateOf)
		}
	}
}

// checkCatalogPhoto checks one row and reports whether it carries GPS.
func checkCatalogPhoto(t *testing.T, photo expectedPhoto) bool {
	t.Helper()

	if photo.Optional != (photo.Format == photoFormatHEIC || photo.Format == photoFormatAVIF) {
		t.Errorf("%s: only HEIC and AVIF are optional", photo.File)
	}

	if (photo.TakenAtSource == takenAtSourceFile) != (photo.Exif == nil || photo.Exif.TakenAt == "") {
		t.Errorf("%s: capture time source %s does not match its EXIF", photo.File, photo.TakenAtSource)
	}

	return photo.Exif != nil && photo.Exif.GPS != nil
}

func TestAssembledPhotosGetTheirModificationTime(t *testing.T) {
	t.Parallel()

	gen := &generator{outDir: t.TempDir(), progress: &strings.Builder{}}
	index := slices.IndexFunc(photoAssemblies(), func(assembly fileAssembly) bool { return !assembly.modified.IsZero() })
	assembly := photoAssemblies()[index]

	if entry := gen.assemble(t.Context(), assembly, caseFilter{}); entry.Status != statusGenerated {
		t.Fatalf("assemble = %+v", entry)
	}

	stat, err := os.Stat(gen.path(path.Join(assembly.folder, assembly.files[0])))
	if err != nil {
		t.Fatal(err)
	}

	if !stat.ModTime().Equal(assembly.modified) {
		t.Errorf("modification time %v, want %v", stat.ModTime(), assembly.modified)
	}
}

func TestHEICAssembliesSkipWithoutTheirSource(t *testing.T) {
	t.Parallel()

	gen := &generator{outDir: t.TempDir(), progress: &strings.Builder{}}

	for _, assembly := range photoAssemblies() {
		if len(assembly.sources) == 0 {
			continue
		}

		entry := gen.assemble(t.Context(), assembly, caseFilter{})
		if entry.Status != statusSkipped || entry.SkipKind != skipMissingInput {
			t.Errorf("%s without source = %s/%s, want skipped for the missing input", assembly.name, entry.Status, entry.SkipKind)
		}
	}
}

func TestWritePhotoSummary(t *testing.T) {
	t.Parallel()

	var empty strings.Builder
	if writePhotoSummary(&empty, expectedPhotos{}); empty.Len() != 0 {
		t.Errorf("empty catalog wrote %q", empty.String())
	}

	var text strings.Builder

	writePhotoSummary(&text, photoCatalog())
	checkSummaryLines(t, text.String(),
		"  photos/ (24 files in 6 albums)\n",
		"    album Holidays 2024/Lake Shore: 4 files (2 jpeg, 1 dng, 1 mp4), raw+jpeg\n",
		"    album Orientation: 8 files (8 jpeg)\n",
	)
}

func TestFixedModifiedTimesAreDistinct(t *testing.T) {
	t.Parallel()

	seen := map[time.Time]bool{}

	for _, spec := range photoSpecs() {
		if spec.modified.IsZero() {
			continue
		}

		if seen[spec.modified] {
			t.Errorf("%s shares its modification time", spec.file)
		}

		seen[spec.modified] = true
	}
}
