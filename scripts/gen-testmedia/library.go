package main

import (
	"context"
	"fmt"
	"io"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"unicode"
)

// Library cases (task M3-17) are derived from the expected catalog: one shared image per
// artwork shape, one clip per item and container whose further files are byte copies, and
// one clip per file with embedded cover art.
const (
	libraryCasePrefix = "library-"
	// categoryArtwork groups the shared artwork images in progress lines and the summary.
	categoryArtwork = "artwork"
	// coverCaseSuffix marks the clip case that embeds the poster as cover art.
	coverCaseSuffix = "-cover"
)

// Artwork image sizes: a 2:3 poster, a 16:9 backdrop (also used for thumbnails) and a
// wide logo.
const (
	posterWidth    = 600
	posterHeight   = 900
	backdropWidth  = 1280
	backdropHeight = 720
	logoWidth      = 800
	logoHeight     = 310
	// logoAlphaFilter makes the logo half transparent, so its PNG has a real alpha channel.
	logoAlphaFilter = "format=rgba,colorchannelmixer=aa=0.5"
	pixelFormatRGBA = "rgba"
	extensionJPEG   = ".jpg"
	extensionPNG    = ".png"
	extensionMP4    = ".mp4"
)

// imageShape is one generated image that every artwork file of its shape copies.
type imageShape struct {
	name      string
	width     int
	height    int
	extension string
}

// imageShapeOf maps an artwork kind to its image: posters are 2:3 JPEGs, logos RGBA PNGs and
// every other kind (backdrop, thumb) a 16:9 JPEG.
func imageShapeOf(kind string) imageShape {
	switch kind {
	case artworkPoster:
		return imageShape{name: artworkPoster, width: posterWidth, height: posterHeight, extension: extensionJPEG}
	case artworkLogo:
		return imageShape{name: artworkLogo, width: logoWidth, height: logoHeight, extension: extensionPNG}
	default:
		return imageShape{name: artworkBackdrop, width: backdropWidth, height: backdropHeight, extension: extensionJPEG}
	}
}

// itemContent is what one catalog item needs on disk, relative to the -library directory.
type itemContent struct {
	videos []string
	// embedded are the video files that carry cover art; they are not in videos.
	embedded []string
	nfos     []sidecarFile
	artwork  []expectedArtwork
}

// itemFiles is one catalog item prepared for case derivation.
type itemFiles struct {
	name     string
	category string
	content  itemContent
}

// libraryCases returns every case of the library tree: the artwork images first, then the
// clips of each item in catalog order, then the music cover and tracks.
func libraryCases() []mediaCase {
	var (
		items   []itemFiles
		artwork []expectedArtwork
	)

	for _, library := range libraryCatalog() {
		for _, item := range library.Items {
			content := contentOf(library, item)
			items = append(items, itemFiles{name: itemCaseName(library, item), category: library.Kind, content: content})
			artwork = append(artwork, content.artwork...)
		}
	}

	cases := artworkCases(artwork)
	cover := posterOutput(cases)

	for _, item := range items {
		cases = append(cases, itemCases(item, cover)...)
	}

	return append(cases, musicCases()...)
}

// posterOutput is the built poster image, the cover art source of embedded-artwork cases.
func posterOutput(cases []mediaCase) string {
	index := slices.IndexFunc(cases, func(c mediaCase) bool { return c.Name == artworkCaseName(artworkPoster) })
	if index < 0 {
		return ""
	}

	return cases[index].Output
}

// contentOf collects the files of an item, its seasons and its episodes.
func contentOf(library expectedLibrary, item expectedItem) itemContent {
	content := itemContent{}
	content.addNFO(library.Path, item.NFO, nfoRootOf(library.Kind))
	content.addArtwork(library.Path, item.Artwork)

	for _, version := range item.Versions {
		content.addVideos(library.Path, version.Parts...)
	}

	for _, extra := range item.Extras {
		content.addVideos(library.Path, extra.File)
	}

	content.addVideos(library.Path, item.Ignored...)

	for _, season := range item.Seasons {
		content.addArtwork(library.Path, season.Artwork)

		for _, episode := range season.Episodes {
			content.addVideos(library.Path, episode.Files...)
			content.addNFO(library.Path, episode.NFO, nfoRootEpisode)
			content.addArtwork(library.Path, episode.Artwork)
		}
	}

	return content
}

func (c *itemContent) addVideos(libraryPath string, files ...string) {
	for _, file := range files {
		if !slices.Contains(c.embedded, path.Join(libraryPath, file)) {
			c.videos = append(c.videos, path.Join(libraryPath, file))
		}
	}
}

// addArtwork records local artwork and the files with embedded artwork. Artwork precedes
// the videos of its level in contentOf, so addVideos can leave embedded files out.
func (c *itemContent) addArtwork(libraryPath string, artwork []expectedArtwork) {
	for _, art := range artwork {
		art.File = path.Join(libraryPath, art.File)

		if art.Source == artworkSourceEmbedded {
			c.embedded = append(c.embedded, art.File)

			continue
		}

		c.artwork = append(c.artwork, art)
	}
}

