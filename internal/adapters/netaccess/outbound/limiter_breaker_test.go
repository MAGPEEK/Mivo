package outbound

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

// fakeClock is a manual clock whose Sleep records requested pauses and advances time.
type fakeClock struct {
	mu     sync.Mutex
	now    time.Time
	sleeps []time.Duration
}

func newFakeClock() *fakeClock { return &fakeClock{now: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)} }

func (f *fakeClock) Now() time.Time {
	f.mu.Lock()
	defer f.mu.Unlock()

	return f.now
}

func (f *fakeClock) Advance(d time.Duration) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.now = f.now.Add(d)
}

func (f *fakeClock) Sleep(ctx context.Context, d time.Duration) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	f.mu.Lock()
	defer f.mu.Unlock()

	f.sleeps = append(f.sleeps, d)
	f.now = f.now.Add(d)

	return nil
}

func (f *fakeClock) Sleeps() []time.Duration {
	f.mu.Lock()
	defer f.mu.Unlock()

	return append([]time.Duration(nil), f.sleeps...)
}

func newTestLimiter(clock *fakeClock, defaults Rate, perHost map[string]Rate) *limiter {
	return &limiter{
		buckets: map[string]*bucket{}, defaults: defaults, perHost: perHost,
		now: clock.Now, sleep: clock.Sleep,
	}
}

func TestLimiterBurstThenSustainedRate(t *testing.T) {
	t.Parallel()

	clock := newFakeClock()
	lim := newTestLimiter(clock, Rate{PerSecond: 1, Burst: 2}, nil)

	for range 2 {
		if err := lim.wait(context.Background(), "a.example"); err != nil {
			t.Fatal(err)
		}
	}

	if got := clock.Sleeps(); len(got) != 0 {
		t.Fatalf("burst slept %v, want no sleeps", got)
	}

	if err := lim.wait(context.Background(), "a.example"); err != nil {
		t.Fatal(err)
	}

	if got := clock.Sleeps(); len(got) != 1 || got[0] != time.Second {
		t.Errorf("sleeps after burst = %v, want [1s]", got)
	}
}

func TestLimiterIsPerHostAndHonoursOverrides(t *testing.T) {
	t.Parallel()

	clock := newFakeClock()
	lim := newTestLimiter(clock, Rate{PerSecond: 1, Burst: 1},
		map[string]Rate{"fast.example": {PerSecond: 20, Burst: 20}, "off.example": {}})

	for range 10 {
		for _, host := range []string{"fast.example", "off.example"} {
			if err := lim.wait(context.Background(), host); err != nil {
				t.Fatal(err)
			}
		}
	}

	if err := lim.wait(context.Background(), "slow.example"); err != nil {
		t.Fatal(err)
	}

	if got := clock.Sleeps(); len(got) != 0 {
		t.Errorf("sleeps = %v, want none: hosts have independent buckets", got)
	}
}

func TestLimiterCancelledWaitRefundsToken(t *testing.T) {
	t.Parallel()

	clock := newFakeClock()
	lim := newTestLimiter(clock, Rate{PerSecond: 1, Burst: 1}, nil)

	if err := lim.wait(context.Background(), "a"); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := lim.wait(ctx, "a"); !errors.Is(err, context.Canceled) {
		t.Fatalf("wait on cancelled ctx = %v, want context.Canceled", err)
	}

	if err := lim.wait(context.Background(), "a"); err != nil {
		t.Fatal(err)
	}

	if got := clock.Sleeps(); len(got) != 1 || got[0] != time.Second {
		t.Errorf("sleeps = %v, want one 1s wait (refund must not double the debt)", got)
	}
}

func TestBreakerLifecycle(t *testing.T) {
	t.Parallel()

	clock := newFakeClock()
	guard := &breaker{threshold: 2, cooldown: time.Minute}

	guard.failure(clock.Now())

	if !guard.allow(clock.Now()) {
		t.Fatal("one failure below threshold must stay closed")
	}

	guard.failure(clock.Now())

	if guard.allow(clock.Now()) {
		t.Fatal("breaker must be open at threshold")
	}

	clock.Advance(time.Minute)

	if !guard.allow(clock.Now()) {
		t.Fatal("first call after cool-down must probe")
	}

	if guard.allow(clock.Now()) {
		t.Fatal("only one probe may run while half-open")
	}

	guard.failure(clock.Now())

	if guard.allow(clock.Now()) {
		t.Fatal("failed probe must reopen the breaker")
	}

	clock.Advance(time.Minute)
	guard.allow(clock.Now())
	guard.abandon()

	if !guard.allow(clock.Now()) {
		t.Fatal("abandoned probe must free the slot")
	}

	guard.success()

	if !guard.allow(clock.Now()) {
		t.Fatal("successful probe must close the breaker")
	}
}
