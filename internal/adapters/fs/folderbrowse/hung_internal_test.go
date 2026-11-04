package folderbrowse

import (
	"errors"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/magpeek/mivo/internal/domain"
)

const (
	hungTimeout  = 30 * time.Millisecond
	hungCalls    = 25
	hungSlack    = 2 * time.Second
	goroutineMax = 8
)

// hungBrowser returns a Browser whose root stat blocks until the cleanup, and a counter of
// started stats.
func hungBrowser(t *testing.T) (*Browser, string, *atomic.Int32) {
	t.Helper()

	root := t.TempDir()
	release := make(chan struct{})
	started := &atomic.Int32{}
	browser := New([]string{root}, Options{Timeout: hungTimeout})
	browser.statRoot = func(string) bool {
		started.Add(1)
		<-release

		return true
	}

	t.Cleanup(func() { close(release) })

	return browser, RootID(root), started
}

func TestHungRootBoundsGoroutinesAndTime(t *testing.T) {
	browser, id, started := hungBrowser(t)
	before := runtime.NumGoroutine()
	begin := time.Now()

	var group sync.WaitGroup

	for range hungCalls {
		group.Go(func() {
			if _, err := browser.List(t.Context(), id, ""); !errors.Is(err, domain.ErrUnavailable) {
				t.Errorf("List err = %v", err)
			}
		})
		group.Go(func() {
			if _, err := browser.Check(t.Context(), id, ""); !errors.Is(err, domain.ErrUnavailable) {
				t.Errorf("Check err = %v", err)
			}
		})
		group.Go(func() {
			if roots := browser.Roots(t.Context()); roots[0].Available {
				t.Error("hung root reported available")
			}
		})
	}

	group.Wait()

	if elapsed := time.Since(begin); elapsed > hungSlack {
		t.Errorf("calls took %s, want about the timeout", elapsed)
	}

	if got := started.Load(); got != 1 {
		t.Errorf("%d blocked stats started, want 1", got)
	}

	if delta := runtime.NumGoroutine() - before; delta > goroutineMax {
		t.Errorf("%d goroutines left behind, want at most %d", delta, goroutineMax)
	}
}

func TestOptionDefaults(t *testing.T) {
	t.Parallel()

	browser := New(nil, Options{})
	if browser.timeout != DefaultTimeout || browser.capacity != DefaultListingCap {
		t.Errorf("defaults = %s, %d", browser.timeout, browser.capacity)
	}
}
