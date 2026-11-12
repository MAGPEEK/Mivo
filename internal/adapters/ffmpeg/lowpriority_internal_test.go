package ffmpeg

import (
	"context"
	"log/slog"
	"runtime"
	"testing"

	"github.com/magpeek/mivo/internal/domain/media"
)

// TestPauseIsSupportedOnLinuxOnly: only Linux has the job control signals the throttle needs.
func TestPauseIsSupportedOnLinuxOnly(t *testing.T) {
	t.Parallel()

	if got, want := PauseSupported(), runtime.GOOS == "linux"; got != want {
		t.Errorf("PauseSupported() = %t on %s, want %t", got, runtime.GOOS, want)
	}
}

type unusedProber struct{}

func (unusedProber) Probe(context.Context, string) (media.Description, error) {
	return media.Description{}, nil
}

// TestBackgroundToolsTakeTheLowPriorityOption: subtitle conversion and trickplay run beside
// playback, so the composition root can lower their priority like the producer's.
func TestBackgroundToolsTakeTheLowPriorityOption(t *testing.T) {
	t.Parallel()

	logger := slog.New(slog.DiscardHandler)

	for _, low := range []bool{false, true} {
		extractor, err := NewSubtitleExtractor(SubtitleExtractorOptions{FFmpegPath: "ffmpeg", Logger: logger, LowPriority: low})
		if err != nil {
			t.Fatalf("NewSubtitleExtractor: %v", err)
		}

		generator, err := NewTrickplayGenerator(TrickplayGeneratorOptions{
			FFmpegPath: "ffmpeg", Prober: unusedProber{}, Logger: logger, LowPriority: low,
		})
		if err != nil {
			t.Fatalf("NewTrickplayGenerator: %v", err)
		}

		if extractor.ffmpeg.lowPriority != low || generator.ffmpeg.lowPriority != low {
			t.Errorf("lowPriority = %t / %t, want %t", extractor.ffmpeg.lowPriority, generator.ffmpeg.lowPriority, low)
		}
	}
}
