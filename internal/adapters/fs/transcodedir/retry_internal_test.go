package transcodedir

import (
	"context"
	"errors"
	"testing"
	"time"
)

var (
	errStillLocked = errors.New("file still locked")
	errPermanent   = errors.New("permission denied for good")
)

// failingRemoval fails the first failures calls with failure, then succeeds, and counts all
// calls.
type failingRemoval struct {
	failures int
	failure  error
	calls    int
}

func (removal *failingRemoval) remove() error {
	removal.calls++
	if removal.calls <= removal.failures {
		return removal.failure
	}

	return nil
}

func retryOnlyLocks(err error) bool {
	return errors.Is(err, errStillLocked)
}

func TestRemoveWithRetry(t *testing.T) {
	t.Parallel()

	const attempts = 4

	testCases := []struct {
		name      string
		failures  int
		failure   error
		wantCalls int
		wantErr   error
	}{
		{name: "first attempt succeeds", failures: 0, wantCalls: 1},
		{name: "succeeds after retries", failures: attempts - 1, failure: errStillLocked, wantCalls: attempts},
		{
			name: "gives up after the last attempt", failures: attempts, failure: errStillLocked,
			wantCalls: attempts, wantErr: errStillLocked,
		},
		{
			name: "returns a permanent error at once", failures: attempts, failure: errPermanent,
			wantCalls: 1, wantErr: errPermanent,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			removal := &failingRemoval{failures: testCase.failures, failure: testCase.failure}
			policy := retryPolicy{attempts: attempts, delay: time.Millisecond, retryable: retryOnlyLocks}

			err := removeWithRetry(t.Context(), policy, removal.remove)
			if !errors.Is(err, testCase.wantErr) || (testCase.wantErr == nil) != (err == nil) {
				t.Errorf("removeWithRetry error = %v, want %v", err, testCase.wantErr)
			}

			if removal.calls != testCase.wantCalls {
				t.Errorf("calls = %d, want %d", removal.calls, testCase.wantCalls)
			}
		})
	}
}

func TestRemoveWithRetryStopsWaitingWhenTheContextEnds(t *testing.T) {
	t.Parallel()

	removal := &failingRemoval{failures: 2, failure: errStillLocked}
	policy := retryPolicy{attempts: 3, delay: time.Hour, retryable: retryOnlyLocks}

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	err := removeWithRetry(ctx, policy, removal.remove)
	if !errors.Is(err, context.Canceled) || !errors.Is(err, errStillLocked) {
		t.Errorf("removeWithRetry error = %v, want context.Canceled and the removal error", err)
	}

	if removal.calls != 1 {
		t.Errorf("calls = %d, want 1 (the first attempt runs even with a cancelled context)", removal.calls)
	}
}

func TestDefaultRetryPolicyIsBounded(t *testing.T) {
	t.Parallel()

	// A removal may hold up a session stop or the server shutdown at most this long.
	const maxTotalRemoveWait = 5 * time.Second

	policy := defaultRetryPolicy()
	if policy.attempts < 2 || policy.delay <= 0 || policy.retryable == nil {
		t.Errorf("defaultRetryPolicy = %+v, want at least one retry with a positive delay", policy)
	}

	if total := time.Duration(policy.attempts) * policy.delay; total > maxTotalRemoveWait {
		t.Errorf("total wait %v exceeds %v", total, maxTotalRemoveWait)
	}
}

func TestRemoveEntriesStopsBeforeTheNextEntryWhenTheContextEnds(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	var removedNames []string

	removeEntry := func(name string) error {
		removedNames = append(removedNames, name)
		cancel() // The context ends while the first entry is being removed.

		return nil
	}

	count, err := removeEntries(ctx, []string{"first", "second", "third"}, removeEntry)
	if !errors.Is(err, context.Canceled) || count != 1 {
		t.Errorf("removeEntries = (%d, %v), want (1, context.Canceled)", count, err)
	}

	if len(removedNames) != 1 || removedNames[0] != "first" {
		t.Errorf("removed %v, want only [first]", removedNames)
	}
}

func TestRemoveEntriesContinuesAfterAFailure(t *testing.T) {
	t.Parallel()

	removeEntry := func(name string) error {
		if name == "broken" {
			return errPermanent
		}

		return nil
	}

	count, err := removeEntries(t.Context(), []string{"first", "broken", "third"}, removeEntry)
	if !errors.Is(err, errPermanent) || count != 2 {
		t.Errorf("removeEntries = (%d, %v), want (2, errPermanent)", count, err)
	}
}

func TestIsSessionID(t *testing.T) {
	t.Parallel()

	testCases := map[string]bool{
		"0b8c6d4e-3f2a-4c1b-9d8e-7f6a5b4c3d2e":          true,
		"00000000-0000-0000-0000-000000000000":          true,
		"ffffffff-ffff-ffff-ffff-ffffffffffff":          true,
		"FFFFFFFF-FFFF-FFFF-FFFF-FFFFFFFFFFFF":          false,
		"0B8C6D4E-3F2A-4C1B-9D8E-7F6A5B4C3D2E":          false,
		"0b8c6d4e3f2a4c1b9d8e7f6a5b4c3d2e":              false,
		"{0b8c6d4e-3f2a-4c1b-9d8e-7f6a5b4c3d2e}":        false,
		"urn:uuid:0b8c6d4e-3f2a-4c1b-9d8e-7f6a5b4c3d2e": false,
		"0b8c6d4e-3f2a-4c1b-9d8e-7f6a5b4c3d2":           false,
		"":                                              false,
		"..":                                            false,
	}

	for text, want := range testCases {
		if got := isSessionID(text); got != want {
			t.Errorf("isSessionID(%q) = %v, want %v", text, got, want)
		}
	}
}
