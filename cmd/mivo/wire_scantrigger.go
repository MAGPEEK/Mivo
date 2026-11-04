package main

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/magpeek/mivo/internal/adapters/fs/watcher"
	"github.com/magpeek/mivo/internal/adapters/sqlite"
	appscan "github.com/magpeek/mivo/internal/app/scan"
	"github.com/magpeek/mivo/internal/app/scantrigger"
)

// newScanTrigger keeps the libraries current: the watcher scans after changes (scan.watch)
// and the periodic scan covers what watching cannot see (scan.interval).
func (app application) newScanTrigger(db *sqlite.DB, scans *appscan.Service) (*scantrigger.Trigger, error) {
	trigger, err := scantrigger.New(scantrigger.Dependencies{
		Libraries: sqlite.NewLibraryRepository(db, time.Now),
		Scanner:   scans,
		Watcher:   watcher.New(),
		Clock:     scantrigger.SystemClock{},
		Logger:    app.logger,
		Settings: scantrigger.Settings{
			Watch:    app.config.Scan.Watch,
			Interval: app.config.Scan.Interval,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("create scan trigger: %w", err)
	}

	return trigger, nil
}

// runner is what startBackground drives until its context is done.
type runner interface {
	Run(ctx context.Context)
}

// startBackground runs the runner on a context derived from ctx and returns a stop function
// that cancels it and waits until Run has returned, so the caller can order the stop before
// closing what the runner uses.
func startBackground(ctx context.Context, work runner) (stop func()) {
	runContext, cancel := context.WithCancel(ctx)

	var group sync.WaitGroup

	group.Go(func() { work.Run(runContext) })

	return func() {
		cancel()
		group.Wait()
	}
}
