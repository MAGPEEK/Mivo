package ffmpeg_test

import (
	"context"
	"errors"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/magpeek/mivo/internal/adapters/ffmpeg"
	"github.com/magpeek/mivo/internal/adapters/fs/libraryfs"
	appplayback "github.com/magpeek/mivo/internal/app/playback"
	"github.com/magpeek/mivo/internal/domain/playback"
	"github.com/magpeek/mivo/internal/domain/playback/playbacktest"
)

const (
	// producerSessionID names the sessions of these tests in logs.
	producerSessionID = "00000000-0000-4000-8000-000000000124"
	// corruptInputSize is the length of the garbage file FFmpeg must reject.
	corruptInputSize = 64 << 10
)

// corpusProducer returns a producer for the local ffmpeg that opens inputs through
// libraryfs, as the server wires it.
func corpusProducer(t *testing.T, tools hlsTools) *ffmpeg.Producer {
	t.Helper()

	return corpusProducerWithLogger(t, tools, slog.New(slog.NewTextHandler(t.Output(), nil)))
}

func corpusProducerWithLogger(t *testing.T, tools hlsTools, logger *slog.Logger) *ffmpeg.Producer {
	t.Helper()

	files, err := libraryfs.New(logger)
	if err != nil {
		t.Fatalf("libraryfs.New: %v", err)
	}

	producer, err := ffmpeg.NewProducer(ffmpeg.ProducerOptions{FFmpegPath: tools.ffmpegPath, Inputs: files, Logger: logger})
	if err != nil {
		t.Fatalf("NewProducer: %v", err)
	}

	return producer
}

// corpusJob decides the clip like the playback service and returns its job, with the
// corpus as the library root.
func corpusJob(t *testing.T, tools hlsTools, manifest corpusManifest, run hlsCorpusRun) appplayback.Job {
	t.Helper()

	clip := corpusCaseByName(t, manifest, run.name)
	source := corpusPath(t, clip.File)
	_, plan := hlsDecide(t, tools.prober, source, run)

	return appplayback.Job{
		SessionID:     producerSessionID,
		Plan:          plan,
		LibraryRoot:   corpusPath(t, "."),
		RelativePath:  clip.File,
		InputPath:     source,
		StartPosition: tools.startPosition(t, source, plan, run.start),
		OutputDir:     t.TempDir(),
	}
}

func startCorpusProduction(t *testing.T, producer *ffmpeg.Producer, job appplayback.Job) appplayback.Production {
	t.Helper()

	production, err := producer.Start(t.Context(), job)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}

	t.Cleanup(production.Stop)

	return production
}

func readyOrFail(t *testing.T, production appplayback.Production) {
	t.Helper()

	ctx, cancel := context.WithTimeout(t.Context(), hlsRunTimeout)
	defer cancel()

	if err := production.Ready(ctx); err != nil {
		t.Fatalf("Ready: %v", err)
	}
}

func TestProducerRemuxesAClipToTheEnd(t *testing.T) {
	tools, manifest := requireHLSTools(t)
	t.Parallel()

	job := corpusJob(t, tools, manifest, hlsCorpusRun{
		name: "video-mkv-h264-aac", profile: playbacktest.ChromiumDesktop(),
		wantMode: playback.ModeRemux, wantVideo: playback.ActionCopy, wantAudio: playback.ActionCopy,
	})
	production := startCorpusProduction(t, corpusProducer(t, tools), job)

	select {
	case <-production.Done():
	case <-time.After(hlsRunTimeout):
		t.Fatalf("the remux did not finish within %s", hlsRunTimeout)
	}

	readyOrFail(t, production)

	// The finished playlist ends with #EXT-X-ENDLIST and the directory holds exactly the
	// playlist, init.mp4 and the listed segments.
	if segments := hlsAssertPlaylist(t, job.OutputDir); len(segments) == 0 {
		t.Error("the playlist lists no segment")
	}
}

func TestProducerStopsARunningTranscode(t *testing.T) {
	tools, manifest := requireHLSTools(t)
	t.Parallel()

	job := corpusJob(t, tools, manifest, hlsCorpusRun{
		name: "video-ts-mpeg2-mp2-long", profile: playbacktest.ChromiumDesktop(),
		wantMode: playback.ModeTranscode, wantVideo: playback.ActionTranscode, wantAudio: playback.ActionTranscode,
	})
	production := startCorpusProduction(t, corpusProducer(t, tools), job)
	readyOrFail(t, production)

	select {
	case <-production.Done():
		t.Log("the transcode finished before Stop; the kill path was not exercised")
	default:
	}

	production.Stop()

	// Stop returns only after the process exited, so Done is closed and no FFmpeg is left.
	select {
	case <-production.Done():
	default:
		t.Fatal("Done is open after Stop returned")
	}

	if err := production.Ready(t.Context()); !errors.Is(err, appplayback.ErrProductionStopped) {
		t.Errorf("Ready after Stop = %v, want ErrProductionStopped", err)
	}

	// Windows refuses to delete files a process still holds; the removal must work at once.
	if err := os.RemoveAll(job.OutputDir); err != nil {
		t.Fatalf("remove the session directory right after Stop: %v", err)
	}

	if _, err := os.Stat(job.OutputDir); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("session directory still exists: %v", err)
	}
}

func TestProducerReportsACorruptInput(t *testing.T) {
	tools, manifest := requireHLSTools(t)
	t.Parallel()

	job := corpusJob(t, tools, manifest, hlsCorpusRun{
		name: "video-mkv-h264-aac", profile: playbacktest.ChromiumDesktop(),
		wantMode: playback.ModeRemux, wantVideo: playback.ActionCopy, wantAudio: playback.ActionCopy,
	})

	// Same plan, but the library file is garbage with a video extension.
	library := t.TempDir()
	job.LibraryRoot, job.RelativePath = library, "corrupt.mkv"
	job.InputPath = filepath.Join(library, job.RelativePath)

	if err := os.WriteFile(job.InputPath, []byte(strings.Repeat("\x5a\xa5", corruptInputSize/2)), 0o600); err != nil {
		t.Fatalf("write corrupt input: %v", err)
	}

	logs := &logCapture{}
	production := startCorpusProduction(t, corpusProducerWithLogger(t, tools, logs.logger()), job)

	ctx, cancel := context.WithTimeout(t.Context(), hlsRunTimeout)
	defer cancel()

	err := production.Ready(ctx)
	if err == nil || !strings.Contains(err.Error(), "ffmpeg exited with code") {
		t.Fatalf("Ready = %v, want an error with FFmpeg's exit code", err)
	}

	if errors.Is(err, appplayback.ErrProductionStopped) {
		t.Errorf("a corrupt input is reported as a stop: %v", err)
	}

	// FFmpeg's error output is in the producer's warning only (ADR 0024 Notes M2-39).
	if tails := logs.exitTails(t); len(tails) != 1 || tails[0] == "" {
		t.Errorf("logged stderr tails = %q, want FFmpeg's error output once", tails)
	}
}
