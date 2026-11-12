package main

import (
	"encoding/json"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestBuildArgsForCommentaryDownmixesAndTitlesTheTracks(t *testing.T) {
	t.Parallel()

	want := slices.Concat(argsPrefix(),
		[]string{
			"-f", "lavfi", "-i", "testsrc2=size=320x240:rate=25:duration=2",
			"-f", "lavfi", "-i", "sine=frequency=440:sample_rate=48000:duration=2",
			"-f", "lavfi", "-i", "sine=frequency=880:sample_rate=48000:duration=2",
			"-f", "lavfi", "-i", "sine=frequency=1320:sample_rate=48000:duration=2",
			"-map", "0:v:0", "-map", "1:a:0", "-map", "2:a:0", "-map", "3:a:0", "-ac", "6",
		},
		reproducibleArgs(),
		[]string{
			"-c:v", "libx264", "-preset", "veryfast", "-profile:v", "high", "-pix_fmt", "yuv420p",
			"-c:a", "aac", "-ac:a:0", "2", "-ac:a:2", "2",
			"-disposition:v:0", "default",
			"-metadata:s:a:0", "language=eng", "-disposition:a:0", "default",
			"-metadata:s:a:1", "language=deu", "-disposition:a:1", "0",
			"-metadata:s:a:2", "language=eng", "-metadata:s:a:2", "title=Commentary", "-disposition:a:2", "0",
			"-t", "2", "-f", "matroska", "out.partial",
		})

	c := findCase(t, caseCommentary)
	if got := buildArgs(c, "", "out.partial"); !slices.Equal(got, want) {
		t.Errorf("buildArgs =\n%q\nwant\n%q", got, want)
	}

	audio := c.expected().Audio
	channels := []int{audio[0].Channels, audio[1].Channels, audio[2].Channels}

	if want := []int{stereoChannels, surround51Channels, stereoChannels}; !slices.Equal(channels, want) {
		t.Errorf("channels = %v, want %v", channels, want)
	}

	if *audio[2].Track != (trackExpectation{Language: "eng", Title: "Commentary"}) || !audio[0].Track.Default {
		t.Errorf("tracks = %+v %+v", *audio[0].Track, *audio[2].Track)
	}
}

func TestBuildArgsForStyledSubtitlesPutASSFirst(t *testing.T) {
	t.Parallel()

	c := findCase(t, caseStyledSubtitles)
	args := strings.Join(buildArgs(c, "in", "out.partial"), " ")

	for _, want := range []string{
		"-f ass -i " + filepath.Join("in", "styled.ass") + " -f srt -i " + filepath.Join("in", "english.srt"),
		"-map 2:s:0 -map 3:s:0",
		"-c:s:0 ass -c:s:1 srt",
		"-metadata:s:s:0 language=eng -metadata:s:s:0 title=Styled -disposition:s:0 default",
	} {
		if !strings.Contains(args, want) {
			t.Errorf("styled subtitle args lack %q:\n%s", want, args)
		}
	}

	subtitles := c.expected().Subtitles
	if len(subtitles) != 2 || subtitles[0].Codec != codecASS || subtitles[1].Codec != codecSubRip {
		t.Errorf("subtitles = %+v", subtitles)
	}
}

func TestFormatStyledASSUsesStylesAndOverrides(t *testing.T) {
	t.Parallel()

	script := formatStyledASS(subtitleCues("one\nmore", "two"), 320, 240)

	for _, want := range []string{
		"PlayResX: 320\nPlayResY: 240\n",
		"Style: Default,Arial,20,",
		"Style: Sign,Courier New,28,&H0000FFFF,",
		"\n\n[Events]\n",
		`Dialogue: 0,0:00:00.00,0:00:00.90,Default,,0,0,0,,{\i1\c&H00FF00&}one\Nmore` + "\n",
		`Dialogue: 0,0:00:01.00,0:00:01.90,Sign,,0,0,0,,{\pos(160,60)\fad(100,100)}two` + "\n",
	} {
		if !strings.Contains(script, want) {
			t.Errorf("styled ASS lacks %q:\n%s", want, script)
		}
	}

	if strings.Contains(script, "%!") {
		t.Errorf("styled ASS has a formatting error:\n%s", script)
	}
}

func TestBitmapSubtitleCasesCopyPGSOrEncodeDVDSubtitles(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name, codecArg, codec string
		encoders              []string
	}{
		{name: casePGS, codecArg: "-c:s copy", codec: "hdmv_pgs_subtitle", encoders: []string{"libx264", "aac"}},
		{name: caseDVDSubtitle, codecArg: "-c:s dvdsub", codec: "dvd_subtitle", encoders: []string{"libx264", "aac", "dvdsub"}},
	}

	for _, test := range tests {
		c := findCase(t, test.name)
		args := strings.Join(buildArgs(c, "in", "out.partial"), " ")
		input := "duration=2 -f sup -i " + filepath.Join("in", "subtitles.sup")

		// Only the re-encoded stream needs durations computed from the clearing display sets.
		if test.name == caseDVDSubtitle {
			input = "duration=2 -fix_sub_duration -f sup -i " + filepath.Join("in", "subtitles.sup")
		}

		for _, want := range []string{input, "-map 2:s:0", test.codecArg} {
			if !strings.Contains(args, want) {
				t.Errorf("%s: args lack %q:\n%s", test.name, want, args)
			}
		}

		if !slices.Equal(c.Encoders, test.encoders) || c.Expect.Subtitles[0].Codec != test.codec {
			t.Errorf("%s: encoders %v, subtitle %+v", test.name, c.Encoders, c.Expect.Subtitles[0])
		}

		if content := c.Inputs[0].Content; !strings.HasPrefix(content, pgsMagic) {
			t.Errorf("%s: the subtitle input is no PGS stream", test.name)
		}
	}
}

