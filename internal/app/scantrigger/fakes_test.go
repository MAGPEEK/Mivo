package scantrigger_test

import (
	"bytes"
	"context"
	"fmt"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/magpeek/mivo/internal/app/scan"
	"github.com/magpeek/mivo/internal/app/scantrigger"
	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/library"
)

// waitTimeout bounds every wait for the trigger's goroutines; the fakes answer at once.
const waitTimeout = 5 * time.Second

// fakeClock is a manual clock with a single ticker channel. Advance moves the time, sends
// one tick and waits until the trigger has handled it.
type fakeClock struct {
	mutex     sync.Mutex
	now       time.Time
	intervals []time.Duration
	stopped   bool
	ticks     chan time.Time
	handled   chan struct{}
}

func newFakeClock(start time.Time) *fakeClock {
	return &fakeClock{now: start, ticks: make(chan time.Time), handled: make(chan struct{})}
}

func (clock *fakeClock) Now() time.Time {
	clock.mutex.Lock()
	defer clock.mutex.Unlock()

	return clock.now
}

func (clock *fakeClock) NewTicker(interval time.Duration) scantrigger.Ticker {
	clock.mutex.Lock()
	defer clock.mutex.Unlock()

	clock.intervals = append(clock.intervals, interval)

	return fakeTicker{clock: clock}
}

func (clock *fakeClock) tickerIntervals() []time.Duration {
	clock.mutex.Lock()
	defer clock.mutex.Unlock()

	return slices.Clone(clock.intervals)
}

func (clock *fakeClock) isStopped() bool {
	clock.mutex.Lock()
	defer clock.mutex.Unlock()

	return clock.stopped
}

// tickHandled is the trigger's after-tick hook.
func (clock *fakeClock) tickHandled() {
	clock.handled <- struct{}{}
}

func (clock *fakeClock) Advance(t *testing.T, duration time.Duration) {
	t.Helper()

	clock.mutex.Lock()
	clock.now = clock.now.Add(duration)
	now := clock.now
	clock.mutex.Unlock()

	select {
	case clock.ticks <- now:
	case <-time.After(waitTimeout):
		t.Fatal("the trigger did not take the tick")
	}

	select {
	case <-clock.handled:
	case <-time.After(waitTimeout):
		t.Fatal("the trigger did not finish the tick")
	}
}

type fakeTicker struct {
	clock *fakeClock
}

func (ticker fakeTicker) Ticks() <-chan time.Time { return ticker.clock.ticks }

func (ticker fakeTicker) Stop() {
	ticker.clock.mutex.Lock()
	defer ticker.clock.mutex.Unlock()

	ticker.clock.stopped = true
}

// memoryLibraries is an in-memory library list.
type memoryLibraries struct {
	mutex     sync.Mutex
	libraries []library.ManagedLibrary
	listErr   error
}

func (source *memoryLibraries) ListLibraries(context.Context) ([]library.ManagedLibrary, error) {
	source.mutex.Lock()
	defer source.mutex.Unlock()

	if source.listErr != nil {
		return nil, source.listErr
	}

	return slices.Clone(source.libraries), nil
}

func (source *memoryLibraries) GetLibrary(_ context.Context, libraryID string) (library.ManagedLibrary, error) {
	source.mutex.Lock()
	defer source.mutex.Unlock()

	for _, entry := range source.libraries {
		if entry.ID == libraryID {
			return entry, nil
		}
	}

	return library.ManagedLibrary{}, fmt.Errorf("library %s: %w", libraryID, domain.ErrNotFound)
}

func (source *memoryLibraries) set(libraries ...library.ManagedLibrary) {
	source.mutex.Lock()
	defer source.mutex.Unlock()

	source.libraries = libraries
}

func (source *memoryLibraries) failWith(err error) {
	source.mutex.Lock()
	defer source.mutex.Unlock()

	source.listErr = err
}

