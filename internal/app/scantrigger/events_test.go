package scantrigger_test

import (
	"testing"
	"time"

	"github.com/magpeek/mivo/internal/app/scan"
	"github.com/magpeek/mivo/internal/app/scantrigger"
)

const (
	extraRoot     = "/media/extra"
	otherRoot     = "/media/other"
	folderWarning = "cannot watch library folder"
	ignoredEvent  = "ignoring event for an unknown library"
	// eventBacklog is the capacity of the channel that reports handled event batches.
	eventBacklog = 64
)

// event sends one library event and waits until the trigger has applied it.
func (h *harness) event(t *testing.T, send func(string), libraryID string) {
	t.Helper()

	send(libraryID)

	select {
	case <-h.events:
	case <-time.After(waitTimeout):
		t.Fatal("the trigger did not handle the library event")
	}
}

func (h *harness) added(t *testing.T, libraryID string) {
	t.Helper()
	h.event(t, h.trigger.Added, libraryID)
}

func (h *harness) changed(t *testing.T, libraryID string) {
	t.Helper()
	h.event(t, h.trigger.Changed, libraryID)
}

func (h *harness) removed(t *testing.T, libraryID string) {
	t.Helper()
	h.event(t, h.trigger.Removed, libraryID)
}

func TestAddedLibraryIsWatchedWithoutWaitingForTheNextCheck(t *testing.T) {
	t.Parallel()

	h := watching(t, movies())
	h.libraries.set(movies(), shows())
	h.added(t, shows().ID)
	h.watcher.waitForAttempt(t, showsRoot)

	h.watcher.emit(t, showsRoot)
	h.clock.Advance(t, scantrigger.QuietPeriod)
	h.expectStarts(t, shows().ID, 1, "30 s after a change in the added library")
}

func TestAddedLibraryJoinsThePeriodicCheck(t *testing.T) {
	t.Parallel()

	h := newHarness(t, scantrigger.Settings{Interval: defaultInterval}, movies())
	h.run(t)

	h.libraries.set(movies(), shows())
	h.added(t, shows().ID)

	h.clock.Advance(t, defaultInterval)
	h.expectStarts(t, shows().ID, 1, "one interval after the library was added")
}

func TestOneWatchPerFolder(t *testing.T) {
	t.Parallel()

	multi := managed("multi-id", "Multi", moviesRoot, extraRoot)
	h := watching(t, multi)
	h.watcher.waitForAttempt(t, extraRoot)

	if got := h.watcher.activeWatches(); got != 2 {
		t.Fatalf("%d active watches, want one per folder", got)
	}

	h.watcher.emit(t, extraRoot)
	h.clock.Advance(t, scantrigger.QuietPeriod)
	h.expectStarts(t, multi.ID, 1, "after a change in the second folder")
}

// A folder change swaps the watches and keeps the pending debounce.
func TestChangedFoldersSwapWatchesAndKeepTheDebounce(t *testing.T) {
	t.Parallel()

	h := watching(t, managed("multi-id", "Multi", moviesRoot, extraRoot))
	h.watcher.waitForAttempt(t, extraRoot)
	h.watcher.emit(t, moviesRoot)
	h.clock.Advance(t, 20*time.Second)

	h.libraries.set(managed("multi-id", "Multi", moviesRoot, otherRoot))
	h.changed(t, "multi-id")
	h.watcher.waitForAttempt(t, otherRoot)
	h.watcher.waitUntilInactive(t, extraRoot)

	if got := h.watcher.activeWatches(); got != 2 {
		t.Errorf("%d active watches, want 2", got)
	}

	h.clock.Advance(t, 10*time.Second)
	h.expectStarts(t, "multi-id", 1, "30 s after the change that came before the folder change")

	if got := h.logs.count(folderWarning); got != 0 {
		t.Errorf("a watch stopped on purpose was logged as failed; log:\n%s", h.logs)
	}
}

func TestRemovedLibraryStopsItsWatchesAndDropsPendingScans(t *testing.T) {
	t.Parallel()

	h := watching(t, managed("multi-id", "Multi", moviesRoot, extraRoot))
	h.watcher.waitForAttempt(t, extraRoot)
	h.watcher.emit(t, moviesRoot)

	h.libraries.set()
	h.removed(t, "multi-id")
	h.watcher.waitUntilInactive(t, moviesRoot)
	h.watcher.waitUntilInactive(t, extraRoot)

	h.clock.Advance(t, scantrigger.MaxDelay)
	h.expectStarts(t, "multi-id", 0, "after the library was removed during its debounce")
}

func TestRemovedLibraryDuringARunningScanSchedulesNoExtraScan(t *testing.T) {
	t.Parallel()

	h := watching(t, movies())
	h.scanner.setStatus(movies().ID, scan.Status{State: scan.StateRunning, StartedAt: startTime()})
	h.watcher.emit(t, moviesRoot)
	h.clock.Advance(t, scantrigger.QuietPeriod)

	h.libraries.set()
	h.removed(t, movies().ID)

	h.scanner.finish(movies().ID)
	h.clock.Advance(t, scantrigger.MaxDelay)
	h.expectStarts(t, movies().ID, 0, "after the running scan finished")
}

