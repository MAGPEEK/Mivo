package enrich

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/magpeek/mivo/internal/domain"
)

// Worker defaults.
const (
	// DefaultConcurrency is the number of jobs enriched at once.
	DefaultConcurrency = 2
	// DefaultLease is how long a claim owns a job; it outlasts any single enrichment, so a
	// lease only runs out when the process died.
	DefaultLease = 10 * time.Minute
	// DefaultPollInterval is how often an idle worker looks for jobs that became due
	// (backoff ended) without an Enqueue waking it.
	DefaultPollInterval = 30 * time.Second
	// finishTimeout bounds recording a job's outcome, which may happen after the run context
	// was cancelled.
	finishTimeout = 5 * time.Second
)

// ItemEnricher enriches one item; implemented by Enricher.
type ItemEnricher interface {
	// EnrichItem looks the item up and stores the result.
	EnrichItem(ctx context.Context, itemID string) (Outcome, error)
}

// WorkerDeps are the ports of a Worker. Queue supplies the wake-ups of Enqueue (nil: the
// worker only polls); a nil Clock means time.Now and a nil Logger discards. Health should be
// the instance the Queue and the caching lookups share; nil gives the worker a private one,
// whose last success no status then sees.
type WorkerDeps struct {
	Jobs     JobStore
	Enricher ItemEnricher
	Health   *Health
	Queue    *Queue
	Clock    func() time.Time
	Logger   *slog.Logger
}

// WorkerOptions tune a Worker; zero values take the defaults above.
type WorkerOptions struct {
	Concurrency  int
	Lease        time.Duration
	PollInterval time.Duration
}

// Worker claims due jobs and enriches their items in the background.
type Worker struct {
	deps    WorkerDeps
	options WorkerOptions
}

// NewWorker returns a worker; it starts nothing until Run.
func NewWorker(deps WorkerDeps, options WorkerOptions) *Worker {
	if deps.Clock == nil {
		deps.Clock = time.Now
	}

	if deps.Health == nil {
		deps.Health = NewHealth(HealthOptions{})
	}

	if deps.Logger == nil {
		deps.Logger = slog.New(slog.DiscardHandler)
	}

	if options.Concurrency <= 0 {
		options.Concurrency = DefaultConcurrency
	}

	if options.Lease <= 0 {
		options.Lease = DefaultLease
	}

	if options.PollInterval <= 0 {
		options.PollInterval = DefaultPollInterval
	}

	return &Worker{deps: deps, options: options}
}

// Run enriches due jobs with the configured concurrency until ctx is cancelled. Running jobs
// see the cancellation and are given back to the queue, so a restart resumes them.
func (worker *Worker) Run(ctx context.Context) {
	var group sync.WaitGroup

	for range worker.options.Concurrency {
		group.Go(func() { worker.loop(ctx) })
	}

	group.Wait()
}

// loop claims and runs jobs until nothing is due, then waits for a wake-up or the poll
// interval.
func (worker *Worker) loop(ctx context.Context) {
	timer := time.NewTimer(worker.options.PollInterval)
	defer timer.Stop()

	for ctx.Err() == nil {
		worked, err := worker.RunOnce(ctx)
		if err != nil && ctx.Err() == nil {
			worker.deps.Logger.WarnContext(ctx, "enrichment queue", "error", err)
		}

		if worked && err == nil {
			continue
		}

		timer.Reset(worker.options.PollInterval)

		select {
		case <-ctx.Done():
		case <-worker.wakeups():
		case <-timer.C:
		}
	}
}

func (worker *Worker) wakeups() <-chan struct{} {
	if worker.deps.Queue == nil {
		return nil // a nil channel never delivers: poll only
	}

	return worker.deps.Queue.wakeups()
}

// RunOnce claims one due job and runs it; worked is false when nothing was due. A job whose
// enrichment failed is rescheduled and is not an error of RunOnce; only queue errors are.
func (worker *Worker) RunOnce(ctx context.Context) (worked bool, err error) {
	job, claimed, err := worker.deps.Jobs.ClaimNextJob(ctx, worker.options.Lease)
	if err != nil {
		return false, fmt.Errorf("claim enrichment job: %w", err)
	}

	if !claimed {
		return false, nil
	}

	outcome, enrichErr := worker.deps.Enricher.EnrichItem(ctx, job.ItemID)

	return true, worker.finish(ctx, job, jobResult{outcome: outcome, err: enrichErr})
}

// jobResult is what EnrichItem returned for a job.
type jobResult struct {
	outcome Outcome
	err     error
}

// finish records the outcome of a job. A lost lease means another worker owns the job now,
// and a failed job that is gone was deleted with its item: neither is an error here.
func (worker *Worker) finish(ctx context.Context, job Job, result jobResult) error {
	err := worker.settle(ctx, job, result)

	switch {
	case err == nil:
		return nil
	case errors.Is(err, domain.ErrConflict), errors.Is(err, domain.ErrNotFound):
		worker.deps.Logger.DebugContext(ctx, "enrichment job no longer ours", "item", job.ItemID, "error", err)

		return nil
	default:
		return fmt.Errorf("finish enrichment of item %s: %w", job.ItemID, err)
	}
}

// settle picks the queue operation for an enrichment outcome. ErrItemGone is checked before
// anything else: the item was deleted, so the job is dropped rather than retried. The write
// runs on a short context of its own: a job that finished (or was interrupted) just as the
// run context was cancelled must still be recorded, else it waits for its lease to run out.
func (worker *Worker) settle(ctx context.Context, job Job, result jobResult) error {
	enrichErr := result.err
	now := worker.deps.Clock()
	interrupted := ctx.Err() != nil

	finishCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), finishTimeout)
	defer cancel()

	switch {
	case enrichErr == nil:
		// A skipped item asked no provider, so it says nothing about the providers' health.
		if result.outcome != OutcomeSkipped {
			worker.deps.Health.recordSuccess(now)
		}

		return worker.deps.Jobs.MarkJobDone(finishCtx, job)
	case errors.Is(enrichErr, ErrItemGone):
		return worker.deps.Jobs.MarkJobDone(finishCtx, job)
	case interrupted:
		return worker.deps.Jobs.ReleaseJob(finishCtx, job)
	case errors.Is(enrichErr, domain.ErrRateLimited):
		return worker.deps.Jobs.MarkJobFailed(finishCtx, job, enrichErr.Error(), now.Add(rateLimitPause(enrichErr)))
	default:
		return worker.deps.Jobs.MarkJobFailed(finishCtx, job, enrichErr.Error(), now.Add(Backoff(job.Attempts)))
	}
}
