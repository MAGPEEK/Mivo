package stream_test

import (
	"bufio"
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/magpeek/mivo/internal/adapters/ffmpeg"
	"github.com/magpeek/mivo/internal/adapters/fs/transcodedir"
	"github.com/magpeek/mivo/internal/adapters/http/stream"
	"github.com/magpeek/mivo/internal/adapters/uuidgen"
	appplayback "github.com/magpeek/mivo/internal/app/playback"
	"github.com/magpeek/mivo/internal/domain/media"
	"github.com/magpeek/mivo/internal/domain/playback"
	"github.com/magpeek/mivo/internal/domain/playback/playbacktest"
)

const (
	// corpusDirectory is the generated test corpus seen from this package (ADR 0021).
	corpusDirectory = "../../../../testdata/media"
	// corpusMKVClip is the MKV H.264/AAC clip, which the Chromium profile remuxes.
	corpusMKVClip = "video/h264_aac.mkv"
	corpusFileID  = "c0000000-0000-4000-8000-000000000001"
	// corpusPollInterval spaces the playlist checks of the test producer.
	corpusPollInterval = 50 * time.Millisecond
	// corpusStopTimeout bounds waiting for a killed FFmpeg.
	corpusStopTimeout = 30 * time.Second
	// corpusResponseLimit bounds what the test reads of one response.
	corpusResponseLimit = 64 << 20
)

// TestCorpusRemuxIsServedOverHTTP plays the MKV corpus clip through the real playback
// service, workspace and library file system with FFmpeg producing the HLS output, and loads
// it like a player over a real HTTP connection: playlist, init segment, first segment.
func TestCorpusRemuxIsServedOverHTTP(t *testing.T) {
	ffmpegPath, ffprobePath, libraryRoot := requireCorpusTools(t)

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	library := mustLibraryFS(t, logger)
	workspace, err := transcodedir.New(t.TempDir())
	if err != nil {
		t.Fatalf("transcodedir.New error = %v", err)
	}

	prober := ffmpeg.NewProber(ffprobePath)

	description, err := prober.Probe(t.Context(), filepath.Join(libraryRoot, filepath.FromSlash(corpusMKVClip)))
	if err != nil {
		t.Fatalf("Probe error = %v", err)
	}

	service := mustService(t, appplayback.Dependencies{
		Files: memoryFiles{files: map[string]appplayback.PlayableFile{corpusFileID: {
			LibraryRoot: libraryRoot,
			File:        media.File{ID: corpusFileID, RelativePath: corpusMKVClip, Description: description},
		}}},
		Paths: library, Keyframes: prober, Producer: ffmpegProducer{ffmpegPath: ffmpegPath}, Workspace: workspace,
		URLs: stream.NewURLBuilder(), IDs: uuidgen.New(), Clock: time.Now, Logger: logger,
	})
	handlers := mustHandlers(t, stream.Options{Sessions: service, Library: library, Output: workspace, Logger: logger})
	server := httptest.NewServer(mustAPI(t, logger, handlers.Mounts()))
	t.Cleanup(server.Close)

	session := startCorpusSession(t, service)

	playlist := fetch(t, server.URL+session.StreamURL, "application/vnd.apple.mpegurl")
	if !strings.Contains(playlist, `#EXT-X-MAP:URI="init.mp4"`) || !strings.Contains(playlist, "seg-00000.m4s") {
		t.Fatalf("playlist lacks the init or first segment reference:\n%s", playlist)
	}

	base := strings.TrimSuffix(server.URL+session.StreamURL, "index.m3u8")
	fetch(t, base+"init.mp4", "video/mp4")
	fetch(t, base+"seg-00000.m4s", "video/iso.segment")
}

func requireCorpusTools(t *testing.T) (ffmpegPath, ffprobePath, libraryRoot string) {
	t.Helper()

	if testing.Short() {
		t.Skip("skipping FFmpeg integration test in -short mode")
	}

	ffmpegPath, ffmpegErr := exec.LookPath("ffmpeg")
	ffprobePath, ffprobeErr := exec.LookPath("ffprobe")

	if ffmpegErr != nil || ffprobeErr != nil {
		t.Skip("skipping FFmpeg integration test: ffmpeg or ffprobe is not on PATH (see docs/development.md)")
	}

	libraryRoot, err := filepath.Abs(corpusDirectory)
	if err != nil {
		t.Fatalf("resolve corpus directory: %v", err)
	}

	if _, err := os.Stat(filepath.Join(libraryRoot, filepath.FromSlash(corpusMKVClip))); err != nil {
		t.Skipf("skipping FFmpeg corpus test: %s is missing (run `go run ./scripts/gen-testmedia`)", corpusMKVClip)
	}

	return ffmpegPath, ffprobePath, libraryRoot
}

