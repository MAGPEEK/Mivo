package auth

import (
	"net/netip"
	"sync"
	"time"
)

// Sign-in throttling of ADR 0032 §5.
const (
	// nameFailuresBeforeLock is the number of failures per account name that start a lock.
	nameFailuresBeforeLock = 5
	// nameFirstLock is the lock after the fifth failure; it doubles per further failure.
	nameFirstLock = time.Minute
	// nameMaxLock caps the doubling.
	nameMaxLock = 15 * time.Minute
	// nameFailureMemory forgets a name's failures after this long without a new one. It
	// equals nameMaxLock, so a forgotten name is never still locked.
	nameFailureMemory = nameMaxLock
	// lockedNameAttemptsInFlight is how many attempts may run at once for a name that already
	// reached the lock threshold, so the attempts admitted after a lock ends cannot overshoot.
	lockedNameAttemptsInFlight = 1
	// peerFailureLimit is the number of failures per peer address within peerWindow.
	peerFailureLimit = 20
	// peerWindow is the fixed window in which peerFailureLimit applies.
	peerWindow = 15 * time.Minute
	// inFlightRetry is the retry hint when all remaining attempts are already running, so
	// parallel requests cannot overshoot the failure limits.
	inFlightRetry = time.Second
	// pruneThreshold is the number of tracked names or peers above which stale entries are
	// dropped on the next failure.
	pruneThreshold = 10_000
)

// throttle counts failed sign-ins per normalised name key and per peer address. Entries exist
// only while an attempt runs or failures are remembered: refused attempts insert nothing and
// finished attempts without remembered failures remove their entries, so memory grows only
// with failures, which the per-peer limit bounds.
type throttle struct {
	mutex sync.Mutex
	names map[string]*nameFailures
	peers map[netip.Addr]*peerFailures
}

type nameFailures struct {
	count       int
	inFlight    int
	last        time.Time
	lockedUntil time.Time
}

type peerFailures struct {
	count       int
	inFlight    int
	windowStart time.Time
}

func newThrottle() *throttle {
	return &throttle{names: map[string]*nameFailures{}, peers: map[netip.Addr]*peerFailures{}}
}

// attempt is one admitted sign-in; finish must be called exactly once.
type attempt struct {
	throttle *throttle
	nameKey  string
	peer     netip.Addr
}

// begin admits a sign-in attempt or returns a *ThrottledError. An invalid peer address is
// not counted per peer (only per name); the HTTP adapter must always set the address.
func (t *throttle) begin(nameKey string, peer netip.Addr, now time.Time) (attempt, error) {
	t.mutex.Lock()
	defer t.mutex.Unlock()

	if wait := t.names[nameKey].refresh(now).wait(now); wait > 0 {
		return attempt{}, &ThrottledError{Wait: wait}
	}

	if peer.IsValid() {
		if wait := t.peers[peer].refresh(now).wait(now); wait > 0 {
			return attempt{}, &ThrottledError{Wait: wait}
		}

		t.peerEntry(peer, now).inFlight++
	}

	t.nameEntry(nameKey).inFlight++

	return attempt{throttle: t, nameKey: nameKey, peer: peer}, nil
}

// outcome is the result of an admitted attempt.
type outcome int

const (
	// outcomeAborted is neither success nor failure, for example a cancelled request or a
	// store error; it changes no counter.
	outcomeAborted outcome = iota
	outcomeSucceeded
	outcomeFailed
)

// finish records the outcome. A success clears the name's failures but not the peer's, so
// one valid account does not reset the guessing budget of an address.
func (a attempt) finish(result outcome, now time.Time) {
	t := a.throttle

	t.mutex.Lock()
	defer t.mutex.Unlock()

	name := t.nameEntry(a.nameKey).refresh(now)
	name.inFlight--

	switch result {
	case outcomeAborted:
	case outcomeSucceeded:
		name.count, name.lockedUntil = 0, time.Time{}
	case outcomeFailed:
		name.recordFailure(now)
	}

	if a.peer.IsValid() {
		peer := t.peerEntry(a.peer, now).refresh(now)
		peer.inFlight--

		if result == outcomeFailed {
			peer.count++
		}
	}

	t.dropIdle(a.nameKey, a.peer)

	if len(t.names) > pruneThreshold || len(t.peers) > pruneThreshold {
		t.pruneLocked(now)
	}
}

