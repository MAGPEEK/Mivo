package main

import (
	"context"
	"fmt"
	"path"
	"slices"
	"strings"
	"time"
)

// Expected outcomes of photometa ReadInfo (M3-15c).
const (
	// metadataRead: the reader returns an Info (a file without EXIF gives only its size).
	metadataRead = "read"
	// metadataUnsupported: the format or layout is unknown (photometa.ErrUnsupported).
	metadataUnsupported = "unsupported"
	// metadataCorrupt: the EXIF structure is broken (photometa.ErrCorrupt).
	metadataCorrupt = "corrupt"
)

// Capture time sources and thumbnail states as the API names them (M3-15b,
// photo.TakenAtSource and photo.ThumbnailState).
const (
	takenAtSourceExif    = "exif"
	takenAtSourceFile    = "file"
	thumbnailReady       = "ready"
	thumbnailUnsupported = "unsupported"
	// firstTransposedOrientation is the first EXIF orientation that swaps width and height.
	firstTransposedOrientation = 5
)

// expectedPhotos is the photo library root below the -library directory.
type expectedPhotos struct {
	// Path is the library root relative to the -library directory; the photo files are
	// relative to this root.
	Path   string          `json:"path"`
	Photos []expectedPhoto `json:"photos"`
}

// expectedPhoto is one file of the photo tree and what reading and showing it must give.
type expectedPhoto struct {
	File string `json:"file"`
	// Album is the folder of the file (an album a scan creates), relative to the root.
	Album  string `json:"album"`
	Format string `json:"format"`
	// AlternateOf names the primary file of a RAW+JPEG pair; a scan files the RAW under it.
	AlternateOf string `json:"alternateOf,omitempty"`
	// Optional files (HEIC, AVIF) are only built when FFmpeg has the encoder.
	Optional bool `json:"optional,omitempty"`
	// Metadata is the outcome of photometa ReadInfo: "read", "unsupported" or "corrupt".
	Metadata string `json:"metadata"`
	// Exif is the Info ReadInfo returns when Metadata is "read"; omitted when it is zero.
	Exif *expectedExif `json:"exif,omitempty"`
	// TakenAtSource is "exif" or "file" (the modification time stands in).
	TakenAtSource string `json:"takenAtSource"`
	// ModifiedAt is the modification time the generator sets (RFC 3339, UTC); omitted for
	// FFmpeg outputs, whose time is the build time.
	ModifiedAt string `json:"modifiedAt,omitempty"`
	// ThumbnailState is the state a scan stores: "ready" or "unsupported".
	ThumbnailState string `json:"thumbnailState"`
	// Renders says whether the photo renderer produces a thumbnail (false for the corrupt
	// file, the unreadable RAW and videos); DisplayWidth and DisplayHeight are the upright
	// size of a still, with orientations 5-8 applied.
	Renders       bool `json:"renders"`
	DisplayWidth  int  `json:"displayWidth,omitempty"`
	DisplayHeight int  `json:"displayHeight,omitempty"`
}

// photoCatalog returns the expected photo tree.
func photoCatalog() expectedPhotos {
	specs := photoSpecs()
	photos := make([]expectedPhoto, 0, len(specs))

	for _, spec := range specs {
		photos = append(photos, spec.expected())
	}

	return expectedPhotos{Path: photosLibraryPath, Photos: photos}
}

// expected writes out the expectation of one file.
func (spec photoSpec) expected() expectedPhoto {
	photo := expectedPhoto{
		File: spec.file, Album: spec.album(), Format: spec.format, AlternateOf: spec.alternateOf,
		Optional: spec.optional, Metadata: spec.metadata(), TakenAtSource: takenAtSourceFile,
		ThumbnailState: thumbnailReady, Renders: true,
	}

	if photo.Metadata == metadataRead {
		photo.Exif = spec.readExif()
	}

	if photo.Exif != nil && photo.Exif.TakenAt != "" {
		photo.TakenAtSource = takenAtSourceExif
	}

	if !spec.modified.IsZero() {
		photo.ModifiedAt = spec.modified.Format(time.RFC3339)
	}

	switch spec.build {
	case buildUnreadableRAW:
		// photo.ThumbnailStateFor: a RAW is shown through its preview, which needs a
		// readable layout.
		photo.ThumbnailState, photo.Renders = thumbnailUnsupported, false
	case buildCorruptJPEG, buildMP4:
		photo.Renders = false
	case buildJPEG, buildPNG, buildGIF, buildTIFF, buildDNG, buildHEICSingle, buildHEICGrid, buildWebP, buildAVIF:
		photo.DisplayWidth, photo.DisplayHeight = spec.displaySize()
	}

	return photo
}

// metadata is the expected ReadInfo outcome: photometa reads JPEG, TIFF-based RAW and the
// HEIF family; PNG, WebP, GIF, MP4 and an unknown RAW layout are unsupported.
func (spec photoSpec) metadata() string {
	switch spec.build {
	case buildCorruptJPEG:
		return metadataCorrupt
	case buildJPEG, buildTIFF, buildDNG, buildHEICSingle, buildHEICGrid, buildAVIF:
		return metadataRead
	case buildPNG, buildGIF, buildWebP, buildMP4, buildUnreadableRAW:
		return metadataUnsupported
	}

	return metadataUnsupported
}

