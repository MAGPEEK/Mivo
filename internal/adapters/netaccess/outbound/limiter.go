package outbound

import (
	"context"
	"sync"
	"time"
)

// Rate configures a token bucket: Burst requests at once, then PerSecond sustained.
// A zero PerSecond disables limiting.
type Rate struct {
	PerSecond float64
	Burst     int
}

func (r Rate) capacity() float64 { return float64(max(r.Burst, 1)) }

type bucket struct {
	tokens float64
	last   time.Time
}

// limiter is a per-host token bucket. Waiters reserve a token up front and sleep for the
// deficit, which keeps the lock short and the order roughly first come, first served.
type limiter struct {
	mu       sync.Mutex
	buckets  map[string]*bucket
	defaults Rate
	perHost  map[string]Rate
	now      func() time.Time
	sleep    func(context.Context, time.Duration) error
}

func (l *limiter) rateFor(host string) Rate {
	if rate, ok := l.perHost[host]; ok {
		return rate
	}

	return l.defaults
}

// wait blocks until the host's bucket grants a token or ctx ends.
func (l *limiter) wait(ctx context.Context, host string) error {
	rate := l.rateFor(host)
	if rate.PerSecond <= 0 {
		return ctx.Err()
	}

	delay := l.reserve(host, rate)
	if delay <= 0 {
		return ctx.Err()
	}

	if err := l.sleep(ctx, delay); err != nil {
		l.refund(host, rate)

		return err
	}

	return nil
}

func (l *limiter) reserve(host string, rate Rate) time.Duration {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := l.now()

	b, ok := l.buckets[host]
	if !ok {
		b = &bucket{tokens: rate.capacity(), last: now}
		l.buckets[host] = b
	}

	b.tokens = min(rate.capacity(), b.tokens+now.Sub(b.last).Seconds()*rate.PerSecond)
	b.last = now
	b.tokens--

	if b.tokens >= 0 {
		return 0
	}

	return time.Duration(-b.tokens / rate.PerSecond * float64(time.Second))
}

// idle reports whether the host's bucket would be full at now, so dropping it loses nothing.
func (l *limiter) idle(host string, now time.Time) bool {
	rate := l.rateFor(host)

	l.mu.Lock()
	defer l.mu.Unlock()

	b, ok := l.buckets[host]
	if !ok || rate.PerSecond <= 0 {
		return true
	}

	return b.tokens+now.Sub(b.last).Seconds()*rate.PerSecond >= rate.capacity()
}

// forget drops the host's bucket.
func (l *limiter) forget(host string) {
	l.mu.Lock()
	defer l.mu.Unlock()

	delete(l.buckets, host)
}

func (l *limiter) refund(host string, rate Rate) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if b, ok := l.buckets[host]; ok {
		b.tokens = min(rate.capacity(), b.tokens+1)
	}
}