func TestRemovedLibraryLeavesThePeriodicCheck(t *testing.T) {
	t.Parallel()

	h := newHarness(t, scantrigger.Settings{Interval: defaultInterval}, movies())
	h.run(t)

	h.libraries.set()
	h.removed(t, movies().ID)

	h.clock.Advance(t, 2*defaultInterval)
	h.expectStarts(t, movies().ID, 0, "after the library was removed")
}

func TestEventsForUnknownLibrariesAreIgnored(t *testing.T) {
	t.Parallel()

	h := watching(t, movies())

	h.added(t, "ghost")
	h.changed(t, "ghost")
	h.removed(t, "ghost")

	if got := h.logs.count(ignoredEvent); got != 3 {
		t.Errorf("%d ignored events logged, want 3; log:\n%s", got, h.logs)
	}

	if got := h.watcher.activeWatches(); got != 1 {
		t.Errorf("%d active watches, want only the known library's", got)
	}
}

// A failing folder is warned about once however many checks retry it; the healthy folder of
// the same library keeps its watch.
func TestFolderWatchFailureIsLoggedOncePerFolder(t *testing.T) {
	t.Parallel()

	h := newHarness(t, scantrigger.Settings{Watch: true, Interval: defaultInterval},
		managed("multi-id", "Multi", moviesRoot, extraRoot))
	h.watcher.fail(extraRoot, errFake)
	h.run(t)
	h.watcher.waitForAttempt(t, moviesRoot)
	h.watcher.waitForAttempt(t, extraRoot)
	h.logs.waitFor(t, folderWarning)

	h.checkUntilWatched(t, extraRoot)
	h.checkUntilWatched(t, extraRoot)

	if got := h.logs.count("level=WARN msg=\"" + folderWarning); got != 1 {
		t.Errorf("%d folder warnings, want 1; log:\n%s", got, h.logs)
	}

	if got := h.logs.count("folder=" + extraRoot); got < 1 {
		t.Errorf("the warning does not name the folder; log:\n%s", h.logs)
	}

	h.watcher.emit(t, moviesRoot)
}

// Many add and remove events leave no watch behind, and Run still ends them all.
func TestManyLibraryEventsLeaveNoWatchRunning(t *testing.T) {
	t.Parallel()

	h := watching(t, movies())

	for range 25 {
		h.libraries.set(movies(), shows())
		h.added(t, shows().ID)
		h.watcher.waitForAttempt(t, showsRoot)

		h.libraries.set(movies())
		h.removed(t, shows().ID)
		h.watcher.waitUntilInactive(t, showsRoot)
	}

	if got := h.watcher.activeWatches(); got != 1 {
		t.Errorf("%d active watches, want 1", got)
	}

	h.stop(t)

	if got := h.watcher.activeWatches(); got != 0 {
		t.Errorf("%d watches running after Run returned", got)
	}
}

// Suspend keeps a due scan back until the library change arrives, then the scan runs on the
// new definition.
func TestSuspendedLibraryStartsNoScanUntilTheChangeArrives(t *testing.T) {
	t.Parallel()

	h := watching(t, movies())
	h.watcher.emit(t, moviesRoot)

	h.trigger.Suspend(movies().ID)
	h.clock.Advance(t, scantrigger.QuietPeriod)
	h.clock.Advance(t, scantrigger.EvaluationInterval)
	h.expectStarts(t, movies().ID, 0, "while suspended")

	h.changed(t, movies().ID)
	h.clock.Advance(t, scantrigger.EvaluationInterval)
	h.expectStarts(t, movies().ID, 1, "after the library change")
}

func TestSuspendEndsAfterTheHoldTimeout(t *testing.T) {
	t.Parallel()

	h := watching(t, movies())
	h.trigger.Suspend(movies().ID)
	h.watcher.emit(t, moviesRoot)

	h.clock.Advance(t, scantrigger.HoldTimeout-time.Second)
	h.expectStarts(t, movies().ID, 0, "within the hold timeout")

	h.clock.Advance(t, time.Second)
	h.expectStarts(t, movies().ID, 1, "after the hold timeout")
}

func TestSuspendedLibraryIsNotScannedPeriodically(t *testing.T) {
	t.Parallel()

	h := newHarness(t, scantrigger.Settings{Interval: time.Minute}, movies())
	h.run(t)
	h.trigger.Suspend(movies().ID)
	h.trigger.Suspend("ghost")

	h.clock.Advance(t, time.Minute)
	h.expectStarts(t, movies().ID, 0, "while suspended")
}

func TestLibraryEventsWithBothPartsOffAreDropped(t *testing.T) {
	t.Parallel()

	h := newHarness(t, scantrigger.Settings{}, movies())
	h.run(t)

	h.trigger.Added(movies().ID)
	h.trigger.Changed(movies().ID)
	h.trigger.Removed(movies().ID)
	h.watcher.expectNoAttempt(t)
}