func startCorpusSession(t *testing.T, service *appplayback.Service) appplayback.Session {
	t.Helper()

	session, err := service.Start(t.Context(), appplayback.StartRequest{
		FileID: corpusFileID, Profile: playbacktest.ChromiumDesktop(),
	})
	if err != nil {
		t.Fatalf("Start error = %v", err)
	}

	// Stop kills FFmpeg and removes the session directory before t.TempDir is cleaned up.
	t.Cleanup(func() {
		if err := service.Stop(context.WithoutCancel(t.Context()), session.ID); err != nil {
			t.Errorf("Stop error = %v", err)
		}
	})

	if session.Plan.Mode != playback.ModeRemux {
		t.Fatalf("plan mode = %s, want remux", session.Plan.Mode)
	}

	return session
}

// fetch GETs url like a player and returns the body after checking 200 and the type.
func fetch(t *testing.T, url, wantType string) string {
	t.Helper()

	request, err := http.NewRequestWithContext(t.Context(), http.MethodGet, url, nil)
	if err != nil {
		t.Fatalf("new request %s: %v", url, err)
	}

	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}

	defer func() { _ = response.Body.Close() }()

	body, err := io.ReadAll(io.LimitReader(response.Body, corpusResponseLimit))
	if err != nil {
		t.Fatalf("read %s: %v", url, err)
	}

	if response.StatusCode != http.StatusOK || response.Header.Get("Content-Type") != wantType || len(body) == 0 {
		t.Fatalf("GET %s = %d %q with %d bytes, want 200 %q with a body",
			url, response.StatusCode, response.Header.Get("Content-Type"), len(body), wantType)
	}

	return string(body)
}

// ffmpegProducer runs FFmpeg with the arguments of ffmpeg.HLSArguments. It is a minimal
// stand-in for the adapter's producer (M2-24), enough to exercise the readiness wait
// against real output.
type ffmpegProducer struct {
	ffmpegPath string
}

func (producer ffmpegProducer) Start(ctx context.Context, job appplayback.Job) (appplayback.Production, error) {
	arguments, err := ffmpeg.HLSArguments(ffmpeg.HLSJob{
		Plan: job.Plan, InputPath: job.InputPath, StartPosition: job.StartPosition, OutputDirectory: job.OutputDir,
	})
	if err != nil {
		return nil, err
	}

	processContext, cancel := context.WithCancel(ctx)
	// The executable comes from PATH and the arguments are a slice; no shell is involved.
	command := exec.CommandContext(processContext, producer.ffmpegPath, arguments...) //nolint:gosec // see above

	if err := command.Start(); err != nil {
		cancel()

		return nil, err
	}

	production := &ffmpegProduction{directory: job.OutputDir, cancel: cancel, done: make(chan struct{})}

	go func() {
		production.exitErr = command.Wait()
		close(production.done)
	}()

	return production, nil
}

type ffmpegProduction struct {
	directory string
	cancel    context.CancelFunc
	done      chan struct{}
	// exitErr is written before done is closed and read only after.
	exitErr  error
	stopOnce sync.Once
	stopped  atomic.Bool
}

func (production *ffmpegProduction) Ready(ctx context.Context) error {
	ticker := time.NewTicker(corpusPollInterval)
	defer ticker.Stop()

	for {
		if production.hasSegment() {
			return nil
		}

		select {
		case <-production.done:
			return production.result()
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func (production *ffmpegProduction) result() error {
	switch {
	case production.stopped.Load():
		return appplayback.ErrProductionStopped
	case production.exitErr != nil:
		return production.exitErr
	case !production.hasSegment():
		return errors.New("ffmpeg exited without a segment")
	default:
		return nil
	}
}

func (production *ffmpegProduction) hasSegment() bool {
	// os.OpenInRoot shares the file for deletion on Windows, so FFmpeg can still rename the
	// next playlist over it.
	file, err := os.OpenInRoot(production.directory, "index.m3u8")
	if err != nil {
		return false
	}

	defer func() { _ = file.Close() }()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		if strings.HasPrefix(scanner.Text(), "seg-") {
			return true
		}
	}

	return false
}

func (production *ffmpegProduction) Stop() {
	production.stopOnce.Do(func() {
		select {
		case <-production.done:
		default:
			production.stopped.Store(true)
		}

		production.cancel()

		select {
		case <-production.done:
		case <-time.After(corpusStopTimeout):
		}
	})
}

func (production *ffmpegProduction) Done() <-chan struct{} {
	return production.done
}
