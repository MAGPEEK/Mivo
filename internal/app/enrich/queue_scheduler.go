package enrich

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/magpeek/mivo/internal/domain"
)

// Cache lifetimes (ADR 0031 §8).
const (
	// CacheTTL is how long a provider answer is used before it is fetched again.
	CacheTTL = 7 * 24 * time.Hour
	// NotFoundTTL is how long a "not found" answer is kept before asking again.
	NotFoundTTL = 30 * 24 * time.Hour
	// refreshBatchLimit bounds one periodic run; the rest is due on the next run.
	refreshBatchLimit = 1000
)

// RefreshSource lists items whose online data is old. Implemented by adapters/sqlite.
type RefreshSource interface {
	// ItemsDueForRefresh returns up to limit items of libraries with online metadata whose
	// last enrichment was before cutoff, oldest first, skipping items whose match is
	// disabled.
	ItemsDueForRefresh(ctx context.Context, cutoff time.Time, limit int) ([]string, error)
}

// Scheduler queues the periodic refresh of items whose cached provider data expired.
type Scheduler struct {
	items RefreshSource
	queue *Queue
}

// NewScheduler returns a scheduler that queues on queue.
func NewScheduler(items RefreshSource, queue *Queue) *Scheduler {
	return &Scheduler{items: items, queue: queue}
}

// RefreshDue queues every item last enriched more than CacheTTL before now and returns how
// many it queued. now comes from the caller, so the periodic task's clock decides.
func (scheduler *Scheduler) RefreshDue(ctx context.Context, now time.Time) (int, error) {
	itemIDs, err := scheduler.items.ItemsDueForRefresh(ctx, now.Add(-CacheTTL), refreshBatchLimit)
	if err != nil {
		return 0, fmt.Errorf("refresh due items: %w", err)
	}

	queued := 0

	for _, itemID := range itemIDs {
		err := scheduler.queue.Enqueue(ctx, itemID, ReasonPeriodic)

		switch {
		case err == nil:
			queued++
		case errors.Is(err, domain.ErrNotFound):
			// deleted since it was listed: nothing to refresh
		default:
			return queued, fmt.Errorf("refresh due items: %w", err)
		}
	}

	return queued, nil
}
