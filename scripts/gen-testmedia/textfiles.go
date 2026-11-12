package main

import (
	"fmt"
	"strings"
	"time"
)

// The generator writes subtitles and chapter lists itself (ADR 0021: only lavfi sources and
// program-written text), either as FFmpeg inputs or as sidecar files next to a media file.

// centisecond is the time resolution of ASS timestamps.
const centisecond = 10 * time.Millisecond

// cue is one subtitle event.
type cue struct {
	Start time.Duration
	End   time.Duration
	// Text may contain line breaks ("\n").
	Text string
}

// chapter is one chapter of an audiobook or film.
type chapter struct {
	Title string
	Start time.Duration
	End   time.Duration
}

// formatSRT writes cues as SubRip: a counter, "HH:MM:SS,mmm --> HH:MM:SS,mmm", the text and
// a blank line per cue.
func formatSRT(cues []cue) string {
	var text strings.Builder

	for index, event := range cues {
		if index > 0 {
			text.WriteString("\n")
		}

		fmt.Fprintf(&text, "%d\n%s --> %s\n%s\n",
			index+1, clockTime(event.Start, ","), clockTime(event.End, ","), event.Text)
	}

	return text.String()
}

// formatWebVTT writes cues as WebVTT with "HH:MM:SS.mmm" timestamps.
func formatWebVTT(cues []cue) string {
	var text strings.Builder

	text.WriteString("WEBVTT\n")

	for _, event := range cues {
		fmt.Fprintf(&text, "\n%s --> %s\n%s\n", clockTime(event.Start, "."), clockTime(event.End, "."), event.Text)
	}

	return text.String()
}

// assHeader is the minimal Advanced SubStation Alpha (v4.00+) header with one default style.
// The two %d verbs take the script resolution, which scales positions and font sizes.
const assHeader = `[Script Info]
ScriptType: v4.00+
PlayResX: %d
PlayResY: %d

[V4+ Styles]
Format: Name, Fontname, Fontsize, PrimaryColour, SecondaryColour, OutlineColour, BackColour, Bold, Italic, Underline, StrikeOut, ScaleX, ScaleY, Spacing, Angle, BorderStyle, Outline, Shadow, Alignment, MarginL, MarginR, MarginV, Encoding
Style: Default,Arial,20,&H00FFFFFF,&H000000FF,&H00000000,&H00000000,0,0,0,0,100,100,0,0,1,1,0,2,10,10,10,1

[Events]
Format: Layer, Start, End, Style, Name, MarginL, MarginR, MarginV, Effect, Text
`

// formatASS writes cues as an ASS script for a video of the given size.
func formatASS(cues []cue, width, height int) string {
	var text strings.Builder

	fmt.Fprintf(&text, assHeader, width, height)

	for _, event := range cues {
		// ASS marks line breaks with \N; a real newline would end the Dialogue line.
		fmt.Fprintf(&text, "Dialogue: 0,%s,%s,Default,,0,0,0,,%s\n",
			assTime(event.Start), assTime(event.End), strings.ReplaceAll(event.Text, "\n", `\N`))
	}

	return text.String()
}

// assSignStyle is a second style that differs from Default in everything a WebVTT conversion
// drops: font, size, yellow text with a blue outline, bold, italic and top-centre alignment.
const assSignStyle = "Style: Sign,Courier New,28,&H0000FFFF,&H000000FF,&H00FF0000,&H80000000," +
	"-1,-1,0,0,100,100,0,0,1,2,1,8,10,10,10,1\n"

const (
	// assFadeMilliseconds is the fade-in and fade-out time of positioned events.
	assFadeMilliseconds = 100
	// assSignRow places positioned events a quarter of the frame height below the top.
	assSignRow = 4
)

// formatStyledASS writes cues as an ASS script that uses what plain text subtitles cannot
// express: every even event has inline override tags (italic, a colour change) in the
// Default style, every odd event uses the Sign style, positioned and faded.
func formatStyledASS(cues []cue, width, height int) string {
	var text strings.Builder

	header := strings.Replace(assHeader, "\n[Events]", assSignStyle+"\n[Events]", 1)
	fmt.Fprintf(&text, header, width, height)

	for index, event := range cues {
		line := strings.ReplaceAll(event.Text, "\n", `\N`)
		style := "Default"
		overrides := `{\i1\c&H00FF00&}`

		if index%2 == 1 {
			style = "Sign"
			overrides = fmt.Sprintf(`{\pos(%d,%d)\fad(%d,%d)}`,
				width/halve, height/assSignRow, assFadeMilliseconds, assFadeMilliseconds)
		}

		fmt.Fprintf(&text, "Dialogue: 0,%s,%s,%s,,0,0,0,,%s%s\n",
			assTime(event.Start), assTime(event.End), style, overrides, line)
	}

	return text.String()
}