func (c *itemContent) addNFO(libraryPath string, nfo *expectedNFO, root string) {
	if nfo != nil {
		c.nfos = append(c.nfos, sidecarFile{Output: path.Join(libraryPath, nfo.File), Content: renderNFO(root, *nfo)})
	}
}

// nfoRootOf returns the root element of item NFO files in a library of the given kind.
func nfoRootOf(libraryKind string) string {
	if libraryKind == libraryKindShows {
		return nfoRootShow
	}

	return nfoRootMovie
}

// artworkCases returns one image case per shape; every further file of a shape is a copy.
func artworkCases(artwork []expectedArtwork) []mediaCase {
	var shapes []imageShape

	filesByShape := map[string][]string{}

	for _, art := range artwork {
		shape := imageShapeOf(art.Kind)
		if _, seen := filesByShape[shape.name]; !seen {
			shapes = append(shapes, shape)
		}

		filesByShape[shape.name] = append(filesByShape[shape.name], art.File)
	}

	cases := make([]mediaCase, 0, len(shapes))
	for _, shape := range shapes {
		files := filesByShape[shape.name]
		cases = append(cases, imageCase(shape, files[0], files[1:]))
	}

	return cases
}

func artworkCaseName(shapeName string) string {
	return libraryCasePrefix + categoryArtwork + "-" + shapeName
}

// imageCase builds a single-frame test pattern of the shape: JPEG, or an RGBA PNG for logos.
func imageCase(shape imageShape, output string, copies []string) mediaCase {
	c := mediaCase{
		Name: artworkCaseName(shape.name), Category: categoryArtwork, Output: output, Copies: copies,
		Source: sourceSpec{
			Width: shape.width, Height: shape.height, FrameRate: defaultFrameRate,
			Frames: stillFrames, Duration: imageSourceDuration,
		},
		Muxer:    image2Muxer,
		Encoders: []string{"mjpeg"},
		Args:     []string{"-c:v", "mjpeg", "-update", "1"},
		Expect:   expect("jpeg", probeImage2, &videoExpectation{Codec: "mjpeg"}, nil),
	}

	if shape.extension == extensionPNG {
		c.Encoders = []string{"png"}
		c.Args = []string{"-vf", logoAlphaFilter, "-c:v", "png", "-pix_fmt", pixelFormatRGBA, "-update", "1"}
		c.Expect = expect("png", "png_pipe", videoOf("png", pixelFormatRGBA), nil)
	}

	return c
}

// itemCases returns the clip cases of one item: one per container of its plain videos (the
// first file is built, the others are copies) and one per file with cover art. The NFO
// files are sidecars of the first case.
func itemCases(item itemFiles, cover string) []mediaCase {
	var cases []mediaCase

	for index, files := range groupByExtension(item.content.videos) {
		caseName := item.name
		if index > 0 {
			caseName += "-" + strings.TrimPrefix(path.Ext(files[0]), ".")
		}

		cases = append(cases, clipCase(caseName, item.category, files[0], files[1:]))
	}

	for _, file := range item.content.embedded {
		c := clipCase(item.name+coverCaseSuffix, item.category, file, nil)
		c.Cover = cover
		cases = append(cases, c)
	}

	if len(cases) > 0 {
		cases[0].Sidecars = item.content.nfos
	}

	return cases
}

// groupByExtension groups files by extension, in the order of first appearance.
func groupByExtension(files []string) [][]string {
	var groups [][]string

	indexByExtension := map[string]int{}

	for _, file := range files {
		extension := path.Ext(file)
		if index, seen := indexByExtension[extension]; seen {
			groups[index] = append(groups[index], file)

			continue
		}

		indexByExtension[extension] = len(groups)
		groups = append(groups, []string{file})
	}

	return groups
}

// clipCase is the standard 2 s H.264/AAC clip in Matroska, or MP4 for ".mp4" outputs. The
// encoder options name the first video stream, so a cover picture stream stays a copy.
func clipCase(name, category, output string, copies []string) mediaCase {
	c := mediaCase{
		Name: name, Category: category, Output: output, Copies: copies,
		Source: standardSource(), Muxer: matroskaMuxer, Encoders: []string{"libx264", "aac"},
		Args: []string{
			"-c:v:0", "libx264", "-preset:v:0", "veryfast", "-profile:v:0", "high",
			"-pix_fmt:v:0", pixelFormat8Bit, "-c:a", "aac",
		},
		Expect: expect("mkv", probeMatroska, h264Video(), audioOf("aac")),
	}

	if path.Ext(output) == extensionMP4 {
		c.Muxer = "mp4"
		c.Expect = expect("mp4", probeISOBMFF, h264Video(), audioOf("aac"))
	}

	return c
}

// itemCaseName is "library-<library path>-<slug of title and year>".
func itemCaseName(library expectedLibrary, item expectedItem) string {
	title := item.Title
	if item.Year > 0 {
		title += fmt.Sprintf(" %d", item.Year)
	}

	return libraryCasePrefix + library.Path + "-" + slug(title)
}

