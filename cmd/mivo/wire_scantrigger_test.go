package main

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

// recordingRunner notes when Run ends so a test can see whether stop waited for it.
type recordingRunner struct {
	running chan struct{}
	ended   atomic.Bool
}

func (work *recordingRunner) Run(ctx context.Context) {
	close(work.running)
	<-ctx.Done()

	// Ends later than the cancellation, as a trigger that still waits for its watches.
	time.Sleep(50 * time.Millisecond)
	work.ended.Store(true)
}

// TestStartBackgroundStopCancelsAndWaits: the scan trigger must have ended before the scan
// service is closed, or it could start a scan on a closed service.
func TestStartBackgroundStopCancelsAndWaits(t *testing.T) {
	t.Parallel()

	work := &recordingRunner{running: make(chan struct{})}
	stop := startBackground(t.Context(), work)

	<-work.running

	if work.ended.Load() {
		t.Fatal("Run ended before stop")
	}

	stop()

	if !work.ended.Load() {
		t.Error("stop returned before Run ended")
	}
}

func TestStartBackgroundEndsWithTheParentContext(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(t.Context())
	work := &recordingRunner{running: make(chan struct{})}
	stop := startBackground(ctx, work)

	<-work.running
	cancel()
	stop()

	if !work.ended.Load() {
		t.Error("Run did not end with the parent context")
	}
}