// fakeScanner records Start calls; a started scan runs until the test finishes it.
type fakeScanner struct {
	mutex    sync.Mutex
	clock    *fakeClock
	statuses map[string]scan.Status
	starts   map[string]int
	startErr error
}

func newFakeScanner(clock *fakeClock) *fakeScanner {
	return &fakeScanner{clock: clock, statuses: map[string]scan.Status{}, starts: map[string]int{}}
}

func (scanner *fakeScanner) Start(_ context.Context, libraryID string) (scan.Status, error) {
	scanner.mutex.Lock()
	defer scanner.mutex.Unlock()

	if scanner.startErr != nil {
		return scan.Status{}, scanner.startErr
	}

	status := scanner.statuses[libraryID]
	if status.State == scan.StateRunning {
		return status, nil
	}

	scanner.starts[libraryID]++
	status = scan.Status{State: scan.StateRunning, StartedAt: scanner.clock.Now()}
	scanner.statuses[libraryID] = status

	return status, nil
}

func (scanner *fakeScanner) Status(libraryID string) scan.Status {
	scanner.mutex.Lock()
	defer scanner.mutex.Unlock()

	status, ok := scanner.statuses[libraryID]
	if !ok {
		return scan.Status{State: scan.StateIdle}
	}

	return status
}

func (scanner *fakeScanner) setStatus(libraryID string, status scan.Status) {
	scanner.mutex.Lock()
	defer scanner.mutex.Unlock()

	scanner.statuses[libraryID] = status
}

// finish ends the running scan of the library now.
func (scanner *fakeScanner) finish(libraryID string) {
	scanner.mutex.Lock()
	defer scanner.mutex.Unlock()

	status := scanner.statuses[libraryID]
	status.State = scan.StateIdle
	status.FinishedAt = scanner.clock.Now()
	scanner.statuses[libraryID] = status
}

func (scanner *fakeScanner) startCount(libraryID string) int {
	scanner.mutex.Lock()
	defer scanner.mutex.Unlock()

	return scanner.starts[libraryID]
}

func (scanner *fakeScanner) failStarts(err error) {
	scanner.mutex.Lock()
	defer scanner.mutex.Unlock()

	scanner.startErr = err
}

// fakeWatcher keeps the callbacks of every watched root until the watch ends. Roots in
// failures fail at once.
type fakeWatcher struct {
	mutex     sync.Mutex
	callbacks map[string]scantrigger.WatchCallbacks
	failures  map[string]error
	active    int
	// attempts counts the Watch calls per root, seen how many of them a test has waited for.
	attempts map[string]int
	seen     map[string]int
}

func newFakeWatcher() *fakeWatcher {
	return &fakeWatcher{
		callbacks: map[string]scantrigger.WatchCallbacks{}, failures: map[string]error{},
		attempts: map[string]int{}, seen: map[string]int{},
	}
}

// Watch counts the attempt only after Ready has returned, so a test that waited for the
// attempt sees the effects of Ready.
func (watcher *fakeWatcher) Watch(ctx context.Context, root string, callbacks scantrigger.WatchCallbacks) error {
	watcher.mutex.Lock()
	failure := watcher.failures[root]

	if failure == nil {
		watcher.callbacks[root] = callbacks
		watcher.active++
	}
	watcher.mutex.Unlock()

	if failure == nil {
		callbacks.Ready()
	}

	watcher.mutex.Lock()
	watcher.attempts[root]++
	watcher.mutex.Unlock()

	if failure != nil {
		return failure
	}

	<-ctx.Done()

	watcher.mutex.Lock()
	defer watcher.mutex.Unlock()

	delete(watcher.callbacks, root)
	watcher.active--

	return nil
}

func (watcher *fakeWatcher) fail(root string, err error) {
	watcher.mutex.Lock()
	defer watcher.mutex.Unlock()

	watcher.failures[root] = err
}

