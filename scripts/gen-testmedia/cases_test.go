package main

import (
	"encoding/json"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

func findCase(t *testing.T, name string) mediaCase {
	t.Helper()

	for _, c := range allCases() {
		if c.Name == name {
			return c
		}
	}

	t.Fatalf("case %q not in registry", name)

	return mediaCase{}
}

// argsPrefix and reproducibleArgs are the generic parts every argument list contains.
func argsPrefix() []string { return []string{"-hide_banner", "-nostdin", "-loglevel", "error", "-y"} }

func reproducibleArgs() []string {
	return []string{"-map_metadata:g", "-1", "-map_metadata:s", "-1", "-fflags", "+bitexact", "-flags", "+bitexact"}
}

func TestBuildArgsForRepresentativeCases(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		want []string
	}{
		{
			name: "video-mkv-h264-aac",
			want: slices.Concat(argsPrefix(),
				[]string{
					"-f", "lavfi", "-i", "testsrc2=size=320x240:rate=25:duration=2",
					"-f", "lavfi", "-i", "sine=frequency=440:sample_rate=48000:duration=2",
					"-map", "0:v:0", "-map", "1:a:0", "-ac", "2",
				},
				reproducibleArgs(),
				[]string{
					"-c:v", "libx264", "-preset", "veryfast", "-profile:v", "high", "-pix_fmt", "yuv420p",
					"-c:a", "aac",
					"-t", "2", "-f", "matroska", "out.partial",
				}),
		},
		{
			// RealAudio 1.0 forces an 8 kHz mono source; the muxer differs from the extension.
			name: "video-rmvb-rv20-ra144",
			want: slices.Concat(argsPrefix(),
				[]string{
					"-f", "lavfi", "-i", "testsrc2=size=320x240:rate=25:duration=2",
					"-f", "lavfi", "-i", "sine=frequency=440:sample_rate=8000:duration=2",
					"-map", "0:v:0", "-map", "1:a:0", "-ac", "1",
				},
				reproducibleArgs(),
				[]string{
					"-c:v", "rv20", "-pix_fmt", "yuv420p", "-c:a", "real_144",
					"-t", "2", "-f", "rm", "out.partial",
				}),
		},
		{
			name: "audio-flac-24bit-96k",
			want: slices.Concat(argsPrefix(),
				[]string{
					"-f", "lavfi", "-i", "sine=frequency=440:sample_rate=96000:duration=5",
					"-map", "0:a:0", "-ac", "2",
				},
				reproducibleArgs(),
				[]string{
					"-c:a", "flac", "-sample_fmt", "s32", "-bits_per_raw_sample", "24",
					"-t", "5", "-f", "flac", "out.partial",
				}),
		},
		{
			// Still images limit the output to one frame and tell image2 the name is fixed.
			name: "image-png",
			want: slices.Concat(argsPrefix(),
				[]string{
					"-f", "lavfi", "-i", "testsrc2=size=320x240:rate=25:duration=1",
					"-map", "0:v:0", "-frames:v", "1",
				},
				reproducibleArgs(),
				[]string{
					"-c:v", "png", "-pix_fmt", "rgb24", "-update", "1",
					"-t", "1", "-f", "image2", "out.partial",
				}),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			got := buildArgs(findCase(t, test.name), "", "out.partial")
			if !slices.Equal(got, test.want) {
				t.Errorf("buildArgs =\n%q\nwant\n%q", got, test.want)
			}
		})
	}
}

