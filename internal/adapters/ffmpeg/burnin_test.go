package ffmpeg_test

import (
	"errors"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/magpeek/mivo/internal/adapters/ffmpeg"
	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/playback"
)

// burnInPlan is a transcode plan whose subtitle mode is burn-in.
func burnInPlan(video playback.StreamDecision) playback.Plan {
	plan := hlsPlan(playback.ModeTranscode, video, hlsCopyAudio(1, "aac"))
	plan.Subtitle = playback.SubtitleDecision{Mode: playback.SubtitleBurnIn, TrackID: "s:2"}

	return plan
}

// burnInArguments returns the arguments of a burn-in job, or fails the test.
func burnInArguments(t *testing.T, plan playback.Plan, burnIn *ffmpeg.BurnIn) []string {
	t.Helper()

	arguments, err := ffmpeg.HLSArguments(ffmpeg.HLSJob{
		Plan: plan, InputPath: hlsAbsolutePath(t, "media", "film.mkv"),
		OutputDirectory: hlsAbsolutePath(t, "sessions", "one"), BurnIn: burnIn,
	})
	if err != nil {
		t.Fatalf("HLSArguments: %v", err)
	}

	return arguments
}

// argumentAfter returns the argument that follows the first flag, "" when flag is missing.
func argumentAfter(arguments []string, flag string) string {
	value, _ := valueAfter(arguments, flag)

	return value
}

func TestHLSArgumentsBurnInGraphs(t *testing.T) {
	t.Parallel()

	file := hlsAbsolutePath(t, "media", "film.mkv")
	fonts := hlsAbsolutePath(t, "fonts")
	slashFile, slashFonts := filepath.ToSlash(file), filepath.ToSlash(fonts)
	// The volume colon of a Windows path is escaped twice; a Linux path has none.
	slashFile = strings.ReplaceAll(slashFile, ":", `\\:`)
	slashFonts = strings.ReplaceAll(slashFonts, ":", `\\:`)

	tests := []struct {
		name   string
		video  playback.StreamDecision
		burnIn ffmpeg.BurnIn
		want   string
	}{
		{
			name:   "image overlay at source size",
			video:  hlsTranscodeVideo(0, 320, 240, false),
			burnIn: ffmpeg.BurnIn{Kind: ffmpeg.BurnInImage, StreamIndex: 2},
			want:   "[0:0][0:2]overlay[burnedin]",
		},
		{
			name:   "image overlay before the downscale",
			video:  hlsTranscodeVideo(0, 1280, 720, true),
			burnIn: ffmpeg.BurnIn{Kind: ffmpeg.BurnInImage, StreamIndex: 4},
			want:   "[0:0][0:4]overlay,scale=1280:720[burnedin]",
		},
		{
			name:   "text after the downscale with the fonts directory",
			video:  hlsTranscodeVideo(0, 0, 720, true),
			burnIn: ffmpeg.BurnIn{Kind: ffmpeg.BurnInText, File: file, Ordinal: 1, FontsDirectory: fonts},
			want:   "[0:0]scale=-2:720,subtitles=filename=" + slashFile + ":si=1:fontsdir=" + slashFonts + "[burnedin]",
		},
		{
			name:   "text without scaling",
			video:  hlsTranscodeVideo(3, 320, 240, false),
			burnIn: ffmpeg.BurnIn{Kind: ffmpeg.BurnInText, File: file},
			want:   "[0:3]subtitles=filename=" + slashFile + ":si=0[burnedin]",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			arguments := burnInArguments(t, burnInPlan(test.video), &test.burnIn)

			if got := argumentAfter(arguments, "-filter_complex"); got != test.want {
				t.Errorf("filter graph = %q, want %q", got, test.want)
			}

			if got := argumentAfter(arguments, "-map"); got != "[burnedin]" {
				t.Errorf("video map = %q, want the graph output", got)
			}

			if slices.Contains(arguments, "-vf") {
				t.Errorf("arguments %q hold a second video filter next to the graph", arguments)
			}

			if !slices.Contains(arguments, "libx264") {
				t.Errorf("arguments %q do not encode the burnt-in video", arguments)
			}
		})
	}
}