// dropIdle removes the entries of a finished attempt that remember nothing.
func (t *throttle) dropIdle(nameKey string, peer netip.Addr) {
	if name := t.names[nameKey]; name != nil && name.count == 0 && name.inFlight == 0 {
		delete(t.names, nameKey)
	}

	if entry := t.peers[peer]; entry != nil && entry.count == 0 && entry.inFlight == 0 {
		delete(t.peers, peer)
	}
}

// prune drops entries that no longer throttle anything.
func (t *throttle) prune(now time.Time) {
	t.mutex.Lock()
	defer t.mutex.Unlock()

	t.pruneLocked(now)
}

func (t *throttle) pruneLocked(now time.Time) {
	for key, name := range t.names {
		if name.refresh(now).count == 0 && name.inFlight == 0 {
			delete(t.names, key)
		}
	}

	for address, peer := range t.peers {
		if peer.refresh(now).count == 0 && peer.inFlight == 0 {
			delete(t.peers, address)
		}
	}
}

func (t *throttle) nameEntry(nameKey string) *nameFailures {
	name, ok := t.names[nameKey]
	if !ok {
		name = &nameFailures{}
		t.names[nameKey] = name
	}

	return name
}

func (t *throttle) peerEntry(peer netip.Addr, now time.Time) *peerFailures {
	entry, ok := t.peers[peer]
	if !ok {
		entry = &peerFailures{windowStart: now}
		t.peers[peer] = entry
	}

	return entry
}

// refresh forgets failures older than nameFailureMemory. It accepts a nil entry (an
// untracked name) and returns it unchanged.
func (name *nameFailures) refresh(now time.Time) *nameFailures {
	if name != nil && name.count > 0 && now.Sub(name.last) > nameFailureMemory {
		name.count, name.lockedUntil = 0, time.Time{}
	}

	return name
}

// wait returns how long the name is refused; a nil entry is never refused.
func (name *nameFailures) wait(now time.Time) time.Duration {
	if name == nil {
		return 0
	}

	if now.Before(name.lockedUntil) {
		return name.lockedUntil.Sub(now)
	}

	allowedInFlight := max(nameFailuresBeforeLock-name.count, lockedNameAttemptsInFlight)
	if name.inFlight >= allowedInFlight {
		return inFlightRetry
	}

	return 0
}

func (name *nameFailures) recordFailure(now time.Time) {
	name.count++
	name.last = now

	if name.count >= nameFailuresBeforeLock {
		name.lockedUntil = now.Add(lockDuration(name.count))
	}
}

// lockDuration is nameFirstLock after the fifth failure, doubled per further failure and
// capped at nameMaxLock.
func lockDuration(failures int) time.Duration {
	lock := nameFirstLock
	for range failures - nameFailuresBeforeLock {
		lock *= 2
		if lock >= nameMaxLock {
			return nameMaxLock
		}
	}

	return lock
}

// refresh starts a new window once peerWindow has passed; it accepts a nil entry.
func (peer *peerFailures) refresh(now time.Time) *peerFailures {
	if peer != nil && now.Sub(peer.windowStart) >= peerWindow {
		peer.count, peer.windowStart = 0, now
	}

	return peer
}

// wait returns how long the peer is refused; a nil entry is never refused.
func (peer *peerFailures) wait(now time.Time) time.Duration {
	if peer == nil {
		return 0
	}

	if peer.count >= peerFailureLimit {
		return peer.windowStart.Add(peerWindow).Sub(now)
	}

	if peer.count+peer.inFlight >= peerFailureLimit {
		return inFlightRetry
	}

	return 0
}