func TestBuildArgsForAudiobookAddsChaptersAndTags(t *testing.T) {
	t.Parallel()

	inputDir := filepath.Join("inputs", "m4b")
	want := slices.Concat(argsPrefix(),
		[]string{
			"-f", "lavfi", "-i", "sine=frequency=440:sample_rate=48000:duration=5",
			// The chapter list is the second input; its chapters are copied, not a stream.
			"-f", "ffmetadata", "-i", filepath.Join(inputDir, "chapters.ffmeta"),
			"-map", "0:a:0", "-ac", "2",
			"-map_chapters", "1",
		},
		reproducibleArgs(),
		[]string{
			"-c:a", "aac",
			"-metadata", "album=Generated Test Corpus",
			"-metadata", "artist=Generated Narrator",
			"-metadata", "title=Generated Test Book",
			"-t", "5", "-f", "ipod", "out.partial",
		})

	c := findCase(t, "audio-m4b-aac-chapters")
	if got := buildArgs(c, inputDir, "out.partial"); !slices.Equal(got, want) {
		t.Errorf("buildArgs =\n%q\nwant\n%q", got, want)
	}

	wantChapters := ";FFMETADATA1\n" +
		"[CHAPTER]\nTIMEBASE=1/1000\nSTART=0\nEND=2500\ntitle=Chapter 1\n" +
		"[CHAPTER]\nTIMEBASE=1/1000\nSTART=2500\nEND=5000\ntitle=Chapter 2\n"
	if len(c.Inputs) != 1 || c.Inputs[0].Content != wantChapters {
		t.Errorf("chapter input = %+v, want content\n%s", c.Inputs, wantChapters)
	}

	expected := c.expected()
	if len(expected.Chapters) != 2 || expected.Chapters[1] != (chapterExpectation{"Chapter 2", 2.5, 5}) {
		t.Errorf("expected chapters = %+v", expected.Chapters)
	}

	if expected.Tags["title"] != "Generated Test Book" || len(expected.Tags) != 3 {
		t.Errorf("expected tags = %v", expected.Tags)
	}
}

func TestBuildArgsForMultiTrackMapsAndTagsEveryStream(t *testing.T) {
	t.Parallel()

	inputDir := "in"
	want := slices.Concat(argsPrefix(),
		[]string{
			"-f", "lavfi", "-i", "testsrc2=size=320x240:rate=25:duration=2",
			// Each audio track has its own tone, so the tracks are audibly different.
			"-f", "lavfi", "-i", "sine=frequency=440:sample_rate=48000:duration=2",
			"-f", "lavfi", "-i", "sine=frequency=880:sample_rate=48000:duration=2",
			"-f", "srt", "-i", filepath.Join(inputDir, "english.srt"),
			"-f", "srt", "-i", filepath.Join(inputDir, "german.srt"),
			"-map", "0:v:0", "-map", "1:a:0", "-map", "2:a:0", "-ac", "2",
			"-map", "3:s:0", "-map", "4:s:0",
		},
		reproducibleArgs(),
		[]string{
			"-c:v", "libx264", "-preset", "veryfast", "-profile:v", "high", "-pix_fmt", "yuv420p",
			"-c:a", "aac", "-c:s", "srt",
			// FFmpeg's Matroska muxer passes dispositions through, so the video track is
			// marked default explicitly; video has no language tag.
			"-disposition:v:0", "default",
			"-metadata:s:a:0", "language=eng", "-disposition:a:0", "default",
			"-metadata:s:a:1", "language=deu", "-disposition:a:1", "0",
			"-metadata:s:s:0", "language=eng", "-disposition:s:0", "0",
			"-metadata:s:s:1", "language=deu", "-disposition:s:1", "forced",
			"-t", "2", "-f", "matroska", "out.partial",
		})

	c := findCase(t, "multitrack-mkv-eng-deu")
	if got := buildArgs(c, inputDir, "out.partial"); !slices.Equal(got, want) {
		t.Errorf("buildArgs =\n%q\nwant\n%q", got, want)
	}

	expected := c.expected()
	if len(expected.Audio) != 2 || len(expected.Subtitles) != 2 {
		t.Fatalf("expected streams = %+v", expected)
	}

	if *expected.Audio[1].Track != (trackExpectation{Language: "deu"}) || expected.Audio[1].SampleRate != defaultSampleRate {
		t.Errorf("second audio expectation = %+v %+v", expected.Audio[1], *expected.Audio[1].Track)
	}

	if *expected.Subtitles[1].Track != (trackExpectation{Language: "deu", Forced: true}) {
		t.Errorf("second subtitle expectation = %+v", *expected.Subtitles[1].Track)
	}

	if expected.Video.Track == nil || *expected.Video.Track != (trackExpectation{Default: true}) {
		t.Errorf("video track expectation = %+v", expected.Video.Track)
	}
}

