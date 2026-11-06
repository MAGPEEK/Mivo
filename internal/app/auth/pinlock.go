package auth

import (
	"sync"
	"time"
)

// PIN lockout of ADR 0032 §4 (US-02).
const (
	// PINFailuresBeforeLock is the number of wrong PINs per profile that lock it.
	PINFailuresBeforeLock = 5
	// PINLockDuration is how long a locked profile refuses every PIN.
	PINLockDuration = 5 * time.Minute
	// pinFailureMemory forgets a profile's wrong PINs after this long without a new one, so
	// rare typos spread over days never add up to a lock.
	pinFailureMemory = PINLockDuration
)

// pinLock counts wrong PINs per profile ID. Like the sign-in throttle, entries exist only
// while a check runs or failures are remembered. Callers check that the profile belongs to
// the session's account before begin, so the keys are existing profiles and memory stays
// bounded by the number of profiles.
type pinLock struct {
	mutex    sync.Mutex
	profiles map[string]*pinFailures
}

type pinFailures struct {
	count       int
	inFlight    int
	last        time.Time
	lockedUntil time.Time
}

func newPINLock() *pinLock {
	return &pinLock{profiles: map[string]*pinFailures{}}
}

// pinAttempt is one admitted PIN check; finish must be called exactly once.
type pinAttempt struct {
	lock      *pinLock
	profileID string
}

// begin admits a PIN check or returns a *ProfileLockedError.
func (lock *pinLock) begin(profileID string, now time.Time) (pinAttempt, error) {
	lock.mutex.Lock()
	defer lock.mutex.Unlock()

	if wait := lock.profiles[profileID].refresh(now).wait(now); wait > 0 {
		return pinAttempt{}, &ProfileLockedError{Wait: wait}
	}

	lock.entry(profileID).inFlight++

	return pinAttempt{lock: lock, profileID: profileID}, nil
}

// finish records the outcome and returns how long the profile is locked now, zero when it
// is not.
func (attempt pinAttempt) finish(result outcome, now time.Time) time.Duration {
	lock := attempt.lock

	lock.mutex.Lock()
	defer lock.mutex.Unlock()

	failures := lock.entry(attempt.profileID).refresh(now)
	failures.inFlight--

	switch result {
	case outcomeAborted:
	case outcomeSucceeded:
		failures.count, failures.lockedUntil = 0, time.Time{}
	case outcomeFailed:
		failures.recordFailure(now)
	}

	lockedFor := max(failures.lockedUntil.Sub(now), 0)

	if failures.count == 0 && failures.inFlight == 0 {
		delete(lock.profiles, attempt.profileID)
	}

	if len(lock.profiles) > pruneThreshold {
		lock.pruneLocked(now)
	}

	return lockedFor
}

// prune drops entries that no longer lock anything.
func (lock *pinLock) prune(now time.Time) {
	lock.mutex.Lock()
	defer lock.mutex.Unlock()

	lock.pruneLocked(now)
}

func (lock *pinLock) pruneLocked(now time.Time) {
	for profileID, failures := range lock.profiles {
		if failures.refresh(now).count == 0 && failures.inFlight == 0 {
			delete(lock.profiles, profileID)
		}
	}
}

func (lock *pinLock) entry(profileID string) *pinFailures {
	failures, ok := lock.profiles[profileID]
	if !ok {
		failures = &pinFailures{}
		lock.profiles[profileID] = failures
	}

	return failures
}

// refresh clears the count when the lock has ended or the failures are forgotten, so a
// profile gets the full number of tries again after the window. It accepts a nil entry.
func (failures *pinFailures) refresh(now time.Time) *pinFailures {
	if failures == nil || failures.count == 0 {
		return failures
	}

	lockEnded := !failures.lockedUntil.IsZero() && !now.Before(failures.lockedUntil)
	forgotten := failures.lockedUntil.IsZero() && now.Sub(failures.last) > pinFailureMemory

	if lockEnded || forgotten {
		failures.count, failures.lockedUntil = 0, time.Time{}
	}

	return failures
}

// wait returns how long the profile refuses PINs; a nil entry never refuses. Checks already
// running count against the remaining tries, so parallel guesses cannot overshoot the limit.
func (failures *pinFailures) wait(now time.Time) time.Duration {
	if failures == nil {
		return 0
	}

	if now.Before(failures.lockedUntil) {
		return failures.lockedUntil.Sub(now)
	}

	if failures.inFlight >= PINFailuresBeforeLock-failures.count {
		return inFlightRetry
	}

	return 0
}

func (failures *pinFailures) recordFailure(now time.Time) {
	failures.count++
	failures.last = now

	if failures.count >= PINFailuresBeforeLock {
		failures.lockedUntil = now.Add(PINLockDuration)
	}
}
