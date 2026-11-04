package imagecache

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// The retry mirrors transcodedir on purpose: the task keeps the two adapters independent
// until a shared helper is worth its own change.
const (
	// retryAttempts bounds how often a rename or removal of a locked file is tried.
	retryAttempts = 20
	// retryDelay is the pause between attempts. Windows keeps a file locked while a request
	// still reads it; serving a picture takes milliseconds, so 2 s leave ample headroom.
	retryDelay = 100 * time.Millisecond
)

// retryPolicy says how often and how far apart an operation is attempted and which errors
// are worth another attempt.
type retryPolicy struct {
	attempts  int
	delay     time.Duration
	retryable func(error) bool
}

func defaultRetryPolicy() retryPolicy {
	return retryPolicy{attempts: retryAttempts, delay: retryDelay, retryable: isLockError}
}

// withRetry calls operation until it succeeds, fails with an error the policy does not
// retry, the attempts are used up or ctx ends while waiting.
func withRetry(ctx context.Context, policy retryPolicy, operation func() error) error {
	for attempt := 1; ; attempt++ {
		err := operation()
		if err == nil {
			return nil
		}

		if !policy.retryable(err) {
			return err
		}

		if attempt >= policy.attempts {
			return fmt.Errorf("giving up after %d attempts: %w", attempt, err)
		}

		timer := time.NewTimer(policy.delay)
		select {
		case <-ctx.Done():
			timer.Stop()

			return fmt.Errorf("stopped retrying after %d attempts: %w", attempt, errors.Join(ctx.Err(), err))
		case <-timer.C:
		}
	}
}
