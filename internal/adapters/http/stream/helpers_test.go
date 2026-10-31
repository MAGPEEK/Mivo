package stream_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/magpeek/mivo/internal/adapters/fs/libraryfs"
	"github.com/magpeek/mivo/internal/adapters/fs/transcodedir"
	"github.com/magpeek/mivo/internal/adapters/http/apigen"
	"github.com/magpeek/mivo/internal/adapters/http/restapi"
	"github.com/magpeek/mivo/internal/adapters/http/stream"
	"github.com/magpeek/mivo/internal/adapters/uuidgen"
	appplayback "github.com/magpeek/mivo/internal/app/playback"
	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/media"
	"github.com/magpeek/mivo/internal/domain/playback"
	"github.com/magpeek/mivo/internal/domain/playback/playbacktest"
	"github.com/magpeek/mivo/internal/domain/system"
)

// File IDs and paths of the catalogue every test environment starts with.
const (
	directFileID       = "f1000000-0000-4000-8000-000000000001"
	remuxFileID        = "f1000000-0000-4000-8000-000000000002"
	directRelativePath = "movies/direct.mp4"
	remuxRelativePath  = "movies/remux.mkv"
	unknownSessionID   = "5d4c3b2a-1f0e-4d9c-8b7a-6f5e4d3c2b1a"
	// directFileSize is large enough for several distinct ranges.
	directFileSize = 4096
	fileDuration   = 60 * time.Second
	maxTranscodes  = 2
	maxHeight      = 1080
)

// directContent is the deterministic body of the direct-play file: byte i is i mod 251, so
// every range has a recognisable value.
func directContent() []byte {
	const modulus = 251

	content := make([]byte, directFileSize)
	for index := range content {
		content[index] = byte(index % modulus)
	}

	return content
}

func h264Video() media.Stream {
	return media.Stream{
		Index: 0, Kind: media.StreamKindVideo, Codec: "h264", Profile: "High",
		PixelFormat: "yuv420p", BitDepth: 8, Width: 1920, Height: 1080, Default: true,
	}
}

func aacAudio() media.Stream {
	return media.Stream{Index: 1, Kind: media.StreamKindAudio, Codec: "aac", Channels: 2, Default: true}
}

// memoryFiles is an in-memory FileSource.
type memoryFiles struct {
	files map[string]appplayback.PlayableFile
}

func (source memoryFiles) PlayableFile(_ context.Context, fileID string) (appplayback.PlayableFile, error) {
	file, found := source.files[fileID]
	if !found {
		return appplayback.PlayableFile{}, fmt.Errorf("file %s: %w", fileID, domain.ErrNotFound)
	}

	return file, nil
}

func catalogFile(root, id, relativePath string, container media.Container) appplayback.PlayableFile {
	return appplayback.PlayableFile{
		LibraryRoot: root,
		File: media.File{
			ID: id, RelativePath: relativePath, Size: directFileSize,
			Description: media.Description{
				Container: container, Duration: fileDuration,
				Streams: []media.Stream{h264Video(), aacAudio()},
			},
		},
	}
}

// zeroKeyframes finds no keyframe; the tests start every session at 0.
type zeroKeyframes struct{}

func (zeroKeyframes) KeyframeAtOrBefore(context.Context, string, time.Duration) (time.Duration, error) {
	return 0, nil
}

// fakeProduction is a Production the test drives: markReady, fail and Stop.
type fakeProduction struct {
	job      appplayback.Job
	ready    chan struct{}
	done     chan struct{}
	waiting  chan struct{}
	once     sync.Once
	mutex    sync.Mutex
	exitErr  error
	stopped  bool
	markOnce sync.Once
}

func newFakeProduction(job appplayback.Job) *fakeProduction {
	return &fakeProduction{
		job: job, ready: make(chan struct{}), done: make(chan struct{}), waiting: make(chan struct{}, 1),
	}
}

