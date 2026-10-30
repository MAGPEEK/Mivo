package transcodedir

import (
	"context"
	"errors"
	"fmt"
	"time"
)

const (
	// removeAttempts bounds how often a deletion of locked files is tried.
	removeAttempts = 20
	// removeRetryDelay is the pause between attempts. Windows releases the files of a killed
	// process within a few hundred milliseconds, so removeAttempts × removeRetryDelay leaves
	// ample headroom without stalling a shutdown noticeably.
	removeRetryDelay = 100 * time.Millisecond
)

// retryPolicy says how often and how far apart a deletion is attempted and which errors are
// worth another attempt.
type retryPolicy struct {
	attempts  int
	delay     time.Duration
	retryable func(error) bool
}

func defaultRetryPolicy() retryPolicy {
	return retryPolicy{attempts: removeAttempts, delay: removeRetryDelay, retryable: isLockError}
}

// removeWithRetry calls remove until it succeeds, fails with an error the policy does not
// retry, the attempts are used up or ctx ends while waiting. The first attempt always runs,
// so cleanup during shutdown with an already cancelled context still deletes what it can.
// Only lock errors are retried: a permanent failure such as a missing permission returns at
// once, so a startup purge never stalls on it.
func removeWithRetry(ctx context.Context, policy retryPolicy, remove func() error) error {
	for attempt := 1; ; attempt++ {
		err := remove()
		if err == nil {
			return nil
		}

		if !policy.retryable(err) {
			return err
		}

		if attempt >= policy.attempts {
			return fmt.Errorf("giving up after %d attempts: %w", attempt, err)
		}

		if waitErr := wait(ctx, policy.delay); waitErr != nil {
			return fmt.Errorf("stopped retrying after %d attempts: %w", attempt, errors.Join(waitErr, err))
		}
	}
}

// wait pauses for delay or until ctx ends, whichever comes first.
func wait(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
