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
	// peerFailureLimit is the number of failures per peer address within peerWindow.
	peerFailureLimit = 20
	// peerWindow is the fixed window in which peerFailureLimit applies.
	peerWindow = 15 * time.Minute
	// inFlightRetry is the retry hint when all remaining attempts are already running, so
	// parallel requests cannot overshoot the failure limits.
	inFlightRetry = time.Second
	// pruneThreshold is the number of tracked names or peers above which stale entries are
	// dropped on the next failure, so attacker-chosen names cannot grow memory without bound.
	pruneThreshold = 10_000
)

// throttle counts failed sign-ins per normalised name key and per peer address.
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

// begin admits a sign-in attempt or returns a *ThrottledError.
func (t *throttle) begin(nameKey string, peer netip.Addr, now time.Time) (attempt, error) {
	t.mutex.Lock()
	defer t.mutex.Unlock()

	name := t.nameEntry(nameKey, now)
	peerEntry := t.peerEntry(peer, now)

	if wait := name.wait(now); wait > 0 {
		return attempt{}, &ThrottledError{Wait: wait}
	}

	if wait := peerEntry.wait(now); wait > 0 {
		return attempt{}, &ThrottledError{Wait: wait}
	}

	name.inFlight++
	peerEntry.inFlight++

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

	name := t.nameEntry(a.nameKey, now)
	peerEntry := t.peerEntry(a.peer, now)
	name.inFlight--
	peerEntry.inFlight--

	switch result {
	case outcomeAborted:
		return
	case outcomeSucceeded:
		name.count, name.lockedUntil = 0, time.Time{}

		return
	case outcomeFailed:
	}

	name.recordFailure(now)
	peerEntry.count++

	if len(t.names) > pruneThreshold || len(t.peers) > pruneThreshold {
		t.pruneLocked(now)
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
		if name.inFlight == 0 && (name.count == 0 || now.Sub(name.last) > nameFailureMemory) {
			delete(t.names, key)
		}
	}

	for address, peer := range t.peers {
		if peer.inFlight == 0 && (peer.count == 0 || now.Sub(peer.windowStart) >= peerWindow) {
			delete(t.peers, address)
		}
	}
}

func (t *throttle) nameEntry(nameKey string, now time.Time) *nameFailures {
	name, ok := t.names[nameKey]
	if !ok {
		name = &nameFailures{}
		t.names[nameKey] = name
	}

	if name.count > 0 && now.Sub(name.last) > nameFailureMemory {
		name.count, name.lockedUntil = 0, time.Time{}
	}

	return name
}

func (t *throttle) peerEntry(peer netip.Addr, now time.Time) *peerFailures {
	entry, ok := t.peers[peer]
	if !ok {
		entry = &peerFailures{windowStart: now}
		t.peers[peer] = entry
	}

	if now.Sub(entry.windowStart) >= peerWindow {
		entry.count, entry.windowStart = 0, now
	}

	return entry
}

func (name *nameFailures) wait(now time.Time) time.Duration {
	if now.Before(name.lockedUntil) {
		return name.lockedUntil.Sub(now)
	}

	if name.count < nameFailuresBeforeLock && name.count+name.inFlight >= nameFailuresBeforeLock {
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

func (peer *peerFailures) wait(now time.Time) time.Duration {
	if peer.count >= peerFailureLimit {
		return peer.windowStart.Add(peerWindow).Sub(now)
	}

	if peer.count+peer.inFlight >= peerFailureLimit {
		return inFlightRetry
	}

	return 0
}
