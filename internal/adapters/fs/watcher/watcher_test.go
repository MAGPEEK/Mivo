package watcher_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/magpeek/mivo/internal/adapters/fs/watcher"
	"github.com/magpeek/mivo/internal/app/scantrigger"
	"github.com/magpeek/mivo/internal/domain"
)

const (
	// eventTimeout bounds the wait for a change notification; file system events arrive
	// within milliseconds, the margin is for loaded CI machines.
	eventTimeout = 10 * time.Second
	// quietWindow is how long a test waits to be sure that no notification follows.
	quietWindow     = 500 * time.Millisecond
	filePermissions = 0o600
	dirPermissions  = 0o750
	// testRootCheckInterval replaces the 30 s root identity check.
	testRootCheckInterval = 50 * time.Millisecond
)

// session is one running Watch call on a temp directory.
type session struct {
	root    string
	changes chan struct{}
	cancel  context.CancelFunc
	// done is closed when Watch has returned result.
	done   chan struct{}
	result error

	errorsMutex sync.Mutex
	reported    []error
}

// startWatching runs Watch on root until the test ends and waits until it is ready. At the
// end of the test it checks that no partial watch failure was reported.
func startWatching(t *testing.T, root string) *session {
	t.Helper()

	watching := &session{root: root, changes: make(chan struct{}, 1), done: make(chan struct{})}

	fileWatcher := watcher.New()
	watcher.SetRootCheckInterval(fileWatcher, testRootCheckInterval)

	ready := make(chan struct{})
	callbacks := scantrigger.WatchCallbacks{
		Changed: watching.changed, Ready: func() { close(ready) }, Degraded: watching.report,
	}

	ctx, cancel := context.WithCancel(context.Background())
	watching.cancel = cancel

	go func() {
		defer close(watching.done)
		watching.result = fileWatcher.Watch(ctx, root, callbacks)
	}()

	// Registered after t.TempDir, so it runs first: the watches close before the directory
	// is removed.
	t.Cleanup(func() { watching.stop(t) })

	select {
	case <-ready:
	case <-watching.done:
		t.Fatalf("Watch ended before it was ready: %v", watching.result)
	case <-time.After(eventTimeout):
		t.Fatal("Watch did not become ready")
	}

	return watching
}

// waitForEnd waits until Watch has returned and returns its result.
func (watching *session) waitForEnd(t *testing.T, cause string) error {
	t.Helper()

	select {
	case <-watching.done:
		return watching.result
	case <-time.After(eventTimeout):
		t.Fatalf("Watch did not end after %s", cause)
		return nil
	}
}

func (watching *session) changed() {
	select {
	case watching.changes <- struct{}{}:
	default:
	}
}

func (watching *session) report(err error) {
	watching.errorsMutex.Lock()
	defer watching.errorsMutex.Unlock()

	watching.reported = append(watching.reported, err)
}

func (watching *session) stop(t *testing.T) {
	t.Helper()

	watching.cancel()
	_ = watching.waitForEnd(t, "the test")

	watching.errorsMutex.Lock()
	defer watching.errorsMutex.Unlock()

	if len(watching.reported) != 0 {
		t.Errorf("reported watch failures %v, want none", watching.reported)
	}
}

// expectChange waits for a notification and then for the burst of follow-up events of the
// same action (create, then write) to pass, so the next step starts clean.
func (watching *session) expectChange(t *testing.T, action string) {
	t.Helper()

	select {
	case <-watching.changes:
	case <-time.After(eventTimeout):
		t.Fatalf("no change notification after %s", action)
	}

	watching.drain()
}

func (watching *session) drain() {
	for {
		select {
		case <-watching.changes:
		case <-time.After(quietWindow):
			return
		}
	}
}

func (watching *session) expectNoChange(t *testing.T, action string) {
	t.Helper()

	select {
	case <-watching.changes:
		t.Fatalf("change notification after %s, want none", action)
	case <-time.After(quietWindow):
	}
}

