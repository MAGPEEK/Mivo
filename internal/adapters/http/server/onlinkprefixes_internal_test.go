package server

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"sync"
	"testing"
	"time"
)

// waitLimit bounds how long a test waits for a goroutine that must not block.
const waitLimit = 5 * time.Second

// fakePrefixProvider is an in-memory InterfacePrefixProvider whose answer tests change. With
// hold set, every call reports on entered and waits until hold is closed.
type fakePrefixProvider struct {
	mutex    sync.Mutex
	prefixes []netip.Prefix
	err      error
	calls    int
	hold     chan struct{}
	entered  chan struct{}
}

func (provider *fakePrefixProvider) InterfacePrefixes() ([]netip.Prefix, error) {
	provider.mutex.Lock()
	provider.calls++
	prefixes, err, hold, entered := provider.prefixes, provider.err, provider.hold, provider.entered
	provider.mutex.Unlock()

	if hold != nil {
		entered <- struct{}{}
		<-hold
	}

	return prefixes, err
}

func (provider *fakePrefixProvider) answer(prefixes []netip.Prefix, err error) {
	provider.mutex.Lock()
	defer provider.mutex.Unlock()

	provider.prefixes, provider.err = prefixes, err
}

// holdCalls makes later calls wait until the returned release function runs; entered
// receives one value per waiting call. release may be called more than once.
func (provider *fakePrefixProvider) holdCalls() (entered <-chan struct{}, release func()) {
	provider.mutex.Lock()
	defer provider.mutex.Unlock()

	// Buffered, so a second waiting call (a bug) shows up in callCount, not as a deadlock.
	provider.hold, provider.entered = make(chan struct{}), make(chan struct{}, 16)
	hold := provider.hold

	var once sync.Once

	return provider.entered, func() { once.Do(func() { close(hold) }) }
}

func (provider *fakePrefixProvider) callCount() int {
	provider.mutex.Lock()
	defer provider.mutex.Unlock()

	return provider.calls
}

// fakeClock is a settable clock for the prefix cache, safe for concurrent use.
type fakeClock struct {
	mutex sync.Mutex
	now   time.Time
}

func newFakeClock() *fakeClock {
	return &fakeClock{now: time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)}
}

func (clock *fakeClock) Now() time.Time {
	clock.mutex.Lock()
	defer clock.mutex.Unlock()

	return clock.now
}

func (clock *fakeClock) advance(duration time.Duration) {
	clock.mutex.Lock()
	defer clock.mutex.Unlock()

	clock.now = clock.now.Add(duration)
}

func TestOnLinkPrefixesFollowRotationAfterRefresh(t *testing.T) {
	t.Parallel()

	const oldPeer, newPeer = "2001:db8:a::1", "2001:db8:b::1"

	logger, _ := newTestLogger()
	clock := newFakeClock()
	provider := &fakePrefixProvider{prefixes: prefixes(t, "2001:db8:a::/64")}
	cache := newOnLinkPrefixes(provider, clock.Now, logger)

	assertContains(t, cache, oldPeer, true)

	// The provider rotates the prefix; until the interval ends the cache answers alone.
	provider.answer(prefixes(t, "2001:db8:b::/64"), nil)
	clock.advance(interfacePrefixRefreshInterval - time.Nanosecond)
	assertContains(t, cache, oldPeer, true)
	assertContains(t, cache, newPeer, false)

	if calls := provider.callCount(); calls != 1 {
		t.Errorf("provider calls within the interval = %d, want 1", calls)
	}

	clock.advance(time.Nanosecond)
	assertContains(t, cache, newPeer, true)
	assertContains(t, cache, oldPeer, false)

	// A clock that jumps back must not freeze the cache.
	provider.answer(prefixes(t, "2001:db8:c::/64"), nil)
	clock.advance(-time.Hour)
	assertContains(t, cache, "2001:db8:c::1", true)
}

func TestOnLinkPrefixesDropStalePrefixesAfterRepeatedFailures(t *testing.T) {
	t.Parallel()

	const knownPeer = "2001:db8:a::1"

	logger, logs := newTestLogger()
	clock := newFakeClock()
	provider := &fakePrefixProvider{prefixes: prefixes(t, "2001:db8:a::/64")}
	cache := newOnLinkPrefixes(provider, clock.Now, logger)

	assertContains(t, cache, knownPeer, true)

	errInterfaces := errors.New("route socket closed")
	provider.answer(nil, errInterfaces)

	// A short failure streak keeps the last known prefixes and warns once.
	for range maxStalePrefixRefreshes - 1 {
		clock.advance(interfacePrefixRefreshInterval)
		assertContains(t, cache, knownPeer, true)
	}

	assertWarnings(t, logs, 1)

	// The next failure drops them: the guard fails closed and says so once.
	for range 2 {
		clock.advance(interfacePrefixRefreshInterval)
		assertContains(t, cache, knownPeer, false)
	}

	assertWarnings(t, logs, 2)

	if want := 2 + maxStalePrefixRefreshes; provider.callCount() != want {
		t.Errorf("provider calls = %d, want %d: one per interval, not per request", provider.callCount(), want)
	}

	// Recovery ends the streak; the next failure keeps the new prefixes and warns again.
	provider.answer(prefixes(t, "2001:db8:b::/64"), nil)
	clock.advance(interfacePrefixRefreshInterval)
	assertContains(t, cache, "2001:db8:b::1", true)

	provider.answer(nil, errInterfaces)
	clock.advance(interfacePrefixRefreshInterval)
	assertContains(t, cache, "2001:db8:b::1", true)
	assertWarnings(t, logs, 3)

	if !strings.Contains(logs.String(), "route socket closed") {
		t.Errorf("log %q lacks the provider error", logs)
	}
}

