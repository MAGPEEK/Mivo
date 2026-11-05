package outbound

import (
	"fmt"
	"testing"
	"time"
)

func TestEvictionPicksLeastRecentlyUsedIdleHost(t *testing.T) {
	t.Parallel()

	clock := newFakeClock()
	client := New(Options{
		Now: clock.Now, Sleep: clock.Sleep,
		DefaultRate: Rate{PerSecond: 1, Burst: 1}, BreakerThreshold: 3,
	})

	for i := range MaxTrackedHosts {
		if _, _, err := client.admit(fmt.Sprintf("h%d.example", i)); err != nil {
			t.Fatal(err)
		}

		clock.Advance(time.Millisecond)
	}

	// h0 is the oldest but has a failure; h1 has a drained bucket; h2 is the oldest idle host.
	client.breakers["h0.example"].failure(clock.Now())
	client.limiter.reserve("h1.example", Rate{PerSecond: 1, Burst: 1})

	if _, _, err := client.admit("new.example"); err != nil {
		t.Fatalf("admit with idle hosts available: %v", err)
	}

	for host, wantKept := range map[string]bool{"h0.example": true, "h1.example": true, "h2.example": false, "h3.example": true} {
		if _, kept := client.breakers[host]; kept != wantKept {
			t.Errorf("%s kept = %v, want %v", host, kept, wantKept)
		}
	}

	if _, ok := client.limiter.buckets["h2.example"]; ok {
		t.Error("evicted host's bucket must be dropped too")
	}
}
