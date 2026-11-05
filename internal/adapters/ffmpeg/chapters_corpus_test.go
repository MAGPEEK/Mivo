package ffmpeg_test

import (
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/magpeek/mivo/internal/adapters/ffmpeg"
	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/media"
)

// chapterMetadata is an ffmetadata file with three one-second chapters, one per second of
// the three-second generated tone.
const chapterMetadata = `;FFMETADATA1
[CHAPTER]
TIMEBASE=1/1000
START=0
END=1000
title=Opening
[CHAPTER]
TIMEBASE=1/1000
START=1000
END=2000
title=Middle
[CHAPTER]
TIMEBASE=1/1000
START=2000
END=3000
title=Ending
`

const chapterToneSource = "sine=frequency=440:duration=3"

func wantChapters() []media.Chapter {
	return []media.Chapter{
		{Title: "Opening", End: time.Second},
		{Title: "Middle", Start: time.Second, End: 2 * time.Second},
		{Title: "Ending", Start: 2 * time.Second, End: 3 * time.Second},
	}
}

func TestReadChaptersWithFFmpeg(t *testing.T) {
	t.Parallel()

	tools := requireMusicTools(t)
	directory := t.TempDir()
	metadataPath := filepath.Join(directory, "chapters.txt")

	if err := os.WriteFile(metadataPath, []byte(chapterMetadata), 0o600); err != nil {
		t.Fatalf("write ffmetadata: %v", err)
	}

	tests := []struct {
		name    string
		encoder []string
		want    []media.Chapter
	}{
		{name: "book.m4b", encoder: []string{"-c:a", "aac"}, want: wantChapters()},
		{name: "book.mka", encoder: []string{"-c:a", "libopus"}, want: wantChapters()},
		{name: "book.mp3", encoder: []string{"-c:a", "libmp3lame"}, want: wantChapters()},
		{name: "plain.m4b", encoder: []string{"-c:a", "aac"}, want: []media.Chapter{}},
	}

	for _, test := range tests {
		path := filepath.Join(directory, test.name)
		arguments := []string{"-f", "lavfi", "-i", chapterToneSource}

		if len(test.want) > 0 {
			arguments = append(arguments, "-i", metadataPath, "-map_chapters", "1")
		}

		arguments = append(append(append(arguments, "-map", "0:a"), test.encoder...), path)
		tools.runFFmpeg(t, arguments...)

		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			chapters, err := tools.prober.ReadChapters(t.Context(), path)
			if err != nil {
				t.Fatalf("ReadChapters: %v", err)
			}

			if chapters == nil || !slices.Equal(chapters, test.want) {
				t.Errorf("chapters = %+v, want %+v", chapters, test.want)
			}
		})
	}
}

func TestReadChaptersFailures(t *testing.T) {
	t.Parallel()

	tools := requireMusicTools(t)
	directory := t.TempDir()

	broken := filepath.Join(directory, "broken.m4b")
	if err := os.WriteFile(broken, []byte("not a media file"), 0o600); err != nil {
		t.Fatalf("write broken file: %v", err)
	}

	if _, err := tools.prober.ReadChapters(t.Context(), broken); !errors.Is(err, domain.ErrUnsupported) {
		t.Errorf("broken file error = %v, want domain.ErrUnsupported", err)
	}
}

func TestReadChaptersRejectsRelativePaths(t *testing.T) {
	t.Parallel()

	// No tool starts for a refused path, so the executable need not exist.
	prober, err := ffmpeg.NewProber(ffmpeg.ProberOptions{FFprobePath: "ffprobe", Logger: slog.New(slog.DiscardHandler)})
	if err != nil {
		t.Fatalf("NewProber: %v", err)
	}

	if _, err := prober.ReadChapters(t.Context(), "book.m4b"); !errors.Is(err, domain.ErrInvalid) {
		t.Errorf("ReadChapters error = %v, want domain.ErrInvalid", err)
	}
}