func TestOnLinkPrefixesFailClosedWithoutKnownPrefixes(t *testing.T) {
	t.Parallel()

	logger, logs := newTestLogger()
	clock := newFakeClock()
	provider := &fakePrefixProvider{err: errors.New("no interfaces")}
	cache := newOnLinkPrefixes(provider, clock.Now, logger)

	for range maxStalePrefixRefreshes + 1 {
		assertContains(t, cache, "2001:db8:a::1", false)
		clock.advance(interfacePrefixRefreshInterval)
	}

	// Nothing was ever trusted, so there is nothing to drop and only the start is logged.
	assertWarnings(t, logs, 1)
}

// TestOnLinkPrefixesRefreshDoesNotBlockOtherCallers: while the elected caller waits for a
// slow provider, every other caller decides with the current snapshot at once.
func TestOnLinkPrefixesRefreshDoesNotBlockOtherCallers(t *testing.T) {
	t.Parallel()

	const oldPeer, newPeer = "2001:db8:a::1", "2001:db8:b::1"

	logger, _ := newTestLogger()
	clock := newFakeClock()
	provider := &fakePrefixProvider{prefixes: prefixes(t, "2001:db8:a::/64")}
	cache := newOnLinkPrefixes(provider, clock.Now, logger)

	assertContains(t, cache, oldPeer, true)

	entered, release := provider.holdCalls()
	t.Cleanup(release)
	provider.answer(prefixes(t, "2001:db8:b::/64"), nil)
	clock.advance(interfacePrefixRefreshInterval)

	refreshed := make(chan bool, 1)

	oldAddress, newAddress := address(t, oldPeer), address(t, newPeer)

	go func() { refreshed <- cache.contains(t.Context(), newAddress) }()

	receiveWithin(t, entered, "the elected caller never reached the provider")

	others := make(chan [2]bool, 1)

	go func() {
		others <- [2]bool{cache.contains(t.Context(), oldAddress), cache.contains(t.Context(), newAddress)}
	}()

	if got := receiveWithin(t, others, "another caller waited for the refresh"); got != [2]bool{true, false} {
		t.Errorf("other caller saw old, new = %v, want the old snapshot (true, false)", got)
	}

	release()

	if !receiveWithin(t, refreshed, "the refresh did not finish") {
		t.Error("the elected caller did not decide with the refreshed prefixes")
	}

	if calls := provider.callCount(); calls != 2 {
		t.Errorf("provider calls = %d, want 2: the first load and one refresh", calls)
	}
}

// TestOnLinkPrefixesFirstLoadRunsOnce: with nothing cached, concurrent callers wait for the
// first read instead of starting their own.
func TestOnLinkPrefixesFirstLoadRunsOnce(t *testing.T) {
	t.Parallel()

	logger, _ := newTestLogger()
	provider := &fakePrefixProvider{prefixes: prefixes(t, "2001:db8:a::/64")}
	cache := newOnLinkPrefixes(provider, newFakeClock().Now, logger)
	peer := address(t, "2001:db8:a::1")

	entered, release := provider.holdCalls()
	t.Cleanup(release)

	results := make(chan bool, 2)

	go func() { results <- cache.contains(t.Context(), peer) }()

	receiveWithin(t, entered, "the first caller never reached the provider")

	go func() { results <- cache.contains(t.Context(), peer) }()

	// Give the second caller time to queue behind the first load; the assertions hold
	// either way, the pause only makes the queued path the usual one.
	time.Sleep(10 * time.Millisecond)
	release()

	for range 2 {
		if !receiveWithin(t, results, "a caller of the first load did not finish") {
			t.Error("a caller of the first load refused a peer inside the prefix")
		}
	}

	if calls := provider.callCount(); calls != 1 {
		t.Errorf("provider calls = %d, want 1 for the first load", calls)
	}
}

func receiveWithin[T any](t *testing.T, values <-chan T, failure string) T {
	t.Helper()

	select {
	case value := <-values:
		return value
	case <-time.After(waitLimit):
		t.Fatal(failure)

		var zero T

		return zero
	}
}

