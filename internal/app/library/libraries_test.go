package library_test

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	applibrary "github.com/magpeek/mivo/internal/app/library"
	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/library"
)

// removalLogMessage is the warn message logged for every library a sync removed.
const removalLogMessage = "library removed from the catalog because its folder is no longer configured"

// availabilityWaitLimit bounds how long a test waits for the service to answer while a root
// check hangs; far above testRootCheckTimeout, so it only fires when the timeout is broken.
const availabilityWaitLimit = 5 * time.Second

// newCheckPollInterval spaces the GetLibrary calls of waitForNewCheck, so polling does not
// spin a CPU while the released check unregisters itself.
const newCheckPollInterval = 5 * time.Millisecond

func TestSyncLibrariesStoresDefinitionsWithGeneratedIDs(t *testing.T) {
	t.Parallel()

	environment := newTestEnvironment()

	synced, err := environment.service(t).SyncLibraries(t.Context(), []library.Definition{
		{Name: "Shows", Root: showsRoot, Kind: library.KindShows},
		{Name: "Movies", Root: moviesRoot, Kind: library.KindMovies},
	})
	if err != nil {
		t.Fatalf("SyncLibraries error = %v", err)
	}

	want := []library.Library{
		{ID: secondGeneratedID, Name: "Movies", Root: moviesRoot, Kind: library.KindMovies},
		{ID: firstGeneratedID, Name: "Shows", Root: showsRoot, Kind: library.KindShows},
	}
	if !slices.Equal(synced, want) {
		t.Errorf("SyncLibraries = %+v, want %+v", synced, want)
	}

	if records := environment.logs.recorded(); len(records) != 0 {
		t.Errorf("logged %d records for a sync that removed nothing, want none", len(records))
	}
}

func TestSyncLibrariesRejectsAnInvalidDefinitionByIndex(t *testing.T) {
	t.Parallel()

	valid := library.Definition{Name: "Movies", Root: moviesRoot, Kind: library.KindMovies}

	tests := []struct {
		name     string
		invalid  library.Definition
		wantText string
	}{
		{
			name:     "blank name",
			invalid:  library.Definition{Name: " ", Root: showsRoot, Kind: library.KindShows},
			wantText: "sync libraries: definition 1: ",
		},
		{
			name:     "empty root",
			invalid:  library.Definition{Name: "Shows", Kind: library.KindShows},
			wantText: "sync libraries: definition 1: ",
		},
		{
			name:     "unknown kind",
			invalid:  library.Definition{Name: "Music", Root: showsRoot, Kind: "music"},
			wantText: "sync libraries: definition 1: ",
		},
		{
			name:     "root configured twice",
			invalid:  library.Definition{Name: "Films", Root: moviesRoot, Kind: library.KindShows},
			wantText: "definition 1: root \"/media/movies\" is already used by definition 0",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			environment := newTestEnvironment()

			synced, err := environment.service(t).SyncLibraries(t.Context(),
				[]library.Definition{valid, test.invalid})
			assertWrappedError(t, err, domain.ErrInvalid, test.wantText)

			if synced != nil {
				t.Errorf("SyncLibraries = %+v on error, want nil", synced)
			}

			if calls := environment.store.syncs(); calls != 0 {
				t.Errorf("store sync calls = %d, want 0 for invalid input", calls)
			}
		})
	}
}

func TestSyncLibrariesWrapsStoreAndGeneratorErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		configure func(*testEnvironment)
	}{
		{name: "store fails", configure: func(environment *testEnvironment) { environment.store.syncErr = errBoom }},
		{name: "id generator fails", configure: func(environment *testEnvironment) { environment.generator.err = errBoom }},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			environment := newTestEnvironment()
			test.configure(environment)

			synced, err := environment.service(t).SyncLibraries(t.Context(), []library.Definition{
				{Name: "Movies", Root: moviesRoot, Kind: library.KindMovies},
			})
			assertWrappedError(t, err, errBoom, "library store: ")

			if synced != nil {
				t.Errorf("SyncLibraries = %+v on error, want nil", synced)
			}
		})
	}
}

