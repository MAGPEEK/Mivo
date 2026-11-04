package scantrigger_test

import (
	"context"
	"errors"
	"log/slog"
	"slices"
	"testing"
	"time"

	"github.com/magpeek/mivo/internal/app/scan"
	"github.com/magpeek/mivo/internal/app/scantrigger"
	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/library"
)

const (
	defaultInterval = 12 * time.Hour
	// watchWarning is the text of the warning logged when a library cannot be watched.
	watchWarning = "relying on the periodic scan"

	moviesRoot = "/media/movies"
	showsRoot  = "/media/shows"
)

// managed builds a library with one folder per path.
func managed(id, name string, paths ...string) library.ManagedLibrary {
	folders := make([]library.Folder, 0, len(paths))
	for _, path := range paths {
		folders = append(folders, library.Folder{ID: "folder-" + path, Path: path})
	}

	return library.ManagedLibrary{ID: id, Name: name, Kind: library.KindMovies, Folders: folders}
}

func startTime() time.Time { return time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC) }

func movies() library.ManagedLibrary {
	return managed("movies-id", "Movies", moviesRoot)
}

func shows() library.ManagedLibrary {
	return managed("shows-id", "Shows", showsRoot)
}

// harness runs one Trigger against fakes.
type harness struct {
	clock     *fakeClock
	libraries *memoryLibraries
	scanner   *fakeScanner
	watcher   *fakeWatcher
	logs      *logBuffer
	trigger   *scantrigger.Trigger
	settings  scantrigger.Settings
	cancel    context.CancelFunc
	events    chan struct{}
	done      chan struct{}
}

func newHarness(t *testing.T, settings scantrigger.Settings, libraries ...library.ManagedLibrary) *harness {
	t.Helper()

	clock := newFakeClock(startTime())
	h := &harness{
		clock:     clock,
		libraries: &memoryLibraries{libraries: libraries},
		scanner:   newFakeScanner(clock),
		watcher:   newFakeWatcher(),
		logs:      &logBuffer{},
		settings:  settings,
		events:    make(chan struct{}, eventBacklog),
	}

	trigger, err := scantrigger.New(scantrigger.Dependencies{
		Libraries: h.libraries,
		Scanner:   h.scanner,
		Watcher:   h.watcher,
		Clock:     clock,
		Logger:    slog.New(slog.NewTextHandler(h.logs, &slog.HandlerOptions{Level: slog.LevelDebug})),
		Settings:  settings,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	scantrigger.SetAfterTick(trigger, clock.tickHandled)
	scantrigger.SetAfterEvents(trigger, func() { h.events <- struct{}{} })
	h.trigger = trigger

	return h
}

// run starts Run in the background; the test's cleanup stops it and checks that it ended.
// Unless both parts are off, it hands Run one tick at the start time: Run takes ticks only
// after its first check, so that check has read the start time before a test moves it.
func (h *harness) run(t *testing.T) {
	t.Helper()

	ctx, cancel := context.WithCancel(context.Background())
	h.cancel = cancel
	h.done = make(chan struct{})

	go func() {
		defer close(h.done)
		h.trigger.Run(ctx)
	}()

	t.Cleanup(func() { h.stop(t) })

	if h.settings.Watch || h.settings.Interval > 0 {
		h.clock.Advance(t, 0)
	}
}

func (h *harness) stop(t *testing.T) {
	t.Helper()

	h.cancel()

	select {
	case <-h.done:
	case <-time.After(waitTimeout):
		t.Fatal("Run did not return after its context was cancelled")
	}
}

// watching runs the trigger with the watcher only and waits until root is watched.
func watching(t *testing.T, libraries ...library.ManagedLibrary) *harness {
	t.Helper()

	h := newHarness(t, scantrigger.Settings{Watch: true}, libraries...)
	h.run(t)

	for _, entry := range libraries {
		h.watcher.waitForAttempt(t, entry.Folders[0].Path)
	}

	return h
}

// checkUntilWatched runs checks until the trigger retries the watch of root. A failed watch
// is cleaned up by its own goroutine, so the check right after the failure may come too
// early to retry it.
func (h *harness) checkUntilWatched(t *testing.T, root string) {
	t.Helper()

	const maxChecks = 50

	for range maxChecks {
		h.clock.Advance(t, scantrigger.CheckInterval)

		if h.watcher.receivedAttempt(root) {
			return
		}
	}

	t.Fatalf("the watch of %s was not retried", root)
}

func (h *harness) expectStarts(t *testing.T, libraryID string, want int, when string) {
	t.Helper()

	if got := h.scanner.startCount(libraryID); got != want {
		t.Fatalf("%s: %d scans of %s started, want %d; log:\n%s", when, got, libraryID, want, h.logs)
	}
}

func TestNewRejectsIncompleteDependencies(t *testing.T) {
	t.Parallel()

	clock := newFakeClock(startTime())
	complete := scantrigger.Dependencies{
		Libraries: &memoryLibraries{},
		Scanner:   newFakeScanner(clock),
		Watcher:   newFakeWatcher(),
		Clock:     clock,
		Logger:    slog.New(slog.DiscardHandler),
		Settings:  scantrigger.Settings{Watch: true, Interval: defaultInterval},
	}

	tests := map[string]func(*scantrigger.Dependencies){
		"libraries":         func(d *scantrigger.Dependencies) { d.Libraries = nil },
		"scanner":           func(d *scantrigger.Dependencies) { d.Scanner = nil },
		"watcher":           func(d *scantrigger.Dependencies) { d.Watcher = nil },
		"clock":             func(d *scantrigger.Dependencies) { d.Clock = nil },
		"logger":            func(d *scantrigger.Dependencies) { d.Logger = nil },
		"negative interval": func(d *scantrigger.Dependencies) { d.Settings.Interval = -time.Hour },
	}

	for name, breakIt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			dependencies := complete
			breakIt(&dependencies)

			if _, err := scantrigger.New(dependencies); !errors.Is(err, domain.ErrInvalid) {
				t.Errorf("New = %v, want ErrInvalid", err)
			}
		})
	}

	if _, err := scantrigger.New(complete); err != nil {
		t.Errorf("New(complete) = %v", err)
	}
}

