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

// isFinal tells whether state is a final state: the production has ended or failed and runs
// no FFmpeg any more.
func isFinal(state State) bool {
	return state == StateEnded || state == StateFailed
}

// idleSession is a session removeIdle took out of the registry, with its idle time computed
// under the lock.
type idleSession struct {
	entry   *sessionEntry
	idleFor time.Duration
}

// removeIdle takes every visible session not accessed for timeout or longer at now out of
// the registry and returns their records for teardown. Starting sessions are kept (their
// clients do not know them yet), and so are sessions with a stream request still being
// served.
func (sessions *registry) removeIdle(now time.Time, timeout time.Duration) []idleSession {
	sessions.mutex.Lock()
	defer sessions.mutex.Unlock()

	cutoff := now.Add(-timeout)

	var idle []idleSession

	for id, entry := range sessions.entries {
		if entry.pending || entry.inFlight > 0 || entry.session.LastAccessedAt.After(cutoff) {
			continue
		}

		delete(sessions.entries, id)
		idle = append(idle, idleSession{entry: entry, idleFor: now.Sub(entry.session.LastAccessedAt)})
	}

	return idle
}

// openRequests returns the number of stream requests of entry still being served. It reads
// under the lock because endAccess may still end a request of a removed record.
func (sessions *registry) openRequests(entry *sessionEntry) int {
	sessions.mutex.Lock()
	defer sessions.mutex.Unlock()

	return entry.inFlight
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
