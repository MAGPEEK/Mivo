package main

import (
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"
)

func sampleCues() []cue {
	return []cue{
		{Start: 0, End: 900 * time.Millisecond, Text: "First"},
		{Start: time.Hour + 2*time.Minute + 3*time.Second + 45*time.Millisecond, End: 2 * time.Hour, Text: "Two\nlines"},
	}
}

func TestFormatSRT(t *testing.T) {
	t.Parallel()

	const want = "1\n00:00:00,000 --> 00:00:00,900\nFirst\n" +
		"\n" +
		"2\n01:02:03,045 --> 02:00:00,000\nTwo\nlines\n"

	if got := formatSRT(sampleCues()); got != want {
		t.Errorf("formatSRT =\n%q\nwant\n%q", got, want)
	}
}

func TestGeneratedSRTFilesAreValid(t *testing.T) {
	t.Parallel()

	// One SubRip cue: counter, timing line with comma milliseconds and at least one line of
	// text. Blocks are separated by exactly one blank line.
	srtBlock := regexp.MustCompile(`^\d+\n\d{2}:\d{2}:\d{2},\d{3} --> \d{2}:\d{2}:\d{2},\d{3}\n(?:[^\n]+\n)+$`)

	for _, content := range []string{formatSRT(englishCues()), formatSRT(germanCues())} {
		blocks := strings.Split(content, "\n\n")
		for index, block := range blocks {
			if index < len(blocks)-1 {
				block += "\n"
			}

			if !srtBlock.MatchString(block) {
				t.Errorf("block %d is no valid SRT cue: %q", index, block)
			}

			if !strings.HasPrefix(block, strconv.Itoa(index+1)+"\n") {
				t.Errorf("block %d does not start with counter %d: %q", index, index+1, block)
			}
		}
	}
}

func TestSubtitleCuesFitIntoTheClip(t *testing.T) {
	t.Parallel()

	cues := englishCues()
	for index, event := range cues {
		if event.Start >= event.End || event.End > clipDuration {
			t.Errorf("cue %d spans %v-%v, outside the %v clip", index, event.Start, event.End, clipDuration)
		}

		if index > 0 && event.Start < cues[index-1].End {
			t.Errorf("cue %d overlaps the previous one", index)
		}
	}
}

func TestFormatWebVTT(t *testing.T) {
	t.Parallel()

	const want = "WEBVTT\n" +
		"\n00:00:00.000 --> 00:00:00.900\nFirst\n" +
		"\n01:02:03.045 --> 02:00:00.000\nTwo\nlines\n"

	if got := formatWebVTT(sampleCues()); got != want {
		t.Errorf("formatWebVTT =\n%q\nwant\n%q", got, want)
	}
}

func TestFormatASS(t *testing.T) {
	t.Parallel()

	got := formatASS(sampleCues(), 640, 480)

	for _, want := range []string{
		"[Script Info]\nScriptType: v4.00+\nPlayResX: 640\nPlayResY: 480\n",
		"\n[V4+ Styles]\nFormat: Name, Fontname,",
		"\n[Events]\nFormat: Layer, Start, End, Style, Name, MarginL, MarginR, MarginV, Effect, Text\n",
		"Dialogue: 0,0:00:00.00,0:00:00.90,Default,,0,0,0,,First\n",
		// Centiseconds are truncated and line breaks become \N.
		`Dialogue: 0,1:02:03.04,2:00:00.00,Default,,0,0,0,,Two\Nlines` + "\n",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("formatASS lacks %q:\n%s", want, got)
		}
	}

	if !strings.HasSuffix(got, "lines\n") {
		t.Errorf("formatASS does not end with the last event:\n%s", got)
	}
}

func TestFormatFFMetadataEscapesReservedCharacters(t *testing.T) {
	t.Parallel()

	got := formatFFMetadata([]chapter{{Title: `A=B; #1 \ end`, Start: 1500 * time.Millisecond, End: 3 * time.Second}})

	const want = ";FFMETADATA1\n[CHAPTER]\nTIMEBASE=1/1000\nSTART=1500\nEND=3000\ntitle=A\\=B\\; \\#1 \\\\ end\n"
	if got != want {
		t.Errorf("formatFFMetadata =\n%q\nwant\n%q", got, want)
	}
}
