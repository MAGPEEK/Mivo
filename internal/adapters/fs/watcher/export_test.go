package watcher

import "time"

// SetRootCheckInterval shortens the interval of the root identity check, so tests of a
// renamed or replaced root finish quickly.
func SetRootCheckInterval(fileWatcher *Watcher, interval time.Duration) {
	fileWatcher.rootCheckInterval = interval
}
