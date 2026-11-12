package ffmpeg_test

import (
	"bytes"
	"context"
	"errors"
	"image/jpeg"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/magpeek/mivo/internal/adapters/ffmpeg"
	"github.com/magpeek/mivo/internal/domain"
)

func newCorpusTrickplayGenerator(t *testing.T) *ffmpeg.TrickplayGenerator {
	t.Helper()

	ffprobePath, _ := requireCorpus(t)
	logger := slog.New(slog.DiscardHandler)

	prober, err := ffmpeg.NewProber(ffmpeg.ProberOptions{FFprobePath: ffprobePath, Logger: logger})
	if err != nil {
		t.Fatalf("NewProber: %v", err)
	}

	generator, err := ffmpeg.NewTrickplayGenerator(ffmpeg.TrickplayGeneratorOptions{
		FFmpegPath: requireFFmpeg(t), Prober: prober, Logger: logger,
	})
	if err != nil {
		t.Fatalf("NewTrickplayGenerator: %v", err)
	}

	return generator
}

// The seek clips are 30 s of 320×240 video with a keyframe every 2 s: 3 thumbnails at the
// 10 s interval, so one sheet of 1600×1200 with the remaining tiles empty.
func TestGenerateTrickplayOnSeekClips(t *testing.T) {
	generator := newCorpusTrickplayGenerator(t)
	_, manifest := requireCorpus(t)

	for _, caseName := range []string{"video-mkv-h264-aac-long", "video-ts-mpeg2-mp2-long"} {
		t.Run(caseName, func(t *testing.T) {
			source := corpusPath(t, corpusCaseByName(t, manifest, caseName).File)
			destination := filepath.Join(t.TempDir(), "trickplay")

			got, err := generator.GenerateTrickplay(t.Context(), source, destination, ffmpeg.TrickplayOptions{})
			if err != nil {
				t.Fatalf("GenerateTrickplay: %v", err)
			}

			want := ffmpeg.Manifest{
				TileWidth: 160, TileHeight: 120, Columns: 10, Rows: 10, Interval: 10 * time.Second, SheetCount: 1,
			}
			if got != want {
				t.Errorf("manifest = %+v, want %+v", got, want)
			}

			assertSheet(t, filepath.Join(destination, "sheet-001.jpg"), 1600, 1200)

			if names := directoryNames(t, destination); len(names) != 1 {
				t.Errorf("destination holds %v, want only sheet-001.jpg", names)
			}
		})
	}
}

// A clip shorter than one tile interval still yields one sheet.
func TestGenerateTrickplayOnAClipShorterThanOneInterval(t *testing.T) {
	generator := newCorpusTrickplayGenerator(t)
	_, manifest := requireCorpus(t)
	source := corpusPath(t, corpusCaseByName(t, manifest, "video-mkv-h264-aac").File)

	got, err := generator.GenerateTrickplay(t.Context(), source, t.TempDir(), ffmpeg.TrickplayOptions{})
	if err != nil {
		t.Fatalf("GenerateTrickplay: %v", err)
	}

	if got.SheetCount != 1 {
		t.Errorf("SheetCount = %d, want 1", got.SheetCount)
	}
}

func TestGenerateTrickplayRefusesAudioOnlyInput(t *testing.T) {
	generator := newCorpusTrickplayGenerator(t)
	_, manifest := requireCorpus(t)
	source := corpusPath(t, corpusCaseByName(t, manifest, "audio-mp3").File)
	destination := t.TempDir()

	_, err := generator.GenerateTrickplay(t.Context(), source, destination, ffmpeg.TrickplayOptions{})
	if !errors.Is(err, domain.ErrInvalid) {
		t.Fatalf("error = %v, want ErrInvalid", err)
	}

	if names := directoryNames(t, destination); len(names) != 0 {
		t.Errorf("destination holds %v, want nothing", names)
	}
}

// A context that ended before the run must stop it and leave the destination clean.
func TestGenerateTrickplayStopsWithACancelledContext(t *testing.T) {
	generator := newCorpusTrickplayGenerator(t)
	_, manifest := requireCorpus(t)
	source := corpusPath(t, corpusCaseByName(t, manifest, "video-mkv-h264-aac-long").File)
	destination := t.TempDir()

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	_, err := generator.GenerateTrickplay(ctx, source, destination, ffmpeg.TrickplayOptions{})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}

	if names := directoryNames(t, destination); len(names) != 0 {
		t.Errorf("destination holds %v, want nothing", names)
	}
}

func assertSheet(t *testing.T, path string, width, height int) {
	t.Helper()

	data, err := os.ReadFile(path) //nolint:gosec // the path is built from the test's temp directory
	if err != nil {
		t.Fatalf("read sheet: %v", err)
	}

	config, err := jpeg.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("decode sheet: %v", err)
	}

	if config.Width != width || config.Height != height {
		t.Errorf("sheet is %d×%d, want %d×%d", config.Width, config.Height, width, height)
	}
}

func directoryNames(t *testing.T, directory string) []string {
	t.Helper()

	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatalf("read %s: %v", directory, err)
	}

	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Name())
	}

	return names
}