// Removing a library deletes its catalog, so it must leave a trace: a typo in the
// configuration or an empty library variable would otherwise empty the catalog silently.
func TestSyncLibrariesLogsEveryRemovedLibraryAtWarnLevel(t *testing.T) {
	t.Parallel()

	environment := newTestEnvironment()
	movies, shows := environment.storedMoviesAndShows()

	synced, err := environment.service(t).SyncLibraries(t.Context(), nil)
	if err != nil {
		t.Fatalf("SyncLibraries error = %v", err)
	}

	if len(synced) != 0 {
		t.Errorf("SyncLibraries = %+v, want no libraries", synced)
	}

	records := environment.logs.recorded()
	if len(records) != 2 {
		t.Fatalf("logged %d records, want one per removed library (2)", len(records))
	}

	for index, removed := range []library.Summary{movies, shows} {
		assertRemovalRecord(t, records[index], removed)
	}
}

func assertRemovalRecord(t *testing.T, record slog.Record, removed library.Summary) {
	t.Helper()

	if record.Level != slog.LevelWarn || record.Message != removalLogMessage {
		t.Errorf("record = %v %q, want WARN %q", record.Level, record.Message, removalLogMessage)
	}

	want := map[string]string{
		"library_id": removed.Library.ID,
		"name":       removed.Library.Name,
		"root":       removed.Library.Root,
		"item_count": strconv.FormatInt(removed.ItemCount, 10),
	}
	if got := attributes(record); !mapsEqual(got, want) {
		t.Errorf("record attributes = %v, want %v", got, want)
	}
}

func mapsEqual(got, want map[string]string) bool {
	if len(got) != len(want) {
		return false
	}

	for key, value := range want {
		if got[key] != value {
			return false
		}
	}

	return true
}

func TestListLibrariesMarksUnreachableRootsUnavailable(t *testing.T) {
	t.Parallel()

	environment := newTestEnvironment()
	environment.roots = availableRoots(moviesRoot)
	movies, shows := environment.storedMoviesAndShows()

	overviews, err := environment.service(t).ListLibraries(t.Context())
	if err != nil {
		t.Fatalf("ListLibraries error = %v", err)
	}

	want := []applibrary.Overview{
		{Summary: movies, Available: true},
		{Summary: shows, Available: false},
	}
	if !slices.Equal(overviews, want) {
		t.Errorf("ListLibraries = %+v, want %+v", overviews, want)
	}
}

func TestListLibrariesOfAnEmptyCatalogIsAnEmptyList(t *testing.T) {
	t.Parallel()

	overviews, err := newTestEnvironment().service(t).ListLibraries(t.Context())
	if err != nil || overviews == nil || len(overviews) != 0 {
		t.Errorf("ListLibraries = %#v, %v, want an empty, non-nil list", overviews, err)
	}
}

func TestListLibrariesWrapsStoreErrors(t *testing.T) {
	t.Parallel()

	environment := newTestEnvironment()
	environment.store.listErr = errBoom

	overviews, err := environment.service(t).ListLibraries(t.Context())
	assertWrappedError(t, err, errBoom, "library store: ")

	if overviews != nil {
		t.Errorf("ListLibraries = %+v on error, want nil", overviews)
	}
}

// errUnreadableKind is what the store reports for a stored row the domain cannot read.
var errUnreadableKind = fmt.Errorf("library %q: stored kind: %w", moviesID, domain.ErrInvalid)

// A stored row the domain cannot read is a server fault: every read path reports it without
// domain.ErrInvalid, which the HTTP adapter would answer with 400 "bad request".
func TestStoreReadsHideErrInvalidOfBrokenStoredData(t *testing.T) {
	t.Parallel()

	reads := map[string]func(*applibrary.Service, context.Context) error{
		"ListLibraries": func(service *applibrary.Service, ctx context.Context) error {
			_, err := service.ListLibraries(ctx)

			return err
		},
		"GetLibrary": func(service *applibrary.Service, ctx context.Context) error {
			_, err := service.GetLibrary(ctx, moviesID)

			return err
		},
		"ListItems": func(service *applibrary.Service, ctx context.Context) error {
			_, err := service.ListItems(ctx, moviesID, library.PageRequest{})

			return err
		},
	}

	for name, read := range reads {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			environment := newTestEnvironment()
			environment.store.listErr, environment.store.getErr = errUnreadableKind, errUnreadableKind

			err := read(environment.service(t), t.Context())
			if err == nil || errors.Is(err, domain.ErrInvalid) {
				t.Fatalf("%s error = %v, want an error that does not wrap domain.ErrInvalid", name, err)
			}

			if !strings.Contains(err.Error(), "unreadable stored library data: "+errUnreadableKind.Error()) {
				t.Errorf("%s error = %q, want the store's message kept for the log", name, err)
			}
		})
	}
}

