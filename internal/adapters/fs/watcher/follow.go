package watcher

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/fsnotify/fsnotify"

	"github.com/magpeek/mivo/internal/domain"
)

// follow handles the notifier's events until ctx is done or the root goes away, and checks
// the root's identity every checkInterval.
func (watched *tree) follow(ctx context.Context, source notifier, checkInterval time.Duration) error {
	ticker := time.NewTicker(checkInterval)
	defer ticker.Stop()

	return watched.followChannels(ctx, source.events(), source.failures(), ticker.C)
}

// followChannels is the event loop of follow.
func (watched *tree) followChannels(
	ctx context.Context, events <-chan fsnotify.Event, failures <-chan error, rootChecks <-chan time.Time,
) error {
	for {
		select {
		case <-ctx.Done():
			return nil
		case event, ok := <-events:
			if !ok {
				return fmt.Errorf("%w: %w", domain.ErrUnavailable, errNotifierClosed)
			}

			if err := watched.handle(ctx, event); err != nil {
				return err
			}
		case err, ok := <-failures:
			if !ok {
				return fmt.Errorf("%w: %w", domain.ErrUnavailable, errNotifierClosed)
			}

			watched.handleFailure(err)
		case <-rootChecks:
			if watched.rootGone() {
				return watched.endWithRootGone()
			}
		}
	}
}

// rootGone reports a root path that no longer leads to the directory the watch started
// with: removed, renamed (Windows reports no event for the watched directory itself) or
// unmounted (fsnotify drops the root watch without an event and the path now shows the
// empty mount point).
func (watched *tree) rootGone() bool {
	info, err := os.Stat(watched.root)

	return err != nil || !os.SameFile(watched.identity, info)
}

func (watched *tree) endWithRootGone() error {
	watched.changed()

	return fmt.Errorf("%w: %w", domain.ErrUnavailable, errRootGone)
}

// handleFailure reports a notifier error. A lost event buffer also counts as a change,
// because the lost events can only be recovered by a scan.
func (watched *tree) handleFailure(err error) {
	if errors.Is(err, fsnotify.ErrEventOverflow) {
		watched.changed()
		err = fmt.Errorf("%w: %w", errEventsLost, err)
	}

	watched.fail(watched.root, err)
}

// handle turns one event into a change notification and keeps the watches in step with
// the tree: created directories are watched, removed or renamed ones forgotten (a renamed
// directory comes back as a Create of its new name).
func (watched *tree) handle(ctx context.Context, event fsnotify.Event) error {
	if !event.Has(libraryChanges) {
		return nil
	}

	path := filepath.Clean(event.Name)
	if path == watched.root {
		return watched.handleRootEvent(event)
	}

	relative, ok := watched.relative(path)
	if !ok || isSkippedPath(relative) || watched.isDirectoryWrite(event, path) {
		return nil
	}

	if !watched.recursive {
		watched.followStructure(ctx, event, path, relative)
	}

	watched.changed()

	return nil
}

// handleRootEvent ends the watch when the root itself goes away. Other events of the root
// carry no information the events of its entries do not.
func (watched *tree) handleRootEvent(event fsnotify.Event) error {
	if !event.Has(fsnotify.Remove | fsnotify.Rename) {
		return nil
	}

	return watched.endWithRootGone()
}

// isDirectoryWrite reports the modification Windows records for a directory whose entries
// changed. The entries report their own events, and the directory's would turn a write of
// a hidden file inside a library folder into a change. inotify sends no such events, so
// only the recursive notifier pays for the Lstat.
func (watched *tree) isDirectoryWrite(event fsnotify.Event, path string) bool {
	if !watched.recursive || event.Op != fsnotify.Write {
		return false
	}

	info, err := os.Lstat(path)

	return err == nil && info.IsDir()
}

// followStructure keeps per-directory watches in step with the tree.
func (watched *tree) followStructure(ctx context.Context, event fsnotify.Event, path, relative string) {
	if event.Has(fsnotify.Remove | fsnotify.Rename) {
		watched.forget(path)
	}

	if event.Has(fsnotify.Create) {
		watched.watchCreated(ctx, path, relative)
	}
}
