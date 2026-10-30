package scan_test

import (
	"context"
	"errors"
	"log/slog"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/magpeek/mivo/internal/app/scan"
	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/library"
)

func TestNewServiceRejectsMissingDependencies(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		mutate func(*scan.Dependencies)
	}{
		{name: "libraries", mutate: func(d *scan.Dependencies) { d.Libraries = nil }},
		{name: "walker", mutate: func(d *scan.Dependencies) { d.Walker = nil }},
		{name: "resolver", mutate: func(d *scan.Dependencies) { d.Resolver = nil }},
		{name: "prober", mutate: func(d *scan.Dependencies) { d.Prober = nil }},
		{name: "catalog", mutate: func(d *scan.Dependencies) { d.Catalog = nil }},
		{name: "id generator", mutate: func(d *scan.Dependencies) { d.IDs = nil }},
		{name: "clock", mutate: func(d *scan.Dependencies) { d.Clock = nil }},
		{name: "logger", mutate: func(d *scan.Dependencies) { d.Logger = nil }},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			dependencies := newHarness(t).dependencies()
			test.mutate(&dependencies)

			service, err := scan.NewService(dependencies)
			if !errors.Is(err, domain.ErrInvalid) {
				t.Fatalf("NewService() error = %v, want ErrInvalid", err)
			}

			if !strings.Contains(err.Error(), test.name) {
				t.Errorf("error %q does not name %q", err, test.name)
			}

			if service != nil {
				t.Error("NewService() returned a service on error")
			}
		})
	}
}

func TestStatusBeforeTheFirstScanIsIdleAndEmpty(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	service := h.start(t)

	if got := service.Status(moviesID); got != (scan.Status{State: scan.StateIdle}) {
		t.Errorf("Status() = %+v, want idle zero status", got)
	}

	if got := service.Status("no-such-library"); got.State != scan.StateIdle {
		t.Errorf("Status(unknown) state = %q, want idle", got.State)
	}
}

func TestStartOfAnUnknownLibraryIsNotFound(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	service := h.start(t)

	_, err := service.Start(context.Background(), "no-such-library")
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("Start() error = %v, want ErrNotFound", err)
	}

	if h.walker.callCount() != 0 {
		t.Errorf("walker called %d times, want 0", h.walker.callCount())
	}
}

func TestStartReportsALibraryLookupFailure(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.libraries.getErr = errors.New("database is locked")
	service := h.start(t)

	if _, err := service.Start(context.Background(), moviesID); err == nil ||
		!strings.Contains(err.Error(), "database is locked") {
		t.Fatalf("Start() error = %v, want the lookup error", err)
	}
}

func TestStartWhileAScanRunsReturnsItsStatusWithoutAnotherScan(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.walker.set(moviesRoot, walkResult{files: []library.FileState{fileAt("a.mkv", 1)}})
	h.walker.started = make(chan struct{})
	h.walker.release = make(chan struct{})
	service := h.start(t)

	first, err := service.Start(context.Background(), moviesID)
	if err != nil {
		t.Fatalf("first Start() error = %v", err)
	}

	receive(t, h.walker.started, "walk start")

	second, err := service.Start(context.Background(), moviesID)
	if err != nil {
		t.Fatalf("second Start() error = %v", err)
	}

	if first.State != scan.StateRunning || first.StartedAt.IsZero() {
		t.Errorf("first status = %+v, want running with a start time", first)
	}

	if second.State != scan.StateRunning || !second.StartedAt.Equal(first.StartedAt) {
		t.Errorf("second status = %+v, want the running scan started at %s", second, first.StartedAt)
	}

	close(h.walker.release)
	h.wait(t, moviesID)

	if h.walker.callCount() != 1 {
		t.Errorf("walker called %d times, want 1", h.walker.callCount())
	}

	assertCounts(t, service.Status(moviesID), scan.Counts{Added: 1})
}

func TestStartIgnoresTheCallersCancellation(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.walker.set(moviesRoot, walkResult{files: []library.FileState{fileAt("a.mkv", 1)}})
	h.walker.started = make(chan struct{})
	h.walker.release = make(chan struct{})
	service := h.start(t)

	requestContext, cancelRequest := context.WithCancel(context.Background())

	if _, err := service.Start(requestContext, moviesID); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	receive(t, h.walker.started, "walk start")
	cancelRequest()
	close(h.walker.release)
	h.wait(t, moviesID)

	status := service.Status(moviesID)
	assertState(t, status, scan.StateIdle, scan.ErrorCodeNone)
	assertCounts(t, status, scan.Counts{Added: 1})
}

func TestStartAfterCloseIsUnavailable(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	service := h.start(t)

	if err := service.Close(context.Background()); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	if _, err := service.Start(context.Background(), moviesID); !errors.Is(err, domain.ErrUnavailable) {
		t.Errorf("Start() after Close error = %v, want ErrUnavailable", err)
	}

	if err := service.StartAll(context.Background()); !errors.Is(err, domain.ErrUnavailable) {
		t.Errorf("StartAll() after Close error = %v, want ErrUnavailable", err)
	}

	if h.walker.callCount() != 0 {
		t.Errorf("walker called %d times after Close, want 0", h.walker.callCount())
	}
}