func TestEmbeddedSubtitleMKVMarksVideoAndAudioDefault(t *testing.T) {
	t.Parallel()

	c := findCase(t, "subtitle-mkv-srt-ass")
	args := strings.Join(buildArgs(c, "in", "out.partial"), " ")

	for _, want := range []string{"-disposition:v:0 default", "-disposition:a:0 default"} {
		if !strings.Contains(args, want) {
			t.Errorf("args lack %q:\n%s", want, args)
		}
	}

	expected := c.expected()
	if expected.Video.Track == nil || !expected.Video.Track.Default ||
		expected.Audio[0].Track == nil || *expected.Audio[0].Track != (trackExpectation{Default: true}) {
		t.Errorf("video track = %+v, audio track = %+v", expected.Video.Track, expected.Audio[0].Track)
	}
}

func TestSeekCasesHaveAFixedKeyframeInterval(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		// want are argument sequences the case must contain.
		want      []string
		wantVideo string
		wantAudio string
		container string
	}{
		{
			name: "video-mkv-h264-aac-long",
			want: []string{
				"testsrc2=size=320x240:rate=25:duration=30", "sine=frequency=440:sample_rate=48000:duration=30",
				"-c:v libx264 -preset veryfast -profile:v high", "-g 50 -keyint_min 50 -sc_threshold 0", "-t 30 -f matroska",
			},
			wantVideo: "h264", wantAudio: "aac", container: "mkv",
		},
		{
			name: "video-ts-mpeg2-mp2-long",
			want: []string{
				"testsrc2=size=320x240:rate=25:duration=30",
				"-c:v mpeg2video", "-g 50 -sc_threshold 1000000000", "-t 30 -f mpegts",
			},
			wantVideo: "mpeg2video", wantAudio: "mp2", container: "mpegts",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			c := findCase(t, test.name)
			args := strings.Join(buildArgs(c, "", "out.partial"), " ")

			for _, want := range test.want {
				if !strings.Contains(args, want) {
					t.Errorf("args lack %q:\n%s", want, args)
				}
			}

			expected := c.expected()
			if expected.DurationSeconds != 30 || expected.Container != test.container ||
				expected.Video.Codec != test.wantVideo || expected.Audio[0].Codec != test.wantAudio {
				t.Errorf("expectation = %+v video %+v audio %+v", expected, *expected.Video, expected.Audio)
			}
		})
	}
}

func TestOnlySeekCasesChangeTheClipDuration(t *testing.T) {
	t.Parallel()

	source := standardSource()
	if long := source.withDuration(seekClipDuration); long.Duration != 30*time.Second || source.Duration != clipDuration {
		t.Errorf("withDuration = %v, original %v", long.Duration, source.Duration)
	}

	var long []string

	for _, c := range allCases() {
		if c.Source.Duration == seekClipDuration {
			long = append(long, c.Name)
		}
	}

	if want := []string{"video-mkv-h264-aac-long", "video-ts-mpeg2-mp2-long"}; !slices.Equal(long, want) {
		t.Errorf("30 s cases = %v, want %v", long, want)
	}
}

func TestBuildArgsForHDR10TagsFramesAndAddsMetadata(t *testing.T) {
	t.Parallel()

	c := findCase(t, "video-mkv-hevc10-hdr10-ac3")
	args := strings.Join(buildArgs(c, "", "out.partial"), " ")

	for _, want := range []string{
		"-vf setparams=color_primaries=bt2020:color_trc=smpte2084:colorspace=bt2020nc",
		"-pix_fmt yuv420p10le",
		"-profile:v main10",
		"hdr10=1",
		"master-display=G(13250,34500)",
		"max-cll=1000,400",
	} {
		if !strings.Contains(args, want) {
			t.Errorf("HDR10 args lack %q:\n%s", want, args)
		}
	}

	if video := c.Expect.Video; video.ColorSpace != "bt2020nc" || video.ColorPrimaries != "bt2020" {
		t.Errorf("HDR10 expectation = %+v", video)
	}
}

