package playback

import (
	"fmt"
	"time"

	"github.com/magpeek/mivo/internal/domain"
)

// transition moves a visible session to the state to (with failureCode for StateFailed) when
// the lifecycle allows it, and returns the changed session. The second result is false when
// nothing changed; final states are kept.
func (sessions *registry) transition(id string, to State, failureCode string) (Session, bool) {
	sessions.mutex.Lock()
	defer sessions.mutex.Unlock()

	entry, found := sessions.visible(id)
	if !found || !canTransition(entry.session.State, to) {
		return Session{}, false
	}

	entry.session.State, entry.session.FailureCode = to, failureCode

	return entry.session.clone(), true
}

// canTransition is the session lifecycle: starting → ready → ended or failed; a production
// may also end or fail before its first segment.
func canTransition(from, to State) bool {
	switch from {
	case StateStarting:
		return to == StateReady || to == StateEnded || to == StateFailed
	case StateReady:
		return to == StateEnded || to == StateFailed
	case StateEnded, StateFailed:
		return false
	default:
		return false
	}
}

// removeIdle takes every visible session last accessed at or before cutoff out of the
// registry and returns their records for teardown. Starting sessions are kept (their clients
// do not know them yet), and so are sessions with a stream request still being served.
func (sessions *registry) removeIdle(cutoff time.Time) []*sessionEntry {
	sessions.mutex.Lock()
	defer sessions.mutex.Unlock()

	var idle []*sessionEntry

	for id, entry := range sessions.entries {
		if entry.pending || entry.inFlight > 0 || entry.session.LastAccessedAt.After(cutoff) {
			continue
		}

		delete(sessions.entries, id)
		idle = append(idle, entry)
	}

	return idle
}

// close refuses new sessions from now on and takes every session out of the registry. It
// returns the visible ones for teardown; a pending one is torn down by its own start, which
// finds it gone. The second result is false when the registry was closed already.
func (sessions *registry) close() ([]*sessionEntry, bool) {
	sessions.mutex.Lock()
	defer sessions.mutex.Unlock()

	if sessions.closed {
		return nil, false
	}

	sessions.closed = true

	var visible []*sessionEntry

	for _, entry := range sessions.entries {
		if !entry.pending {
			visible = append(visible, entry)
		}
	}

	clear(sessions.entries)

	return visible, true
}

// track registers background work that Close must wait for. It returns false once the
// registry is closed; the work must then not start.
func (sessions *registry) track() bool {
	sessions.mutex.Lock()
	defer sessions.mutex.Unlock()

	if sessions.closed {
		return false
	}

	sessions.holders.Add(1)

	return true
}

// untrack ends work registered by reserve, remove, track or beginPurge.
func (sessions *registry) untrack() {
	sessions.holders.Done()
}

// wait blocks until all registered work has ended.
func (sessions *registry) wait() {
	sessions.holders.Wait()
}

// beginPurge admits a workspace purge: only before the first session, never twice at once,
// and new sessions are refused until endPurge. The purge counts as holder work, so Close
// waits for it instead of letting the server exit mid-purge.
func (sessions *registry) beginPurge() error {
	sessions.mutex.Lock()
	defer sessions.mutex.Unlock()

	if sessions.closed {
		return ErrServiceClosed
	}

	if sessions.used || sessions.purging {
		return fmt.Errorf("the workspace can only be purged once at a time and before the first session: %w",
			domain.ErrConflict)
	}

	sessions.purging = true
	sessions.holders.Add(1)

	return nil
}

// endPurge ends a purge admitted by beginPurge.
func (sessions *registry) endPurge() {
	sessions.mutex.Lock()
	sessions.purging = false
	sessions.mutex.Unlock()

	sessions.holders.Done()
}
