package auth

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestLockDuration(t *testing.T) {
	t.Parallel()

	tests := []struct {
		failures int
		want     time.Duration
	}{
		{5, time.Minute},
		{6, 2 * time.Minute},
		{7, 4 * time.Minute},
		{8, 8 * time.Minute},
		{9, nameMaxLock},
		{1_000_000, nameMaxLock},
	}

	for _, test := range tests {
		if got := lockDuration(test.failures); got != test.want {
			t.Errorf("lockDuration(%d) = %s, want %s", test.failures, got, test.want)
		}
	}
}

func TestThrottleLimitsAttemptsInFlight(t *testing.T) {
	t.Parallel()

	limiter := newThrottle()
	now := newFakeClock().Now()
	peer := netip.MustParseAddr("10.0.0.1")

	attempts := make([]attempt, 0, nameFailuresBeforeLock)

	for range nameFailuresBeforeLock {
		admitted, err := limiter.begin("alice", peer, now)
		if err != nil {
			t.Fatalf("begin: %v", err)
		}

		attempts = append(attempts, admitted)
	}

	var throttled *ThrottledError
	if _, err := limiter.begin("alice", peer, now); !errors.As(err, &throttled) || throttled.Wait != inFlightRetry {
		t.Fatalf("begin beyond the in-flight budget = %v, want a short retry", err)
	}

	for _, admitted := range attempts {
		admitted.finish(outcomeAborted, now)
	}

	if _, err := limiter.begin("alice", peer, now); err != nil {
		t.Fatalf("begin after aborted attempts: %v", err)
	}
}

func TestThrottleLimitsPeerAttemptsInFlight(t *testing.T) {
	t.Parallel()

	limiter := newThrottle()
	now := newFakeClock().Now()
	peer := netip.MustParseAddr("10.0.0.1")

	for index := range peerFailureLimit {
		if _, err := limiter.begin(fmt.Sprintf("name-%d", index), peer, now); err != nil {
			t.Fatalf("begin %d: %v", index, err)
		}
	}

	var throttled *ThrottledError
	if _, err := limiter.begin("another", peer, now); !errors.As(err, &throttled) || throttled.Wait != inFlightRetry {
		t.Fatalf("begin beyond the peer in-flight budget = %v, want a short retry", err)
	}
}

func TestParallelWrongPasswordsCannotOvershootTheLock(t *testing.T) {
	t.Parallel()

	const parallel = 20

	h, _ := withAlice(t)
	h.hasher.gate = make(chan struct{})

	var (
		wait     sync.WaitGroup
		mutex    sync.Mutex
		verified int
	)

	for range parallel {
		wait.Add(1)

		go func() {
			defer wait.Done()

			_, err := h.service.SignIn(context.Background(), signInRequest("Alice", "wrong password"))
			if errors.Is(err, ErrInvalidCredentials) {
				mutex.Lock()
				verified++
				mutex.Unlock()
			}
		}()
	}

	// Let the admitted attempts finish once every goroutine has been admitted or refused.
	waitUntil(t, func() bool {
		_, verifies, _ := h.hasher.counts()

		return verifies == nameFailuresBeforeLock
	})
	close(h.hasher.gate)
	wait.Wait()

	if verified != nameFailuresBeforeLock {
		t.Fatalf("%d wrong passwords were verified in parallel, want at most %d", verified, nameFailuresBeforeLock)
	}
}

func waitUntil(t *testing.T, condition func() bool) {
	t.Helper()

	deadline := time.Now().Add(5 * time.Second)
	for !condition() {
		if time.Now().After(deadline) {
			t.Fatal("condition not reached")
		}

		time.Sleep(time.Millisecond)
	}
}

func TestThrottlePruneDropsStaleEntries(t *testing.T) {
	t.Parallel()

	limiter := newThrottle()
	clock := newFakeClock()
	peer := netip.MustParseAddr("10.0.0.1")

	admitted, err := limiter.begin("alice", peer, clock.Now())
	if err != nil {
		t.Fatalf("begin: %v", err)
	}

	admitted.finish(outcomeFailed, clock.Now())
	limiter.prune(clock.Now())

	if len(limiter.names) != 1 || len(limiter.peers) != 1 {
		t.Fatalf("prune dropped live entries: names %d, peers %d", len(limiter.names), len(limiter.peers))
	}

	clock.Advance(nameFailureMemory + time.Second)
	limiter.prune(clock.Now())

	if len(limiter.names) != 0 || len(limiter.peers) != 0 {
		t.Fatalf("prune kept stale entries: names %d, peers %d", len(limiter.names), len(limiter.peers))
	}
}

func TestThrottlePrunesWhenManyNamesAreTracked(t *testing.T) {
	t.Parallel()

	limiter := newThrottle()
	clock := newFakeClock()

	for index := range pruneThreshold + 1 {
		peer := netip.AddrFrom4([4]byte{10, byte(index >> 16), byte(index >> 8), byte(index)})

		admitted, err := limiter.begin(fmt.Sprintf("name-%d", index), peer, clock.Now())
		if err != nil {
			t.Fatalf("begin %d: %v", index, err)
		}

		admitted.finish(outcomeSucceeded, clock.Now())
	}

	clock.Advance(peerWindow)

	admitted, err := limiter.begin("last", netip.MustParseAddr("10.255.255.255"), clock.Now())
	if err != nil {
		t.Fatalf("begin: %v", err)
	}

	admitted.finish(outcomeFailed, clock.Now())

	if len(limiter.names) > 1 || len(limiter.peers) > 1 {
		t.Fatalf("names %d, peers %d after pruning; want only the last failure", len(limiter.names), len(limiter.peers))
	}
}

