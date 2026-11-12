package ffmpeg_test

import (
	"io"
	"log/slog"
	"testing"

	"github.com/magpeek/mivo/internal/adapters/ffmpeg"
	"github.com/magpeek/mivo/internal/domain/playback"
	"github.com/magpeek/mivo/internal/domain/playback/playbacktest"
)

// TestHLSArgumentsHardwareEncodeOnRealHardware transcodes a capped, downscaled clip with every
// hardware encoder this machine passes the detection test for, and checks the HLS output keeps
// its keyframe-aligned segments. It skips on a machine without a usable encoder (CI, Docker).
func TestHLSArgumentsHardwareEncodeOnRealHardware(t *testing.T) {
	tools, _ := requireHLSTools(t)

	encoders, err := ffmpeg.DetectHardware(t.Context(), ffmpeg.HardwareDetectionOptions{
		Executable: tools.ffmpegPath, Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if err != nil {
		t.Fatalf("DetectHardware: %v", err)
	}

	source := generateCapClip(t, tools.ffmpegPath)
	ran := false

	for _, encoder := range encoders {
		if !encoder.Available() {
			continue
		}

		ran = true

		t.Logf("%s: hardware decode verified = %v", encoder.Name, encoder.Decode)

		t.Run(encoder.Name, func(t *testing.T) {
			description, err := tools.prober.Probe(t.Context(), source)
			if err != nil {
				t.Fatalf("Probe: %v", err)
			}

			capabilities := playbacktest.FullCapabilities()
			capabilities.HardwareEncoders = encoders

			plan, err := playback.Decide(playback.Input{
				Description: description, Profile: playbacktest.ChromiumDesktop(), Capabilities: capabilities,
				Request: playback.Request{MaxBitrate: capBitsPerSecond},
				Policy: playback.Policy{
					MaxTranscodeHeight: hlsMaxHeight, HardwareAcceleration: playback.HardwareAcceleration(encoder.API),
				},
			})
			if err != nil {
				t.Fatalf("Decide: %v", err)
			}

			if video, _ := plan.Video(); !video.Hardware || video.Encoder != encoder.Name {
				t.Fatalf("video = %+v, want the %s encoder", video, encoder.Name)
			}

			directory := t.TempDir()
			hlsRunFFmpeg(t, tools.ffmpegPath, ffmpeg.HLSJob{
				Plan: plan, InputPath: source, OutputDirectory: directory, HardwareEncoders: encoders,
			})

			segments := hlsAssertPlaylist(t, directory)
			tools.assertSegmentStarts(t, directory, segments, plan)
		})
	}

	if !ran {
		t.Skip("no hardware H.264 encoder passed the detection test on this machine")
	}
}
