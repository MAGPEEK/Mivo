package corpustest_test

import (
	"context"
	"errors"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/magpeek/mivo/internal/adapters/ffmpeg"
	"github.com/magpeek/mivo/internal/adapters/fs/libraryfs"
	"github.com/magpeek/mivo/internal/adapters/fs/transcodedir"
	"github.com/magpeek/mivo/internal/adapters/http/stream"
	"github.com/magpeek/mivo/internal/adapters/sqlite"
	"github.com/magpeek/mivo/internal/adapters/uuidgen"
	appplayback "github.com/magpeek/mivo/internal/app/playback"
	"github.com/magpeek/mivo/internal/app/scan"
	"github.com/magpeek/mivo/internal/domain/library"
	"github.com/magpeek/mivo/internal/domain/playback"
)

const (
	// databaseFileName and transcodeDirName mirror the server's data layout inside the
	// test's temporary directory.
	databaseFileName = "server.db"
	transcodeDirName = "transcode"
	libraryName      = "Corpus"
	// maxTranscodeHeight is the transcode height cap of OD-5 B, as cmd configures it.
	maxTranscodeHeight = 1080
	// scanTimeout bounds the scan of the whole corpus (a few dozen short files).
	scanTimeout = 2 * time.Minute
	// pollInterval spaces the status checks of scans and sessions.
	pollInterval = 50 * time.Millisecond
	// shutdownTimeout bounds closing a service at the end of the test.
	shutdownTimeout = time.Minute
)

// corpusGraph is the part of the server this test needs, wired like cmd/mivo wires it, with
// the corpus as the only library and a temporary data directory.
type corpusGraph struct {
	database *sqlite.DB
	// corpusRoot is the absolute library root, the corpus directory.
	corpusRoot    string
	files         *libraryfs.FileSystem
	prober        *ffmpeg.Prober
	ffmpegPath    string
	transcodeRoot string
	libraryID     string
	logger        *slog.Logger
}

// newCorpusGraph opens and migrates a temporary database and syncs the corpus library into it.
func newCorpusGraph(t *testing.T, environment corpusEnvironment) *corpusGraph {
	t.Helper()

	// Warnings only: the producer logs a failed FFmpeg at warn, which explains a failure.
	logger := slog.New(slog.NewTextHandler(t.Output(), &slog.HandlerOptions{Level: slog.LevelWarn}))
	dataDirectory := t.TempDir()

	database, err := sqlite.Open(t.Context(), sqlite.Options{Path: filepath.Join(dataDirectory, databaseFileName), Logger: logger})
	if err != nil {
		t.Fatalf("open the database: %v", err)
	}

	t.Cleanup(func() {
		if err := database.Close(); err != nil {
			t.Errorf("close the database: %v", err)
		}
	})

	if err := database.Migrate(t.Context()); err != nil {
		t.Fatalf("migrate the database: %v", err)
	}

	files, err := libraryfs.New(logger)
	if err != nil {
		t.Fatalf("libraryfs.New: %v", err)
	}

	prober, err := ffmpeg.NewProber(ffmpeg.ProberOptions{FFprobePath: environment.ffprobePath, Logger: logger})
	if err != nil {
		t.Fatalf("ffmpeg.NewProber: %v", err)
	}

	graph := &corpusGraph{
		database: database, corpusRoot: environment.root, files: files, prober: prober,
		ffmpegPath: environment.ffmpegPath, transcodeRoot: filepath.Join(dataDirectory, transcodeDirName),
		logger: logger,
	}
	graph.libraryID = graph.syncLibrary(t, environment.root)

	return graph
}

func (graph *corpusGraph) syncLibrary(t *testing.T, root string) string {
	t.Helper()

	repository := sqlite.NewLibraryRepository(graph.database, time.Now)
	definition := library.Definition{Name: libraryName, Root: root, Kind: library.KindMovies}

	result, err := repository.SyncLibraries(t.Context(), []library.Definition{definition}, uuidgen.New().NewID)
	if err != nil {
		t.Fatalf("sync the corpus library: %v", err)
	}

	if len(result.Libraries) != 1 {
		t.Fatalf("sync returned %d libraries, want 1", len(result.Libraries))
	}

	return result.Libraries[0].ID
}

// requireTranscoding skips the test when the local FFmpeg cannot do what FullCapabilities
// claims: the transcode cases would fail for the machine, not for the server.
func requireTranscoding(t *testing.T, ffmpegPath string) {
	t.Helper()

	detected, err := ffmpeg.DetectCapabilities(t.Context(), ffmpegPath, slog.New(slog.NewTextHandler(t.Output(), nil)))
	if err != nil {
		t.Fatalf("detect the FFmpeg capabilities: %v", err)
	}

	if !detected.HLSMuxing || detected.H264Encoder == "" || detected.AACEncoder == "" {
		t.Skipf("skipping the corpus playback integration test: the local FFmpeg %s lacks HLS muxing or "+
			"the libx264/aac encoders (%+v)", detected.FFmpegVersion, detected)
	}

	t.Logf("FFmpeg %s", detected.FFmpegVersion)
}

