package watcher

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/fsnotify/fsnotify"

	"github.com/magpeek/mivo/internal/domain"
)

// fakeTree is a tree on a temp directory whose notifier is replaced by recording functions.
type fakeTree struct {
	*tree
	added    []string
	removed  []string
	reported []error
	changes  int
	// failAdd returns the error for a directory, or nil to accept it.
	failAdd func(path string) error
}

func newFakeTree(t *testing.T, root string, recursive bool) *fakeTree {
	t.Helper()

	fake := &fakeTree{failAdd: func(string) error { return nil }}
	fake.tree = newTree(treeOptions{
		root:      root,
		recursive: recursive,
		add: func(path string) error {
			if err := fake.failAdd(path); err != nil {
				return err
			}

			fake.added = append(fake.added, path)

			return nil
		},
		remove:  func(path string) error { fake.removed = append(fake.removed, path); return nil },
		report:  func(err error) { fake.reported = append(fake.reported, err) },
		changed: func() { fake.changes++ },
	})

	return fake
}

func mkdirAll(t *testing.T, paths ...string) {
	t.Helper()

	for _, path := range paths {
		if err := os.MkdirAll(path, 0o750); err != nil {
			t.Fatalf("mkdir %s: %v", path, err)
		}
	}
}

// A reached watch limit must not stop the other watches, and is reported once with a hint
// instead of inotify's misleading "no space left on device".
func TestWatchLimitIsReportedOnceAndOtherDirectoriesStayWatched(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	mkdirAll(t, filepath.Join(root, "A"), filepath.Join(root, "B"), filepath.Join(root, "C"))

	fake := newFakeTree(t, root, false)
	fake.failAdd = func(path string) error {
		if path == root || filepath.Base(path) == "C" {
			return nil
		}

		return fmt.Errorf("inotify_add_watch: %w", syscall.ENOSPC)
	}

	if err := fake.watchRoot(t.Context()); err != nil {
		t.Fatalf("watchRoot: %v", err)
	}

	want := []string{root, filepath.Join(root, "C")}
	if !slices.Equal(fake.added, want) {
		t.Errorf("added = %v, want %v", fake.added, want)
	}

	if len(fake.reported) != 1 || !errors.Is(fake.reported[0], errWatchLimit) || !errors.Is(fake.reported[0], syscall.ENOSPC) {
		t.Fatalf("reported = %v, want one watch limit error", fake.reported)
	}

	if !strings.Contains(fake.reported[0].Error(), "watch A:") {
		t.Errorf("report = %q, want it to name the first unwatched directory relative to the root", fake.reported[0])
	}
}

func TestWatchRootFailureIsReturned(t *testing.T) {
	t.Parallel()

	fake := newFakeTree(t, t.TempDir(), false)
	fake.failAdd = func(string) error { return syscall.ENOSPC }

	if err := fake.watchRoot(t.Context()); !errors.Is(err, errWatchLimit) {
		t.Errorf("watchRoot = %v, want the watch limit error", err)
	}
}

func TestRecursiveTreeWatchesOnlyTheRoot(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	mkdirAll(t, filepath.Join(root, "A", "B"))

	fake := newFakeTree(t, root, true)
	if err := fake.watchRoot(t.Context()); err != nil {
		t.Fatalf("watchRoot: %v", err)
	}

	created := fsnotify.Event{Name: filepath.Join(root, "A"), Op: fsnotify.Create}
	if err := fake.handle(t.Context(), created); err != nil {
		t.Fatalf("handle: %v", err)
	}

	if !slices.Equal(fake.added, []string{root}) || fake.changes != 1 {
		t.Errorf("added = %v, changes = %d, want only the root and one change", fake.added, fake.changes)
	}
}

func TestDirectoriesBelowTheDepthLimitAreNotWatched(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	fake := newFakeTree(t, root, false)

	fake.watchDirectory(t.Context(), filepath.Join(root, "deep"), maxDirectoryDepth+1)

	if len(fake.added) != 0 || len(fake.reported) != 1 || !errors.Is(fake.reported[0], errTooDeep) {
		t.Errorf("added = %v, reported = %v, want nothing added and one depth report", fake.added, fake.reported)
	}
}

// Lost events can only be recovered by a scan, so an overflow is a change as well as a
// reported failure.
func TestOverflowCountsAsChangeAndIsReported(t *testing.T) {
	t.Parallel()

	fake := newFakeTree(t, t.TempDir(), false)

	fake.handleFailure(fsnotify.ErrEventOverflow)
	fake.handleFailure(errors.New("second failure"))

	if fake.changes != 1 {
		t.Errorf("changes = %d, want 1", fake.changes)
	}

	if len(fake.reported) != 1 || !errors.Is(fake.reported[0], errEventsLost) {
		t.Errorf("reported = %v, want one lost-events report", fake.reported)
	}

	if !strings.Contains(fake.reported[0].Error(), "watch library root:") {
		t.Errorf("report = %q, want it to name the library root", fake.reported[0])
	}
}