// waitForAttempt waits for the next Watch call for root that no test has waited for yet.
func (watcher *fakeWatcher) waitForAttempt(t *testing.T, root string) {
	t.Helper()

	if !watcher.awaitAttempt(root, waitTimeout) {
		t.Fatalf("Watch(%s) was not called", root)
	}
}

// receivedAttempt is waitForAttempt with a short timeout and without failing.
func (watcher *fakeWatcher) receivedAttempt(root string) bool {
	return watcher.awaitAttempt(root, attemptPollTimeout)
}

func (watcher *fakeWatcher) awaitAttempt(root string, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		watcher.mutex.Lock()
		if watcher.attempts[root] > watcher.seen[root] {
			watcher.seen[root]++
			watcher.mutex.Unlock()

			return true
		}
		watcher.mutex.Unlock()

		time.Sleep(time.Millisecond)
	}

	return false
}

func (watcher *fakeWatcher) expectNoAttempt(t *testing.T) {
	t.Helper()

	watcher.mutex.Lock()
	defer watcher.mutex.Unlock()

	if len(watcher.attempts) != 0 {
		t.Fatalf("Watch was called for %v, want no call", watcher.attempts)
	}
}

// emit reports a change below root through the callback of its running watch.
func (watcher *fakeWatcher) emit(t *testing.T, root string) {
	t.Helper()

	watcher.running(t, root).Changed()
}

// degrade reports a partial failure through the callbacks of the running watch of root.
func (watcher *fakeWatcher) degrade(t *testing.T, root string, err error) {
	t.Helper()

	watcher.running(t, root).Degraded(err)
}

func (watcher *fakeWatcher) running(t *testing.T, root string) scantrigger.WatchCallbacks {
	t.Helper()

	watcher.mutex.Lock()
	callbacks, ok := watcher.callbacks[root]
	watcher.mutex.Unlock()

	if !ok {
		t.Fatalf("no running watch of %s", root)
	}

	return callbacks
}

func (watcher *fakeWatcher) activeWatches() int {
	watcher.mutex.Lock()
	defer watcher.mutex.Unlock()

	return watcher.active
}

// waitUntilInactive waits for a cancelled watch of root to end.
func (watcher *fakeWatcher) waitUntilInactive(t *testing.T, root string) {
	t.Helper()

	deadline := time.Now().Add(waitTimeout)

	for time.Now().Before(deadline) {
		watcher.mutex.Lock()
		_, running := watcher.callbacks[root]
		watcher.mutex.Unlock()

		if !running {
			return
		}

		time.Sleep(time.Millisecond)
	}

	t.Fatalf("the watch of %s did not end", root)
}

// logBuffer collects log output from several goroutines.
type logBuffer struct {
	mutex  sync.Mutex
	buffer bytes.Buffer
}

func (logs *logBuffer) Write(data []byte) (int, error) {
	logs.mutex.Lock()
	defer logs.mutex.Unlock()

	written, err := logs.buffer.Write(data)
	if err != nil {
		return written, fmt.Errorf("write log: %w", err)
	}

	return written, nil
}

func (logs *logBuffer) count(text string) int {
	logs.mutex.Lock()
	defer logs.mutex.Unlock()

	return strings.Count(logs.buffer.String(), text)
}

func (logs *logBuffer) String() string {
	logs.mutex.Lock()
	defer logs.mutex.Unlock()

	return logs.buffer.String()
}

// waitFor waits until text was logged; lines of a failed watch are written by its goroutine.
func (logs *logBuffer) waitFor(t *testing.T, text string) {
	t.Helper()

	deadline := time.Now().Add(waitTimeout)

	for logs.count(text) == 0 {
		if time.Now().After(deadline) {
			t.Fatalf("%q was not logged; log:\n%s", text, logs)
		}

		time.Sleep(time.Millisecond)
	}
}

// attemptPollTimeout is how long one check waits for a retried watch.
const attemptPollTimeout = 50 * time.Millisecond

// errFake is a failure of a fake port.
var errFake = fmt.Errorf("fake failure: %w", domain.ErrUnavailable)