// slug lowercases text and joins its runs of letters and digits with dashes.
func slug(text string) string {
	var result strings.Builder

	for _, word := range strings.FieldsFunc(strings.ToLower(text), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	}) {
		if result.Len() > 0 {
			result.WriteByte('-')
		}

		result.WriteString(word)
	}

	return result.String()
}

// splitCoverPlans separates the cases that embed a cover from the others, which build the
// cover image and therefore run first.
func splitCoverPlans(plans []plannedCase) (first, covers []plannedCase) {
	for _, plan := range plans {
		if plan.Case.Cover != "" {
			covers = append(covers, plan)

			continue
		}

		first = append(first, plan)
	}

	return first, covers
}

// requireCovers turns builds whose cover image is missing (skipped, failed or not selected
// in the first phase) into a missing-input plan instead of an FFmpeg failure.
func (g *generator) requireCovers(plans []plannedCase) []plannedCase {
	result := slices.Clone(plans)

	for index, plan := range result {
		if plan.Action == actionBuild && !isNonEmptyFile(g.path(plan.Case.Cover)) {
			result[index].Action = actionMissingInput
			result[index].Reason = fmt.Sprintf("cover image %s was not built", plan.Case.Cover)
		}
	}

	return result
}

// generateLibrary builds the library tree into s.opts.libraryDir in two phases (cover art
// needs the poster image) and writes its manifest and summary. It returns the manifest.
func (s session) generateLibrary(ctx context.Context) (libraryManifest, error) {
	cases := libraryCases()
	gen := &generator{
		ffmpeg: s.ffmpeg, outDir: s.opts.libraryDir, force: s.opts.force, jobs: s.opts.jobs,
		progress: s.stdout, previous: previousEncoders{},
	}

	first, covers := splitCoverPlans(planCases(cases, s.caps, s.opts.only))
	entries := gen.generateAll(ctx, first)
	entries = append(entries, gen.generateAll(ctx, gen.requireCovers(covers))...)
	result := newLibraryManifest(s.caps.version, cases, entries)

	if err := writeLibraryManifest(s.opts.libraryDir, result); err != nil {
		return libraryManifest{}, err
	}

	// The summary is informational; the manifest is the authoritative result.
	_ = writeLibrarySummary(s.stdout, result, filepath.Join(s.opts.libraryDir, manifestFileName))

	return result, nil
}

// writeLibrarySummary prints the case tallies and the expected tree: one line per top-level
// item with its versions, extras, seasons and artwork.
func writeLibrarySummary(output io.Writer, m libraryManifest, manifestPath string) error {
	entries := make([]manifestEntry, 0, len(m.Cases))
	for _, entry := range m.Cases {
		entries = append(entries, manifestEntry{
			Name: entry.Name, Category: entry.Category, Status: entry.Status, SkipKind: entry.SkipKind, kept: entry.kept,
		})
	}

	var text strings.Builder

	text.WriteString("library\n")
	writeTallies(&text, entries)

	for _, library := range m.Libraries {
		fmt.Fprintf(&text, "  %s/ (%d %s)\n", library.Path, len(library.Items), library.Kind)

		for _, item := range library.Items {
			fmt.Fprintf(&text, "    %-5s %s%s\n", item.Kind, item.displayTitle(), item.details())
		}
	}

	writeMusicSummary(&text, m.Music)

	fmt.Fprintf(&text, "manifest: %s\n", manifestPath)

	if _, err := io.WriteString(output, text.String()); err != nil {
		return fmt.Errorf("write library summary: %w", err)
	}

	return nil
}

func (item expectedItem) displayTitle() string {
	if item.Year > 0 {
		return fmt.Sprintf("%s (%d)", item.Title, item.Year)
	}

	return item.Title
}

// details lists what the item holds, e.g. ": 2 versions, 2 extras, artwork poster backdrop".
func (item expectedItem) details() string {
	var parts []string

	appendCount := func(count int, noun string) {
		switch {
		case count == 1:
			parts = append(parts, "1 "+noun)
		case count > 1:
			parts = append(parts, fmt.Sprintf("%d %ss", count, noun))
		}
	}

	appendCount(len(item.Versions), "version")
	appendCount(len(item.Extras), "extra")
	appendCount(len(item.Seasons), "season")
	appendCount(item.episodeCount(), "episode")

	if item.NFO != nil {
		parts = append(parts, "nfo "+item.NFO.Format)
	}

	if len(item.Artwork) > 0 {
		kinds := make([]string, 0, len(item.Artwork))
		for _, art := range item.Artwork {
			if art.Source == artworkSourceEmbedded {
				kinds = append(kinds, art.Kind+"(embedded)")

				continue
			}

			kinds = append(kinds, art.Kind)
		}

		parts = append(parts, "artwork "+strings.Join(kinds, " "))
	}

	if len(parts) == 0 {
		return ""
	}

	return ": " + strings.Join(parts, ", ")
}

func (item expectedItem) episodeCount() int {
	total := 0
	for _, season := range item.Seasons {
		total += len(season.Episodes)
	}

	return total
}