// Ready follows the port: after Done it reports how the production ended, before that it
// waits for markReady or ctx.
func (production *fakeProduction) Ready(ctx context.Context) error {
	select {
	case <-production.done:
		return production.result()
	default:
	}

	// Only a stream request waits with a deadline (the readiness timeout); the service's own
	// production watcher waits without one and must not be mistaken for the request.
	if _, hasDeadline := ctx.Deadline(); hasDeadline {
		select {
		case production.waiting <- struct{}{}:
		default:
		}
	}

	select {
	case <-production.ready:
		return nil
	case <-production.done:
		return production.result()
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (production *fakeProduction) result() error {
	production.mutex.Lock()
	defer production.mutex.Unlock()

	if production.stopped {
		return appplayback.ErrProductionStopped
	}

	return production.exitErr
}

func (production *fakeProduction) Stop() {
	production.finish(func() { production.stopped = true })
}

func (production *fakeProduction) Done() <-chan struct{} {
	return production.done
}

func (production *fakeProduction) markReady() {
	production.markOnce.Do(func() { close(production.ready) })
}

func (production *fakeProduction) fail(err error) {
	production.finish(func() { production.exitErr = err })
}

func (production *fakeProduction) finish(record func()) {
	production.once.Do(func() {
		production.mutex.Lock()
		record()
		production.mutex.Unlock()
		close(production.done)
	})
}

// fakeProducer starts fakeProductions and remembers them by session.
type fakeProducer struct {
	mutex       sync.Mutex
	productions map[string]*fakeProduction
}

func (producer *fakeProducer) Start(_ context.Context, job appplayback.Job) (appplayback.Production, error) {
	producer.mutex.Lock()
	defer producer.mutex.Unlock()

	production := newFakeProduction(job)
	producer.productions[job.SessionID] = production

	return production, nil
}

func (producer *fakeProducer) production(t *testing.T, sessionID string) *fakeProduction {
	t.Helper()

	producer.mutex.Lock()
	defer producer.mutex.Unlock()

	production, found := producer.productions[sessionID]
	if !found {
		t.Fatalf("no production for session %s", sessionID)
	}

	return production
}

// manualClock is the service clock; tests move it to see access times.
type manualClock struct {
	mutex sync.Mutex
	now   time.Time
}

func (clock *manualClock) Now() time.Time {
	clock.mutex.Lock()
	defer clock.mutex.Unlock()

	return clock.now
}

func (clock *manualClock) advance(step time.Duration) time.Time {
	clock.mutex.Lock()
	defer clock.mutex.Unlock()

	clock.now = clock.now.Add(step)

	return clock.now
}

// healthySystem is the SystemService the REST API requires; the stream tests only request
// the mounts, so the other mandatory services are stand-ins that panic if ever called.
type healthySystem struct{}

func (healthySystem) Health(context.Context) system.Health {
	return system.Health{}
}

func (healthySystem) Info(context.Context) (system.Info, error) {
	return system.Info{}, nil
}

type (
	unusedLibraries struct{ restapi.LibraryCatalog }
	unusedItems     struct{ restapi.ItemCatalog }
	unusedScans     struct{ restapi.ScanService }
	unusedPlayback  struct{ restapi.PlaybackService }
)

// syncBuffer collects log output; handlers log from the request goroutines.
type syncBuffer struct {
	mutex  sync.Mutex
	buffer bytes.Buffer
}

func (buffer *syncBuffer) Write(data []byte) (int, error) {
	buffer.mutex.Lock()
	defer buffer.mutex.Unlock()

	return buffer.buffer.Write(data)
}

func (buffer *syncBuffer) String() string {
	buffer.mutex.Lock()
	defer buffer.mutex.Unlock()

	return buffer.buffer.String()
}

// environment is a real playback service with a fake producer behind the stream handlers,
// mounted on the real REST API handler.
type environment struct {
	service     *appplayback.Service
	handlers    *stream.Handlers
	api         http.Handler
	producer    *fakeProducer
	clock       *manualClock
	logs        *syncBuffer
	libraryRoot string
	content     []byte
}

func newEnvironment(t *testing.T) *environment {
	t.Helper()

	env := &environment{
		producer:    &fakeProducer{productions: map[string]*fakeProduction{}},
		clock:       &manualClock{now: time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)},
		logs:        &syncBuffer{},
		libraryRoot: t.TempDir(),
		content:     directContent(),
	}

	writeFile(t, filepath.Join(env.libraryRoot, filepath.FromSlash(directRelativePath)), env.content)
	writeFile(t, filepath.Join(env.libraryRoot, filepath.FromSlash(remuxRelativePath)), []byte("matroska"))

	logger := slog.New(slog.NewTextHandler(env.logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	library := mustLibraryFS(t, logger)
	workspace := mustWorkspace(t)

	env.service = mustService(t, appplayback.Dependencies{
		Files: memoryFiles{files: map[string]appplayback.PlayableFile{
			directFileID: catalogFile(env.libraryRoot, directFileID, directRelativePath, media.ContainerMP4),
			remuxFileID:  catalogFile(env.libraryRoot, remuxFileID, remuxRelativePath, media.ContainerMKV),
		}},
		Paths: library, Keyframes: zeroKeyframes{}, Producer: env.producer, Workspace: workspace,
		URLs: stream.NewURLBuilder(), IDs: uuidgen.New(), Clock: env.clock.Now, Logger: logger,
	})

	env.handlers = mustHandlers(t, stream.Options{Sessions: env.service, Library: library, Output: workspace, Logger: logger})
	env.api = mustAPI(t, logger, env.handlers.Mounts())

	return env
}

func mustLibraryFS(t *testing.T, logger *slog.Logger) *libraryfs.FileSystem {
	t.Helper()

	library, err := libraryfs.New(logger)
	if err != nil {
		t.Fatalf("libraryfs.New error = %v", err)
	}

	return library
}

func mustWorkspace(t *testing.T) *transcodedir.Workspace {
	t.Helper()

	workspace, err := transcodedir.New(t.TempDir())
	if err != nil {
		t.Fatalf("transcodedir.New error = %v", err)
	}

	return workspace
}

// neverFires is the service timer: the idle reaper never runs, so moving the clock by
// minutes cannot stop a session in the middle of a test.
func neverFires(time.Duration) <-chan time.Time {
	return nil
}

// mustService returns a playback service that is closed when the test ends, before the
// temporary directories registered earlier are removed.
func mustService(t *testing.T, dependencies appplayback.Dependencies) *appplayback.Service {
	t.Helper()

	dependencies.After = neverFires

	service, err := appplayback.NewService(dependencies, appplayback.Options{
		Capabilities:         playbacktest.FullCapabilities(),
		Policy:               playback.Policy{MaxTranscodeHeight: maxHeight},
		MaxTranscodeSessions: maxTranscodes,
	})
	if err != nil {
		t.Fatalf("NewService error = %v", err)
	}

	t.Cleanup(func() {
		if err := service.Close(context.WithoutCancel(t.Context())); err != nil {
			t.Errorf("Close error = %v", err)
		}
	})

	return service
}

func mustHandlers(t *testing.T, options stream.Options) *stream.Handlers {
	t.Helper()

	handlers, err := stream.New(options)
	if err != nil {
		t.Fatalf("stream.New error = %v", err)
	}

	return handlers
}

func mustAPI(t *testing.T, logger *slog.Logger, mounts []restapi.Mount) http.Handler {
	t.Helper()

	api, err := restapi.NewHandler(restapi.Options{
		System: healthySystem{}, Libraries: unusedLibraries{}, Items: unusedItems{},
		Scan: unusedScans{}, Playback: unusedPlayback{}, Logger: logger, Mounts: mounts,
	})
	if err != nil {
		t.Fatalf("restapi.NewHandler error = %v", err)
	}

	return api
}

func writeFile(t *testing.T, path string, content []byte) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatalf("create directory of %s: %v", path, err)
	}

	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// start creates a session for fileID with the Chromium reference profile.
func (env *environment) start(t *testing.T, fileID string) appplayback.Session {
	t.Helper()

	session, err := env.service.Start(t.Context(), appplayback.StartRequest{
		FileID: fileID, Profile: playbacktest.ChromiumDesktop(),
	})
	if err != nil {
		t.Fatalf("Start(%s) error = %v", fileID, err)
	}

	return session
}

// startHLS starts the remux session and returns it with its production.
func (env *environment) startHLS(t *testing.T) (appplayback.Session, *fakeProduction) {
	t.Helper()

	session := env.start(t, remuxFileID)
	if session.Plan.Delivery != playback.DeliveryHLS {
		t.Fatalf("remux session delivery = %q, want %q", session.Plan.Delivery, playback.DeliveryHLS)
	}

	return session, env.producer.production(t, session.ID)
}

// writeOutput writes an HLS output file into the session directory.
func writeOutput(t *testing.T, production *fakeProduction, name string, content []byte) {
	t.Helper()

	writeFile(t, filepath.Join(production.job.OutputDir, name), content)
}

func newRequest(t *testing.T, method, target string) *http.Request {
	t.Helper()

	return httptest.NewRequestWithContext(t.Context(), method, target, nil)
}

func (env *environment) serve(request *http.Request) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	env.api.ServeHTTP(recorder, request)

	return recorder
}

func (env *environment) get(t *testing.T, target string) *httptest.ResponseRecorder {
	t.Helper()

	return env.serve(newRequest(t, http.MethodGet, target))
}

func fileURL(sessionID string) string {
	return stream.NewURLBuilder().StreamURL(sessionID, playback.DeliveryFile)
}

func hlsURL(sessionID, name string) string {
	return "/api/v1/playback/sessions/" + sessionID + "/hls/" + name
}

func assertProblem(t *testing.T, recorder *httptest.ResponseRecorder, wantStatus int, wantCode string) {
	t.Helper()

	if recorder.Code != wantStatus {
		t.Fatalf("status = %d, want %d (body %q)", recorder.Code, wantStatus, recorder.Body.String())
	}

	if contentType := recorder.Header().Get("Content-Type"); contentType != restapi.ProblemMediaType {
		t.Fatalf("Content-Type = %q, want %q", contentType, restapi.ProblemMediaType)
	}

	var problem apigen.Problem
	if err := json.Unmarshal(recorder.Body.Bytes(), &problem); err != nil {
		t.Fatalf("decode problem %q: %v", recorder.Body.String(), err)
	}

	if problem.Code != wantCode || int(problem.Status) != wantStatus {
		t.Errorf("problem = %+v, want status %d code %q", problem, wantStatus, wantCode)
	}
}

func assertHeader(t *testing.T, header http.Header, name, want string) {
	t.Helper()

	if got := header.Get(name); got != want {
		t.Errorf("%s = %q, want %q", name, got, want)
	}
}
