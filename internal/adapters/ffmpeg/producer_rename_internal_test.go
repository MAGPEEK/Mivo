package ffmpeg

import (
	"context"
	"errors"
	"io/fs"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	appplayback "github.com/magpeek/mivo/internal/app/playback"
	"github.com/magpeek/mivo/internal/domain/media"
	"github.com/magpeek/mivo/internal/domain/playback"
)

const (
	// renameCorpusClip is the 30 s seek clip of the generated corpus (ADR 0021): its remux
	// rewrites the playlist once per 6 s segment within a fraction of a second.
	renameCorpusClip = "../../../testdata/media/video/h264_aac_long.mkv"
	// renameRuns repeats the remux, so the readers meet many playlist rewrites.
	renameRuns = 20
	// renamePollInterval is the ~1 ms poll interval the test runs Ready with.
	renamePollInterval = time.Millisecond
	// renameRunTimeout bounds one remux of the clip.
	renameRunTimeout = 2 * time.Minute
	// renameMinimumSegments is the segment count of the 30 s clip at 6 s per segment.
	renameMinimumSegments = 5
)

// TestProducerRenamesThePlaylistWhileReadyPolls: FFmpeg replaces index.m3u8 by renaming its
// temporary file over it, which Windows refuses while any reader has the playlist open, and
// FFmpeg ignores the failure. With Ready polling every millisecond plus a reader doing the
// same in a loop, some rewrites are lost on Windows; every run must still end cleanly with
// the complete playlist and #EXT-X-ENDLIST (completePlaylist) and no temporary file.
func TestProducerRenamesThePlaylistWhileReadyPolls(t *testing.T) {
	ffmpegPath, clip := requireRenameTools(t)
	logs := &syncBuffer{}

	producer, err := NewProducer(ProducerOptions{
		FFmpegPath: ffmpegPath, Inputs: rootOpener{}, Logger: slog.New(slog.NewJSONHandler(logs, nil)),
	})
	if err != nil {
		t.Fatalf("NewProducer: %v", err)
	}

	producer.pollInterval = renamePollInterval

	for run := range renameRuns {
		job := renameJob(t, clip)
		reads := produceWhileReading(t, producer, job)

		if _, err := os.Stat(filepath.Join(job.OutputDir, temporaryPlaylistName)); !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("run %d: the temporary playlist is left: %v", run, err)
		}

		if text := readPlaylist(t, job.OutputDir); !strings.HasSuffix(strings.TrimSpace(text), "#EXT-X-ENDLIST") ||
			strings.Count(text, ".m4s") < renameMinimumSegments {
			t.Fatalf("run %d: playlist is not complete:\n%s", run, text)
		}

		t.Logf("run %d: the reader opened the playlist %d times", run, reads)
	}

	assertNoWarnings(t, logs)
}

func requireRenameTools(t *testing.T) (string, string) {
	t.Helper()

	if testing.Short() {
		t.Skip("skipping FFmpeg integration test in -short mode")
	}

	ffmpegPath, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("skipping FFmpeg integration test: ffmpeg is not on PATH (install FFmpeg, see docs/development.md)")
	}

	clip, err := filepath.Abs(filepath.FromSlash(renameCorpusClip))
	if err != nil {
		t.Fatalf("absolute clip path: %v", err)
	}

	if _, err := os.Stat(clip); errors.Is(err, fs.ErrNotExist) {
		t.Skip("skipping FFmpeg corpus integration test: the corpus is missing " +
			"(generate it with `go run ./scripts/gen-testmedia`)")
	}

	return ffmpegPath, clip
}

func renameJob(t *testing.T, clip string) appplayback.Job {
	t.Helper()

	return appplayback.Job{
		SessionID: testSessionID,
		Plan: playback.Plan{
			Mode: playback.ModeRemux, Delivery: playback.DeliveryHLS,
			Streams: []playback.StreamDecision{
				{Kind: media.StreamKindVideo, SourceIndex: 0, Action: playback.ActionCopy, TargetCodec: "h264"},
				{Kind: media.StreamKindAudio, SourceIndex: 1, Action: playback.ActionCopy, TargetCodec: "aac"},
			},
		},
		LibraryRoot:  filepath.Dir(clip),
		RelativePath: filepath.Base(clip),
		InputPath:    clip,
		OutputDir:    t.TempDir(),
	}
}

// produceWhileReading runs job to its end while Ready and a second reader poll the playlist,
// and returns how often the second reader opened it.
func produceWhileReading(t *testing.T, producer *Producer, job appplayback.Job) int {
	t.Helper()

	ctx, cancel := context.WithTimeout(t.Context(), renameRunTimeout)
	defer cancel()

	production, err := producer.Start(ctx, job)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}

	defer production.Stop()

	var (
		group sync.WaitGroup
		reads int
	)

	group.Go(func() {
		ticker := time.NewTicker(renamePollInterval)
		defer ticker.Stop()

		for {
			playlistListsSegment(job.OutputDir)
			reads++

			select {
			case <-production.Done():
				return
			case <-ticker.C:
			}
		}
	})

	if err := production.Ready(ctx); err != nil {
		t.Errorf("Ready: %v", err)
	}

	group.Wait()

	// After a clean exit with segments, Ready reports nil again.
	if err := production.Ready(ctx); err != nil {
		t.Fatalf("Ready after the end = %v, want a clean end", err)
	}

	return reads
}

func readPlaylist(t *testing.T, directory string) string {
	t.Helper()

	data, err := os.ReadFile(filepath.Join(directory, hlsPlaylistName)) //nolint:gosec // test directory
	if err != nil {
		t.Fatalf("read playlist: %v", err)
	}

	return string(data)
}