// A caller that gave up gets its context error instead of a list in which every library
// looks unavailable.
func TestListLibrariesReturnsTheContextErrorOfACancelledCaller(t *testing.T) {
	t.Parallel()

	environment := newTestEnvironment()
	environment.storedMoviesAndShows()

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	_, err := environment.service(t).ListLibraries(ctx)
	assertWrappedError(t, err, context.Canceled, "list libraries")
}

func TestGetLibraryReturnsTheLibraryWithItsAvailability(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		roots     applibrary.RootChecker
		available bool
	}{
		{name: "reachable root", roots: availableRoots(moviesRoot), available: true},
		{name: "unreachable root", roots: availableRoots(), available: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			environment := newTestEnvironment()
			environment.roots = test.roots
			movies, _ := environment.storedMoviesAndShows()

			overview, err := environment.service(t).GetLibrary(t.Context(), moviesID)
			if err != nil {
				t.Fatalf("GetLibrary error = %v", err)
			}

			if want := (applibrary.Overview{Summary: movies, Available: test.available}); overview != want {
				t.Errorf("GetLibrary = %+v, want %+v", overview, want)
			}
		})
	}
}

func TestGetLibraryFailures(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		configure func(*testEnvironment)
		lookupID  string
		ctx       func(t *testing.T) context.Context
		wantErr   error
		wantText  string
	}{
		{
			name:      "unknown id",
			configure: func(environment *testEnvironment) { environment.storedMoviesAndShows() },
			lookupID:  unknownID,
			wantErr:   domain.ErrNotFound,
			wantText:  "library store: ",
		},
		{
			name:      "store fails",
			configure: func(environment *testEnvironment) { environment.store.getErr = errBoom },
			lookupID:  moviesID,
			wantErr:   errBoom,
			wantText:  "library store: ",
		},
		{
			name:      "cancelled caller",
			configure: func(environment *testEnvironment) { environment.storedMoviesAndShows() },
			lookupID:  moviesID,
			ctx:       cancelledContext,
			wantErr:   context.Canceled,
			wantText:  "get library",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			environment := newTestEnvironment()
			test.configure(environment)

			ctx := t.Context()
			if test.ctx != nil {
				ctx = test.ctx(t)
			}

			overview, err := environment.service(t).GetLibrary(ctx, test.lookupID)
			assertWrappedError(t, err, test.wantErr, test.wantText)

			if overview != (applibrary.Overview{}) {
				t.Errorf("GetLibrary = %+v on error, want zero value", overview)
			}
		})
	}
}

func cancelledContext(t *testing.T) context.Context {
	t.Helper()

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	return ctx
}

// A stat on a hung network mount may ignore cancellation. The list must still answer after
// the root check timeout, with the hung library unavailable and the others checked normally.
func TestListLibrariesDoesNotWaitForAHungRootCheck(t *testing.T) {
	t.Parallel()

	environment := newTestEnvironment()
	environment.timeout = testRootCheckTimeout
	checker := newHungRootChecker(showsRoot)
	t.Cleanup(checker.releaseChecks)

	environment.roots = checker
	movies, shows := environment.storedMoviesAndShows()

	overviews := listWithin(t, environment.service(t), availabilityWaitLimit)

	want := []applibrary.Overview{{Summary: movies, Available: true}, {Summary: shows, Available: false}}
	if !slices.Equal(overviews, want) {
		t.Errorf("ListLibraries = %+v, want %+v", overviews, want)
	}
}

