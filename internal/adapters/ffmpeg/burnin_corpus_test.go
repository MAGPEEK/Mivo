package ffmpeg_test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"testing"
	"time"

	"github.com/magpeek/mivo/internal/adapters/ffmpeg"
	"github.com/magpeek/mivo/internal/domain/playback"
	"github.com/magpeek/mivo/internal/domain/playback/playbacktest"
)

const (
	// burnInFrameTime is where the frame is read from the produced stream. The corpus cues
	// show 0-0.9 s and 1.0-1.9 s of the source, so 0.5 s of output lies inside a cue for a
	// start of 0 and, with a start of 1 s, shows the second cue.
	burnInFrameTime = 500 * time.Millisecond
	// burnInMinimumChangedPixels is how many of the 76800 gray pixels of a 320x240 frame a
	// drawn subtitle must change; a line of text covers far more.
	burnInMinimumChangedPixels = 200
	// burnInPixelDelta is the gray difference that counts as changed, above encoder noise.
	burnInPixelDelta = 24
	// burnInScaledHeight is the height of the downscale case.
	burnInScaledHeight = 120
	burnInSourceWidth  = 320
	burnInSourceHeight = 240
	burnInSeek         = time.Second
)

// burnInCase is one subtitle drawn into one corpus clip.
type burnInCase struct {
	name string
	clip string
	// build returns the BurnIn for the clip at source.
	build func(t *testing.T, source string) *ffmpeg.BurnIn
	// start is the seek position.
	start time.Duration
	// height is the output height cap; 0 keeps the source size.
	height int
}

func imageBurnIn(index int) func(*testing.T, string) *ffmpeg.BurnIn {
	return func(*testing.T, string) *ffmpeg.BurnIn {
		return &ffmpeg.BurnIn{Kind: ffmpeg.BurnInImage, StreamIndex: index}
	}
}

func embeddedTextBurnIn(ordinal int) func(*testing.T, string) *ffmpeg.BurnIn {
	return func(_ *testing.T, source string) *ffmpeg.BurnIn {
		return &ffmpeg.BurnIn{Kind: ffmpeg.BurnInText, File: source, Ordinal: ordinal}
	}
}

// TestBurnInDrawsTheSubtitleIntoTheFrame transcodes corpus clips with a subtitle burnt in and
// compares one decoded frame with the same frame of the run without subtitle: it must differ
// in a visible part of the picture, also after a seek and on a downscaled picture.
func TestBurnInDrawsTheSubtitleIntoTheFrame(t *testing.T) {
	tools, manifest := requireHLSTools(t)

	cases := []burnInCase{
		{name: "PGS", clip: "tracks-mkv-pgs", build: imageBurnIn(2)},
		{name: "PGS after a seek", clip: "tracks-mkv-pgs", build: imageBurnIn(2), start: burnInSeek},
		{name: "PGS on a downscaled picture", clip: "tracks-mkv-pgs", build: imageBurnIn(2), height: burnInScaledHeight},
		{name: "DVD subtitle", clip: "tracks-mkv-dvdsub", build: imageBurnIn(2)},
		{name: "styled ASS", clip: "tracks-mkv-ass-styled-srt", build: embeddedTextBurnIn(0)},
		{name: "SRT of the same file", clip: "tracks-mkv-ass-styled-srt", build: embeddedTextBurnIn(1)},
		{name: "text on a downscaled picture", clip: "tracks-mkv-ass-styled-srt", build: embeddedTextBurnIn(0), height: burnInScaledHeight},
	}

	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			source := corpusPath(t, corpusCaseByName(t, manifest, test.clip).File)
			tools.assertBurnIn(t, source, test.build(t, source), test)
		})
	}
}

// TestBurnInReadsAnExternalSubtitleFileWithAnyName draws an external SRT file whose name is
// built from every character the filtergraph syntax treats specially. The file is found and
// rendered, and nothing of the name ends up as filter syntax.
func TestBurnInReadsAnExternalSubtitleFileWithAnyName(t *testing.T) {
	tools, manifest := requireHLSTools(t)

	source := corpusPath(t, corpusCaseByName(t, manifest, "tracks-mkv-pgs").File)
	sidecar, err := os.ReadFile(corpusPath(t, "subtitle/sidecar.srt"))
	if err != nil {
		t.Fatalf("read the corpus SRT: %v", err)
	}

	names := []string{
		"plain.srt",
		"it's, [a];b=c %d.srt",
		"x',drawtext=text=pwned,a='y.srt",
		"semi & $(touch x) `id`.srt",
		"unicode äöü 字幕.srt",
	}

	if runtime.GOOS != "windows" {
		names = append(names, "quote\"s.srt", "colon:name.srt", `back\slash.srt`, `x\',drawtext=text=pwned:a=\'y.srt`, "C:\\x.srt")
	}

	for position, name := range names {
		// Numbered, because the test name becomes the temporary directory, which must stay
		// free of "%" (output template) whatever the subtitle file is called.
		t.Run("name "+strconv.Itoa(position), func(t *testing.T) {
			t.Parallel()

			file := filepath.Join(t.TempDir(), name)
			if err := os.WriteFile(file, sidecar, 0o600); err != nil { //nolint:gosec // a temporary directory and a fixed list of names
				t.Fatalf("write %q: %v", name, err)
			}

			burnIn := &ffmpeg.BurnIn{Kind: ffmpeg.BurnInText, File: file}
			tools.assertBurnIn(t, source, burnIn, burnInCase{name: name})
		})
	}
}

