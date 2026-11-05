package outbound

import (
	"sync"
	"time"
)

// breaker opens after threshold consecutive failures, rejects calls during the cool-down
// and then lets a single probe through (half-open). A successful probe closes it again.
type breaker struct {
	mu        sync.Mutex
	failures  int
	openedAt  time.Time
	probing   bool
	threshold int
	cooldown  time.Duration
}

func (b *breaker) allow(now time.Time) bool {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.failures < b.threshold {
		return true
	}

	if now.Sub(b.openedAt) < b.cooldown || b.probing {
		return false
	}

	b.probing = true

	return true
}

func (b *breaker) success() {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.failures = 0
	b.probing = false
}

func (b *breaker) failure(now time.Time) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.failures++
	if b.failures >= b.threshold {
		b.openedAt = now
	}

	b.probing = false
}

// idle reports a closed breaker without recorded failures, whose state is safe to forget.
func (b *breaker) idle() bool {
	b.mu.Lock()
	defer b.mu.Unlock()

	return b.failures == 0 && !b.probing
}

// abandon releases a half-open probe that ended without a verdict (cancelled caller).
func (b *breaker) abandon() {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.probing = false
}