func TestHLSArgumentsBurnInKeepsTheSeekBeforeTheInput(t *testing.T) {
	t.Parallel()

	video := hlsTranscodeVideo(0, 320, 240, false)
	arguments, err := ffmpeg.HLSArguments(ffmpeg.HLSJob{
		Plan: burnInPlan(video), InputPath: hlsAbsolutePath(t, "media", "film.mkv"),
		OutputDirectory: hlsAbsolutePath(t, "sessions", "one"), StartPosition: 30 * time.Second,
		BurnIn: &ffmpeg.BurnIn{Kind: ffmpeg.BurnInImage, StreamIndex: 2},
	})
	if err != nil {
		t.Fatalf("HLSArguments: %v", err)
	}

	// One -ss before the only -i seeks the video and the subtitle stream of the graph alike.
	if seek, input := slices.Index(arguments, "-ss"), slices.Index(arguments, "-i"); seek < 0 || seek > input {
		t.Errorf("-ss at %d, -i at %d in %q, want the seek before the input", seek, input, arguments)
	}

	if count := strings.Count(strings.Join(arguments, " "), " -i "); count != 1 {
		t.Errorf("%d inputs, want 1", count)
	}
}

func TestHLSArgumentsRejectsBrokenBurnIns(t *testing.T) {
	t.Parallel()

	file := hlsAbsolutePath(t, "media", "film.mkv")
	transcode := hlsTranscodeVideo(0, 320, 240, false)
	copied := hlsCopyVideo(0, "h264")

	tests := []struct {
		name   string
		plan   playback.Plan
		burnIn *ffmpeg.BurnIn
	}{
		{"plan burns in, job carries none", burnInPlan(transcode), nil},
		{"copied video cannot carry an overlay", burnInPlan(copied), &ffmpeg.BurnIn{Kind: ffmpeg.BurnInImage, StreamIndex: 2}},
		{"negative stream index", burnInPlan(transcode), &ffmpeg.BurnIn{Kind: ffmpeg.BurnInImage, StreamIndex: -1}},
		{"unknown kind", burnInPlan(transcode), &ffmpeg.BurnIn{Kind: "pixels"}},
		{"relative file", burnInPlan(transcode), &ffmpeg.BurnIn{Kind: ffmpeg.BurnInText, File: "film.srt"}},
		{"negative ordinal", burnInPlan(transcode), &ffmpeg.BurnIn{Kind: ffmpeg.BurnInText, File: file, Ordinal: -1}},
		{"newline in the file name", burnInPlan(transcode), &ffmpeg.BurnIn{Kind: ffmpeg.BurnInText, File: file + "\n.srt"}},
		{"trailing blank", burnInPlan(transcode), &ffmpeg.BurnIn{Kind: ffmpeg.BurnInText, File: file + " "}},
		{
			"relative fonts directory", burnInPlan(transcode),
			&ffmpeg.BurnIn{Kind: ffmpeg.BurnInText, File: file, FontsDirectory: "fonts"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			_, err := ffmpeg.HLSArguments(ffmpeg.HLSJob{
				Plan: test.plan, InputPath: file, OutputDirectory: hlsAbsolutePath(t, "sessions", "one"), BurnIn: test.burnIn,
			})
			if !errors.Is(err, domain.ErrInvalid) {
				t.Errorf("error = %v, want domain.ErrInvalid", err)
			}
		})
	}
}

func TestHLSArgumentsWithoutBurnInAreUnchanged(t *testing.T) {
	t.Parallel()

	plan := hlsPlan(playback.ModeTranscode, hlsTranscodeVideo(0, 1280, 720, true), hlsCopyAudio(1, "aac"))
	plan.Subtitle = playback.SubtitleDecision{Mode: playback.SubtitleSidecar, TrackID: "s:2"}

	arguments := burnInArguments(t, plan, nil)

	if slices.Contains(arguments, "-filter_complex") || argumentAfter(arguments, "-vf") != "scale=1280:720" {
		t.Errorf("arguments %q: a sidecar plan must keep the plain scale filter", arguments)
	}
}
