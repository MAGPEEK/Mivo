package scantrigger

import (
	"context"
	"log/slog"
	"time"

	"github.com/magpeek/mivo/internal/domain/library"
)

// libraryState is what the trigger knows about one library.
type libraryState struct {
	library library.Library
	// since is when the trigger first saw the library; the periodic scan of a library
	// without a finished scan counts from here.
	since time.Time
	// pending marks changes waiting for the quiet period; firstChange and lastChange bound
	// them.
	pending     bool
	firstChange time.Time
	lastChange  time.Time
	// due marks a debounce that has elapsed and waits for a running scan to finish.
	due bool
	// watch is the running watch, nil when none runs (watching is off or failed).
	watch *watchHandle
	// warned is set after the one warning about watching this library.
	warned bool
	// unwatched is set when a watch failed: changes may have gone unseen until the next
	// watch is ready.
	unwatched bool
}

// watchHandle identifies one watch goroutine, so a watch that ends late cannot clear the
// state of its successor.
type watchHandle struct {
	cancel context.CancelFunc
}

// syncLibraries adopts the current library list: removed libraries and libraries whose
// root changed lose their state and watch, new ones are added, and every library is
// watched unless its watch runs.
func (trigger *Trigger) syncLibraries(ctx context.Context, summaries []library.Summary, now time.Time) {
	configured := make(map[string]library.Library, len(summaries))
	for _, summary := range summaries {
		configured[summary.Library.ID] = summary.Library
	}

	trigger.mutex.Lock()
	defer trigger.mutex.Unlock()

	for libraryID, state := range trigger.states {
		current, ok := configured[libraryID]
		if !ok || current.Root != state.library.Root {
			state.stopWatch()
			delete(trigger.states, libraryID)
		}
	}

	for libraryID, configuredLibrary := range configured {
		state := trigger.states[libraryID]
		if state == nil {
			state = &libraryState{since: now}
			trigger.states[libraryID] = state
		}

		state.library = configuredLibrary

		if trigger.settings.Watch && state.watch == nil {
			trigger.startWatch(ctx, state)
		}
	}
}

func (state *libraryState) stopWatch() {
	if state.watch != nil {
		state.watch.cancel()
		state.watch = nil
	}
}

// startWatch starts the watch goroutine of a library. The caller holds the mutex.
func (trigger *Trigger) startWatch(ctx context.Context, state *libraryState) {
	watchContext, cancel := context.WithCancel(ctx)
	handle := &watchHandle{cancel: cancel}
	state.watch = handle
	libraryID, root := state.library.ID, state.library.Root

	trigger.watches.Add(1)

	callbacks := WatchCallbacks{
		Changed:  func() { trigger.recordChange(libraryID) },
		Ready:    func() { trigger.watchReady(handle, libraryID) },
		Degraded: func(err error) { trigger.watchDegraded(handle, libraryID, err) },
	}

	go func() {
		defer trigger.watches.Done()
		defer cancel()

		err := trigger.watcher.Watch(watchContext, root, callbacks)
		trigger.watchEnded(watchContext, handle, libraryID, err)
	}()
}

// watchEnded clears the watch of a library, so the next check retries it, and warns once.
// A watch that was stopped on purpose ends silently.
func (trigger *Trigger) watchEnded(ctx context.Context, handle *watchHandle, libraryID string, err error) {
	if ctx.Err() != nil {
		return
	}

	if err == nil {
		err = errWatchEnded
	}

	trigger.mutex.Lock()
	defer trigger.mutex.Unlock()

	state := trigger.currentState(handle, libraryID)
	if state == nil {
		return
	}

	state.watch = nil
	state.unwatched = true
	trigger.warnOnce(state, watchFailedMessage, err)
}

// watchReady records one change for a library whose previous watch failed: what changed
// while nobody watched is only found by a scan.
func (trigger *Trigger) watchReady(handle *watchHandle, libraryID string) {
	now := trigger.clock.Now()

	trigger.mutex.Lock()
	defer trigger.mutex.Unlock()

	state := trigger.currentState(handle, libraryID)
	if state == nil || !state.unwatched {
		return
	}

	state.unwatched = false
	state.markChange(now)
}

// watchDegraded takes the failures that leave part of a library unwatched while its watch
// keeps running; they share the one warning per library with a failed watch.
func (trigger *Trigger) watchDegraded(handle *watchHandle, libraryID string, err error) {
	trigger.mutex.Lock()
	defer trigger.mutex.Unlock()

	if state := trigger.currentState(handle, libraryID); state != nil {
		trigger.warnOnce(state, watchPartialMessage, err)
	}
}

// currentState returns the state of a library while handle is its running watch. The
// caller holds the mutex.
func (trigger *Trigger) currentState(handle *watchHandle, libraryID string) *libraryState {
	state := trigger.states[libraryID]
	if state == nil || state.watch != handle {
		return nil
	}

	return state
}

// warnOnce logs the first watch problem of a library as a warning and later ones at debug
// level, so a share that stays offline does not fill the log. The caller holds the mutex.
func (trigger *Trigger) warnOnce(state *libraryState, message string, err error) {
	attributes := []any{
		slog.String("library", state.library.ID), slog.String("name", state.library.Name),
		slog.String("root", state.library.Root), slog.String("error", err.Error()),
	}

	if state.warned {
		trigger.logger.Debug(message, attributes...)
		return
	}

	state.warned = true
	trigger.logger.Warn(message+"; "+trigger.fallback(), attributes...)
}

// recordChange notes a change of a library; the next ticks decide when its scan starts.
func (trigger *Trigger) recordChange(libraryID string) {
	now := trigger.clock.Now()

	trigger.mutex.Lock()
	defer trigger.mutex.Unlock()

	if state := trigger.states[libraryID]; state != nil {
		state.markChange(now)
	}
}

// markChange starts or extends the debounce. The caller holds the mutex.
func (state *libraryState) markChange(now time.Time) {
	if !state.pending {
		state.pending = true
		state.firstChange = now
	}

	state.lastChange = now
}

// forgetLibraries drops every state after Run, so a later Run starts fresh.
func (trigger *Trigger) forgetLibraries() {
	trigger.mutex.Lock()
	defer trigger.mutex.Unlock()

	clear(trigger.states)
}
