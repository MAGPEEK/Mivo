package ffmpeg_test

import (
	"errors"
	"testing"

	"github.com/magpeek/mivo/internal/adapters/ffmpeg"
	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/playback"
)

func TestHLSArgumentsCapTheVideoBitRate(t *testing.T) {
	t.Parallel()

	surround := hlsTranscodeAudio(1)
	surround.TargetChannels = 6

	tests := []struct {
		name        string
		cap         int64
		audio       []playback.StreamDecision
		wantMaxRate string
		wantBufSize string
	}{
		{name: "no cap keeps constant quality", cap: 0, audio: []playback.StreamDecision{hlsTranscodeAudio(1)}},
		{name: "4 Mbit/s stereo", cap: 4_000_000, audio: []playback.StreamDecision{hlsTranscodeAudio(1)}, wantMaxRate: "3808k", wantBufSize: "7616k"},
		{name: "8 Mbit/s surround", cap: 8_000_000, audio: []playback.StreamDecision{surround}, wantMaxRate: "7616k", wantBufSize: "15232k"},
		{name: "copied audio is reserved too", cap: 2_000_000, audio: []playback.StreamDecision{hlsCopyAudio(1, "aac")}, wantMaxRate: "1808k", wantBufSize: "3616k"},
		{name: "no audio", cap: 1_000_000, wantMaxRate: "1000k", wantBufSize: "2000k"},
		{name: "budget never below 200 kbit/s", cap: 300_000, audio: []playback.StreamDecision{hlsTranscodeAudio(1)}, wantMaxRate: "200k", wantBufSize: "400k"},
		{name: "tiny cap is clamped", cap: 1, wantMaxRate: "200k", wantBufSize: "400k"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			video := hlsTranscodeVideo(0, 1280, 720, true)
			video.MaxBitrate = test.cap

			arguments, err := ffmpeg.HLSArguments(ffmpeg.HLSJob{
				Plan:      hlsPlan(playback.ModeTranscode, append([]playback.StreamDecision{video}, test.audio...)...),
				InputPath: hlsAbsolutePath(t, "media", "a.mkv"), OutputDirectory: t.TempDir(),
			})
			if err != nil {
				t.Fatalf("HLSArguments: %v", err)
			}

			assertRateLimits(t, arguments, test.wantMaxRate, test.wantBufSize)

			if crf, _ := valueAfter(arguments, "-crf"); crf != "23" {
				t.Errorf("-crf = %q, want the constant quality 23 kept", crf)
			}

			if scale, _ := valueAfter(arguments, "-vf"); scale != "scale=1280:720" {
				t.Errorf("-vf = %q, want scale=1280:720", scale)
			}
		})
	}
}

// assertRateLimits checks -maxrate and -bufsize; empty wanted values mean both are absent.
func assertRateLimits(t *testing.T, arguments []string, wantMaxRate, wantBufSize string) {
	t.Helper()

	maxRate, hasMaxRate := valueAfter(arguments, "-maxrate")
	bufSize, hasBufSize := valueAfter(arguments, "-bufsize")

	if maxRate != wantMaxRate || bufSize != wantBufSize || hasMaxRate != (wantMaxRate != "") || hasBufSize != (wantBufSize != "") {
		t.Errorf("-maxrate %q -bufsize %q, want %q %q", maxRate, bufSize, wantMaxRate, wantBufSize)
	}
}

func TestHLSArgumentsRejectsAnInvalidBitRateCap(t *testing.T) {
	t.Parallel()

	for name, bitRate := range map[string]int64{"negative": -1, "above one Gbit/s": 1_000_000_001} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			video := hlsTranscodeVideo(0, 320, 240, false)
			video.MaxBitrate = bitRate

			_, err := ffmpeg.HLSArguments(ffmpeg.HLSJob{
				Plan:      hlsPlan(playback.ModeTranscode, video),
				InputPath: hlsAbsolutePath(t, "media", "a.mkv"), OutputDirectory: t.TempDir(),
			})
			if !errors.Is(err, domain.ErrInvalid) {
				t.Errorf("error = %v, want domain.ErrInvalid", err)
			}
		})
	}
}

func TestHLSArgumentsIgnoreTheCapOfCopiedVideo(t *testing.T) {
	t.Parallel()

	video := hlsCopyVideo(0, "h264")
	video.MaxBitrate = 4_000_000

	arguments, err := ffmpeg.HLSArguments(ffmpeg.HLSJob{
		Plan:      hlsPlan(playback.ModeRemux, video),
		InputPath: hlsAbsolutePath(t, "media", "a.mkv"), OutputDirectory: t.TempDir(),
	})
	if err != nil {
		t.Fatalf("HLSArguments: %v", err)
	}

	if _, found := valueAfter(arguments, "-maxrate"); found {
		t.Error("a copied video must not get -maxrate")
	}
}
