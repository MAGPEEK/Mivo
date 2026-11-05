package enrich_test

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/magpeek/mivo/internal/app/enrich"
	"github.com/magpeek/mivo/internal/domain"
)

// waitTimeout bounds every wait for a background worker.
const waitTimeout = 5 * time.Second

type workerHarness struct {
	clock    *fakeClock
	jobs     *fakeJobs
	health   *enrich.Health
	queue    *enrich.Queue
	enricher *fakeEnricher
}

func newWorkerHarness() *workerHarness {
	clock := newFakeClock()
	jobs := newFakeJobs(clock)
	health := enrich.NewHealth(enrich.HealthOptions{})

	return &workerHarness{
		clock: clock, jobs: jobs, health: health,
		queue:    enrich.NewQueue(enrich.QueueDeps{Jobs: jobs, Health: health}),
		enricher: &fakeEnricher{},
	}
}

func (h *workerHarness) worker(options enrich.WorkerOptions) *enrich.Worker {
	return enrich.NewWorker(enrich.WorkerDeps{
		Jobs: h.jobs, Enricher: h.enricher, Health: h.health, Queue: h.queue, Clock: h.clock.Now,
	}, options)
}

func (h *workerHarness) runOnce(t *testing.T) bool {
	t.Helper()

	worked, err := h.worker(enrich.WorkerOptions{}).RunOnce(context.Background())
	if err != nil {
		t.Fatalf("RunOnce: %v", err)
	}

	return worked
}

func TestWorkerRunOnceWithoutDueJob(t *testing.T) {
	t.Parallel()

	h := newWorkerHarness()

	if h.runOnce(t) {
		t.Fatal("worked = true on an empty queue")
	}
}

func TestWorkerSuccessRemovesJob(t *testing.T) {
	t.Parallel()

	h := newWorkerHarness()
	mustEnqueue(t, h.queue, "item-a")

	if !h.runOnce(t) {
		t.Fatal("worked = false, want the queued job")
	}

	if h.jobs.length() != 0 {
		t.Fatal("job still queued after success")
	}

	if got := h.health.LastSuccess(); !got.Equal(queueStart()) {
		t.Fatalf("last success = %v, want %v", got, queueStart())
	}
}

func TestWorkerDropsJobOfGoneItem(t *testing.T) {
	t.Parallel()

	h := newWorkerHarness()
	h.enricher.err = fmt.Errorf("load: %w", errors.Join(enrich.ErrItemGone, domain.ErrNotFound))
	mustEnqueue(t, h.queue, "item-a")

	h.runOnce(t)

	if h.jobs.length() != 0 || len(h.jobs.failures) != 0 {
		t.Fatalf("queued = %d, failures = %d; want the job dropped without a retry", h.jobs.length(), len(h.jobs.failures))
	}

	if !h.health.LastSuccess().IsZero() {
		t.Fatal("a gone item counted as a success")
	}
}

func TestWorkerReschedulesFailures(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		err      error
		attempts int
		want     time.Duration
	}{
		{name: "offline first", err: errUnavailable(), want: enrich.BaseBackoff},
		{name: "offline third", err: errUnavailable(), attempts: 2, want: 4 * enrich.BaseBackoff},
		{name: "offline long", err: errUnavailable(), attempts: 30, want: enrich.MaxBackoff},
		{name: "stale reference", err: fmt.Errorf("details: %w", domain.ErrNotFound), want: enrich.BaseBackoff},
		{name: "rate limited", err: fmt.Errorf("search: %w", domain.ErrRateLimited), want: enrich.DefaultRateLimitPause},
		{name: "retry after", err: fmt.Errorf("search: %w", retryAfterErr{after: 90 * time.Second}), want: 90 * time.Second},
		{name: "retry after capped", err: retryAfterErr{after: 48 * time.Hour}, want: enrich.MaxBackoff},
		{name: "retry after zero", err: retryAfterErr{}, want: enrich.DefaultRateLimitPause},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			h := newWorkerHarness()
			h.enricher.err = test.err
			mustEnqueue(t, h.queue, "item-a")
			h.jobs.jobs["item-a"].Attempts = test.attempts

			h.runOnce(t)

			if len(h.jobs.failures) != 1 {
				t.Fatalf("failures = %d, want 1", len(h.jobs.failures))
			}

			got := h.jobs.failures[0]
			if want := queueStart().Add(test.want); !got.notBefore.Equal(want) || got.message != test.err.Error() {
				t.Fatalf("failure = %+v, want not before %v with the error message", got, want)
			}
		})
	}
}

