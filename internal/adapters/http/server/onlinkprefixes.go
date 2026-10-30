package server

import (
	"context"
	"log/slog"
	"net/netip"
	"sync"
	"sync/atomic"
	"time"

	"github.com/magpeek/mivo/internal/adapters/http/logtext"
)

const (
	// interfacePrefixRefreshInterval bounds how long the LAN guard trusts the interface
	// prefixes it read: ISPs rotate delegated IPv6 prefixes, so a stale list would refuse the
	// LAN after a rotation and keep admitting the old prefix's new owner.
	interfacePrefixRefreshInterval = 30 * time.Second

	// maxStalePrefixRefreshes is the number of consecutive failed refreshes after which the
	// last known prefixes are dropped (about two minutes). Until then a short provider hiccup
	// does not lock the LAN out; afterwards the guard fails closed instead of trusting
	// prefixes the ISP may have handed to someone else.
	maxStalePrefixRefreshes = 4
)

// prefixSnapshot is one state of the cache. It is replaced as a whole, never changed, so
// readers need no lock.
type prefixSnapshot struct {
	prefixes []netip.Prefix
	loadedAt time.Time
	// failedRefreshes counts the consecutive failed refreshes since the last success.
	failedRefreshes int
}

// onLinkPrefixes caches the trusted IPv6 prefixes of the host interfaces. A snapshot older
// than interfacePrefixRefreshInterval is refreshed by exactly one caller while all others
// keep using it, so a slow provider (Windows walks every adapter) stalls one request, not
// all of them. Only the very first load blocks concurrent callers, because there is nothing
// to fall back on yet.
//
// When the provider fails, the last known prefixes (or none) stay in use for at most
// maxStalePrefixRefreshes refreshes and are dropped afterwards. A failure streak logs a
// warning when it starts and when the stale prefixes are dropped, never per request.
type onLinkPrefixes struct {
	provider InterfacePrefixProvider
	now      func() time.Time
	logger   *slog.Logger

	snapshot atomic.Pointer[prefixSnapshot]
	// refreshing elects the one caller that refreshes an expired snapshot.
	refreshing atomic.Bool
	// firstLoad serialises the first read of the provider.
	firstLoad sync.Mutex
}

func newOnLinkPrefixes(provider InterfacePrefixProvider, now func() time.Time, logger *slog.Logger) *onLinkPrefixes {
	return &onLinkPrefixes{provider: provider, now: now, logger: logger}
}

// contains reports whether address lies inside a trusted interface prefix.
func (cache *onLinkPrefixes) contains(ctx context.Context, address netip.Addr) bool {
	for _, prefix := range cache.current(ctx).prefixes {
		if prefix.Contains(address) {
			return true
		}
	}

	return false
}

// current returns the snapshot to decide with: the cached one, or a fresh one when this
// caller won the election to refresh an expired snapshot.
func (cache *onLinkPrefixes) current(ctx context.Context) *prefixSnapshot {
	snapshot := cache.snapshot.Load()
	if snapshot == nil {
		return cache.loadFirst(ctx)
	}

	if !cache.isExpired(snapshot) || !cache.refreshing.CompareAndSwap(false, true) {
		return snapshot
	}

	defer cache.refreshing.Store(false)

	// Another caller may have finished a refresh between Load and CompareAndSwap.
	latest := cache.snapshot.Load()
	if !cache.isExpired(latest) {
		return latest
	}

	return cache.refresh(ctx, latest)
}

func (cache *onLinkPrefixes) loadFirst(ctx context.Context) *prefixSnapshot {
	cache.firstLoad.Lock()
	defer cache.firstLoad.Unlock()

	if snapshot := cache.snapshot.Load(); snapshot != nil {
		return snapshot
	}

	return cache.refresh(ctx, nil)
}

// isExpired reports a snapshot older than the interval. A clock that went backwards counts
// as expired, so the cache never freezes.
func (cache *onLinkPrefixes) isExpired(snapshot *prefixSnapshot) bool {
	age := cache.now().Sub(snapshot.loadedAt)

	return age < 0 || age >= interfacePrefixRefreshInterval
}

// refresh reads the provider and publishes the result. Only the elected caller or the first
// load runs it. A failed read publishes a snapshot too, so a broken provider is asked once
// per interval, not once per request.
func (cache *onLinkPrefixes) refresh(ctx context.Context, previous *prefixSnapshot) *prefixSnapshot {
	loadedAt := cache.now()

	var next *prefixSnapshot

	prefixes, err := cache.provider.InterfacePrefixes()
	if err != nil {
		next = cache.afterFailure(ctx, previous, loadedAt, err)
	} else {
		next = &prefixSnapshot{prefixes: trustedOnLinkPrefixes(prefixes), loadedAt: loadedAt}
	}

	cache.snapshot.Store(next)

	return next
}

// afterFailure keeps the previous prefixes until maxStalePrefixRefreshes consecutive
// refreshes have failed, then drops them.
func (cache *onLinkPrefixes) afterFailure(
	ctx context.Context, previous *prefixSnapshot, loadedAt time.Time, err error,
) *prefixSnapshot {
	next := &prefixSnapshot{loadedAt: loadedAt, failedRefreshes: 1}
	if previous != nil {
		next.prefixes = previous.prefixes
		next.failedRefreshes = previous.failedRefreshes + 1
	}

	switch {
	case next.failedRefreshes == 1:
		cache.logger.WarnContext(ctx, "reading the network interface prefixes failed; the LAN guard keeps the last known IPv6 prefixes for a while",
			slog.Duration("kept_for_at_most", maxStalePrefixRefreshes*interfacePrefixRefreshInterval), logtext.Error(err))
	case next.failedRefreshes == maxStalePrefixRefreshes && len(next.prefixes) > 0:
		cache.logger.WarnContext(ctx, "reading the network interface prefixes keeps failing; the LAN guard dropped the stale IPv6 prefixes",
			logtext.Error(err))
	}

	if next.failedRefreshes >= maxStalePrefixRefreshes {
		next.prefixes = nil
	}

	return next
}
