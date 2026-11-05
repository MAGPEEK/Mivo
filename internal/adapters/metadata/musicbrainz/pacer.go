package musicbrainz

import (
	"context"
	"sync"
	"time"
)

// pacer spaces request starts at least interval apart. Callers reserve their slot under the
// lock and sleep outside it, so concurrent lookups queue in arrival order.
type pacer struct {
	mu       sync.Mutex
	next     time.Time
	interval time.Duration
	now      func() time.Time
	sleep    func(ctx context.Context, pause time.Duration) error
}

func newPacer(interval time.Duration, now func() time.Time, sleep func(context.Context, time.Duration) error) *pacer {
	if now == nil {
		now = time.Now
	}

	if sleep == nil {
		sleep = realSleep
	}

	return &pacer{interval: interval, now: now, sleep: sleep}
}

// wait blocks until the caller's slot has come or ctx ends.
func (p *pacer) wait(ctx context.Context) error {
	p.mu.Lock()
	now := p.now()
	start := now

	if p.next.After(now) {
		start = p.next
	}

	p.next = start.Add(p.interval)
	p.mu.Unlock()

	pause := start.Sub(now)
	if pause <= 0 {
		return ctx.Err()
	}

	return p.sleep(ctx, pause)
}

func realSleep(ctx context.Context, pause time.Duration) error {
	timer := time.NewTimer(pause)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