// A burst of changes starts one scan once the library has been quiet for 30 s.
func TestChangesStartAScanAfterTheQuietPeriod(t *testing.T) {
	t.Parallel()

	h := watching(t, movies())
	root := moviesRoot

	h.watcher.emit(t, root)
	h.clock.Advance(t, 29*time.Second)
	h.expectStarts(t, movies().ID, 0, "29 s after the first change")

	h.watcher.emit(t, root)
	h.clock.Advance(t, 29*time.Second)
	h.expectStarts(t, movies().ID, 0, "29 s after the second change")

	h.clock.Advance(t, time.Second)
	h.expectStarts(t, movies().ID, 1, "30 s after the second change")

	h.scanner.finish(movies().ID)
	h.clock.Advance(t, time.Hour)
	h.expectStarts(t, movies().ID, 1, "without further changes")
}

// Changes that never pause (a long copy) still start a scan 5 min after the first one.
func TestContinuousChangesStartAScanAfterTheMaximumDelay(t *testing.T) {
	t.Parallel()

	h := watching(t, movies())
	step := 20 * time.Second

	for elapsed := time.Duration(0); elapsed < scantrigger.MaxDelay; elapsed += step {
		h.expectStarts(t, movies().ID, 0, "before the maximum delay")
		h.watcher.emit(t, moviesRoot)
		h.clock.Advance(t, step)
	}

	h.expectStarts(t, movies().ID, 1, "at the maximum delay")
}

