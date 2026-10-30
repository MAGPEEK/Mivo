package scan_test

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/magpeek/mivo/internal/app/scan"
	"github.com/magpeek/mivo/internal/domain/library"
)

const (
	// testTimeout bounds every wait of a test, so a bug fails the test instead of hanging it.
	testTimeout = 10 * time.Second
	moviesID    = "lib-movies"
	moviesRoot  = "movies"
	showsID     = "lib-shows"
	showsRoot   = "shows"
)

// harness wires a Service to fakes. Fields may be adjusted before the first scan; the walker,
// prober and catalog may also be changed between scans.
type harness struct {
	libraries *memoryLibraries
	walker    *scriptedWalker
	resolver  prefixResolver
	prober    *recordingProber
	catalog   *memoryCatalog
	ids       *counterIDs
	clock     *tickingClock
	logs      *syncBuffer
	// wrapHandler, when set, wraps the text handler that writes to logs.
	wrapHandler func(slog.Handler) slog.Handler
	service     *scan.Service
}

func newHarness(t *testing.T) *harness {
	t.Helper()

	h := &harness{
		libraries: &memoryLibraries{libraries: []library.Library{
			{ID: moviesID, Name: "Movies", Root: moviesRoot, Kind: library.KindMovies},
			{ID: showsID, Name: "Shows", Root: showsRoot, Kind: library.KindShows},
		}},
		walker:   newScriptedWalker(),
		resolver: prefixResolver{errs: map[string]error{}},
		prober:   newRecordingProber(),
		catalog:  newMemoryCatalog(),
		ids:      &counterIDs{},
		clock:    &tickingClock{now: time.Date(2026, time.October, 30, 18, 0, 0, 0, time.UTC)},
		logs:     &syncBuffer{},
	}

	h.catalog.libraryKinds = map[string]library.Kind{moviesID: library.KindMovies, showsID: library.KindShows}

	return h
}

func (h *harness) dependencies() scan.Dependencies {
	var handler slog.Handler = slog.NewTextHandler(h.logs, &slog.HandlerOptions{Level: slog.LevelDebug})
	if h.wrapHandler != nil {
		handler = h.wrapHandler(handler)
	}

	return scan.Dependencies{
		Libraries: h.libraries,
		Walker:    h.walker,
		Resolver:  h.resolver,
		Prober:    h.prober,
		Catalog:   h.catalog,
		IDs:       h.ids,
		Clock:     h.clock.Now,
		Logger:    slog.New(handler),
	}
}

// start creates the service; Close runs at cleanup.
func (h *harness) start(t *testing.T) *scan.Service {
	t.Helper()

	service, err := scan.NewService(h.dependencies())
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	h.service = service

	// Registered first, so it runs after Close: no scan may still write then.
	t.Cleanup(func() {
		if violations := h.catalog.violationList(); len(violations) > 0 {
			t.Errorf("the scan broke catalog rules: %q", violations)
		}
	})

	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
		defer cancel()

		if err := service.Close(ctx); err != nil {
			t.Errorf("Close() at cleanup error = %v", err)
		}
	})

	return service
}

// scan starts a scan of libraryID, waits for it and returns its final status.
func (h *harness) scan(t *testing.T, libraryID string) scan.Status {
	t.Helper()

	if _, err := h.service.Start(context.Background(), libraryID); err != nil {
		t.Fatalf("Start(%s) error = %v", libraryID, err)
	}

	h.wait(t, libraryID)

	return h.service.Status(libraryID)
}

func (h *harness) wait(t *testing.T, libraryID string) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	if err := h.service.WaitForScan(ctx, libraryID); err != nil {
		t.Fatalf("WaitForScan(%s) error = %v", libraryID, err)
	}
}

// fileAt returns a file state with a size and a modification time derived from size.
func fileAt(relativePath string, size int64) library.FileState {
	return library.FileState{
		RelativePath: relativePath,
		Size:         size,
		ModifiedAt:   time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC).Add(time.Duration(size) * time.Millisecond),
	}
}

func assertCounts(t *testing.T, status scan.Status, want scan.Counts) {
	t.Helper()

	if status.Counts != want {
		t.Errorf("counts = %+v, want %+v", status.Counts, want)
	}
}

func assertState(t *testing.T, status scan.Status, state scan.State, code scan.ErrorCode) {
	t.Helper()

	if status.State != state || status.ErrorCode != code {
		t.Errorf("state, error code = %q, %q, want %q, %q", status.State, status.ErrorCode, state, code)
	}
}

func receive(t *testing.T, signal <-chan struct{}, what string) {
	t.Helper()

	select {
	case <-signal:
	case <-time.After(testTimeout):
		t.Fatalf("timed out waiting for %s", what)
	}
}