func TestTrackArgsDispositions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		track trackExpectation
		want  string
	}{
		{track: trackExpectation{Language: "eng"}, want: "0"},
		{track: trackExpectation{Language: "eng", Default: true}, want: "default"},
		{track: trackExpectation{Language: "eng", Forced: true}, want: "forced"},
		{track: trackExpectation{Language: "eng", Default: true, Forced: true}, want: "default+forced"},
	}

	for _, test := range tests {
		want := []string{"-metadata:s:s:0", "language=eng", "-disposition:s:0", test.want}
		if got := trackArgs("s:0", test.track); !slices.Equal(got, want) {
			t.Errorf("trackArgs(%+v) = %q, want %q", test.track, got, want)
		}
	}

	// Without a language only the disposition is set; Matroska then stores "und".
	want := []string{"-disposition:v:0", "default"}
	if got := trackArgs("v:0", trackExpectation{Default: true}); !slices.Equal(got, want) {
		t.Errorf("trackArgs without language = %q, want %q", got, want)
	}
}

func TestSourceArgs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		source      sourceSpec
		wantInputs  []string
		wantOutputs []string
	}{
		{
			name:        "audio only maps the first input",
			source:      sourceSpec{SampleRate: 44100, Channels: 2, Duration: 5 * time.Second},
			wantInputs:  []string{"-f", "lavfi", "-i", "sine=frequency=440:sample_rate=44100:duration=5"},
			wantOutputs: []string{"-map", "0:a:0", "-ac", "2"},
		},
		{
			name:        "video only",
			source:      sourceSpec{Width: 64, Height: 48, FrameRate: 10, Duration: 500 * time.Millisecond},
			wantInputs:  []string{"-f", "lavfi", "-i", "testsrc2=size=64x48:rate=10:duration=0.5"},
			wantOutputs: []string{"-map", "0:v:0"},
		},
		{
			name:        "frame limit",
			source:      sourceSpec{Width: 64, Height: 48, FrameRate: 10, Frames: 10, Duration: time.Second},
			wantInputs:  []string{"-f", "lavfi", "-i", "testsrc2=size=64x48:rate=10:duration=1"},
			wantOutputs: []string{"-map", "0:v:0", "-frames:v", "10"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			inputs, outputs := sourceArgs(test.source)
			if !slices.Equal(inputs, test.wantInputs) || !slices.Equal(outputs, test.wantOutputs) {
				t.Errorf("sourceArgs = %q %q, want %q %q", inputs, outputs, test.wantInputs, test.wantOutputs)
			}
		})
	}
}

func TestRegistryCasesAreConsistent(t *testing.T) {
	t.Parallel()

	outputByName := map[string]string{}
	nameByOutput := map[string]string{}

	for _, c := range allCases() {
		checkCaseShape(t, c)

		if output, seen := outputByName[c.Name]; seen && output != c.Output {
			t.Errorf("alternatives of %s write different files: %s and %s", c.Name, output, c.Output)
		}

		outputByName[c.Name] = c.Output

		for _, output := range caseOutputs(c) {
			if name, seen := nameByOutput[output]; seen && name != c.Name {
				t.Errorf("%s and %s both write %s", name, c.Name, output)
			}

			nameByOutput[output] = c.Name
		}
	}
}

// caseOutputs returns the media file and the sidecar files of a case.
func caseOutputs(c mediaCase) []string {
	outputs := []string{c.Output}
	for _, sidecar := range c.Sidecars {
		outputs = append(outputs, sidecar.Output)
	}

	return outputs
}