// Changes during a running scan lead to exactly one more scan after it, however many there
// are and however long the scan takes.
func TestChangesDuringARunningScanScheduleOneMoreScan(t *testing.T) {
	t.Parallel()

	h := watching(t, movies())
	h.scanner.setStatus(movies().ID, scan.Status{State: scan.StateRunning, StartedAt: startTime()})

	for range 3 {
		h.watcher.emit(t, moviesRoot)
		h.clock.Advance(t, scantrigger.QuietPeriod)
	}

	h.clock.Advance(t, time.Minute)
	h.expectStarts(t, movies().ID, 0, "while the scan runs")

	h.scanner.finish(movies().ID)
	h.clock.Advance(t, scantrigger.EvaluationInterval)
	h.expectStarts(t, movies().ID, 1, "after the running scan finished")

	h.scanner.finish(movies().ID)
	h.clock.Advance(t, time.Hour)
	h.expectStarts(t, movies().ID, 1, "an hour later")
}

func TestChangesAreDebouncedPerLibrary(t *testing.T) {
	t.Parallel()

	h := watching(t, movies(), shows())

	h.watcher.emit(t, moviesRoot)
	h.clock.Advance(t, 20*time.Second)
	h.watcher.emit(t, showsRoot)
	h.clock.Advance(t, 10*time.Second)

	h.expectStarts(t, movies().ID, 1, "30 s after the movies change")
	h.expectStarts(t, shows().ID, 0, "10 s after the shows change")

	h.clock.Advance(t, 20*time.Second)
	h.expectStarts(t, shows().ID, 1, "30 s after the shows change")
}

func TestPeriodicScanStartsWhenTheLastScanIsOlderThanTheInterval(t *testing.T) {
	t.Parallel()

	h := newHarness(t, scantrigger.Settings{Interval: defaultInterval}, movies(), shows())
	h.scanner.setStatus(movies().ID, scan.Status{State: scan.StateIdle, FinishedAt: startTime().Add(-11 * time.Hour)})
	h.scanner.setStatus(shows().ID, scan.Status{State: scan.StateFailed, FinishedAt: startTime().Add(-13 * time.Hour)})
	h.run(t)

	h.expectStarts(t, shows().ID, 1, "at start, 13 h after the last scan")
	h.expectStarts(t, movies().ID, 0, "at start, 11 h after the last scan")

	h.clock.Advance(t, 59*time.Minute)
	h.expectStarts(t, movies().ID, 0, "11 h 59 min after the last scan")

	h.clock.Advance(t, time.Minute)
	h.expectStarts(t, movies().ID, 1, "12 h after the last scan")

	// The interval counts from the end of the last scan, not from its start.
	h.clock.Advance(t, 2*time.Hour)
	h.scanner.finish(movies().ID)
	h.clock.Advance(t, defaultInterval-time.Minute)
	h.expectStarts(t, movies().ID, 1, "one minute before the interval after the end")

	h.clock.Advance(t, time.Minute)
	h.expectStarts(t, movies().ID, 2, "the interval after the end of the last scan")
}

// A library without a finished scan (its startup scan still runs or never started) is
// first due one interval after the trigger saw it; a running scan is never started twice.
func TestPeriodicScanOfLibrariesWithoutFinishedScan(t *testing.T) {
	t.Parallel()

	h := newHarness(t, scantrigger.Settings{Interval: defaultInterval}, movies(), shows())
	h.scanner.setStatus(shows().ID, scan.Status{State: scan.StateRunning, StartedAt: startTime().Add(-24 * time.Hour)})
	h.run(t)

	h.clock.Advance(t, defaultInterval-time.Minute)
	h.expectStarts(t, movies().ID, 0, "before the interval")

	h.clock.Advance(t, time.Minute)
	h.expectStarts(t, movies().ID, 1, "one interval after the trigger saw the library")
	h.expectStarts(t, shows().ID, 0, "while its scan runs")
}