func TestSessionCacheGenerationGuard(t *testing.T) {
	t.Parallel()

	cache := newSessionCache()
	now := newFakeClock().Now()
	resolution := Resolution{Session: Session{ID: "s", AccountID: "a"}}

	generation := cache.currentGeneration()
	cache.evict(func(Resolution) bool { return false })
	cache.put("hash", resolution, now, generation)

	if _, ok := cache.get("hash", now); ok {
		t.Fatal("put after an eviction stored a result loaded before it")
	}

	cache.put("hash", resolution, now, cache.currentGeneration())

	if _, ok := cache.get("hash", now); !ok {
		t.Fatal("put with the current generation was dropped")
	}

	if _, ok := cache.get("hash", now.Add(-time.Second)); ok {
		t.Fatal("an entry from the future counted as fresh")
	}

	cache.prune(now.Add(CacheTTL))

	if len(cache.entries) != 0 {
		t.Fatal("prune kept a stale entry")
	}
}

func TestSessionCachePrunesWhenFull(t *testing.T) {
	t.Parallel()

	cache := newSessionCache()
	clock := newFakeClock()

	for index := range pruneThreshold {
		cache.put(fmt.Sprintf("hash-%d", index), Resolution{}, clock.Now(), cache.currentGeneration())
	}

	clock.Advance(CacheTTL)
	cache.put("fresh", Resolution{}, clock.Now(), cache.currentGeneration())

	if len(cache.entries) != 1 {
		t.Fatalf("entries = %d after a full cache was pruned, want 1", len(cache.entries))
	}
}

func TestThrottleMemoryStaysBoundedForRefusedAndSuccessfulAttempts(t *testing.T) {
	t.Parallel()

	const uniqueNames = 1_000

	limiter := newThrottle()
	now := newFakeClock().Now()
	peer := netip.MustParseAddr("10.0.0.1")

	for index := range peerFailureLimit {
		admitted, err := limiter.begin(fmt.Sprintf("failed-%d", index), peer, now)
		if err != nil {
			t.Fatalf("begin %d: %v", index, err)
		}

		admitted.finish(outcomeFailed, now)
	}

	for index := range uniqueNames {
		var throttled *ThrottledError
		if _, err := limiter.begin(fmt.Sprintf("refused-%d", index), peer, now); !errors.As(err, &throttled) {
			t.Fatalf("begin at the peer limit = %v, want *ThrottledError", err)
		}
	}

	successPeer := netip.MustParseAddr("10.0.0.2")

	for index := range uniqueNames {
		admitted, err := limiter.begin(fmt.Sprintf("ok-%d", index), successPeer, now)
		if err != nil {
			t.Fatalf("begin: %v", err)
		}

		admitted.finish(outcomeSucceeded, now)
	}

	if len(limiter.names) != peerFailureLimit || len(limiter.peers) != 1 {
		t.Fatalf("tracked names %d, peers %d; want only the %d failures of one peer",
			len(limiter.names), len(limiter.peers), peerFailureLimit)
	}
}

func TestThrottleSkipsThePeerCountForAnInvalidAddress(t *testing.T) {
	t.Parallel()

	limiter := newThrottle()
	now := newFakeClock().Now()

	for index := range peerFailureLimit + 1 {
		admitted, err := limiter.begin(fmt.Sprintf("name-%d", index), netip.Addr{}, now)
		if err != nil {
			t.Fatalf("begin %d without a peer address: %v (the per-peer limit must not apply)", index, err)
		}

		admitted.finish(outcomeFailed, now)
	}

	if len(limiter.peers) != 0 {
		t.Fatalf("an invalid address was tracked: %d peers", len(limiter.peers))
	}

	for range nameFailuresBeforeLock {
		admitted, err := limiter.begin("alice", netip.Addr{}, now)
		if err != nil {
			t.Fatalf("begin: %v", err)
		}

		admitted.finish(outcomeFailed, now)
	}

	var throttled *ThrottledError
	if _, err := limiter.begin("alice", netip.Addr{}, now); !errors.As(err, &throttled) || throttled.Wait != nameFirstLock {
		t.Fatalf("begin after %d failures without a peer = %v, want the name lock", nameFailuresBeforeLock, err)
	}
}

func TestParallelAttemptsAfterALockEndAdmitOnlyOne(t *testing.T) {
	t.Parallel()

	const parallel = 10

	h, _ := withAlice(t)

	for range nameFailuresBeforeLock {
		_, _ = h.service.SignIn(context.Background(), signInRequest("Alice", "wrong password"))
	}

	h.clock.Advance(nameFirstLock)
	h.hasher.mutex.Lock()
	h.hasher.gate = make(chan struct{})
	h.hasher.mutex.Unlock()

	_, before, _ := h.hasher.counts()

	var (
		wait    sync.WaitGroup
		refused atomic.Int32
	)

	for range parallel {
		wait.Add(1)

		go func() {
			defer wait.Done()

			request := signInRequest("Alice", "wrong password")
			request.Peer = otherPeer()

			var throttled *ThrottledError
			if _, err := h.service.SignIn(context.Background(), request); errors.As(err, &throttled) {
				refused.Add(1)
			}
		}()
	}

	waitUntil(t, func() bool { return refused.Load() == parallel-1 })
	close(h.hasher.gate)
	wait.Wait()

	if _, after, _ := h.hasher.counts(); after-before != 1 {
		t.Fatalf("%d attempts were verified after the lock ended, want exactly 1", after-before)
	}
}
