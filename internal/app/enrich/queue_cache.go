package enrich

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/magpeek/mivo/internal/domain"
)

// CacheDeps are the ports of the caching lookups. Health sees the outcome of every provider
// call and should be the instance the Queue reports (nil means a private one); a nil Clock
// means time.Now and a nil Logger discards.
type CacheDeps struct {
	Cache  ProviderCache
	Health *Health
	Clock  func() time.Time
	Logger *slog.Logger
}

// readThrough serves provider answers from the cache (ADR 0031 §8): a fresh entry without a
// call, a missing or expired one by asking the provider, and an expired one again when the
// provider is unreachable, rate limited or disabled. That keeps enrichment working offline.
type readThrough struct {
	deps     CacheDeps
	provider ProviderName
}

func newReadThrough(deps CacheDeps, provider ProviderName) readThrough {
	if deps.Clock == nil {
		deps.Clock = time.Now
	}

	if deps.Logger == nil {
		deps.Logger = slog.New(slog.DiscardHandler)
	}

	if deps.Health == nil {
		deps.Health = NewHealth(HealthOptions{})
	}

	return readThrough{deps: deps, provider: provider}
}

// cachedAnswer is a decoded cache entry.
type cachedAnswer[T any] struct {
	value     T
	notFound  bool
	expiresAt time.Time
}

func (answer cachedAnswer[T]) result(key string) (T, error) {
	if answer.notFound {
		var zero T

		return zero, fmt.Errorf("cached answer %q: %w", key, domain.ErrNotFound)
	}

	return answer.value, nil
}

// codec turns a provider answer into the cache's JSON payload and back.
type codec[T any] struct {
	encode func(T) ([]byte, error)
	decode func([]byte) (T, error)
}

// cacheSlot is one cache key of one provider and the codec of its answers.
type cacheSlot[T any] struct {
	through readThrough
	key     string
	coder   codec[T]
}

func slotOf[T any](through readThrough, key string, coder codec[T]) cacheSlot[T] {
	return cacheSlot[T]{through: through, key: key, coder: coder}
}

// cachedCall answers the slot from the cache or by fetch, following readThrough.
func cachedCall[T any](ctx context.Context, slot cacheSlot[T], fetch func(context.Context) (T, error)) (T, error) {
	through := slot.through
	answer, cached := slot.read(ctx)

	if cached && through.deps.Clock().Before(answer.expiresAt) {
		return answer.result(slot.key)
	}

	if !through.deps.Health.Usable(through.provider) {
		if cached {
			return answer.result(slot.key)
		}

		var zero T

		return zero, fmt.Errorf("%s: %w", through.provider, errProviderDisabled)
	}

	value, err := fetch(ctx)
	through.deps.Health.Observe(through.provider, err)

	switch {
	case err == nil:
		slot.write(ctx, value)

		return value, nil
	case errors.Is(err, domain.ErrNotFound):
		through.writeEntry(ctx, slot.key, CacheEntry{NotFound: true}, NotFoundTTL)
	case cached && servesStale(err):
		return answer.result(slot.key)
	}

	var zero T

	return zero, err
}

// servesStale reports whether a failed call falls back to an expired entry.
func servesStale(err error) bool {
	return errors.Is(err, domain.ErrUnavailable) || errors.Is(err, domain.ErrRateLimited)
}

// read returns the decoded entry; a read error or an undecodable payload counts as a miss,
// because the cache only saves calls and must never fail enrichment.
func (slot cacheSlot[T]) read(ctx context.Context) (cachedAnswer[T], bool) {
	through := slot.through

	entry, err := through.deps.Cache.ReadProviderCache(ctx, string(through.provider), slot.key)
	if err != nil {
		if !errors.Is(err, domain.ErrNotFound) {
			through.deps.Logger.WarnContext(ctx, "read provider cache", "provider", through.provider, "error", err)
		}

		return cachedAnswer[T]{}, false
	}

	answer := cachedAnswer[T]{notFound: entry.NotFound, expiresAt: entry.ExpiresAt}
	if entry.NotFound {
		return answer, true
	}

	value, err := slot.coder.decode(entry.Payload)
	if err != nil {
		through.deps.Logger.WarnContext(ctx, "decode provider cache", "provider", through.provider, "error", err)

		return cachedAnswer[T]{}, false
	}

	answer.value = value

	return answer, true
}

// write caches a successful answer for CacheTTL.
func (slot cacheSlot[T]) write(ctx context.Context, value T) {
	payload, err := slot.coder.encode(value)
	if err != nil {
		slot.through.deps.Logger.WarnContext(ctx, "encode provider cache", "provider", slot.through.provider, "error", err)

		return
	}

	slot.through.writeEntry(ctx, slot.key, CacheEntry{Payload: payload}, CacheTTL)
}

// writeEntry stores entry with the given lifetime; a failed write only costs a later call.
func (through readThrough) writeEntry(ctx context.Context, key string, entry CacheEntry, ttl time.Duration) {
	now := through.deps.Clock()
	entry.FetchedAt = now
	entry.ExpiresAt = now.Add(ttl)

	if err := through.deps.Cache.WriteProviderCache(ctx, string(through.provider), key, entry); err != nil {
		through.deps.Logger.WarnContext(ctx, "write provider cache", "provider", through.provider, "error", err)
	}
}