// formatFFMetadata writes chapters in FFmpeg's metadata text format, read with -f ffmetadata.
func formatFFMetadata(chapters []chapter) string {
	var text strings.Builder

	text.WriteString(";FFMETADATA1\n")

	for _, part := range chapters {
		fmt.Fprintf(&text, "[CHAPTER]\nTIMEBASE=1/1000\nSTART=%d\nEND=%d\ntitle=%s\n",
			part.Start.Milliseconds(), part.End.Milliseconds(), escapeFFMetadata(part.Title))
	}

	return text.String()
}

// escapeFFMetadata escapes the characters the FFmetadata format reserves.
func escapeFFMetadata(value string) string {
	return strings.NewReplacer(`\`, `\\`, "=", `\=`, ";", `\;`, "#", `\#`, "\n", "\\\n").Replace(value)
}

// Kodi NFO root elements (ADR 0027 section 7).
const (
	nfoRootMovie   = "movie"
	nfoRootShow    = "tvshow"
	nfoRootEpisode = "episodedetails"
	// tmdbMovieURL prefixes the TMDB ID in a URL-only NFO.
	tmdbMovieURL = "https://www.themoviedb.org/movie/"
	// malformedNFO ends inside an element: no XML parser accepts it, so a reader must fall
	// back to the file name.
	malformedNFO = "<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n<movie>\n  <title>Never read</title>\n  <plot>The file ends before"
	// maxLatin1 is the highest code point ISO-8859-1 encodes.
	maxLatin1 = 0xFF
	// latin1Replacement stands for runes ISO-8859-1 cannot encode; the tests keep the catalog
	// free of them.
	latin1Replacement = '?'
)

// renderNFO writes the NFO file whose readable content is nfo.Fields. root is the XML root
// element: movie, tvshow or episodedetails.
func renderNFO(root string, nfo expectedNFO) string {
	switch nfo.Format {
	case nfoFormatURL:
		return tmdbMovieURL + nfo.Fields.ProviderIDs[providerTMDB] + "\n"
	case nfoFormatMalformed:
		return malformedNFO
	}

	text := nfoXML(root, nfo.Encoding, nfo.Fields)
	if nfo.Encoding == encodingLatin1 {
		return toLatin1(text)
	}

	return text
}

// nfoXML writes the fields as Kodi XML in a fixed element order.
func nfoXML(root, encoding string, fields nfoFields) string {
	var text strings.Builder

	fmt.Fprintf(&text, "<?xml version=\"1.0\" encoding=\"%s\" standalone=\"yes\"?>\n<%s>\n", encoding, root)
	writeNFOElement(&text, "title", fields.Title)
	writeNFOElement(&text, "originaltitle", fields.OriginalTitle)
	writeNFONumber(&text, "year", fields.Year)
	writeNFONumber(&text, "season", fields.Season)
	writeNFONumber(&text, "episode", fields.Episode)
	writeNFOElement(&text, "aired", fields.Aired)
	writeNFOElement(&text, "plot", fields.Plot)

	for _, genre := range fields.Genres {
		writeNFOElement(&text, "genre", genre)
	}

	isDefault := true

	for _, provider := range []string{providerTMDB, providerIMDb, providerTVDB} {
		id, found := fields.ProviderIDs[provider]
		if !found {
			continue
		}

		fmt.Fprintf(&text, "  <uniqueid type=\"%s\" default=\"%t\">%s</uniqueid>\n", provider, isDefault, escapeXML(id))
		isDefault = false
	}

	fmt.Fprintf(&text, "</%s>\n", root)

	return text.String()
}

func writeNFOElement(text *strings.Builder, name, value string) {
	if value != "" {
		fmt.Fprintf(text, "  <%s>%s</%s>\n", name, escapeXML(value), name)
	}
}

func writeNFONumber(text *strings.Builder, name string, value int) {
	if value > 0 {
		fmt.Fprintf(text, "  <%s>%d</%s>\n", name, value, name)
	}
}

// escapeXML escapes the characters XML reserves in element content and attribute values.
func escapeXML(value string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;").Replace(value)
}

// toLatin1 encodes text as ISO-8859-1 bytes; Go strings may hold any bytes.
func toLatin1(text string) string {
	encoded := make([]byte, 0, len(text))

	for _, character := range text {
		if character > maxLatin1 {
			character = latin1Replacement
		}

		encoded = append(encoded, byte(character))
	}

	return string(encoded)
}

// clockTime formats d as "HH:MM:SS<separator>mmm", the SRT and WebVTT timestamp.
func clockTime(d time.Duration, separator string) string {
	hours, minutes, seconds := clockParts(d)

	return fmt.Sprintf("%02d:%02d:%02d%s%03d", hours, minutes, seconds, separator,
		(d%time.Second)/time.Millisecond)
}

// assTime formats d as "H:MM:SS.cc", the ASS timestamp with centiseconds.
func assTime(d time.Duration) string {
	hours, minutes, seconds := clockParts(d)

	return fmt.Sprintf("%d:%02d:%02d.%02d", hours, minutes, seconds, (d%time.Second)/centisecond)
}

func clockParts(d time.Duration) (hours, minutes, seconds int64) {
	return int64(d / time.Hour), int64(d % time.Hour / time.Minute), int64(d % time.Minute / time.Second)
}