// TestOnLinkPrefixesConcurrentCallers runs many callers while the clock crosses the
// interval several times; with -race it also proves the snapshot handling race-free.
func TestOnLinkPrefixesConcurrentCallers(t *testing.T) {
	t.Parallel()

	const callers, crossings = 16, 5

	logger, _ := newTestLogger()
	clock := newFakeClock()
	provider := &fakePrefixProvider{prefixes: prefixes(t, "2001:db8:a::/64")}
	cache := newOnLinkPrefixes(provider, clock.Now, logger)
	peer := address(t, "2001:db8:a::1")

	var (
		group    sync.WaitGroup
		failures sync.Map
	)

	stop := make(chan struct{})

	for caller := range callers {
		group.Go(func() {
			for {
				select {
				case <-stop:
					return
				default:
				}

				if !cache.contains(t.Context(), peer) {
					failures.Store(caller, true)
				}
			}
		})
	}

	for range crossings {
		time.Sleep(time.Millisecond)
		clock.advance(interfacePrefixRefreshInterval)
	}

	time.Sleep(time.Millisecond)
	close(stop)
	group.Wait()

	failures.Range(func(caller, _ any) bool {
		t.Errorf("caller %v saw the peer refused although the prefix never changed", caller)

		return true
	})

	if calls := provider.callCount(); calls < 1 || calls > 1+crossings {
		t.Errorf("provider calls = %d, want 1 to %d: at most one refresh per crossing", calls, 1+crossings)
	}
}

func assertContains(t *testing.T, cache *onLinkPrefixes, peer string, want bool) {
	t.Helper()

	if got := cache.contains(t.Context(), address(t, peer)); got != want {
		t.Errorf("contains(%s) = %v, want %v", peer, got, want)
	}
}

func assertWarnings(t *testing.T, logs *syncBuffer, want int) {
	t.Helper()

	if got := strings.Count(logs.String(), "level=WARN"); got != want {
		t.Errorf("log %q has %d warnings, want %d", logs, got, want)
	}
}

// TestLANGuardFollowsPrefixRotation drives the whole chain with the clock of the settings.
func TestLANGuardFollowsPrefixRotation(t *testing.T) {
	t.Parallel()

	const newPrefixPeer = "[2001:db8:b::1]:5000"

	logger, _ := newTestLogger()
	clock := newFakeClock()
	provider := &fakePrefixProvider{prefixes: prefixes(t, "2001:db8:a::/64")}
	options := Options{Logger: logger, APIHandler: textHandler(http.StatusOK, "api"), InterfacePrefixes: provider}

	settings, err := newChainSettings(options)
	if err != nil {
		t.Fatalf("newChainSettings: %v", err)
	}

	settings.now = clock.Now
	chain := newHandler(options, settings)

	assertStatusFrom(t, chain, newPrefixPeer, http.StatusForbidden)

	provider.answer(prefixes(t, "2001:db8:b::/64"), nil)
	assertStatusFrom(t, chain, newPrefixPeer, http.StatusForbidden)

	clock.advance(interfacePrefixRefreshInterval)
	assertStatusFrom(t, chain, newPrefixPeer, http.StatusOK)
}

func assertStatusFrom(t *testing.T, chain http.Handler, remoteAddr string, want int) {
	t.Helper()

	request := newRequest(t, http.MethodGet, "/api/v1/libraries")
	request.RemoteAddr = remoteAddr

	recorder := httptest.NewRecorder()
	chain.ServeHTTP(recorder, request)

	if recorder.Code != want {
		t.Errorf("GET from %s = %d, want %d", remoteAddr, recorder.Code, want)
	}
}

// TestLANGuardAsksForPrefixesOnlyForIPv6Peers: IPv4 internet scanners must not cost an
// interface lookup, and LAN peers are decided by the fixed ranges alone.
func TestLANGuardAsksForPrefixesOnlyForIPv6Peers(t *testing.T) {
	t.Parallel()

	provider := &fakePrefixProvider{}
	logger, _ := newTestLogger()
	chain := newTestChain(t, Options{Logger: logger, APIHandler: textHandler(http.StatusOK, "api"), InterfacePrefixes: provider})

	for _, peer := range []string{publicPeer, "[::ffff:8.8.8.8]:1", lanPeer, "[fd00::1]:1", "garbage"} {
		request := newRequest(t, http.MethodGet, "/api/v1/libraries")
		request.RemoteAddr = peer
		chain.ServeHTTP(httptest.NewRecorder(), request)
	}

	if calls := provider.callCount(); calls != 0 {
		t.Errorf("provider calls = %d, want 0 for IPv4, mapped, fixed-range and malformed peers", calls)
	}

	assertStatusFrom(t, chain, "[2606:4700:4700::1111]:443", http.StatusForbidden)

	if calls := provider.callCount(); calls != 1 {
		t.Errorf("provider calls = %d, want 1 for a global IPv6 peer", calls)
	}
}