// scanCorpus scans the library with the real walker, resolver and prober and returns the
// catalogued file IDs by relative path.
func (graph *corpusGraph) scanCorpus(t *testing.T) map[string]string {
	t.Helper()

	catalog := sqlite.NewCatalogWriter(graph.database)

	service, err := scan.NewService(scan.Dependencies{
		Libraries: sqlite.NewLibraryRepository(graph.database, time.Now),
		Walker:    graph.files, Resolver: graph.files, Prober: graph.prober, Catalog: catalog,
		IDs: uuidgen.New(), Clock: time.Now, Logger: graph.logger,
	})
	if err != nil {
		t.Fatalf("scan.NewService: %v", err)
	}

	defer closeWithTimeout(t, "scan service", service.Close)

	if _, err := service.Start(t.Context(), graph.libraryID); err != nil {
		t.Fatalf("start the scan: %v", err)
	}

	status := waitForScan(t, service, graph.libraryID)
	if status.State != scan.StateIdle || status.Counts.Failed != 0 {
		t.Fatalf("scan ended with state %s, code %q, counts %+v; want idle without failed files",
			status.State, status.ErrorCode, status.Counts)
	}

	stored, err := catalog.FileStates(t.Context(), graph.libraryID)
	if err != nil {
		t.Fatalf("list the catalogued files: %v", err)
	}

	fileIDs := make(map[string]string, len(stored))
	for _, file := range stored {
		fileIDs[file.State.RelativePath] = file.ID
	}

	return fileIDs
}

func waitForScan(t *testing.T, service *scan.Service, libraryID string) scan.Status {
	t.Helper()

	deadline := time.Now().Add(scanTimeout)

	for {
		status := service.Status(libraryID)
		if status.State != scan.StateRunning {
			return status
		}

		if time.Now().After(deadline) {
			t.Fatalf("the corpus scan did not finish within %s: %+v", scanTimeout, status.Counts)
		}

		time.Sleep(pollInterval)
	}
}

// newPlaybackService wires the playback use case with the real adapters and the given
// capabilities. It allows maxParallelSessions re-encoding sessions, so the bounded
// parallelism of the test never trips a session limit. The service is closed when the test
// ends.
func (graph *corpusGraph) newPlaybackService(t *testing.T, capabilities playback.ServerCapabilities) *appplayback.Service {
	t.Helper()

	workspace, err := transcodedir.New(graph.transcodeRoot)
	if err != nil {
		t.Fatalf("transcodedir.New: %v", err)
	}

	producer, err := ffmpeg.NewProducer(ffmpeg.ProducerOptions{FFmpegPath: graph.ffmpegPath, Inputs: graph.files, Logger: graph.logger})
	if err != nil {
		t.Fatalf("ffmpeg.NewProducer: %v", err)
	}

	service, err := appplayback.NewService(appplayback.Dependencies{
		Files: sqlite.NewCatalogReader(graph.database), Paths: graph.files, Keyframes: graph.prober,
		Producer: producer, Workspace: workspace, URLs: stream.NewURLBuilder(), IDs: uuidgen.New(),
		Clock: time.Now, After: time.After, Logger: graph.logger,
	}, appplayback.Options{
		Capabilities:         capabilities,
		Policy:               playback.Policy{MaxTranscodeHeight: maxTranscodeHeight},
		MaxTranscodeSessions: maxParallelSessions,
	})
	if err != nil {
		t.Fatalf("appplayback.NewService: %v", err)
	}

	t.Cleanup(func() { closeWithTimeout(t, "playback service", service.Close) })

	return service
}

// closeWithTimeout closes a service detached from the test's context, which is already
// cancelled when cleanups run.
func closeWithTimeout(t *testing.T, name string, closeService func(context.Context) error) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.WithoutCancel(t.Context()), shutdownTimeout)
	defer cancel()

	if err := closeService(ctx); err != nil {
		t.Errorf("close the %s: %v", name, err)
	}
}

// assertWorkspaceEmpty checks that every stopped session removed its directory: the managed
// <transcode>/sessions directory is missing or empty.
func assertWorkspaceEmpty(t *testing.T, transcodeRoot string) {
	t.Helper()

	entries, err := os.ReadDir(filepath.Join(transcodeRoot, "sessions"))
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("list the transcode workspace: %v", err)
	}

	for _, entry := range entries {
		t.Errorf("the transcode workspace still holds %s after every session was stopped", entry.Name())
	}
}
