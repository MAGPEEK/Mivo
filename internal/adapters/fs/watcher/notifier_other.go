//go:build !windows

package watcher

import (
	"fmt"

	"github.com/fsnotify/fsnotify"
)

// recursiveNotifier is false: inotify watches single directories, so the tree adds one
// watch per directory.
const recursiveNotifier = false

// fsnotifyNotifier adapts an fsnotify watcher, one instance per watched root.
type fsnotifyNotifier struct {
	watcher *fsnotify.Watcher
}

// openNotifier creates the fsnotify instance; the root itself is watched by the tree.
// Creating it fails when the inotify instance limit is reached.
func openNotifier(string) (notifier, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("create file system notifier: %w", err)
	}

	return fsnotifyNotifier{watcher: watcher}, nil
}

func (adapted fsnotifyNotifier) events() <-chan fsnotify.Event { return adapted.watcher.Events }
func (adapted fsnotifyNotifier) failures() <-chan error        { return adapted.watcher.Errors }

func (adapted fsnotifyNotifier) add(path string) error {
	if err := adapted.watcher.Add(path); err != nil {
		return fmt.Errorf("add watch: %w", err)
	}

	return nil
}

func (adapted fsnotifyNotifier) remove(path string) error {
	if err := adapted.watcher.Remove(path); err != nil {
		return fmt.Errorf("remove watch: %w", err)
	}

	return nil
}

func (adapted fsnotifyNotifier) close() error {
	if err := adapted.watcher.Close(); err != nil {
		return fmt.Errorf("close file system notifier: %w", err)
	}

	return nil
}
