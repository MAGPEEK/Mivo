package library_test

import (
	"context"
	"testing"
	"time"

	applibrary "github.com/magpeek/mivo/internal/app/library"
	"github.com/magpeek/mivo/internal/domain/library"
)

// Timeouts of the folder check tests: the short one keeps hung-check tests fast; the long
// one is far above any check that answers, so a loaded machine running -race cannot make a
// check look timed out.
const (
	testRootCheckTimeout     = 20 * time.Millisecond
	generousRootCheckTimeout = time.Minute
)

// availabilityWaitLimit bounds how long a test waits for the catalog to answer while a check
// hangs; far above testRootCheckTimeout, so it only fires when the timeout is broken.
const availabilityWaitLimit = 5 * time.Second

// newCheckPollInterval spaces the calls of waitForNewCheck, so polling does not spin a CPU
// while the released check unregisters itself.
const newCheckPollInterval = 5 * time.Millisecond

func moviesAndShows() []library.ManagedLibrary {
	return []library.ManagedLibrary{
		{ID: moviesID, Name: "Movies", Kind: library.KindMovies, Folders: []library.Folder{{ID: "m1", Path: moviesRoot}}},
		{ID: showsID, Name: "Shows", Kind: library.KindShows, Folders: []library.Folder{{ID: "s1", Path: showsRoot}}},
	}
}

func catalogWithChecker(t *testing.T, checker applibrary.RootChecker, timeout time.Duration) *applibrary.ManagedCatalog {
	t.Helper()

	catalog, err := applibrary.NewManagedCatalog(applibrary.ManagedCatalogDependencies{
		Libraries:        newMemoryManaged(&callLog{}, moviesAndShows()...),
		Counts:           fakeCounts{},
		Roots:            checker,
		RootCheckTimeout: timeout,
	})
	if err != nil {
		t.Fatalf("NewManagedCatalog error = %v", err)
	}

	return catalog
}

func availability(overviews []applibrary.ManagedOverview) map[string]bool {
	result := map[string]bool{}
	for _, overview := range overviews {
		result[overview.Library.ID] = overview.Available
	}

	return result
}

// A stat on a hung network mount may ignore cancellation. The list must still answer after
// the check timeout, with the hung library unavailable and the others checked normally.
func TestListLibrariesDoesNotWaitForAHungRootCheck(t *testing.T) {
	t.Parallel()

	checker := newHungRootChecker(showsRoot)
	t.Cleanup(checker.releaseChecks)

	overviews := listWithin(t, catalogWithChecker(t, checker, testRootCheckTimeout), availabilityWaitLimit)

	got := availability(overviews)
	if !got[moviesID] || got[showsID] {
		t.Errorf("availability = %v, want movies available and shows unavailable", got)
	}
}

// Every list while a check hangs joins the running check instead of starting another one,
// so a client polling the list cannot pile up blocked goroutines. Once the hung check
// returns, the next list checks the folder again.
func TestHungRootChecksAreJoinedAndRetriedAfterTheyFinish(t *testing.T) {
	t.Parallel()

	checker := newHungRootChecker(showsRoot)
	checker.availableAfterRelease = true
	t.Cleanup(checker.releaseChecks)

	catalog := catalogWithChecker(t, checker, testRootCheckTimeout)

	for range 3 {
		listWithin(t, catalog, availabilityWaitLimit)
	}

	if checks := checker.checkCount(showsRoot); checks != 1 {
		t.Errorf("checks of the hung folder = %d, want 1 (later lists join the running check)", checks)
	}

	checker.releaseChecks()
	waitForNewCheck(t, catalog, checker)
}

// waitForNewCheck polls until the checker sees a second check of the shows folder: the
// released check unregisters itself asynchronously, so the first calls may still join it.
func waitForNewCheck(t *testing.T, catalog *applibrary.ManagedCatalog, checker *hungRootChecker) {
	t.Helper()

	deadline := time.Now().Add(availabilityWaitLimit)

	for time.Now().Before(deadline) {
		overview, err := catalog.GetLibrary(t.Context(), showsID)
		if err != nil {
			t.Fatalf("GetLibrary error = %v", err)
		}

		if checker.checkCount(showsRoot) >= 2 && overview.Available {
			return
		}

		time.Sleep(newCheckPollInterval)
	}

	t.Fatalf("no new, successful check within %v after the hung check returned (checks: %d)",
		availabilityWaitLimit, checker.checkCount(showsRoot))
}

// The checks of all folders run at the same time, so several hung mounts cost one timeout.
// The movies check only succeeds once the shows check has started; checked one after the
// other it would time out.
func TestListLibrariesChecksAllRootsConcurrently(t *testing.T) {
	t.Parallel()

	showsStarted := make(chan struct{})
	checker := rootCheckerFunc(func(ctx context.Context, root string) bool {
		if root == showsRoot {
			close(showsStarted)

			return true
		}

		select {
		case <-showsStarted:
			return true
		case <-ctx.Done():
			return false
		}
	})

	overviews := listWithin(t, catalogWithChecker(t, checker, availabilityWaitLimit), 2*availabilityWaitLimit)

	if got := availability(overviews); !got[moviesID] || !got[showsID] {
		t.Errorf("availability = %v, want both available (checks must run concurrently)", got)
	}
}

// A cooperating checker gets a deadline, so it can stop by itself at the timeout.
func TestRootChecksCarryTheTimeoutAsDeadline(t *testing.T) {
	t.Parallel()

	deadlines := make(chan time.Duration, 2)
	checker := rootCheckerFunc(func(ctx context.Context, _ string) bool {
		deadline, hasDeadline := ctx.Deadline()
		if !hasDeadline {
			deadlines <- 0

			return true
		}

		deadlines <- time.Until(deadline)

		return true
	})

	if _, err := catalogWithChecker(t, checker, generousRootCheckTimeout).GetLibrary(t.Context(), moviesID); err != nil {
		t.Fatalf("GetLibrary error = %v", err)
	}

	remaining := <-deadlines
	if remaining <= 0 || remaining > generousRootCheckTimeout {
		t.Errorf("check deadline in %v, want within the timeout of %v", remaining, generousRootCheckTimeout)
	}
}

// listWithin calls ListLibraries and fails the test when it does not answer within limit.
func listWithin(t *testing.T, catalog *applibrary.ManagedCatalog, limit time.Duration) []applibrary.ManagedOverview {
	t.Helper()

	type outcome struct {
		overviews []applibrary.ManagedOverview
		err       error
	}

	done := make(chan outcome, 1)

	go func() {
		overviews, err := catalog.ListLibraries(t.Context())
		done <- outcome{overviews: overviews, err: err}
	}()

	select {
	case result := <-done:
		if result.err != nil {
			t.Fatalf("ListLibraries error = %v", result.err)
		}

		return result.overviews
	case <-time.After(limit):
		t.Fatalf("ListLibraries did not answer within %v", limit)

		return nil
	}
}
