package enrich_test

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/magpeek/mivo/internal/app/enrich"
)

// signallingSource reports every ItemsDueForRefresh call on a channel.
type signallingSource struct {
	calls  chan time.Time
	itemID string
	err    error
}

func (source *signallingSource) ItemsDueForRefresh(_ context.Context, cutoff time.Time, _ int) ([]string, error) {
	source.calls <- cutoff

	if source.err != nil {
		return nil, source.err
	}

	return []string{source.itemID}, nil
}

// lockedBuffer is a log sink safe for the periodic goroutine.
type lockedBuffer struct {
	mutex  sync.Mutex
	buffer bytes.Buffer
}

func (sink *lockedBuffer) Write(data []byte) (int, error) {
	sink.mutex.Lock()
	defer sink.mutex.Unlock()

	return sink.buffer.Write(data)
}

func (sink *lockedBuffer) String() string {
	sink.mutex.Lock()
	defer sink.mutex.Unlock()

	return sink.buffer.String()
}

func runPeriodic(t *testing.T, source *signallingSource, logs *lockedBuffer) (stop func()) {
	t.Helper()

	jobs := newFakeJobs(newFakeClock())
	queue := enrich.NewQueue(enrich.QueueDeps{Jobs: jobs})
	periodic := enrich.NewScheduler(source, queue).Periodic(enrich.PeriodicOptions{
		Interval: 5 * time.Millisecond, Clock: queueStart, Logger: slog.New(slog.NewTextHandler(logs, nil)),
	})

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})

	go func() {
		periodic.Run(ctx)
		close(done)
	}()

	return func() {
		cancel()

		// Drain a call that raced the cancellation so Run can return.
		for {
			select {
			case <-source.calls:
			case <-done:
				return
			}
		}
	}
}

func TestPeriodicRefreshRunsAtStartAndThenEveryInterval(t *testing.T) {
	t.Parallel()

	source := &signallingSource{calls: make(chan time.Time), itemID: "item-a"}
	logs := &lockedBuffer{}
	stop := runPeriodic(t, source, logs)

	for call := range 2 {
		select {
		case cutoff := <-source.calls:
			if !cutoff.Equal(queueStart().Add(-enrich.CacheTTL)) {
				t.Errorf("call %d cutoff = %v, want the clock minus the cache TTL", call, cutoff)
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("call %d did not happen", call)
		}
	}

	stop()

	if !strings.Contains(logs.String(), "periodic metadata refresh queued items") {
		t.Errorf("logs = %q, want the queued items logged", logs.String())
	}
}

func TestPeriodicRefreshLogsFailures(t *testing.T) {
	t.Parallel()

	source := &signallingSource{calls: make(chan time.Time), err: errQueueStore}
	logs := &lockedBuffer{}
	stop := runPeriodic(t, source, logs)

	select {
	case <-source.calls:
	case <-time.After(5 * time.Second):
		t.Fatal("RefreshDue did not run")
	}

	// The second call starts after the first one logged.
	select {
	case <-source.calls:
	case <-time.After(5 * time.Second):
		t.Fatal("RefreshDue did not run again")
	}

	stop()

	if !strings.Contains(logs.String(), "periodic metadata refresh failed") {
		t.Errorf("logs = %q, want the failure logged", logs.String())
	}
}

func TestPeriodicDefaults(t *testing.T) {
	t.Parallel()

	source := &signallingSource{calls: make(chan time.Time, 1), itemID: "item-a"}
	periodic := enrich.NewScheduler(source, enrich.NewQueue(enrich.QueueDeps{Jobs: newFakeJobs(newFakeClock())})).
		Periodic(enrich.PeriodicOptions{})

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})

	go func() {
		periodic.Run(ctx)
		close(done)
	}()

	<-source.calls
	cancel()
	<-done
}