func TestWorkerTreatsLostClaimAsNotMine(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		enrichErr error
		setUp     func(jobs *fakeJobs)
	}{
		{name: "done after lease ran out", setUp: func(jobs *fakeJobs) { jobs.doneErr = conflict() }},
		{name: "failed after lease ran out", enrichErr: errUnavailable(), setUp: func(jobs *fakeJobs) { jobs.failErr = conflict() }},
		{
			name: "failed job deleted with its item", enrichErr: errUnavailable(),
			setUp: func(jobs *fakeJobs) { jobs.failErr = fmt.Errorf("mark: %w", domain.ErrNotFound) },
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			h := newWorkerHarness()
			h.enricher.err = test.enrichErr
			mustEnqueue(t, h.queue, "item-a")
			test.setUp(h.jobs)

			worked, err := h.worker(enrich.WorkerOptions{}).RunOnce(context.Background())
			if !worked || err != nil {
				t.Fatalf("RunOnce = %v, %v; want worked without error", worked, err)
			}
		})
	}
}

func TestWorkerReportsQueueErrors(t *testing.T) {
	t.Parallel()

	h := newWorkerHarness()
	h.jobs.claimErr = errQueueStore

	if _, err := h.worker(enrich.WorkerOptions{}).RunOnce(context.Background()); !errors.Is(err, errQueueStore) {
		t.Fatalf("claim failure: err = %v, want it wrapped", err)
	}

	h = newWorkerHarness()
	h.jobs.doneErr = errQueueStore
	mustEnqueue(t, h.queue, "item-a")

	worked, err := h.worker(enrich.WorkerOptions{}).RunOnce(context.Background())
	if !worked || !errors.Is(err, errQueueStore) {
		t.Fatalf("done failure: RunOnce = %v, %v; want worked with the error", worked, err)
	}
}

func TestWorkerKeepsRequestThatArrivedWhileRunning(t *testing.T) {
	t.Parallel()

	h := newWorkerHarness()
	h.enricher.started = make(chan string, 1)
	h.enricher.proceed = make(chan struct{})
	mustEnqueue(t, h.queue, "item-a")

	done := make(chan error, 1)

	go func() {
		_, err := h.worker(enrich.WorkerOptions{}).RunOnce(context.Background())
		done <- err
	}()

	<-h.enricher.started
	mustEnqueue(t, h.queue, "item-a") // a refresh while the scan job runs
	close(h.enricher.proceed)

	if err := <-done; err != nil {
		t.Fatalf("RunOnce: %v", err)
	}

	if h.jobs.length() != 1 {
		t.Fatal("the newer request was dropped with the finished job")
	}
}

func TestWorkerReleasesJobsOnShutdownAndResumes(t *testing.T) {
	t.Parallel()

	h := newWorkerHarness()
	h.enricher.block = true
	h.enricher.started = make(chan string, 1)
	mustEnqueue(t, h.queue, "item-a")

	ctx, cancel := context.WithCancel(context.Background())
	finished := runInBackground(ctx, h.worker(enrich.WorkerOptions{Concurrency: 1}))

	waitFor(t, h.enricher.started)
	cancel()
	waitDone(t, finished)

	job, queued := h.jobs.job("item-a")
	if !queued || job.Attempts != 0 || len(h.jobs.released) != 1 || h.jobs.releaseCtxErrs[0] != nil {
		t.Fatalf("job = %+v (queued %v), released %v with ctx errs %v; want it given back unpunished on a live context",
			job, queued, h.jobs.released, h.jobs.releaseCtxErrs)
	}

	// Restart: a new worker picks the job up.
	h.enricher.block = false

	if !h.runOnce(t) || h.jobs.length() != 0 {
		t.Fatal("the released job was not resumed")
	}
}

func TestWorkerReleaseFailureIsReported(t *testing.T) {
	t.Parallel()

	h := newWorkerHarness()
	h.enricher.block = true
	h.jobs.releaseErr = errQueueStore
	mustEnqueue(t, h.queue, "item-a")

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := h.worker(enrich.WorkerOptions{}).RunOnce(ctx); !errors.Is(err, errQueueStore) {
		t.Fatalf("err = %v, want the release failure", err)
	}
}

func TestWorkerRunBoundsConcurrency(t *testing.T) {
	t.Parallel()

	const items = 5

	h := newWorkerHarness()
	h.enricher.block = true
	h.enricher.started = make(chan string, items)

	for index := range items {
		mustEnqueue(t, h.queue, fmt.Sprintf("item-%d", index))
	}

	ctx, cancel := context.WithCancel(context.Background())
	finished := runInBackground(ctx, h.worker(enrich.WorkerOptions{PollInterval: time.Millisecond}))

	waitFor(t, h.enricher.started)
	waitFor(t, h.enricher.started)
	cancel()
	waitDone(t, finished)

	if got := h.enricher.peak(); got != enrich.DefaultConcurrency {
		t.Fatalf("peak concurrency = %d, want %d", got, enrich.DefaultConcurrency)
	}

	if h.jobs.length() != items {
		t.Fatalf("queued = %d after shutdown, want all %d kept", h.jobs.length(), items)
	}
}

