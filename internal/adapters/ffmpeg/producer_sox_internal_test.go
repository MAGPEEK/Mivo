package ffmpeg

import (
	"context"
	"log/slog"
	"path/filepath"
	"slices"
	"testing"

	appplayback "github.com/magpeek/mivo/internal/app/playback"
	"github.com/magpeek/mivo/internal/domain/media"
	"github.com/magpeek/mivo/internal/domain/playback"
)

type switchingEncoders struct{ encoders []playback.HardwareEncoder }

func (source *switchingEncoders) HardwareEncoders() []playback.HardwareEncoder {
	return source.encoders
}

// TestProducerAsksTheHardwareSourceForEveryJob: detection ends after the producer was built,
// so a fixed list would never see the encoders it finds.
func TestProducerAsksTheHardwareSourceForEveryJob(t *testing.T) {
	t.Parallel()

	source := &switchingEncoders{}

	producer, err := NewProducer(ProducerOptions{
		FFmpegPath: "ffmpeg", Inputs: rootOpener{}, Logger: slog.New(slog.DiscardHandler),
		HardwareEncoders: []playback.HardwareEncoder{{Name: "fixed"}}, HardwareSource: source,
	})
	if err != nil {
		t.Fatalf("NewProducer: %v", err)
	}

	if got := producer.hardwareEncoders(); len(got) != 0 {
		t.Errorf("encoders before the detection = %v, want none", got)
	}

	source.encoders = []playback.HardwareEncoder{{Name: "h264_vaapi"}}

	if got := producer.hardwareEncoders(); len(got) != 1 || got[0].Name != "h264_vaapi" {
		t.Errorf("encoders after the detection = %v, want h264_vaapi", got)
	}
}

// TestProducerPassesTheSoxResamplerToTheJob: the composition root detects libsoxr once and the
// producer must hand it to every job, or builds with libsoxr would never use it.
func TestProducerPassesTheSoxResamplerToTheJob(t *testing.T) {
	t.Parallel()

	for _, soxResampler := range []bool{false, true} {
		producer, err := NewProducer(ProducerOptions{
			FFmpegPath: "ffmpeg", Inputs: rootOpener{}, Logger: slog.New(slog.DiscardHandler),
			SoxResampler: soxResampler,
		})
		if err != nil {
			t.Fatalf("NewProducer: %v", err)
		}

		producer.inheritInput = false
		directory := t.TempDir()

		prepared, err := producer.prepareRun(context.Background(), appplayback.Job{
			SessionID: testSessionID, InputPath: filepath.Join(directory, "track.flac"),
			OutputDir: filepath.Join(directory, "session"),
			Plan: playback.Plan{
				Mode: playback.ModeTranscode, Delivery: playback.DeliveryHLS,
				SourceContainer: media.ContainerMKV, TargetContainer: media.ContainerMP4,
				Streams: []playback.StreamDecision{{
					Kind: media.StreamKindAudio, Action: playback.ActionTranscode, SourceCodec: "flac",
					TargetCodec: "aac", TargetChannels: 2, TargetSampleRate: 48000, Encoder: playback.EncoderAAC,
				}},
			},
		})
		if err != nil {
			t.Fatalf("prepareRun: %v", err)
		}

		if got := slices.Contains(prepared.arguments, "aresample=48000:resampler=soxr"); got != soxResampler {
			t.Errorf("soxr in arguments = %t, want %t (%v)", got, soxResampler, prepared.arguments)
		}
	}
}