// readExif is what ReadInfo returns: the written EXIF, the frame size of a JPEG without
// EXIF, nothing for an AVIF without an Exif item.
func (spec photoSpec) readExif() *expectedExif {
	switch {
	case spec.exif != nil:
		exif := *spec.exif
		if spec.gps != nil {
			exif.GPS = spec.gps.expected()
		}

		return &exif
	case spec.build == buildJPEG:
		return &expectedExif{Width: photoWidth, Height: photoHeight}
	default:
		return nil
	}
}

// displaySize is the stored size with the EXIF orientation applied. HEIF files are turned by
// their container (irot), which the generator never writes, and keep the stored size.
func (spec photoSpec) displaySize() (width, height int) {
	if spec.exif != nil && spec.exif.Orientation >= firstTransposedOrientation && !spec.isHEIF() {
		return photoHeight, photoWidth
	}

	return photoWidth, photoHeight
}

func (spec photoSpec) isHEIF() bool {
	return spec.build == buildHEICSingle || spec.build == buildHEICGrid || spec.build == buildAVIF
}

// output is the path of the file relative to the -library directory.
func (spec photoSpec) output() string { return path.Join(photosLibraryPath, spec.file) }

// caseName is "library-photos-<slug of the file>"; the extension tells a RAW from its JPEG.
func (spec photoSpec) caseName() string {
	return libraryCasePrefix + categoryPhotos + "-" + slug(spec.file)
}

// libraryPhotoCases returns the FFmpeg cases of the photo tree: WebP, AVIF (one alternative
// per AV1 encoder), the video and the HEVC sources of the HEIC files.
func libraryPhotoCases() []mediaCase {
	var cases []mediaCase

	for _, spec := range photoSpecs() {
		switch spec.build {
		case buildWebP:
			cases = append(cases, photoWebPCase(spec))
		case buildAVIF:
			cases = append(cases, photoAVIFCases(spec)...)
		case buildMP4:
			cases = append(cases, clipCase(spec.caseName(), categoryPhotos, spec.output(), nil))
		case buildJPEG, buildPNG, buildGIF, buildTIFF, buildDNG, buildCorruptJPEG, buildUnreadableRAW,
			buildHEICSingle, buildHEICGrid:
			// Written in Go by photoAssemblies.
		}
	}

	return append(cases, heicSourceCase("single", heicSingleSource, 1, 1),
		heicSourceCase("grid", heicGridSource, heicGridRows, heicGridColumns))
}

// photoStill is a photoWidth x photoHeight single-frame source.
func photoStill() sourceSpec {
	return sourceSpec{
		Width: photoWidth, Height: photoHeight, FrameRate: defaultFrameRate, Frames: stillFrames,
		Duration: imageSourceDuration,
	}
}

func photoWebPCase(spec photoSpec) mediaCase {
	return mediaCase{
		Name: spec.caseName(), Category: categoryPhotos, Output: spec.output(), Source: photoStill(),
		Muxer: "webp", Encoders: []string{"libwebp"},
		Args:   []string{"-c:v", "libwebp", "-lossless", "0", "-pix_fmt", pixelFormat8Bit},
		Expect: expect("webp", "webp_pipe", videoOf("webp", pixelFormat8Bit), nil),
	}
}

// photoAVIFCases reuses the AV1 encoder alternatives of the image corpus.
func photoAVIFCases(spec photoSpec) []mediaCase {
	cases := avifCases()

	for index := range cases {
		cases[index].Name, cases[index].Category, cases[index].Output = spec.caseName(), categoryPhotos, spec.output()
		cases[index].Source = photoStill()
	}

	return cases
}

// heicSourceCase is an all-intra 8-bit HEVC stream with one coded picture per tile of a
// rows x columns grid (1x1: a single picture). The tiles are successive test pattern frames,
// so each shows its index in the frame counter and a wrong tile order is visible.
func heicSourceCase(name, output string, rows, columns int) mediaCase {
	pictures := rows * columns
	source := photoStill()
	source.Width, source.Height = photoWidth/columns, photoHeight/rows
	source.FrameRate, source.Frames = pictures, pictures

	return mediaCase{
		Name: libraryCasePrefix + categoryPhotos + "-source-heic-" + name, Category: categoryPhotos,
		Output: output, Source: source, Muxer: "hevc", Encoders: []string{"libx265"},
		Args:   slices.Concat(simpleVideoArgs("libx265"), []string{"-x265-params", "keyint=1:log-level=error"}),
		Expect: expect("hevc", "hevc", videoOf("hevc", pixelFormat8Bit), nil),
	}
}

