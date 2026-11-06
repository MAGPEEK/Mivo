package enrich

import (
	"context"
	"log/slog"
	"time"
)

// RefreshInterval is the cadence of the periodic refresh (ADR 0031 §8): once a day it queues
// the items whose provider data is older than CacheTTL.
const RefreshInterval = 24 * time.Hour

// PeriodicOptions tune a PeriodicRefresh. A zero Interval means RefreshInterval, a nil Clock
// time.Now and a nil Logger discards.
type PeriodicOptions struct {
	Interval time.Duration
	Clock    func() time.Time
	Logger   *slog.Logger
}

// PeriodicRefresh runs the scheduler's RefreshDue once at start and then every Interval.
type PeriodicRefresh struct {
	scheduler *Scheduler
	options   PeriodicOptions
}

// Periodic returns the periodic task of the scheduler; it starts nothing until Run.
func (scheduler *Scheduler) Periodic(options PeriodicOptions) *PeriodicRefresh {
	if options.Interval <= 0 {
		options.Interval = RefreshInterval
	}

	if options.Clock == nil {
		options.Clock = time.Now
	}

	if options.Logger == nil {
		options.Logger = slog.New(slog.DiscardHandler)
	}

	return &PeriodicRefresh{scheduler: scheduler, options: options}
}

// Run queues the due items now and after every Interval until ctx is done. It only writes
// queue rows; the worker does the lookups.
func (periodic *PeriodicRefresh) Run(ctx context.Context) {
	ticker := time.NewTicker(periodic.options.Interval)
	defer ticker.Stop()

	for {
		periodic.refresh(ctx)

		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (periodic *PeriodicRefresh) refresh(ctx context.Context) {
	queued, err := periodic.scheduler.RefreshDue(ctx, periodic.options.Clock())
	if err != nil {
		if ctx.Err() == nil {
			periodic.options.Logger.LogAttrs(ctx, slog.LevelWarn, "periodic metadata refresh failed",
				slog.String("error", err.Error()))
		}

		return
	}

	if queued > 0 {
		periodic.options.Logger.LogAttrs(ctx, slog.LevelInfo, "periodic metadata refresh queued items",
			slog.Int("items", queued))
	}
}