func TestStartRechecksCloseAfterTheLibraryLookup(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	service := h.start(t)
	h.libraries.beforeGet = func() {
		if err := service.Close(context.Background()); err != nil {
			t.Errorf("Close() error = %v", err)
		}
	}

	if _, err := service.Start(context.Background(), moviesID); !errors.Is(err, domain.ErrUnavailable) {
		t.Errorf("Start() racing Close error = %v, want ErrUnavailable", err)
	}
}

func TestStartAllScansEveryLibrary(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.walker.set(moviesRoot, walkResult{files: []library.FileState{fileAt("movie.mkv", 1)}})
	h.walker.set(showsRoot, walkResult{files: []library.FileState{fileAt("S01E01.mkv", 2)}})
	service := h.start(t)

	if err := service.StartAll(context.Background()); err != nil {
		t.Fatalf("StartAll() error = %v", err)
	}

	h.wait(t, moviesID)
	h.wait(t, showsID)

	for _, libraryID := range []string{moviesID, showsID} {
		status := service.Status(libraryID)
		assertState(t, status, scan.StateIdle, scan.ErrorCodeNone)
		assertCounts(t, status, scan.Counts{Added: 1})
	}
}

func TestStartAllReportsAListFailure(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.libraries.listErr = errors.New("database is locked")
	service := h.start(t)

	if err := service.StartAll(context.Background()); err == nil ||
		!strings.Contains(err.Error(), "database is locked") {
		t.Fatalf("StartAll() error = %v, want the list error", err)
	}
}

func TestCloseCancelsARunningScanWithoutRecordingFailures(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.walker.set(moviesRoot, walkResult{files: []library.FileState{
		fileAt("a.mkv", 1), fileAt("b.mkv", 2), fileAt("c.mkv", 3), fileAt("d.mkv", 4),
	}})
	h.prober.gate = make(chan struct{})
	h.prober.entered = make(chan struct{}, 4)
	service := h.start(t)

	if _, err := service.Start(context.Background(), moviesID); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	receive(t, h.prober.entered, "first probe")
	receive(t, h.prober.entered, "second probe")

	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	if err := service.Close(ctx); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	active, _, canceled := h.prober.stats()
	if active != 0 || canceled != scan.MaxConcurrentProbes {
		t.Errorf("after Close: %d probes active, %d canceled, want 0 and %d",
			active, canceled, scan.MaxConcurrentProbes)
	}

	status := service.Status(moviesID)
	assertState(t, status, scan.StateFailed, scan.ErrorCodeCanceled)
	assertCounts(t, status, scan.Counts{})

	if h.catalog.saveCount() != 0 {
		t.Errorf("catalog saved %d files after cancellation, want 0", h.catalog.saveCount())
	}

	if status.FinishedAt.IsZero() {
		t.Error("FinishedAt is zero after the canceled scan ended")
	}
}

func TestCloseIsBoundedByItsContextAndCanBeRepeated(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.walker.set(moviesRoot, walkResult{})
	h.walker.started = make(chan struct{})
	h.walker.release = make(chan struct{})
	h.walker.stubborn = true
	service := h.start(t)

	if _, err := service.Start(context.Background(), moviesID); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	receive(t, h.walker.started, "walk start")

	expired, cancel := context.WithTimeout(context.Background(), time.Millisecond)
	defer cancel()

	if err := service.Close(expired); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Close() of a stuck scan error = %v, want DeadlineExceeded", err)
	}

	close(h.walker.release)

	if err := service.Close(context.Background()); err != nil {
		t.Fatalf("second Close() error = %v", err)
	}

	assertState(t, service.Status(moviesID), scan.StateFailed, scan.ErrorCodeCanceled)
}

// TestCloseLeavesNoGoroutineBehind is not parallel: parallel tests are paused while it runs,
// so the goroutine count is stable enough to compare (a goleak-style check without the
// dependency).
func TestCloseLeavesNoGoroutineBehind(t *testing.T) {
	baseline := runtime.NumGoroutine()

	h := newHarness(t)
	h.walker.set(moviesRoot, walkResult{files: []library.FileState{
		fileAt("a.mkv", 1), fileAt("b.mkv", 2), fileAt("c.mkv", 3),
	}})
	h.prober.gate = make(chan struct{})
	h.prober.entered = make(chan struct{}, 3)

	service, err := scan.NewService(h.dependencies())
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	if _, err := service.Start(context.Background(), moviesID); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	receive(t, h.prober.entered, "first probe")

	if err := service.Close(context.Background()); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	deadline := time.Now().Add(testTimeout)
	for runtime.NumGoroutine() > baseline {
		if time.Now().After(deadline) {
			t.Fatalf("%d goroutines after Close, want at most %d", runtime.NumGoroutine(), baseline)
		}

		time.Sleep(time.Millisecond)
	}
}