// photoAssemblies are the files the generator writes in Go, in catalog order.
func photoAssemblies() []fileAssembly {
	var assemblies []fileAssembly

	for _, spec := range photoSpecs() {
		build, sources := spec.builder()
		if build == nil {
			continue
		}

		assemblies = append(assemblies, fileAssembly{
			name: spec.caseName(), category: categoryPhotos, label: spec.output(),
			folder: path.Dir(spec.output()), files: []string{path.Base(spec.file)}, sources: sources,
			build: func(contents [][]byte) ([]assembledFile, error) {
				data, err := build(contents)

				return []assembledFile{{name: path.Base(spec.file), data: data}}, err
			},
			modified: spec.modified,
		})
	}

	return assemblies
}

// builder returns the Go encoder of the file and its sources; nil for FFmpeg outputs.
func (spec photoSpec) builder() (func(sources [][]byte) ([]byte, error), []string) {
	switch spec.build {
	case buildJPEG:
		return func([][]byte) ([]byte, error) { return jpegWithExif(photoWidth, photoHeight, spec.exif, spec.gps) }, nil
	case buildPNG:
		return func([][]byte) ([]byte, error) { return encodePNG(testPattern(photoWidth, photoHeight, 0)) }, nil
	case buildGIF:
		return func([][]byte) ([]byte, error) { return encodeGIF(photoWidth, photoHeight) }, nil
	case buildTIFF:
		return func([][]byte) ([]byte, error) { return rgbTIFF(photoWidth, photoHeight, *spec.exif) }, nil
	case buildDNG:
		return func([][]byte) ([]byte, error) { return dngStandIn(photoWidth, photoHeight, *spec.exif, spec.gps) }, nil
	case buildCorruptJPEG:
		return func([][]byte) ([]byte, error) { return corruptJPEG(photoWidth, photoHeight) }, nil
	case buildUnreadableRAW:
		return func([][]byte) ([]byte, error) { return []byte(unreadableRAWText), nil }, nil
	case buildHEICSingle:
		return spec.heic(1, 1), []string{heicSingleSource}
	case buildHEICGrid:
		return spec.heic(heicGridRows, heicGridColumns), []string{heicGridSource}
	case buildWebP, buildAVIF, buildMP4:
		return nil, nil
	}

	return nil, nil
}

// unreadableRAWText fills the RAW file no reader understands; it explains itself to anyone
// who opens it.
const unreadableRAWText = "Generated test fixture: a RAW file name over bytes that are no RAW layout.\n"

// heic wraps the HEVC source of the spec in a HEIF file with its EXIF item.
func (spec photoSpec) heic(rows, columns int) func(sources [][]byte) ([]byte, error) {
	return func(sources [][]byte) ([]byte, error) {
		stream, err := parseHEVC(sources[0])
		if err != nil {
			return nil, err
		}

		exif, err := exifBlock(*spec.exif, spec.gps)
		if err != nil {
			return nil, err
		}

		return encodeHEIF(heifImage{
			stream: stream, tileWidth: photoWidth / columns, tileHeight: photoHeight / rows,
			rows: rows, columns: columns, exif: exif,
		})
	}
}

// photoAssemblyCases returns the manifest cases of the Go-built photos.
func photoAssemblyCases() []mediaCase {
	assemblies := photoAssemblies()
	cases := make([]mediaCase, 0, len(assemblies))

	for _, assembly := range assemblies {
		cases = append(cases, assembly.asCase())
	}

	return cases
}

// assemblePhotos writes the Go-built photos after the FFmpeg cases ran.
func (g *generator) assemblePhotos(ctx context.Context, filter caseFilter) []manifestEntry {
	return g.assembleAll(ctx, photoAssemblies(), filter)
}

// writePhotoSummary adds one line per album, e.g.
// "album Holidays 2024/Lake Shore: 4 files (jpeg, jpeg, dng, mp4), raw+jpeg".
func writePhotoSummary(text *strings.Builder, photos expectedPhotos) {
	if len(photos.Photos) == 0 {
		return
	}

	var albums []string

	formats := map[string][]string{}
	pairs := map[string]bool{}

	for _, photo := range photos.Photos {
		if _, seen := formats[photo.Album]; !seen {
			albums = append(albums, photo.Album)
		}

		formats[photo.Album] = append(formats[photo.Album], photo.Format)
		pairs[photo.Album] = pairs[photo.Album] || photo.AlternateOf != ""
	}

	fmt.Fprintf(text, "  %s/ (%d files in %d albums)\n", photos.Path, len(photos.Photos), len(albums))

	for _, album := range albums {
		details := fmt.Sprintf("%d %s (%s)", len(formats[album]), plural(len(formats[album]), "file"),
			formatCounts(formats[album]))

		if pairs[album] {
			details += ", raw+jpeg"
		}

		fmt.Fprintf(text, "    album %s: %s\n", album, details)
	}
}

// formatCounts lists the formats in order of first appearance with their counts, e.g.
// "2 jpeg, 1 dng, 1 mp4".
func formatCounts(formats []string) string {
	var (
		order  []string
		counts = map[string]int{}
	)

	for _, format := range formats {
		if counts[format] == 0 {
			order = append(order, format)
		}

		counts[format]++
	}

	parts := make([]string, 0, len(order))
	for _, format := range order {
		parts = append(parts, fmt.Sprintf("%d %s", counts[format], format))
	}

	return strings.Join(parts, ", ")
}