func writeFile(t *testing.T, path string) {
	t.Helper()

	if err := os.WriteFile(path, []byte("data"), filePermissions); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func makeDir(t *testing.T, path string) {
	t.Helper()

	if err := os.MkdirAll(path, dirPermissions); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
}

func TestWatchReportsFileChanges(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	watching := startWatching(t, root)
	movie := filepath.Join(root, "Movie (2010).mkv")

	writeFile(t, movie)
	watching.expectChange(t, "create")

	if err := os.WriteFile(movie, []byte("more data"), filePermissions); err != nil {
		t.Fatalf("rewrite: %v", err)
	}

	watching.expectChange(t, "write")

	renamed := filepath.Join(root, "Movie (2011).mkv")
	if err := os.Rename(movie, renamed); err != nil {
		t.Fatalf("rename: %v", err)
	}

	watching.expectChange(t, "rename")

	if err := os.Remove(renamed); err != nil {
		t.Fatalf("remove: %v", err)
	}

	watching.expectChange(t, "remove")
}

func TestWatchFollowsNewAndExistingDirectories(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	makeDir(t, filepath.Join(root, "Existing", "Season 01"))

	watching := startWatching(t, root)

	writeFile(t, filepath.Join(root, "Existing", "Season 01", "Show S01E01.mkv"))
	watching.expectChange(t, "create in an existing nested directory")

	nested := filepath.Join(root, "New Show")
	makeDir(t, nested)
	watching.expectChange(t, "mkdir")

	season := filepath.Join(nested, "Season 02")
	makeDir(t, season)
	watching.expectChange(t, "nested mkdir")

	writeFile(t, filepath.Join(season, "Show S02E01.mkv"))
	watching.expectChange(t, "create in a new nested directory")
}

// Renaming a folder with a subfolder must work while it is watched: on Windows a handle on
// the subfolder (one fsnotify watch per directory) made the rename fail with "Access is
// denied".
func TestWatchFollowsRenamedDirectories(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	makeDir(t, filepath.Join(root, "Old Name", "Extras"))

	watching := startWatching(t, root)

	renamed := filepath.Join(root, "New Name")
	if err := os.Rename(filepath.Join(root, "Old Name"), renamed); err != nil {
		t.Fatalf("rename directory: %v", err)
	}

	watching.expectChange(t, "rename directory")

	writeFile(t, filepath.Join(renamed, "Extras", "trailer.mkv"))
	watching.expectChange(t, "create in a renamed directory")

	if err := os.RemoveAll(renamed); err != nil {
		t.Fatalf("remove directory: %v", err)
	}

	watching.expectChange(t, "remove directory")
}

func TestWatchIgnoresSystemFoldersAndHiddenEntries(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	makeDir(t, filepath.Join(root, "@eaDir"))

	watching := startWatching(t, root)

	writeFile(t, filepath.Join(root, "@eaDir", "thumb.jpg"))
	watching.expectNoChange(t, "write in an existing system folder")

	makeDir(t, filepath.Join(root, "#recycle"))
	watching.expectNoChange(t, "mkdir of a system folder")

	writeFile(t, filepath.Join(root, "#recycle", "deleted.mkv"))
	watching.expectNoChange(t, "write in a new system folder")

	writeFile(t, filepath.Join(root, ".DS_Store"))
	watching.expectNoChange(t, "write of a hidden file")

	makeDir(t, filepath.Join(root, "Movie"))
	watching.expectChange(t, "mkdir of a library folder")

	writeFile(t, filepath.Join(root, "Movie", "._Movie.mkv"))
	watching.expectNoChange(t, "write of a hidden file in a library folder")
}

func TestWatchRefusesUnusableRoots(t *testing.T) {
	t.Parallel()

	base := t.TempDir()
	file := filepath.Join(base, "file.mkv")
	writeFile(t, file)

	tests := []struct {
		name string
		root string
		want error
	}{
		{name: "missing", root: filepath.Join(base, "missing"), want: domain.ErrUnavailable},
		{name: "file", root: file, want: domain.ErrUnavailable},
		// Configured roots are absolute; a relative one is a wiring bug.
		{name: "relative", root: "relative", want: domain.ErrInvalid},
	}

	fileWatcher := watcher.New()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := fileWatcher.Watch(t.Context(), tt.root, scantrigger.WatchCallbacks{Changed: func() {}})
			if !errors.Is(err, tt.want) {
				t.Errorf("Watch(%s) = %v, want %v", tt.root, err, tt.want)
			}
		})
	}
}

func TestWatchRejectsMissingCallback(t *testing.T) {
	t.Parallel()

	err := watcher.New().Watch(t.Context(), t.TempDir(), scantrigger.WatchCallbacks{})
	if !errors.Is(err, domain.ErrInvalid) {
		t.Errorf("Watch without change callback = %v, want ErrInvalid", err)
	}
}

// A watch whose context ends during the initial walk returns without becoming ready.
func TestWatchCancelledBeforeReadyReturnsQuietly(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	makeDir(t, filepath.Join(root, "A", "B"))

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	ready := false
	callbacks := scantrigger.WatchCallbacks{Changed: func() {}, Ready: func() { ready = true }}

	if err := watcher.New().Watch(ctx, root, callbacks); err != nil || ready {
		t.Errorf("Watch with a cancelled context = %v, ready %t; want nil and not ready", err, ready)
	}
}

// Windows reports no event when the watched directory itself is renamed, and fsnotify drops
// the root watch without an event when a share is unmounted (the path then shows the empty
// mount point). The root identity check ends the watch in both cases.
func TestWatchEndsWhenTheRootIsRenamedOrReplaced(t *testing.T) {
	t.Parallel()

	tests := map[string]bool{"renamed": false, "replaced by another directory": true}

	for name, recreate := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			base := t.TempDir()
			root := filepath.Join(base, "library")
			makeDir(t, root)

			watching := startWatching(t, root)

			if err := os.Rename(root, filepath.Join(base, "moved")); err != nil {
				t.Fatalf("rename the watched root: %v", err)
			}

			if recreate {
				makeDir(t, root)
			}

			if err := watching.waitForEnd(t, "renaming the root"); !errors.Is(err, domain.ErrUnavailable) {
				t.Errorf("Watch after renaming the root = %v, want ErrUnavailable", err)
			}

			watching.expectChange(t, "rename root")
		})
	}
}

func TestWatchEndsWithTheContext(t *testing.T) {
	t.Parallel()

	watching := startWatching(t, t.TempDir())
	watching.cancel()

	if err := watching.waitForEnd(t, "cancel"); err != nil {
		t.Errorf("Watch after cancel = %v, want nil", err)
	}
}

// A root that disappears (an unmounted share, a deleted folder) ends the watch with
// ErrUnavailable after one last notification, so the caller can fall back to the periodic
// scan and retry later.
func TestWatchEndsWhenTheRootDisappears(t *testing.T) {
	t.Parallel()

	root := filepath.Join(t.TempDir(), "library")
	makeDir(t, root)

	watching := startWatching(t, root)

	// The watch must not keep the root from being removed (on Windows the root handle is
	// opened with FILE_SHARE_DELETE).
	if err := os.Remove(root); err != nil {
		t.Fatalf("remove the watched root: %v", err)
	}

	if err := watching.waitForEnd(t, "removing the root"); !errors.Is(err, domain.ErrUnavailable) {
		t.Errorf("Watch after removing the root = %v, want ErrUnavailable", err)
	}

	watching.expectChange(t, "remove root")
}