// Every list while a check hangs joins the running check instead of starting another one,
// so a client polling the list cannot pile up blocked goroutines (and OS threads). Once the
// hung check returns, the next list checks the root again.
func TestHungRootChecksAreJoinedAndRetriedAfterTheyFinish(t *testing.T) {
	t.Parallel()

	environment := newTestEnvironment()
	environment.timeout = testRootCheckTimeout
	checker := newHungRootChecker(showsRoot)
	checker.availableAfterRelease = true
	t.Cleanup(checker.releaseChecks)
	environment.roots = checker
	environment.storedMoviesAndShows()
	service := environment.service(t)

	for range 3 {
		listWithin(t, service, availabilityWaitLimit)
	}

	if checks := checker.checkCount(showsRoot); checks != 1 {
		t.Errorf("checks of the hung root = %d, want 1 (later lists join the running check)", checks)
	}

	checker.releaseChecks()
	waitForNewCheck(t, service, checker)
}

// waitForNewCheck polls GetLibrary of the shows library until the checker sees a second
// check of its root: the released check unregisters itself asynchronously, so the first
// calls after the release may still join it.
func waitForNewCheck(t *testing.T, service *applibrary.Service, checker *hungRootChecker) {
	t.Helper()

	deadline := time.Now().Add(availabilityWaitLimit)

	for time.Now().Before(deadline) {
		overview, err := service.GetLibrary(t.Context(), showsID)
		if err != nil {
			t.Fatalf("GetLibrary error = %v", err)
		}

		if checker.checkCount(showsRoot) >= 2 && overview.Available {
			return
		}

		time.Sleep(newCheckPollInterval)
	}

	t.Fatalf("no new, successful check of the root within %v after the hung check returned (checks: %d)",
		availabilityWaitLimit, checker.checkCount(showsRoot))
}

// The checks of all libraries run at the same time, so several hung mounts cost one timeout,
// not one each. The movies check only succeeds once the shows check has started; checked one
// after the other it would time out.
func TestListLibrariesChecksAllRootsConcurrently(t *testing.T) {
	t.Parallel()

	environment := newTestEnvironment()
	environment.timeout = availabilityWaitLimit
	showsStarted := make(chan struct{})
	environment.roots = rootCheckerFunc(func(ctx context.Context, root string) bool {
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
	movies, shows := environment.storedMoviesAndShows()

	overviews := listWithin(t, environment.service(t), 2*availabilityWaitLimit)

	want := []applibrary.Overview{{Summary: movies, Available: true}, {Summary: shows, Available: true}}
	if !slices.Equal(overviews, want) {
		t.Errorf("ListLibraries = %+v, want %+v (checks must run concurrently)", overviews, want)
	}
}

// A cooperating checker gets a deadline, so it can stop by itself at the timeout.
func TestRootChecksCarryTheTimeoutAsDeadline(t *testing.T) {
	t.Parallel()

	environment := newTestEnvironment()
	environment.timeout = time.Minute
	deadlines := make(chan time.Duration, 1)
	environment.roots = rootCheckerFunc(func(ctx context.Context, _ string) bool {
		deadline, hasDeadline := ctx.Deadline()
		if !hasDeadline {
			deadlines <- 0

			return true
		}

		deadlines <- time.Until(deadline)

		return true
	})
	environment.storedMoviesAndShows()

	if _, err := environment.service(t).GetLibrary(t.Context(), moviesID); err != nil {
		t.Fatalf("GetLibrary error = %v", err)
	}

	remaining := <-deadlines
	if remaining <= 0 || remaining > time.Minute {
		t.Errorf("root check deadline in %v, want within the timeout of %v", remaining, time.Minute)
	}
}

// listWithin calls ListLibraries and fails the test when it does not answer within limit.
func listWithin(t *testing.T, service *applibrary.Service, limit time.Duration) []applibrary.Overview {
	t.Helper()

	type outcome struct {
		overviews []applibrary.Overview
		err       error
	}

	done := make(chan outcome, 1)

	go func() {
		overviews, err := service.ListLibraries(t.Context())
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