// pauseFinishLine makes the harness block on the "library scan finished" line, the window
// between the end of the scan work and the end of its goroutine.
func pauseFinishLine(h *harness) (entered, release chan struct{}) {
	entered, release = make(chan struct{}, 4), make(chan struct{})
	h.wrapHandler = func(next slog.Handler) slog.Handler {
		return blockingHandler{Handler: next, message: "library scan finished", entered: entered, release: release}
	}

	return entered, release
}

func TestStartWhileAScanLogsItsEndDoesNotStartAnother(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.walker.set(moviesRoot, walkResult{files: []library.FileState{fileAt("a.mkv", 1)}})
	entered, release := pauseFinishLine(h)
	service := h.start(t)

	if _, err := service.Start(context.Background(), moviesID); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	receive(t, entered, "finish line")

	status, err := service.Start(context.Background(), moviesID)
	if err != nil {
		t.Fatalf("Start() during the finish line error = %v", err)
	}

	assertState(t, status, scan.StateRunning, scan.ErrorCodeNone)

	if calls := h.walker.callCount(); calls != 1 {
		t.Errorf("walker called %d times, want 1", calls)
	}

	close(release)
	h.wait(t, moviesID)
	assertState(t, service.Status(moviesID), scan.StateIdle, scan.ErrorCodeNone)

	h.scan(t, moviesID)

	if calls := h.walker.callCount(); calls != 2 {
		t.Errorf("walker called %d times after a later Start, want 2", calls)
	}
}

func TestCloseWaitsForAScanThatLogsItsEnd(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.walker.set(moviesRoot, walkResult{})
	entered, release := pauseFinishLine(h)
	service := h.start(t)

	if _, err := service.Start(context.Background(), moviesID); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	receive(t, entered, "finish line")

	expired, cancel := context.WithTimeout(context.Background(), time.Millisecond)
	defer cancel()

	if err := service.Close(expired); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Close() during the finish line error = %v, want DeadlineExceeded", err)
	}

	close(release)

	if err := service.Close(context.Background()); err != nil {
		t.Fatalf("second Close() error = %v", err)
	}

	assertState(t, service.Status(moviesID), scan.StateIdle, scan.ErrorCodeNone)
}

func TestStartAllKeepsAScanThatAlreadyRuns(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.walker.set(moviesRoot, walkResult{files: []library.FileState{fileAt("movie.mkv", 1)}})
	h.walker.set(showsRoot, walkResult{})
	h.walker.started = make(chan struct{}, 2)
	h.walker.release = make(chan struct{})
	service := h.start(t)

	if _, err := service.Start(context.Background(), moviesID); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	receive(t, h.walker.started, "movies walk")

	if err := service.StartAll(context.Background()); err != nil {
		t.Fatalf("StartAll() error = %v", err)
	}

	receive(t, h.walker.started, "shows walk")
	close(h.walker.release)
	h.wait(t, moviesID)
	h.wait(t, showsID)

	if calls := h.walker.callCount(); calls != 2 {
		t.Errorf("walker called %d times, want 2 (one per library)", calls)
	}

	assertCounts(t, service.Status(moviesID), scan.Counts{Added: 1})
}

func TestTheProbeLimitIsSharedByAllScans(t *testing.T) {
	t.Parallel()

	// Long enough for a probe beyond the limit to start if the limit were per scan.
	const settle = 50 * time.Millisecond

	h := newHarness(t)
	h.walker.set(moviesRoot, walkResult{files: []library.FileState{fileAt("m1.mkv", 1), fileAt("m2.mkv", 2), fileAt("m3.mkv", 3)}})
	h.walker.set(showsRoot, walkResult{files: []library.FileState{fileAt("s1.mkv", 1), fileAt("s2.mkv", 2), fileAt("s3.mkv", 3)}})
	h.prober.gate = make(chan struct{})
	h.prober.entered = make(chan struct{}, 6)
	service := h.start(t)

	if err := service.StartAll(context.Background()); err != nil {
		t.Fatalf("StartAll() error = %v", err)
	}

	for range scan.MaxConcurrentProbes {
		receive(t, h.prober.entered, "probe")
	}

	time.Sleep(settle)

	if active, _, _ := h.prober.stats(); active != scan.MaxConcurrentProbes {
		t.Errorf("%d probes active across two scans, want %d", active, scan.MaxConcurrentProbes)
	}

	close(h.prober.gate)
	h.wait(t, moviesID)
	h.wait(t, showsID)

	if _, maxActive, _ := h.prober.stats(); maxActive != scan.MaxConcurrentProbes {
		t.Errorf("max concurrent probes across two scans = %d, want %d", maxActive, scan.MaxConcurrentProbes)
	}

	assertCounts(t, service.Status(moviesID), scan.Counts{Added: 3})
	assertCounts(t, service.Status(showsID), scan.Counts{Added: 3})
}
