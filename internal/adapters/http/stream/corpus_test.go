package stream_test

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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

	prober, err := ffmpeg.NewProber(ffmpeg.ProberOptions{FFprobePath: ffprobePath, Logger: logger})
	if err != nil {
		t.Fatalf("NewProber error = %v", err)
	}

	description, err := prober.Probe(t.Context(), filepath.Join(libraryRoot, filepath.FromSlash(corpusMKVClip)))
	if err != nil {
		t.Fatalf("Probe error = %v", err)
	}

	service := mustService(t, appplayback.Dependencies{
		Files: memoryFiles{files: map[string]appplayback.PlayableFile{corpusFileID: {
			LibraryRoot: libraryRoot,
			File:        media.File{ID: corpusFileID, RelativePath: corpusMKVClip, Description: description},
		}}},
		Paths: library, Keyframes: prober, Producer: mustProducer(t, ffmpegPath, library, logger), Workspace: workspace,
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

// mustProducer returns the adapter's FFmpeg producer, the one cmd/mivo wires.
func mustProducer(t *testing.T, ffmpegPath string, inputs ffmpeg.InputOpener, logger *slog.Logger) *ffmpeg.Producer {
	t.Helper()

	producer, err := ffmpeg.NewProducer(ffmpeg.ProducerOptions{FFmpegPath: ffmpegPath, Inputs: inputs, Logger: logger})
	if err != nil {
		t.Fatalf("ffmpeg.NewProducer error = %v", err)
	}

	return producer
}