func TestPeriodicScanChecksOncePerMinute(t *testing.T) {
	t.Parallel()

	h := newHarness(t, scantrigger.Settings{Interval: defaultInterval}, movies())
	h.scanner.setStatus(movies().ID, scan.Status{State: scan.StateIdle, FinishedAt: startTime().Add(-defaultInterval + 10*time.Second)})
	h.run(t)

	h.clock.Advance(t, 30*time.Second)
	h.expectStarts(t, movies().ID, 0, "due, but within a minute of the last check")

	h.clock.Advance(t, 30*time.Second)
	h.expectStarts(t, movies().ID, 1, "at the next check")

	if got := h.clock.tickerIntervals(); !slices.Equal(got, []time.Duration{scantrigger.EvaluationInterval}) {
		t.Errorf("ticker intervals = %v, want one ticker every %s", got, scantrigger.EvaluationInterval)
	}
}

func TestDisabledWatcherWatchesNothing(t *testing.T) {
	t.Parallel()

	h := newHarness(t, scantrigger.Settings{Interval: defaultInterval}, movies())
	h.run(t)

	h.clock.Advance(t, 10*time.Minute)
	h.watcher.expectNoAttempt(t)
}

func TestDisabledIntervalStartsNoPeriodicScan(t *testing.T) {
	t.Parallel()

	h := watching(t, movies())
	h.scanner.setStatus(movies().ID, scan.Status{State: scan.StateIdle, FinishedAt: startTime().Add(-1000 * time.Hour)})

	h.clock.Advance(t, 1000*time.Hour)
	h.expectStarts(t, movies().ID, 0, "with the periodic scan off")
}

// With both parts off Run only waits for its context: no ticker, no watch, no scan.
func TestBothPartsDisabledDoNothing(t *testing.T) {
	t.Parallel()

	h := newHarness(t, scantrigger.Settings{}, movies())
	h.run(t)
	h.stop(t)

	if got := h.clock.tickerIntervals(); len(got) != 0 {
		t.Errorf("tickers = %v, want none", got)
	}

	h.watcher.expectNoAttempt(t)
	h.expectStarts(t, movies().ID, 0, "with both parts off")
}

// A library that cannot be watched is retried at every check, with one warning in total.
// Once a retried watch is ready, one scan catches the changes nobody saw meanwhile.
func TestWatchFailureIsRetriedAndWarnedOnce(t *testing.T) {
	t.Parallel()

	h := newHarness(t, scantrigger.Settings{Watch: true, Interval: defaultInterval}, movies())
	h.watcher.fail(moviesRoot, errFake)
	h.run(t)
	h.watcher.waitForAttempt(t, moviesRoot)
	h.logs.waitFor(t, watchWarning)

	for range 3 {
		h.checkUntilWatched(t, moviesRoot)
	}

	h.expectStarts(t, movies().ID, 0, "while the watch fails")

	h.watcher.fail(moviesRoot, nil)
	h.checkUntilWatched(t, moviesRoot)

	h.clock.Advance(t, scantrigger.QuietPeriod)
	h.expectStarts(t, movies().ID, 1, "after the retried watch was ready, without a change")

	h.scanner.finish(movies().ID)
	h.clock.Advance(t, time.Hour)
	h.expectStarts(t, movies().ID, 1, "an hour later without a change")

	if got := h.logs.count(watchWarning); got != 1 {
		t.Errorf("%d watch warnings, want 1; log:\n%s", got, h.logs)
	}
}

// A watch that is ready the first time records no change: the startup scan covers it.
func TestFirstReadyWatchStartsNoScan(t *testing.T) {
	t.Parallel()

	h := watching(t, movies())

	h.clock.Advance(t, scantrigger.MaxDelay)
	h.expectStarts(t, movies().ID, 0, "after the first watch was ready")
}

func TestWatchFailureWithoutPeriodicScanSaysSo(t *testing.T) {
	t.Parallel()

	h := newHarness(t, scantrigger.Settings{Watch: true}, movies())
	h.watcher.fail(moviesRoot, errFake)
	h.run(t)
	h.watcher.waitForAttempt(t, moviesRoot)
	h.logs.waitFor(t, "changes appear after the next manual scan")

	if got := h.logs.count(watchWarning); got != 0 {
		t.Errorf("log:\n%s\nwant no mention of a periodic scan that is off", h.logs)
	}
}

