package enrich

import (
	"context"
	"fmt"
	"time"

	"github.com/magpeek/mivo/internal/domain"
)

// Reason tells why an item was queued for enrichment.
type Reason string

// Reasons an item can be queued for.
const (
	ReasonScan     Reason = "scan"
	ReasonRefresh  Reason = "refresh"
	ReasonIdentify Reason = "identify"
	ReasonPeriodic Reason = "periodic"
)

// IsValid reports whether reason is one of the known reasons.
func (reason Reason) IsValid() bool {
	switch reason {
	case ReasonScan, ReasonRefresh, ReasonIdentify, ReasonPeriodic:
		return true
	default:
		return false
	}
}

// Job is a claimed queue entry. Revision identifies the request the claim saw, so finishing
// a job keeps a request that came in while it ran; LeaseUntil proves the claim still owns it.
type Job struct {
	ItemID     string
	Reason     Reason
	NotBefore  time.Time
	Attempts   int
	LastError  string
	Revision   int64
	LeaseUntil time.Time
}

// JobEnqueuer persists enrichment requests. Implemented by adapters/sqlite (M6-03).
type JobEnqueuer interface {
	// Enqueue queues an item, due now; a queued item is queued once (dedupe). The error
	// wraps domain.ErrNotFound for an unknown item.
	Enqueue(ctx context.Context, itemID string, reason Reason) error
	// QueueLength returns the number of queued jobs, claimed or not.
	QueueLength(ctx context.Context) (int, error)
}

// JobClaimer leases due jobs and gives them back unfinished.
type JobClaimer interface {
	// ClaimNextJob leases the next due job and reports false when none is due.
	ClaimNextJob(ctx context.Context, lease time.Duration) (Job, bool, error)
	// ReleaseJob makes a claimed job claimable again at once without counting an attempt.
	// The error wraps domain.ErrConflict when another claim owns the job now and
	// domain.ErrNotFound when the job is gone.
	ReleaseJob(ctx context.Context, job Job) error
}

// JobFinisher records how a claimed job ended. Both methods wrap domain.ErrConflict when the
// lease ran out and another worker claimed the job: it is no longer this worker's job.
type JobFinisher interface {
	// MarkJobDone removes the job unless it was requested again while it ran.
	MarkJobDone(ctx context.Context, job Job) error
	// MarkJobFailed counts an attempt and makes the job due again at notBefore. The error
	// wraps domain.ErrNotFound when the job is gone (its item was deleted).
	MarkJobFailed(ctx context.Context, job Job, message string, notBefore time.Time) error
}

// JobStore is what the worker needs of the persisted queue.
type JobStore interface {
	JobClaimer
	JobFinisher
}

// QueueDeps are the ports of a Queue. A nil Settings reports every provider by its health
// only. Health should be shared with the worker and the caching lookups; nil means a
// private one in which every provider stays ok.
type QueueDeps struct {
	Jobs     JobEnqueuer
	Health   *Health
	Settings SettingsSource
}

// Queue accepts enrichment requests and reports the queue's status. Enqueue only writes one
// row and signals the worker, so a scan never waits for the network (ADR 0031 §8).
type Queue struct {
	deps QueueDeps
	wake chan struct{}
}

// NewQueue returns a queue on deps.
func NewQueue(deps QueueDeps) *Queue {
	if deps.Health == nil {
		deps.Health = NewHealth(HealthOptions{})
	}

	return &Queue{deps: deps, wake: make(chan struct{}, 1)}
}

// Enqueue queues an item for enrichment. A repeated request for a queued item does not add
// a second job. The error wraps domain.ErrInvalid for an unknown reason and
// domain.ErrNotFound for an unknown item.
func (queue *Queue) Enqueue(ctx context.Context, itemID string, reason Reason) error {
	if !reason.IsValid() {
		return fmt.Errorf("enqueue item %s: unknown reason %q: %w", itemID, reason, domain.ErrInvalid)
	}

	if err := queue.deps.Jobs.Enqueue(ctx, itemID, reason); err != nil {
		return fmt.Errorf("enqueue item %s: %w", itemID, err)
	}

	queue.signal()

	return nil
}

// signal wakes an idle worker without ever blocking; one pending wake-up is enough because
// a woken worker keeps claiming until nothing is due.
func (queue *Queue) signal() {
	select {
	case queue.wake <- struct{}{}:
	default:
	}
}

// wakeups delivers the signals of Enqueue to the worker.
func (queue *Queue) wakeups() <-chan struct{} {
	return queue.wake
}
