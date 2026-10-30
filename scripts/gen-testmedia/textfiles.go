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
