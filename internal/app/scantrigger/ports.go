package scantrigger

import (
	"context"
	"time"

	"github.com/magpeek/mivo/internal/app/scan"
	"github.com/magpeek/mivo/internal/domain/library"
)

// LibraryLister reads the libraries to keep current, with their folders. It is the same
// source the scan service uses (sqlite.ScanLibrarySource).
type LibraryLister interface {
	// GetLibrary returns one library or an error wrapping domain.ErrNotFound.
	GetLibrary(ctx context.Context, libraryID string) (library.ManagedLibrary, error)
	// ListLibraries returns every library.
	ListLibraries(ctx context.Context) ([]library.ManagedLibrary, error)
}

// Scanner starts library scans and reports their state. Implemented by *scan.Service.
type Scanner interface {
	// Start starts a scan of the library unless one runs already (ADR 0022).
	Start(ctx context.Context, libraryID string) (scan.Status, error)
	// Status returns the state of the current or last scan of the library.
	Status(libraryID string) scan.Status
}

// WatchCallbacks receive what one watch reports. They are called from the watching
// goroutine and must not block.
type WatchCallbacks struct {
	// Changed is called for every change below the root that a scan would see. Required.
	Changed func()
	// Ready, when set, is called once the initial watches are in place: changes from then
	// on are reported.
	Ready func()
	// Degraded, when set, receives failures that leave part of the tree unwatched while the
	// watch keeps running (a reached watch limit, lost events).
	Degraded func(err error)
}

// Watcher reports changes below a library root. Implemented by adapters/fs/watcher.
type Watcher interface {
	// Watch reports changes below root through callbacks until ctx is done, then returns
	// nil. It returns an error wrapping domain.ErrUnavailable when the root cannot be
	// watched or disappears (removed, renamed, unmounted).
	Watch(ctx context.Context, root string, callbacks WatchCallbacks) error
}

// Clock reads the time and creates tickers; tests replace it to control time.
type Clock interface {
	// Now returns the current time.
	Now() time.Time
	// NewTicker returns a ticker that ticks every interval until it is stopped.
	NewTicker(interval time.Duration) Ticker
}

// Ticker delivers ticks until it is stopped.
type Ticker interface {
	// Ticks returns the channel that receives the time of each tick.
	Ticks() <-chan time.Time
	// Stop ends the ticks.
	Stop()
}