func TestWorkerRunWakesOnEnqueue(t *testing.T) {
	t.Parallel()

	h := newWorkerHarness()
	h.enricher.started = make(chan string, 1)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// The poll interval is far beyond the test timeout: only the wake-up can start the job.
	finished := runInBackground(ctx, h.worker(enrich.WorkerOptions{Concurrency: 1, PollInterval: time.Hour}))

	mustEnqueue(t, h.queue, "item-a")

	if got := waitFor(t, h.enricher.started); got != "item-a" {
		t.Fatalf("enriched %q, want item-a", got)
	}

	cancel()
	waitDone(t, finished)
}

func TestWorkerRunLogsQueueErrorsAndKeepsGoing(t *testing.T) {
	t.Parallel()

	h := newWorkerHarness()
	h.jobs.claimErr = errQueueStore

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	logs := &countingHandler{}
	worker := enrich.NewWorker(enrich.WorkerDeps{Jobs: h.jobs, Enricher: h.enricher, Health: h.health, Logger: slog.New(logs)},
		enrich.WorkerOptions{Concurrency: 1, PollInterval: time.Millisecond})
	worker.Run(ctx) // returns once the context ends, despite the failing claims

	h.jobs.mutex.Lock()
	claims := h.jobs.claimCalls
	h.jobs.mutex.Unlock()

	if claims < 2 || logs.warnings() < 1 {
		t.Fatalf("claims = %d, warnings = %d; want repeated claims and logged failures", claims, logs.warnings())
	}
}

// A job that finishes just as shutdown cancels the run context must still be recorded, not
// left to wait for its lease.
func TestWorkerRecordsOutcomesOnALiveContextAtShutdown(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		enrichErr error
		wantQueue int
	}{
		{name: "success", wantQueue: 0},
		{name: "gone item", enrichErr: enrich.ErrItemGone, wantQueue: 0},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			h := newWorkerHarness()
			h.enricher.err = test.enrichErr
			mustEnqueue(t, h.queue, "item-a")

			ctx, cancel := context.WithCancel(context.Background())
			h.enricher.onEnrich = cancel // shutdown arrives while the job runs

			if _, err := h.worker(enrich.WorkerOptions{}).RunOnce(ctx); err != nil {
				t.Fatalf("RunOnce: %v", err)
			}

			if h.jobs.length() != test.wantQueue || len(h.jobs.finishCtxErrs) != 1 || h.jobs.finishCtxErrs[0] != nil {
				t.Fatalf("queued = %d, finish ctx errs = %v; want the outcome written on a live context",
					h.jobs.length(), h.jobs.finishCtxErrs)
			}
		})
	}
}

// countingHandler counts warnings; it stands in for the log output.
type countingHandler struct {
	mutex sync.Mutex
	count int
}

func (handler *countingHandler) Enabled(context.Context, slog.Level) bool { return true }

func (handler *countingHandler) Handle(_ context.Context, record slog.Record) error {
	if record.Level >= slog.LevelWarn {
		handler.mutex.Lock()
		handler.count++
		handler.mutex.Unlock()
	}

	return nil
}

func (handler *countingHandler) WithAttrs([]slog.Attr) slog.Handler { return handler }
func (handler *countingHandler) WithGroup(string) slog.Handler      { return handler }

func (handler *countingHandler) warnings() int {
	handler.mutex.Lock()
	defer handler.mutex.Unlock()

	return handler.count
}

func runInBackground(ctx context.Context, worker *enrich.Worker) <-chan struct{} {
	finished := make(chan struct{})

	go func() {
		defer close(finished)
		worker.Run(ctx)
	}()

	return finished
}

func waitFor(t *testing.T, started <-chan string) string {
	t.Helper()

	select {
	case itemID := <-started:
		return itemID
	case <-time.After(waitTimeout):
		t.Fatal("timed out waiting for a job to start")

		return ""
	}
}

func waitDone(t *testing.T, finished <-chan struct{}) {
	t.Helper()

	select {
	case <-finished:
	case <-time.After(waitTimeout):
		t.Fatal("timed out waiting for Run to return")
	}
}

func conflict() error {
	return fmt.Errorf("job is claimed by another worker: %w", domain.ErrConflict)
}
