package scantrigger

import (
	"context"
)

// startWatch starts the watch goroutine of a library folder. The caller holds the mutex and
// is the loop goroutine, the only one that adds to the watch group.
func (trigger *Trigger) startWatch(ctx context.Context, state *libraryState, folder *folderState) {
	watchContext, cancel := context.WithCancel(ctx)
	handle := &watchHandle{cancel: cancel, libraryID: state.library.ID, folder: folder}
	folder.watch = handle
	libraryID, root := handle.libraryID, folder.path

	trigger.watches.Add(1)

	callbacks := WatchCallbacks{
		Changed:  func() { trigger.recordChange(libraryID) },
		Ready:    func() { trigger.watchReady(handle) },
		Degraded: func(err error) { trigger.watchDegraded(handle, err) },
	}

	go func() {
		defer trigger.watches.Done()
		defer cancel()

		err := trigger.watcher.Watch(watchContext, root, callbacks)
		trigger.watchEnded(watchContext, handle, err)
	}()
}

// watchEnded clears the watch of a folder, so the next check retries it, and warns once. A
// watch that was stopped on purpose ends silently.
func (trigger *Trigger) watchEnded(ctx context.Context, handle *watchHandle, err error) {
	if ctx.Err() != nil {
		return
	}

	if err == nil {
		err = errWatchEnded
	}

	trigger.mutex.Lock()
	defer trigger.mutex.Unlock()

	state := trigger.currentState(handle)
	if state == nil {
		return
	}

	handle.folder.watch = nil
	handle.folder.unwatched = true
	trigger.warnOnce(state, handle.folder, watchFailedMessage, err)
}

// watchReady records one change for a library whose folder watch failed before: what
// changed while nobody watched is only found by a scan.
func (trigger *Trigger) watchReady(handle *watchHandle) {
	now := trigger.clock.Now()

	trigger.mutex.Lock()
	defer trigger.mutex.Unlock()

	state := trigger.currentState(handle)
	if state == nil || !handle.folder.unwatched {
		return
	}

	handle.folder.unwatched = false
	state.markChange(now)
}

// watchDegraded takes the failures that leave part of a folder unwatched while its watch
// keeps running; they share the one warning per folder with a failed watch.
func (trigger *Trigger) watchDegraded(handle *watchHandle, err error) {
	trigger.mutex.Lock()
	defer trigger.mutex.Unlock()

	if state := trigger.currentState(handle); state != nil {
		trigger.warnOnce(state, handle.folder, watchPartialMessage, err)
	}
}

// currentState returns the state of the library while handle is the running watch of its
// folder. The caller holds the mutex.
func (trigger *Trigger) currentState(handle *watchHandle) *libraryState {
	state := trigger.states[handle.libraryID]
	if state == nil || handle.folder.watch != handle || state.folders[handle.folder.path] != handle.folder {
		return nil
	}

	return state
}

// warnOnce logs the first watch problem of a folder as a warning and later ones at debug
// level, so a share that stays offline does not fill the log. The caller holds the mutex.
func (trigger *Trigger) warnOnce(state *libraryState, folder *folderState, message string, err error) {
	attributes := folderAttributes(state, folder, err)

	if folder.warned {
		trigger.logger.Debug(message, attributes...)
		return
	}

	folder.warned = true
	trigger.logger.Warn(message+"; "+trigger.fallback(), attributes...)
}
