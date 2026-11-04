package watcher

import "github.com/fsnotify/fsnotify"

// notifier delivers the file system events of one watched tree. On Windows one recursive
// notification covers the whole root (notifier_windows.go); elsewhere fsnotify watches each
// directory and the tree adds and removes the watches (notifier_other.go).
type notifier interface {
	// events and failures are closed when the notifier stops delivering, for example when
	// it lost its handle on the root.
	events() <-chan fsnotify.Event
	failures() <-chan error
	// add watches one directory, remove drops its watch. Both are no-ops for a recursive
	// notifier.
	add(path string) error
	remove(path string) error
	close() error
}
