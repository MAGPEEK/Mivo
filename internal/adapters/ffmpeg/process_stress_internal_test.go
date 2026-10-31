package ffmpeg

import (
	"os"
	"runtime"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
)

const (
	// stressStartsVariable raises the number of starts of TestManyShortLivedStartsSucceed,
	// for example to 5000 before a release or after touching the lifetime guard.
	stressStartsVariable = "MIVO_FFMPEG_STRESS_STARTS"
	// windowsStressStarts keeps the normal suite at about two seconds on Windows while still
	// starting a thousand tools that exit at once, the case that raced the job object
	// assignment (M2-24b test report F1, about 1 in 5000 starts).
	windowsStressStarts = 1000
	// otherStressStarts is the default elsewhere, where no such race exists: it still
	// exercises the start and wait on one locked thread under load, while a race-detector run
	// in a container, about 60 ms per start, stays at seconds.
	otherStressStarts = 100
	// stressConcurrency loads the machine like a scan plus several sessions; the race showed
	// up under load.
	stressConcurrency = 16
)

// TestManyShortLivedStartsSucceed starts many tools that exit immediately, like ffprobe on a
// small file. No start may fail: the lifetime guard has to cope with a tool that ends right
// after it was created (lifetime_windows.go, isTerminating).
func TestManyShortLivedStartsSucceed(t *testing.T) {
	starts := stressStarts(t)
	fake := fakeTool(t, fakeModeRespond)

	var (
		group    sync.WaitGroup
		next     atomic.Int64
		failures atomic.Int64
		first    sync.Once
	)

	for range stressConcurrency {
		group.Go(func() {
			for next.Add(1) <= int64(starts) {
				if _, err := fake.output(t.Context(), nil); err != nil {
					failures.Add(1)
					first.Do(func() { t.Errorf("first failed start: %v", err) })
				}
			}
		})
	}

	group.Wait()

	if count := failures.Load(); count > 0 {
		t.Errorf("%d of %d starts failed", count, starts)
	}
}

func stressStarts(t *testing.T) int {
	t.Helper()

	text := os.Getenv(stressStartsVariable)
	if text == "" && runtime.GOOS == "windows" {
		return windowsStressStarts
	}

	if text == "" {
		return otherStressStarts
	}

	starts, err := strconv.Atoi(text)
	if err != nil || starts <= 0 {
		t.Fatalf("%s=%q is not a positive number", stressStartsVariable, text)
	}

	return starts
}