func TestRemovedDirectoryForgetsItsSubtree(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	mkdirAll(t, filepath.Join(root, "Show", "Season 01"), filepath.Join(root, "Show 2"))

	fake := newFakeTree(t, root, false)
	if err := fake.watchRoot(t.Context()); err != nil {
		t.Fatalf("watchRoot: %v", err)
	}

	if err := fake.handle(t.Context(), fsnotify.Event{Name: filepath.Join(root, "Show"), Op: fsnotify.Rename}); err != nil {
		t.Fatalf("handle: %v", err)
	}

	slices.Sort(fake.removed)

	want := []string{filepath.Join(root, "Show"), filepath.Join(root, "Show", "Season 01")}
	if !slices.Equal(fake.removed, want) {
		t.Errorf("removed = %v, want %v (a sibling sharing the name prefix stays)", fake.removed, want)
	}

	if _, ok := fake.watched[filepath.Join(root, "Show 2")]; !ok {
		t.Error("the sibling Show 2 lost its watch")
	}
}

func TestHandleIgnoresEventsThatDoNotChangeTheLibrary(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	fake := newFakeTree(t, root, false)

	events := []fsnotify.Event{
		{Name: filepath.Join(root, "movie.mkv"), Op: fsnotify.Chmod},
		{Name: filepath.Join(filepath.Dir(root), "outside.mkv"), Op: fsnotify.Create},
		{Name: filepath.Join(root, "@eaDir", "movie.mkv", "SYNOPHOTO_THUMB_XL.jpg"), Op: fsnotify.Create},
		{Name: filepath.Join(root, "Movie", ".partial.mkv"), Op: fsnotify.Write},
		{Name: root, Op: fsnotify.Write},
	}

	for _, event := range events {
		if err := fake.handle(t.Context(), event); err != nil {
			t.Fatalf("handle(%v): %v", event, err)
		}
	}

	if fake.changes != 0 {
		t.Errorf("changes = %d, want 0", fake.changes)
	}
}

func TestRootRemovalEndsTheWatch(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	fake := newFakeTree(t, root, false)

	err := fake.handle(t.Context(), fsnotify.Event{Name: root, Op: fsnotify.Remove})
	if !errors.Is(err, domain.ErrUnavailable) || !errors.Is(err, errRootGone) || fake.changes != 1 {
		t.Errorf("handle(root removed) = %v with %d changes, want ErrUnavailable after one change", err, fake.changes)
	}
}

func TestFollowEndsWhenTheNotifierCloses(t *testing.T) {
	t.Parallel()

	fake := newFakeTree(t, t.TempDir(), false)

	for name, closeEvents := range map[string]bool{"events": true, "failures": false} {
		events, failures := make(chan fsnotify.Event), make(chan error)
		if closeEvents {
			close(events)
		} else {
			close(failures)
		}

		if err := fake.followChannels(t.Context(), events, failures, nil); !errors.Is(err, domain.ErrUnavailable) {
			t.Errorf("follow with closed %s = %v, want ErrUnavailable", name, err)
		}
	}
}

func statDirectory(t *testing.T, path string) os.FileInfo {
	t.Helper()

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}

	return info
}

// The periodic identity check ends the watch when the root path leads to another
// directory, as after an unmount (the path then shows the mount point).
func TestRootIdentityCheckEndsTheWatchOfAReplacedRoot(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	fake := newFakeTree(t, root, false)
	fake.identity = statDirectory(t, t.TempDir())

	checks := make(chan time.Time, 1)
	checks <- time.Now()

	err := fake.followChannels(t.Context(), nil, nil, checks)
	if !errors.Is(err, errRootGone) || !errors.Is(err, domain.ErrUnavailable) || fake.changes != 1 {
		t.Errorf("follow = %v with %d changes, want errRootGone after one change", err, fake.changes)
	}
}

func TestRootIdentityCheckKeepsAnUnchangedRoot(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	fake := newFakeTree(t, root, false)
	fake.identity = statDirectory(t, root)

	ctx, cancel := context.WithCancel(t.Context())
	checks := make(chan time.Time)
	ended := make(chan error, 1)

	go func() { ended <- fake.followChannels(ctx, nil, nil, checks) }()

	// The unbuffered channel hands over the second check only after the first one kept the
	// watch running.
	checks <- time.Now()
	checks <- time.Now()
	cancel()

	if err := <-ended; err != nil || fake.changes != 0 {
		t.Errorf("follow = %v with %d changes, want nil and none", err, fake.changes)
	}
}

func TestCancelledWalkStopsEarly(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	mkdirAll(t, filepath.Join(root, "A"), filepath.Join(root, "B"))

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	fake := newFakeTree(t, root, false)
	if err := fake.watchRoot(ctx); err != nil {
		t.Fatalf("watchRoot: %v", err)
	}

	if !slices.Equal(fake.added, []string{root}) {
		t.Errorf("added = %v, want only the root", fake.added)
	}
}

// Windows reports a directory as written when an entry inside it changes; only the
// entry's own event counts.
func TestRecursiveTreeIgnoresDirectoryWrites(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	mkdirAll(t, filepath.Join(root, "Movie"))

	movie := filepath.Join(root, "Movie", "movie.mkv")
	if err := os.WriteFile(movie, []byte("data"), 0o600); err != nil {
		t.Fatalf("write %s: %v", movie, err)
	}

	fake := newFakeTree(t, root, true)

	for _, event := range []fsnotify.Event{
		{Name: filepath.Join(root, "Movie"), Op: fsnotify.Write},
		{Name: movie, Op: fsnotify.Write},
	} {
		if err := fake.handle(t.Context(), event); err != nil {
			t.Fatalf("handle(%v): %v", event, err)
		}
	}

	if fake.changes != 1 {
		t.Errorf("changes = %d, want 1 (the file only)", fake.changes)
	}
}
