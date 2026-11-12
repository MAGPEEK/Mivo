package trickplay_test

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"sync"
	"time"

	appaccess "github.com/magpeek/mivo/internal/app/access"
	appplayback "github.com/magpeek/mivo/internal/app/playback"
	"github.com/magpeek/mivo/internal/app/trickplay"
	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/access"
	"github.com/magpeek/mivo/internal/domain/media"
	"github.com/magpeek/mivo/internal/domain/playback"
)

const cacheRoot = "/cache/trickplay"

func epoch() time.Time { return time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC) }

func viewerContext(libraries access.LibraryAccess) context.Context {
	viewer := access.Viewer{Libraries: libraries, Limit: access.NoLimit()}

	return appaccess.WithViewer(context.Background(), viewer)
}

func videoFile(id, libraryID string) appplayback.PlayableFile {
	return appplayback.PlayableFile{
		LibraryRoot: "/lib",
		File: media.File{
			ID: id, LibraryID: libraryID, RelativePath: id + ".mkv", Size: 100, ModifiedAt: epoch(),
			Description: media.Description{
				Container: media.ContainerMKV,
				Streams:   []media.Stream{{Index: 0, Kind: media.StreamKindVideo, Codec: "h264", Width: 640, Height: 360}},
			},
		},
	}
}

type fakeFiles struct {
	mu    sync.Mutex
	files map[string]appplayback.PlayableFile
}

func (f *fakeFiles) PlayableFile(_ context.Context, fileID string, restriction access.Restriction) (appplayback.PlayableFile, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	file, ok := f.files[fileID]
	if !ok || !restriction.AllowsLibrary(file.File.LibraryID) {
		return appplayback.PlayableFile{}, fmt.Errorf("file %s: %w", fileID, domain.ErrNotFound)
	}

	return file, nil
}

func (f *fakeFiles) set(file appplayback.PlayableFile) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.files[file.File.ID] = file
}

type fakePaths struct{}

func (fakePaths) Resolve(root, relative string) (string, error) {
	return root + "/" + relative, nil
}

type generated struct{ source, destination string }

type fakeGenerator struct {
	calls   chan generated
	err     error
	block   bool
	count   int
	mu      sync.Mutex
	started chan struct{}
}

func newGenerator() *fakeGenerator {
	return &fakeGenerator{calls: make(chan generated, 1000), started: make(chan struct{}, 1000)}
}

func (g *fakeGenerator) GenerateTrickplay(ctx context.Context, source, destination string) (trickplay.Manifest, error) {
	g.mu.Lock()
	g.count++
	block, err := g.block, g.err
	g.mu.Unlock()

	g.calls <- generated{source, destination}
	g.started <- struct{}{}

	if block {
		<-ctx.Done()

		return trickplay.Manifest{}, fmt.Errorf("generate: %w", ctx.Err())
	}

	if err != nil {
		return trickplay.Manifest{}, err
	}

	return trickplay.Manifest{TileWidth: 320, TileHeight: 180, Columns: 10, Rows: 10, Interval: 10 * time.Second, SheetCount: 3}, nil
}

func (g *fakeGenerator) callCount() int {
	g.mu.Lock()
	defer g.mu.Unlock()

	return g.count
}

type fakeStore struct {
	mu      sync.Mutex
	stored  map[string]trickplay.Stored
	touched []string
}

func (s *fakeStore) Directory(fileID string) string { return filepath.Join(cacheRoot, fileID) }

func (s *fakeStore) Load(_ context.Context, fileID string) (trickplay.Stored, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	stored, ok := s.stored[fileID]
	if !ok {
		return trickplay.Stored{}, fmt.Errorf("manifest %s: %w", fileID, domain.ErrNotFound)
	}

	return stored, nil
}

func (s *fakeStore) Save(_ context.Context, fileID string, stored trickplay.Stored) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.stored[fileID] = stored

	return nil
}

func (s *fakeStore) Touch(_ context.Context, fileID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.touched = append(s.touched, fileID)

	return nil
}

type fakeCache struct {
	mu       sync.Mutex
	entries  []trickplay.Entry
	removed  []string
	swept    bool
	sweepErr error
}

func (c *fakeCache) Entries(context.Context) ([]trickplay.Entry, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	return append([]trickplay.Entry(nil), c.entries...), nil
}

func (c *fakeCache) Remove(_ context.Context, fileID string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.removed = append(c.removed, fileID)

	return nil
}

func (c *fakeCache) SweepStaging(context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.swept = true

	return c.sweepErr
}

func (c *fakeCache) removedIDs() []string {
	c.mu.Lock()
	defer c.mu.Unlock()

	return append([]string(nil), c.removed...)
}

// fakeSessions reports a transcode for the first busyPolls calls.
type fakeSessions struct {
	mu        sync.Mutex
	busyPolls int
	always    bool
	polls     int
}

func (s *fakeSessions) List(context.Context) ([]appplayback.SessionInfo, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.polls++
	if s.always || s.polls <= s.busyPolls {
		return []appplayback.SessionInfo{
			{ID: "direct", Mode: playback.ModeDirectPlay},
			{ID: "viewer", Mode: playback.ModeTranscode},
		}, nil
	}

	return []appplayback.SessionInfo{{ID: "direct", Mode: playback.ModeDirectPlay}}, nil
}

func (s *fakeSessions) pollCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.polls
}

// fakeClock advances by the waited duration on every After and fires at once.
type fakeClock struct {
	mu    sync.Mutex
	now   time.Time
	waits []time.Duration
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.now
}

func (c *fakeClock) After(d time.Duration) <-chan time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.now = c.now.Add(d)
	c.waits = append(c.waits, d)
	fired := make(chan time.Time, 1)
	fired <- c.now

	return fired
}

func (c *fakeClock) waitCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()

	return len(c.waits)
}

// syncBuffer collects log output safely across goroutines.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()

	return b.buf.String()
}

func newLogger(out *syncBuffer) *slog.Logger {
	return slog.New(slog.NewTextHandler(out, &slog.HandlerOptions{Level: slog.LevelWarn}))
}