func TestClipBitmapCuesLieInsideTheClip(t *testing.T) {
	t.Parallel()

	cues := clipBitmapCues()
	if len(cues) != 2 {
		t.Fatalf("%d cues, want 2", len(cues))
	}

	for _, cue := range cues {
		if cue.start < 0 || cue.end <= cue.start || cue.end > clipDuration {
			t.Errorf("cue %v-%v lies outside the %v clip", cue.start, cue.end, clipDuration)
		}
	}
}

func TestToneMappingCaseIsTheLongHDR10Clip(t *testing.T) {
	t.Parallel()

	long := findCase(t, caseToneMapping)
	short := findCase(t, "video-mkv-hevc10-hdr10-ac3")

	if long.Source.Duration != 20*time.Second || long.Category != categoryHDR || long.Output != "hdr/hevc_main10_hdr10_ac3_20s.mkv" {
		t.Errorf("tone-mapping case = %s in %s, %v", long.Output, long.Category, long.Source.Duration)
	}

	if !slices.Equal(long.Args, short.Args) || *long.Expect.Video != *short.Expect.Video {
		t.Errorf("the long HDR10 clip must be encoded like the short one")
	}

	// The refactoring must not move or rename the clip corpus tests and E2E depend on.
	if short.Category != categoryVideo || short.Output != "video/hevc_main10_hdr10_ac3.mkv" || short.Source != standardSource() {
		t.Errorf("short HDR10 case = %+v", short)
	}
}

// TestCaseTagsNameRegisteredCases keeps the tag table honest: a renamed case would otherwise
// silently lose its tags.
func TestCaseTagsNameRegisteredCases(t *testing.T) {
	t.Parallel()

	registered := map[string]bool{}
	for _, c := range allCases() {
		registered[c.Name] = true
	}

	for name, tags := range caseTags() {
		if !registered[name] {
			t.Errorf("caseTags names %s, which is no registered case", name)
		}

		if len(tags) == 0 {
			t.Errorf("caseTags lists %s without tags", name)
		}
	}
}

// casesByTag lists the case names of every tag and whether a tag has a case that is no gap.
func casesByTag(t *testing.T) (tagged map[string][]string, buildable map[string]bool) {
	t.Helper()

	tagged = map[string][]string{}
	buildable = map[string]bool{}

	for _, c := range allCases() {
		if !slices.IsSorted(c.Tags) {
			t.Errorf("%s: tags %v are not sorted", c.Name, c.Tags)
		}

		for _, tag := range c.Tags {
			tagged[tag] = append(tagged[tag], c.Name)
			buildable[tag] = buildable[tag] || !c.isGap()
		}
	}

	return tagged, buildable
}

func TestRegistryTagsTheMilestone4Inputs(t *testing.T) {
	t.Parallel()

	tagged, buildable := casesByTag(t)

	want := map[string][]string{
		tagImageSubtitle:  {"subtitle-sup-pgs", "subtitle-vobsub", "subtitle-ts-dvb", casePGS, caseDVDSubtitle},
		tagStyledSubtitle: {caseStyledSubtitles},
		tagTextSubtitle:   {"subtitle-mkv-srt-ass", "subtitle-mp4-movtext"},
		tagCommentary:     {caseCommentary},
		tagMultiAudio:     {caseCommentary, "multitrack-mkv-eng-deu"},
		tagSurround:       {"video-mp4-h264-aac-51", caseCommentary},
		tagHiRes:          {"audio-flac-24bit-96k"},
		tagDSD:            {"audio-dsf-dsd"},
		tagToneMapping:    {"video-mkv-hevc10-hdr10-ac3", caseToneMapping},
		tagLong:           {"video-mkv-h264-aac-long", caseToneMapping},
	}

	for tag, names := range want {
		for _, name := range names {
			if !slices.Contains(tagged[tag], name) {
				t.Errorf("tag %s lacks %s (has %v)", tag, name, tagged[tag])
			}
		}

		// DSD is the one feature without a file: FFmpeg cannot write DSF or DFF (ADR 0021).
		if wantBuildable := tag != tagDSD; buildable[tag] != wantBuildable {
			t.Errorf("tag %s: buildable = %t, want %t", tag, buildable[tag], wantBuildable)
		}
	}

	if untagged := findCase(t, "video-mkv-h264-aac"); len(untagged.Tags) != 0 {
		t.Errorf("video-mkv-h264-aac has tags %v", untagged.Tags)
	}
}

func TestManifestEntryCarriesTagsAndTrackTitles(t *testing.T) {
	t.Parallel()

	c := findCase(t, caseCommentary)
	entry := newBuiltEntry(c)

	if want := []string{tagCommentary, tagMultiAudio, tagSurround}; !slices.Equal(entry.Tags, want) {
		t.Errorf("entry tags = %v, want %v", entry.Tags, want)
	}

	// The entry must not share the slice with the case.
	entry.Tags[0] = "changed"
	if c.Tags[0] != tagCommentary {
		t.Errorf("the entry shares its tags with the case")
	}

	data, err := json.Marshal(newSkippedEntry(findCase(t, "audio-dsf-dsd"), skipKnownGap, "gap"))
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(string(data), `"tags":["dsd"],"expected":`) {
		t.Errorf("gap entry JSON lacks its tags: %s", data)
	}

	data, err = json.Marshal(c.expected().Audio[2].Track)
	if err != nil {
		t.Fatal(err)
	}

	if want := `{"language":"eng","title":"Commentary","default":false,"forced":false}`; string(data) != want {
		t.Errorf("track JSON = %s, want %s", data, want)
	}
}
