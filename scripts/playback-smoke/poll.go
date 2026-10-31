package main

import (
	"context"
	"fmt"
	"time"
)

// pollSettings controls how the smoke test waits for the server: scans, playlists that are
// not finished yet, and answers with Retry-After.
type pollSettings struct {
	// interval is the pause between two polls without a hint from the server.
	interval time.Duration
	// maxRetryAfter caps a server's Retry-After, so a long hint does not stall the run.
	maxRetryAfter time.Duration
}

// wait pauses for hint (a Retry-After value), capped at maxRetryAfter, or for the interval
// when the hint is 0. It returns early with the context's error when ctx is done.
func (settings pollSettings) wait(ctx context.Context, hint time.Duration) error {
	pause := settings.interval
	if hint > 0 {
		pause = min(hint, settings.maxRetryAfter)
	}

	timer := time.NewTimer(pause)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return fmt.Errorf("stopped waiting: %w", context.Cause(ctx))
	case <-timer.C:
		return nil
	}
}