// A partly watched tree (watch limit, lost events) is reported through the Degraded
// callback of its watch; it shares the one warning per library.
func TestDegradedWatchWarnsOncePerLibrary(t *testing.T) {
	t.Parallel()

	h := newHarness(t, scantrigger.Settings{Watch: true, Interval: defaultInterval}, movies(), shows())
	h.run(t)
	h.watcher.waitForAttempt(t, moviesRoot)
	h.watcher.waitForAttempt(t, showsRoot)

	h.watcher.degrade(t, moviesRoot, errFake)
	h.watcher.degrade(t, moviesRoot, errFake)
	h.watcher.degrade(t, showsRoot, errFake)

	if got := h.logs.count("only partly watched; " + watchWarning); got != 2 {
		t.Errorf("%d watch warnings, want one per library; log:\n%s", got, h.logs)
	}
}

func TestLibraryChangesStartAndStopWatches(t *testing.T) {
	t.Parallel()

	h := watching(t, movies())

	h.libraries.set(shows())
	h.clock.Advance(t, scantrigger.CheckInterval)
	h.watcher.waitForAttempt(t, showsRoot)
	h.watcher.waitUntilInactive(t, moviesRoot)

	moved := shows()
	moved.Folders = []library.Folder{{Path: "/media/series"}}
	h.libraries.set(moved)
	h.clock.Advance(t, scantrigger.CheckInterval)
	h.watcher.waitForAttempt(t, moved.Folders[0].Path)
	h.watcher.waitUntilInactive(t, showsRoot)

	if got := h.watcher.activeWatches(); got != 1 {
		t.Errorf("%d active watches, want 1", got)
	}
}

// Events of a removed library are dropped instead of starting a scan.
func TestChangesOfARemovedLibraryAreDropped(t *testing.T) {
	t.Parallel()

	h := watching(t, movies())
	h.watcher.emit(t, moviesRoot)

	h.libraries.set()
	h.clock.Advance(t, scantrigger.CheckInterval)

	h.expectStarts(t, movies().ID, 0, "after the library was removed")
}

func TestListFailureKeepsTheKnownLibraries(t *testing.T) {
	t.Parallel()

	h := watching(t, movies())

	h.libraries.failWith(errFake)
	h.clock.Advance(t, scantrigger.CheckInterval)
	h.watcher.emit(t, moviesRoot)
	h.clock.Advance(t, scantrigger.QuietPeriod)

	h.expectStarts(t, movies().ID, 1, "after a failed library refresh")

	if got := h.logs.count("list libraries"); got != 1 {
		t.Errorf("log:\n%s\nwant one line about the failed library list", h.logs)
	}
}

func TestStartFailureIsLoggedOnceAndRetriedAfterTheRetryDelay(t *testing.T) {
	t.Parallel()

	h := watching(t, movies())
	h.scanner.failStarts(errFake)

	h.watcher.emit(t, moviesRoot)
	h.clock.Advance(t, scantrigger.QuietPeriod)
	h.clock.Advance(t, scantrigger.QuietPeriod)

	if got := h.logs.count("automatic scan could not start"); got != 1 {
		t.Errorf("log:\n%s\nwant one failed start", h.logs)
	}

	// The due scan is kept and retried after RetryDelay, without a new change.
	h.scanner.failStarts(nil)
	h.clock.Advance(t, scantrigger.RetryDelay)
	h.expectStarts(t, movies().ID, 1, "after the retry delay")
}

// Run returns only after every watch it started has ended.
func TestRunEndsEveryWatch(t *testing.T) {
	t.Parallel()

	h := watching(t, movies(), shows())
	h.stop(t)

	if got := h.watcher.activeWatches(); got != 0 {
		t.Errorf("%d watches still running after Run returned", got)
	}

	if !h.clock.isStopped() {
		t.Error("the ticker was not stopped")
	}
}
