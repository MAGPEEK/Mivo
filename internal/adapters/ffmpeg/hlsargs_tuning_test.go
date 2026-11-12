package ffmpeg_test

import (
	"errors"
	"slices"
	"testing"

	"github.com/magpeek/mivo/internal/adapters/ffmpeg"
	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/playback"
)

// valueAfter returns the argument following flag, and whether flag is present.
func valueAfter(arguments []string, flag string) (string, bool) {
	position := slices.Index(arguments, flag)
	if position < 0 || position+1 >= len(arguments) {
		return "", false
	}

	return arguments[position+1], true
}

func TestHLSArgumentsTuneTheSoftwareEncode(t *testing.T) {
	t.Parallel()

	transcodePlan := hlsPlan(playback.ModeTranscode, hlsTranscodeVideo(0, 320, 240, false), hlsCopyAudio(1, "aac"))
	remuxPlan := hlsPlan(playback.ModeRemux, hlsCopyVideo(0, "h264"), hlsCopyAudio(1, "aac"))

	tests := []struct {
		name        string
		plan        playback.Plan
		preset      string
		threads     int
		wantPreset  string
		wantThreads string
	}{
		{name: "defaults", plan: transcodePlan, wantPreset: "veryfast"},
		{name: "preset", plan: transcodePlan, preset: "ultrafast", wantPreset: "ultrafast"},
		{name: "slowest accepted preset", plan: transcodePlan, preset: "medium", wantPreset: "medium"},
		{name: "threads", plan: transcodePlan, threads: 2, wantPreset: "veryfast", wantThreads: "2"},
		{name: "both", plan: transcodePlan, preset: "faster", threads: 1, wantPreset: "faster", wantThreads: "1"},
		{name: "remux ignores both", plan: remuxPlan, preset: "faster", threads: 4},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			arguments, err := ffmpeg.HLSArguments(ffmpeg.HLSJob{
				Plan: test.plan, InputPath: hlsAbsolutePath(t, "media", "a.mkv"),
				OutputDirectory: t.TempDir(), Preset: test.preset, Threads: test.threads,
			})
			if err != nil {
				t.Fatalf("HLSArguments: %v", err)
			}

			preset, hasPreset := valueAfter(arguments, "-preset")
			if preset != test.wantPreset || hasPreset != (test.wantPreset != "") {
				t.Errorf("-preset = %q (present %t), want %q", preset, hasPreset, test.wantPreset)
			}

			threads, hasThreads := valueAfter(arguments, "-threads")
			if threads != test.wantThreads || hasThreads != (test.wantThreads != "") {
				t.Errorf("-threads = %q (present %t), want %q", threads, hasThreads, test.wantThreads)
			}
		})
	}
}

func TestHLSArgumentsRejectsInvalidTuning(t *testing.T) {
	t.Parallel()

	plan := hlsPlan(playback.ModeTranscode, hlsTranscodeVideo(0, 320, 240, false))

	for name, job := range map[string]ffmpeg.HLSJob{
		"unknown preset":  {Preset: "placebo"},
		"option as value": {Preset: "-i"},
		"negative":        {Threads: -1},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			job.Plan, job.InputPath, job.OutputDirectory = plan, hlsAbsolutePath(t, "media", "a.mkv"), t.TempDir()

			if _, err := ffmpeg.HLSArguments(job); !errors.Is(err, domain.ErrInvalid) {
				t.Errorf("error = %v, want domain.ErrInvalid", err)
			}
		})
	}
}