// assertBurnIn produces the clip with and without the subtitle and compares one frame.
func (tools hlsTools) assertBurnIn(t *testing.T, source string, burnIn *ffmpeg.BurnIn, test burnInCase) {
	t.Helper()

	withSubtitle := tools.burnInFrame(t, source, burnIn, test)
	without := tools.burnInFrame(t, source, nil, test)

	wantSize := burnInSourceWidth * burnInSourceHeight
	if test.height > 0 {
		wantSize = (burnInSourceWidth * test.height / burnInSourceHeight) * test.height
	}

	if len(withSubtitle) != wantSize || len(without) != wantSize {
		t.Fatalf("frame sizes %d and %d, want %d gray pixels (the planned output size)", len(withSubtitle), len(without), wantSize)
	}

	changed := 0

	for index := range withSubtitle {
		if difference(withSubtitle[index], without[index]) > burnInPixelDelta {
			changed++
		}
	}

	if minimum := burnInMinimumChangedPixels * wantSize / (burnInSourceWidth * burnInSourceHeight); changed < minimum {
		t.Errorf("%d pixels differ from the frame without subtitle, want at least %d: the subtitle is not drawn", changed, minimum)
	}
}

func difference(a, b byte) byte {
	if a > b {
		return a - b
	}

	return b - a
}

// burnInFrame runs the HLS job of the clip (with burnIn, or without a subtitle when nil) and
// returns the gray pixels of the frame at burnInFrameTime of the output.
func (tools hlsTools) burnInFrame(t *testing.T, source string, burnIn *ffmpeg.BurnIn, test burnInCase) []byte {
	t.Helper()

	description, err := tools.prober.Probe(t.Context(), source)
	if err != nil {
		t.Fatalf("Probe: %v", err)
	}

	profile := playbacktest.ChromiumDesktop()
	// A client that renders no WebVTT gets every subtitle drawn into the picture.
	profile.SubtitleFormats = nil

	// Every clip has a subtitle at stream 2; selecting it makes the plan burn it in, which
	// re-encodes the video. The run without subtitle keeps that plan and only drops the subtitle,
	// so both frames come from the same encode.
	track := "s:2"
	request := playback.Request{StartPosition: test.start, SubtitleTrackID: &track}

	plan, err := playback.Decide(playback.Input{
		Description: description, Profile: profile, Capabilities: playbacktest.FullCapabilities(),
		Request: request, Policy: playback.Policy{MaxTranscodeHeight: test.height},
	})
	if err != nil {
		t.Fatalf("Decide: %v", err)
	}

	if plan.Mode != playback.ModeTranscode || plan.Subtitle.Mode != playback.SubtitleBurnIn {
		t.Fatalf("plan = %s with subtitle %+v, want a transcode that burns the subtitle in", plan.Mode, plan.Subtitle)
	}

	if burnIn == nil {
		plan.Subtitle = playback.SubtitleDecision{Mode: playback.SubtitleOff}
	}

	return tools.frameOfJob(t, ffmpeg.HLSJob{
		Plan: plan, InputPath: source, StartPosition: test.start, OutputDirectory: t.TempDir(), BurnIn: burnIn,
	})
}

// frameOfJob runs job and decodes the frame at burnInFrameTime of its output.
func (tools hlsTools) frameOfJob(t *testing.T, job ffmpeg.HLSJob) []byte {
	t.Helper()

	hlsRunFFmpeg(t, tools.ffmpegPath, job)

	segments := hlsAssertPlaylist(t, job.OutputDirectory)
	joined := hlsJoin(t, job.OutputDirectory, segments...)

	// The executable comes from PATH and the arguments are a slice; no shell is involved.
	command := exec.CommandContext(t.Context(), tools.ffmpegPath, //nolint:gosec // see above
		"-hide_banner", "-nostdin", "-loglevel", "error", "-ss", strconv.FormatFloat(burnInFrameTime.Seconds(), 'f', 3, 64),
		"-i", joined, "-frames:v", "1", "-f", "rawvideo", "-pix_fmt", "gray", "-")

	var stderr bytes.Buffer

	command.Stderr = &stderr

	frame, err := command.Output()
	if err != nil {
		t.Fatalf("decode a frame of the output: %v\n%s", err, stderr.Bytes())
	}

	return frame
}
