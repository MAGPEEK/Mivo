package ffmpeg_test

import (
	"log/slog"
	"os/exec"
	"strings"
	"testing"

	"github.com/magpeek/mivo/internal/adapters/ffmpeg"
	"github.com/magpeek/mivo/internal/domain/playback"
)

// TestDetectCapabilitiesOnLocalFFmpeg checks the real FFmpeg on PATH: the dev machine's and
// the Docker image's builds both mux HLS and carry libx264 and aac.
func TestDetectCapabilitiesOnLocalFFmpeg(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping FFmpeg integration test in -short mode")
	}

	ffmpegPath, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("skipping FFmpeg integration test: ffmpeg is not on PATH " +
			"(install FFmpeg, see docs/development.md)")
	}

	capabilities, err := ffmpeg.DetectCapabilities(t.Context(), ffmpegPath, slog.New(slog.NewTextHandler(t.Output(), nil)))
	if err != nil {
		t.Fatalf("DetectCapabilities: %v", err)
	}

	t.Logf("detected %+v", capabilities)

	if capabilities.FFmpegVersion == "" || strings.ContainsAny(capabilities.FFmpegVersion, " \t\r\n") {
		t.Errorf("FFmpeg version = %q, want one non-empty token", capabilities.FFmpegVersion)
	}

	if !capabilities.HLSMuxing {
		t.Error("HLS muxing not reported")
	}

	if capabilities.H264Encoder != playback.EncoderLibx264 {
		t.Errorf("H.264 encoder = %q, want %q", capabilities.H264Encoder, playback.EncoderLibx264)
	}

	if capabilities.AACEncoder != playback.EncoderAAC {
		t.Errorf("AAC encoder = %q, want %q", capabilities.AACEncoder, playback.EncoderAAC)
	}
}