func checkCaseShape(t *testing.T, c mediaCase) {
	t.Helper()

	if !strings.HasPrefix(c.Name, c.Category+"-") {
		t.Errorf("%s: name does not start with category %q", c.Name, c.Category)
	}

	for _, output := range caseOutputs(c) {
		if path.Dir(output) != c.Category || path.Ext(output) == "" || strings.Contains(output, `\`) {
			t.Errorf("%s: output %q must be <category>/<file>.<ext> with forward slashes", c.Name, output)
		}
	}

	if c.Expect.Container == "" || c.Expect.ProbeFormat == "" {
		t.Errorf("%s: expected container and probe format are required", c.Name)
	}

	if c.isGap() {
		checkGapShape(t, c)

		return
	}

	if c.Muxer == "" || len(c.Encoders) == 0 {
		t.Errorf("%s: muxer and encoders are required", c.Name)
	}

	checkSourceShape(t, c)
	checkStreamShape(t, c)
}

func checkGapShape(t *testing.T, c mediaCase) {
	t.Helper()

	if !strings.HasPrefix(c.Gap, gapReasonPrefix) || c.Muxer != "" || len(c.Encoders) != 0 || len(c.Args) != 0 {
		t.Errorf("%s: a known gap has a reason and nothing to build", c.Name)
	}
}

func checkSourceShape(t *testing.T, c mediaCase) {
	t.Helper()

	wantDuration := map[string]time.Duration{
		categoryVideo: clipDuration, categoryAudio: audioDuration, categoryImage: imageSourceDuration,
		categorySubtitle: clipDuration, categoryMultiTrack: clipDuration,
	}[c.Category]
	// Seek cases are the only long clips; their names say so.
	if strings.HasSuffix(c.Name, seekCaseSuffix) {
		wantDuration = seekClipDuration
	}

	if c.Source.Duration != wantDuration {
		t.Errorf("%s: %s sources last %v, got %v", c.Name, c.Category, wantDuration, c.Source.Duration)
	}

	if isImage := c.Category == categoryImage; isImage != (c.Source.Frames > 0) {
		t.Errorf("%s: only images limit the frame count", c.Name)
	}
}

func checkStreamShape(t *testing.T, c mediaCase) {
	t.Helper()

	if (c.Expect.Video != nil) != c.Source.hasVideo() || len(c.Expect.Audio) != c.Source.audioTrackCount() {
		t.Errorf("%s: expected streams do not match the source", c.Name)
	}

	subtitleInputs := 0

	for _, input := range c.Inputs {
		if input.Role == roleSubtitle {
			subtitleInputs++
		}
	}

	if len(c.Expect.Subtitles) != subtitleInputs {
		t.Errorf("%s: %d subtitle inputs but %d expected subtitles", c.Name, subtitleInputs, len(c.Expect.Subtitles))
	}
}

func TestRegistryCoversRequiredFormats(t *testing.T) {
	t.Parallel()

	extensions := map[string]bool{}

	for _, c := range allCases() {
		if c.isGap() {
			continue
		}

		for _, output := range caseOutputs(c) {
			extensions[path.Ext(output)] = true
		}
	}

	required := []string{
		// Video containers (M1-18).
		".mkv", ".mp4", ".m4v", ".mov", ".avi", ".ts", ".m2ts", ".mts", ".webm", ".flv", ".wmv",
		".ogv", ".3gp", ".mpg", ".vob", ".mxf", ".rm", ".rmvb",
		// Audio, images and sidecar subtitles (M1-19).
		".flac", ".m4a", ".mp3", ".opus", ".ogg", ".wav", ".aiff", ".wv", ".m4b",
		".jpg", ".png", ".webp", ".gif", ".tiff", ".avif",
		".srt", ".vtt", ".ass",
	}
	for _, extension := range required {
		if !extensions[extension] {
			t.Errorf("no case writes a %s file", extension)
		}
	}
}

func TestRegistryDeclaresKnownGaps(t *testing.T) {
	t.Parallel()

	// ADR 0021 lists these formats as gaps; each one must appear in every manifest.
	want := []string{
		"audio-ape", "audio-dsf-dsd", "audio-dff-dsd", "image-heic", "image-jpeg-exif",
		"subtitle-sup-pgs", "subtitle-vobsub", "subtitle-ts-dvb", "video-rmvb-rv40-cook",
	}

	var got []string

	for _, c := range allCases() {
		if c.isGap() {
			got = append(got, c.Name)
		}
	}

	slices.Sort(got)
	slices.Sort(want)

	if !slices.Equal(got, want) {
		t.Errorf("known gaps = %v, want %v", got, want)
	}
}

func TestExpectedFillsPropertiesFromSource(t *testing.T) {
	t.Parallel()

	c := findCase(t, "video-3gp-h263-aac")
	got := c.expected()

	if got.Video.Width != cifWidth || got.Video.Height != cifHeight {
		t.Errorf("video size = %dx%d, want CIF", got.Video.Width, got.Video.Height)
	}

	if got.Audio[0].SampleRate != defaultSampleRate || got.Audio[0].Channels != stereoChannels {
		t.Errorf("audio = %d Hz/%d ch", got.Audio[0].SampleRate, got.Audio[0].Channels)
	}

	if got.DurationSeconds != clipDuration.Seconds() {
		t.Errorf("duration = %v", got.DurationSeconds)
	}

	if c.Expect.Video.Width != 0 || c.Expect.Audio[0].SampleRate != 0 {
		t.Error("expected() modified the shared case expectation")
	}
}

func TestExpectedForImagesAndSidecars(t *testing.T) {
	t.Parallel()

	if got := findCase(t, "image-gif-animated").expected(); got.DurationSeconds != 0 || got.Video.Width != defaultWidth {
		t.Errorf("image expectation = %+v", got)
	}

	sidecars := findCase(t, "subtitle-sidecar-srt-vtt-ass").expected().Sidecars
	want := []sidecarExpectation{
		{File: "subtitle/sidecar.srt", ProbeFormat: "srt", Codec: "subrip"},
		{File: "subtitle/sidecar.vtt", ProbeFormat: "webvtt", Codec: "webvtt"},
		{File: "subtitle/sidecar.ass", ProbeFormat: "ass", Codec: "ass"},
	}

	if !slices.Equal(sidecars, want) {
		t.Errorf("sidecars = %+v, want %+v", sidecars, want)
	}
}

func TestTextInputArgsIgnoresUnknownRoles(t *testing.T) {
	t.Parallel()

	inputs, outputs := textInputArgs([]textInput{{FileName: "x.txt", Format: "srt", Role: inputRole(9)}}, "dir", 3)

	if !slices.Equal(inputs, []string{"-f", "srt", "-i", filepath.Join("dir", "x.txt")}) || len(outputs) != 0 {
		t.Errorf("textInputArgs = %q %q", inputs, outputs)
	}
}

// TestExpectedReturnsIndependentCopies mutates one result of expected() everywhere and checks
// that neither a second result nor the case itself changed.
func TestExpectedReturnsIndependentCopies(t *testing.T) {
	t.Parallel()

	track := trackExpectation{Language: "eng", Default: true}
	c := sampleCase("video-a", "video/a.mkv")
	c.Expect.Video.Track = &trackExpectation{Default: true}
	c.Expect.Audio[0].Track = &track
	c.Expect.Subtitles = []subtitleExpectation{{Codec: codecSubRip, Track: &trackExpectation{Language: "deu"}}}
	c.Expect.Chapters = []chapterExpectation{{Title: "One", EndSeconds: 1}}
	c.Expect.Tags = map[string]string{"title": "T"}
	// Spare capacity is what let appended sidecars of two results share one array.
	c.Expect.Sidecars = make([]sidecarExpectation, 1, 4)
	c.Expect.Sidecars[0] = sidecarExpectation{File: "video/declared.srt"}
	c.Sidecars = []sidecarFile{{Output: "video/a.srt"}, {Output: "video/a.vtt"}}

	// JSON snapshots cannot alias anything, unlike values compared with reflect.DeepEqual.
	declared := snapshot(t, c.Expect)
	earlier := c.expected()
	want := snapshot(t, earlier)

	mutated := c.expected()
	mutateExpectation(&mutated)

	for name, got := range map[string]string{
		"a later result":    snapshot(t, c.expected()),
		"an earlier result": snapshot(t, earlier),
	} {
		if got != want {
			t.Errorf("mutating one result changed %s:\n%s\nwant\n%s", name, got, want)
		}
	}

	if got := snapshot(t, c.Expect); got != declared {
		t.Errorf("expected() or its result changed the case:\n%s\nwant\n%s", got, declared)
	}
}

func snapshot(t *testing.T, e expectation) string {
	t.Helper()

	data, err := json.Marshal(e)
	if err != nil {
		t.Fatal(err)
	}

	return string(data)
}

// mutateExpectation changes every value an expectation holds through a reference.
func mutateExpectation(e *expectation) {
	const changed = "changed"

	e.Video.Codec = changed
	e.Video.Track.Language = changed
	e.Audio[0].Codec = changed
	e.Audio[0].Track.Language = changed
	e.Subtitles[0].Codec = changed
	e.Subtitles[0].Track.Forced = true
	e.Chapters[0].Title = changed
	e.Tags["title"] = changed

	for index := range e.Sidecars {
		e.Sidecars[index].File = changed
	}
}
