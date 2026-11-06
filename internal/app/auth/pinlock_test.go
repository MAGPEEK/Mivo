package auth

import (
	"testing"
	"time"
)

func (lock *pinLock) size() int {
	lock.mutex.Lock()
	defer lock.mutex.Unlock()

	return len(lock.profiles)
}

func TestPINLockKeepsOnlyRememberedFailures(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.November, 6, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name     string
		outcomes []outcome
		after    time.Duration
		want     int
	}{
		{"success leaves nothing", []outcome{outcomeSucceeded}, 0, 0},
		{"aborted leaves nothing", []outcome{outcomeAborted}, 0, 0},
		{"failure is remembered", []outcome{outcomeFailed}, 0, 1},
		{"forgotten failure is pruned", []outcome{outcomeFailed}, pinFailureMemory + time.Second, 0},
		{"lock is kept while it runs", repeat(outcomeFailed, PINFailuresBeforeLock), PINLockDuration - time.Second, 1},
		{"ended lock is pruned", repeat(outcomeFailed, PINFailuresBeforeLock), PINLockDuration, 0},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			lock := newPINLock()

			for _, result := range test.outcomes {
				attempt, err := lock.begin("profile", now)
				if err != nil {
					t.Fatalf("begin: %v", err)
				}

				attempt.finish(result, now)
			}

			lock.prune(now.Add(test.after))

			if got := lock.size(); got != test.want {
				t.Fatalf("entries = %d, want %d", got, test.want)
			}
		})
	}
}

func TestPINLockPrunesAboveTheThreshold(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.November, 6, 12, 0, 0, 0, time.UTC)
	lock := newPINLock()

	for index := range pruneThreshold + 1 {
		lock.entry(string(rune(index))).count = 1
		lock.profiles[string(rune(index))].last = now
	}

	later := now.Add(pinFailureMemory + time.Second)

	attempt, err := lock.begin("fresh", later)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}

	attempt.finish(outcomeFailed, later)

	if got := lock.size(); got != 1 {
		t.Fatalf("entries after the prune = %d, want only the fresh failure", got)
	}
}

func repeat(result outcome, count int) []outcome {
	outcomes := make([]outcome, count)
	for index := range outcomes {
		outcomes[index] = result
	}

	return outcomes
}
