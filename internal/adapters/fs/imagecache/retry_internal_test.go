package imagecache

import (
	"context"
	"errors"
	"testing"
	"time"
)

var errLocked = errors.New("locked")

func testPolicy(attempts int) retryPolicy {
	return retryPolicy{
		attempts:  attempts,
		delay:     time.Millisecond,
		retryable: func(err error) bool { return errors.Is(err, errLocked) },
	}
}

func TestWithRetry(t *testing.T) {
	t.Parallel()

	permanent := errors.New("permanent")
	testCases := []struct {
		name      string
		failures  []error
		wantCalls int
		wantErr   error
	}{
		{name: "first attempt succeeds", wantCalls: 1},
		{name: "lock then success", failures: []error{errLocked, errLocked}, wantCalls: 3},
		{name: "permanent error returns at once", failures: []error{permanent}, wantCalls: 1, wantErr: permanent},
		{name: "attempts used up", failures: []error{errLocked, errLocked, errLocked, errLocked}, wantCalls: 3, wantErr: errLocked},
	}

	for _, testCase := range testCases {
		calls := 0
		err := withRetry(t.Context(), testPolicy(3), func() error {
			calls++
			if calls <= len(testCase.failures) {
				return testCase.failures[calls-1]
			}

			return nil
		})

		if calls != testCase.wantCalls || !errors.Is(err, testCase.wantErr) || (err == nil) != (testCase.wantErr == nil) {
			t.Errorf("%s: calls = %d, err = %v; want %d, %v", testCase.name, calls, err, testCase.wantCalls, testCase.wantErr)
		}
	}
}

func TestWithRetryStopsWhenContextEnds(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	err := withRetry(ctx, testPolicy(5), func() error { return errLocked })
	if !errors.Is(err, context.Canceled) || !errors.Is(err, errLocked) {
		t.Fatalf("err = %v, want Canceled and the lock error", err)
	}
}

func TestParseFileName(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		ok   bool
	}{
		{name: "0123456789abcdef-320.jpg", ok: true},
		{name: "0123456789abcdef-1920.png", ok: true},
		{name: "0123456789abcdef-321.jpg"},
		{name: "0123456789abcdef-320.gif"},
		{name: "0123456789abcdef-320"},
		{name: "0123456789abcdef.jpg"},
		{name: "0123456789ABCDEF-320.jpg"},
		{name: "thumbs.db"},
	}

	for _, testCase := range testCases {
		if _, ok := parseFileName(testCase.name); ok != testCase.ok {
			t.Errorf("parseFileName(%q) ok = %v, want %v", testCase.name, ok, testCase.ok)
		}
	}
}
