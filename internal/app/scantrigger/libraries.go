package scantrigger

import (
	"context"
	"log/slog"
	"time"

	"github.com/magpeek/mivo/internal/domain/library"
)

// libraryState is what the trigger knows about one library.
type libraryState struct {
	library library.ManagedLibrary
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
	// retryAt delays the due scan after a failed start.
	retryAt time.Time
	// heldUntil, when in the future, keeps the trigger from starting scans of the library
	// (see Suspend); a library event for it clears it, a periodic check does not.
	heldUntil time.Time
	// folders holds one watch state per folder path of the library.
	folders map[string]*folderState
}

// folderState is the watch of one library folder.
type folderState struct {
	path string
	// watch is the running watch, nil when none runs (watching is off or failed).
	watch *watchHandle
	// warned is set after the one warning about watching this folder.
	warned bool
	// unwatched is set when a watch failed: changes may have gone unseen until the next
	// watch is ready.
	unwatched bool
}

// watchHandle identifies one watch goroutine, so a watch that ends late cannot clear the
// state of its successor.
type watchHandle struct {
	cancel    context.CancelFunc
	libraryID string
	folder    *folderState
}

// syncLibraries adopts the current library list: removed libraries lose their state and
// watches, the others are applied.
func (trigger *Trigger) syncLibraries(ctx context.Context, libraries []library.ManagedLibrary, now time.Time) {
	configured := make(map[string]struct{}, len(libraries))
	for _, configuredLibrary := range libraries {
		configured[configuredLibrary.ID] = struct{}{}
	}

	trigger.mutex.Lock()
	defer trigger.mutex.Unlock()

	for libraryID := range trigger.states {
		if _, ok := configured[libraryID]; !ok {
			trigger.removeLocked(libraryID)
		}
	}

	for _, configuredLibrary := range libraries {
		trigger.applyLocked(ctx, configuredLibrary, now)
	}
}

// applyLocked adds a library or adopts its changed definition: watches of folders that are
// gone stop, folders without a watch get one, and the debounce state is kept. The caller
// holds the mutex.
func (trigger *Trigger) applyLocked(ctx context.Context, configured library.ManagedLibrary, now time.Time) {
	state := trigger.states[configured.ID]
	if state == nil {
		state = &libraryState{since: now, folders: map[string]*folderState{}}
		trigger.states[configured.ID] = state
	}

	state.library = configured

	keep := make(map[string]struct{}, len(configured.Folders))
	for _, folder := range configured.Folders {
		keep[folder.Path] = struct{}{}
	}

	for path, folder := range state.folders {
		if _, ok := keep[path]; !ok {
			folder.stopWatch()
			delete(state.folders, path)
		}
	}

	for _, folder := range configured.Folders {
		watched := state.folders[folder.Path]
		if watched == nil {
			watched = &folderState{path: folder.Path}
			state.folders[folder.Path] = watched
		}

		if trigger.settings.Watch && watched.watch == nil {
			trigger.startWatch(ctx, state, watched)
		}
	}
}

// removeLocked drops a library with its watches and debounce state and reports whether it
// was known. The caller holds the mutex.
func (trigger *Trigger) removeLocked(libraryID string) bool {
	state := trigger.states[libraryID]
	if state == nil {
		return false
	}

	for _, folder := range state.folders {
		folder.stopWatch()
	}

	delete(trigger.states, libraryID)

	return true
}

func (folder *folderState) stopWatch() {
	if folder.watch != nil {
		folder.watch.cancel()
		folder.watch = nil
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

// held reports whether scans of the library are suspended at now.
func (state *libraryState) held(now time.Time) bool {
	return now.Before(state.heldUntil)
}

// forgetLibraries drops every state after Run, so a later Run starts fresh.
func (trigger *Trigger) forgetLibraries() {
	trigger.mutex.Lock()
	defer trigger.mutex.Unlock()

	clear(trigger.states)
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

// folderAttributes names a folder in the log.
func folderAttributes(state *libraryState, folder *folderState, err error) []any {
	return []any{
		slog.String("library", state.library.ID), slog.String("name", state.library.Name),
		slog.String("folder", folder.path), slog.String("error", err.Error()),
	}
}
