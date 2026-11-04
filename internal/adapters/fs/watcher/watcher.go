package watcher

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/magpeek/mivo/internal/app/scantrigger"
	"github.com/magpeek/mivo/internal/domain"
)

// rootCheckInterval is how often a watch checks that its root is still the directory it
// started with. Neither backend reports every way a root goes away: fsnotify drops the
// root watch silently on an unmount, and Windows does not report a rename of the watched
// directory itself.
const rootCheckInterval = 30 * time.Second

var (
	// errRootGone ends a watch whose root was removed, renamed or unmounted.
	errRootGone = errors.New("library root was removed, renamed or unmounted")
	// errNotDirectory reports a root that is a file.
	errNotDirectory = errors.New("library root is not a directory")
	// errNotAbsolute reports a relative root, a wiring bug: configured roots are absolute.
	errNotAbsolute = errors.New("library root is not an absolute path")
	// errNotifierClosed reports that the notifier closed its channels on its own.
	errNotifierClosed = errors.New("file system notifier closed")
)

// Watcher watches library roots. It holds no state between Watch calls and is safe for
// concurrent use; each Watch call has its own notifier.
type Watcher struct {
	// rootCheckInterval is rootCheckInterval; tests shorten it.
	rootCheckInterval time.Duration
}

// New returns a Watcher.
func New() *Watcher {
	return &Watcher{rootCheckInterval: rootCheckInterval}
}

// Watch watches the absolute root and every real directory below it until ctx is done,
// then returns nil. callbacks.Changed (required) is called for every created, written,
// removed or renamed file or directory that a scan would see; callbacks.Ready (optional)
// once the initial watches are in place; callbacks.Degraded (optional) at most once, for
// the first failure that leaves part of the tree unwatched (a reached watch limit, an
// unwatchable directory, lost events). All are called from the watching goroutine.
//
// A root that cannot be watched returns an error wrapping domain.ErrUnavailable at once; a
// root that is removed, renamed or unmounted later calls Changed once more and then returns
// such an error, so the caller can retry later.
func (fileWatcher *Watcher) Watch(ctx context.Context, root string, callbacks scantrigger.WatchCallbacks) error {
	if callbacks.Changed == nil {
		return fmt.Errorf("watch library root %s: change callback is nil: %w", root, domain.ErrInvalid)
	}

	root = filepath.Clean(root)

	identity, err := checkRoot(root)
	if err != nil {
		return fmt.Errorf("watch library root %s: %w", root, err)
	}

	source, err := openNotifier(root)
	if err != nil {
		return fmt.Errorf("watch library root %s: %w: %w", root, domain.ErrUnavailable, err)
	}

	// Close errors carry nothing actionable: the watch has ended either way.
	defer func() { _ = source.close() }()

	watched := newTree(treeOptions{
		root: root, identity: identity, recursive: recursiveNotifier,
		add: source.add, remove: source.remove,
		report: optional(callbacks.Degraded), changed: callbacks.Changed,
	})

	if err := watched.watchRoot(ctx); err != nil {
		return fmt.Errorf("watch library root %s: %w: %w", root, domain.ErrUnavailable, err)
	}

	if callbacks.Ready != nil && ctx.Err() == nil {
		callbacks.Ready()
	}

	if err := watched.follow(ctx, source, fileWatcher.rootCheckInterval); err != nil {
		return fmt.Errorf("watch library root %s: %w", root, err)
	}

	return nil
}

func optional(report func(error)) func(error) {
	if report == nil {
		return func(error) {}
	}

	return report
}

// checkRoot accepts an absolute path to an existing directory, following a link at the
// root itself like the library walker does, and returns its identity.
func checkRoot(root string) (os.FileInfo, error) {
	if !filepath.IsAbs(root) {
		return nil, fmt.Errorf("%w: %w", domain.ErrInvalid, errNotAbsolute)
	}

	info, err := os.Stat(root)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", domain.ErrUnavailable, err)
	}

	if !info.IsDir() {
		return nil, fmt.Errorf("%w: %w", domain.ErrUnavailable, errNotDirectory)
	}

	// On Windows os.SameFile reads the file ID lazily through the path, so an identity read
	// only at the first comparison would describe whatever the path leads to by then.
	// Comparing now pins the ID of the directory that is watched.
	_ = os.SameFile(info, info)

	return info, nil
}
