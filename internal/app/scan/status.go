package scan

import (
	"sync"
	"time"
)

// State is the state of the current or last scan of a library (API contract ScanState).
type State string

// Scan states.
const (
	// StateIdle means no scan runs; the last one, if any, finished.
	StateIdle State = "idle"
	// StateRunning means a scan runs.
	StateRunning State = "running"
	// StateFailed means the last scan stopped early; ErrorCode says why.
	StateFailed State = "failed"
)

// ErrorCode is the stable, machine-readable reason a scan failed. Clients translate it and
// must tolerate codes added later.
type ErrorCode string

// Scan error codes.
const (
	// ErrorCodeNone is the code of a scan that did not fail.
	ErrorCodeNone ErrorCode = ""
	// ErrorCodeRootUnavailable means the library folder is missing or became unreadable.
	ErrorCodeRootUnavailable ErrorCode = "root_unavailable"
	// ErrorCodeProberUnavailable means the probing tool (ffprobe) cannot run at all.
	ErrorCodeProberUnavailable ErrorCode = "prober_unavailable"
	// ErrorCodeCatalogFailed means the catalog could not be read or written.
	ErrorCodeCatalogFailed ErrorCode = "catalog_failed"
	// ErrorCodeCanceled means the scan was stopped because the server shuts down.
	ErrorCodeCanceled ErrorCode = "scan_canceled"
	// ErrorCodeFailed means the scan failed for another reason.
	ErrorCodeFailed ErrorCode = "scan_failed"
)

// Counts are the file counts of the current or last scan. They grow while a scan runs.
type Counts struct {
	// Added counts new video files that were probed and stored.
	Added int64
	// Updated counts changed video files that were probed again.
	Updated int64
	// Removed counts files that disappeared and were deleted from the catalog.
	Removed int64
	// Unchanged counts files whose size and modification time did not change.
	Unchanged int64
	// Failed counts files that could not be probed or resolved.
	Failed int64
	// Skipped counts files with a video extension but without a video stream.
	Skipped int64
}

// Status is the progress and result of the current or last scan of a library.
type Status struct {
	State State
	// StartedAt is the start of the current or last scan; zero before the first scan.
	StartedAt time.Time
	// FinishedAt is the end of the last scan; zero while a scan runs and before the first
	// scan finished.
	FinishedAt time.Time
	Counts     Counts
	// ErrorCode is ErrorCodeNone unless State is StateFailed.
	ErrorCode ErrorCode
}

// tracker holds the status of one scan. The scan goroutine writes it while Status reads it,
// hence the mutex. done is closed when the scan goroutine has finished, in the same critical
// section that publishes the final status, so "running" and the status never disagree.
type tracker struct {
	mutex  sync.Mutex
	status Status
	done   chan struct{}
}

func newTracker(startedAt time.Time) *tracker {
	return &tracker{
		status: Status{State: StateRunning, StartedAt: startedAt},
		done:   make(chan struct{}),
	}
}

func (scanTracker *tracker) snapshot() Status {
	scanTracker.mutex.Lock()
	defer scanTracker.mutex.Unlock()

	return scanTracker.status
}

// current returns the status and whether the scan goroutine is still running. A scan counts
// as running until done is closed, even while it logs its final line.
func (scanTracker *tracker) current() (Status, bool) {
	scanTracker.mutex.Lock()
	defer scanTracker.mutex.Unlock()

	select {
	case <-scanTracker.done:
		return scanTracker.status, false
	default:
		return scanTracker.status, true
	}
}

// count applies change to the counts of the running scan.
func (scanTracker *tracker) count(change func(*Counts)) {
	scanTracker.mutex.Lock()
	defer scanTracker.mutex.Unlock()

	change(&scanTracker.status.Counts)
}

// final returns the status the scan ends with, without publishing it.
func (scanTracker *tracker) final(code ErrorCode, finishedAt time.Time) Status {
	status := scanTracker.snapshot()

	status.State = StateIdle
	if code != ErrorCodeNone {
		status.State = StateFailed
	}

	status.ErrorCode = code
	status.FinishedAt = finishedAt

	return status
}

// publish stores the final status and marks the scan goroutine as finished. It is the last
// thing a scan goroutine does.
func (scanTracker *tracker) publish(status Status) {
	scanTracker.mutex.Lock()
	defer scanTracker.mutex.Unlock()

	scanTracker.status = status
	close(scanTracker.done)
}
