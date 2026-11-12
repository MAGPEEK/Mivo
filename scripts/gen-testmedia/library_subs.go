package main

import (
	"fmt"
	"path"
	"strings"
)

const (
	// subtitlesLibraryPath is the library of the sidecar subtitle discovery (task M4-03).
	subtitlesLibraryPath = "subs"
	// categorySubtitles groups its case in progress lines and the summary.
	categorySubtitles = "subs"
	// subtitlesFolder is the subfolder releases keep further subtitle files in.
	subtitlesFolder = "Subs"

	subtitleFormatSRT = "srt"
	subtitleFormatASS = "ass"
)

// expectedSubtitles is the sidecar subtitle library root below the -library directory.
type expectedSubtitles struct {
	// Path is the library root relative to the -library directory; the paths of its movies
	// and sidecars are relative to this root.
	Path   string                   `json:"path"`
	Movies []expectedSubtitledMovie `json:"movies"`
}

// expectedSubtitledMovie is one movie file and the external subtitles a scan must attach.
type expectedSubtitledMovie struct {
	Title    string                    `json:"title"`
	Year     int                       `json:"year"`
	File     string                    `json:"file"`
	Sidecars []expectedSubtitleSidecar `json:"sidecars"`
}

// expectedSubtitleSidecar is one external subtitle file and what its name says about it.
type expectedSubtitleSidecar struct {
	File string `json:"file"`
	// Language is the code in the file name (ISO 639-1), e.g. "en".
	Language string `json:"language"`
	// Forced and HearingImpaired are the ".forced" and ".sdh" flags of the file name.
	Forced          bool `json:"forced"`
	HearingImpaired bool `json:"hearingImpaired"`
	// Format is "srt" or "ass".
	Format string `json:"format"`

	// cues are the events written to the file; they are not part of the manifest.
	cues []cue
}

// subtitleCatalog returns the expected sidecar subtitle library: one loose movie whose
// subtitles lie next to it (plain, forced and hearing-impaired) and in a Subs folder.
func subtitleCatalog() expectedSubtitles {
	const name = "Movie (2020)"

	return expectedSubtitles{
		Path: subtitlesLibraryPath,
		Movies: []expectedSubtitledMovie{{
			Title: "Movie", Year: yearOf(name), File: name + ".mkv",
			Sidecars: []expectedSubtitleSidecar{
				{File: name + ".en.srt", Language: "en", Format: subtitleFormatSRT, cues: englishCues()},
				{
					File: name + ".de.forced.srt", Language: "de", Forced: true, Format: subtitleFormatSRT,
					cues: germanCues(),
				},
				{
					File: name + ".fr.sdh.ass", Language: "fr", HearingImpaired: true, Format: subtitleFormatASS,
					cues: subtitleCues("[une porte claque]\nSous-titre un", "[musique]\nSous-titre deux"),
				},
				{
					File: path.Join(subtitlesFolder, name+".es.srt"), Language: "es", Format: subtitleFormatSRT,
					cues: subtitleCues("Subtítulo uno", "Subtítulo dos"),
				},
			},
		}},
	}
}

// subtitleLibraryCases returns one case per movie: the standard clip with its sidecars.
func subtitleLibraryCases() []mediaCase {
	catalog := subtitleCatalog()
	cases := make([]mediaCase, 0, len(catalog.Movies))

	for _, movie := range catalog.Movies {
		name := libraryCasePrefix + catalog.Path + "-" + slug(strings.TrimSuffix(movie.File, path.Ext(movie.File)))
		c := clipCase(name, categorySubtitles, path.Join(catalog.Path, movie.File), nil)

		for _, sidecar := range movie.Sidecars {
			c.Sidecars = append(c.Sidecars, sidecar.file(catalog.Path))
		}

		cases = append(cases, c)
	}

	return cases
}

// file renders the sidecar below the library root.
func (sidecar expectedSubtitleSidecar) file(root string) sidecarFile {
	output := path.Join(root, sidecar.File)

	if sidecar.Format == subtitleFormatASS {
		return sidecarFile{
			Output: output, Content: formatASS(sidecar.cues, defaultWidth, defaultHeight),
			ProbeFormat: "ass", Codec: codecASS,
		}
	}

	return sidecarFile{Output: output, Content: formatSRT(sidecar.cues), ProbeFormat: "srt", Codec: codecSubRip}
}

// writeSubtitleSummary adds one line per movie, e.g. "Movie (2020): en, de (forced), …".
func writeSubtitleSummary(text *strings.Builder, subtitles expectedSubtitles) {
	if len(subtitles.Movies) == 0 {
		return
	}

	fmt.Fprintf(text, "  %s/ (%d movies with sidecar subtitles)\n", subtitles.Path, len(subtitles.Movies))

	for _, movie := range subtitles.Movies {
		labels := make([]string, 0, len(movie.Sidecars))
		for _, sidecar := range movie.Sidecars {
			labels = append(labels, sidecar.label())
		}

		fmt.Fprintf(text, "    %s (%d): %s\n", movie.Title, movie.Year, strings.Join(labels, ", "))
	}
}

// label is the language with the flags of the sidecar, e.g. "de (forced)".
func (sidecar expectedSubtitleSidecar) label() string {
	switch {
	case sidecar.Forced:
		return sidecar.Language + " (forced)"
	case sidecar.HearingImpaired:
		return sidecar.Language + " (sdh)"
	default:
		return sidecar.Language
	}
}
