package scan

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/media"
)

// MaxConcurrentProbes bounds the probes that run at the same time across all scans of a
// Service (ADR 0022): NAS disks slow down sharply when several readers seek at once, and
// StartAll scans every library together.
const MaxConcurrentProbes = 2

// Reasons logged for a file that could not be resolved. The resolver's error text is not
// logged because it may contain the absolute library root.
const (
	reasonNotFound     = "file not found"
	reasonForbidden    = "path leaves the library folder"
	reasonUnresolvable = "path could not be resolved"
)

// outcome classifies the result of one probe job.
type outcome int

const (
	outcomeProbed outcome = iota
	outcomeNoVideo
	outcomeProbeFailed
)

// probeResult is the result of one probe job. A non-nil err ends the scan.
type probeResult struct {
	job         probeJob
	outcome     outcome
	description media.Description
	probeError  media.ProbeError
	reason      string
	err         error
}

// probeAll probes the jobs with at most MaxConcurrentProbes workers and records the results
// on this goroutine, so the catalog is written by one writer per scan. The first error that
// ends the scan cancels the workers; the remaining results are drained, not recorded.
func (scanRun *run) probeAll(ctx context.Context, jobs []probeJob) error {
	workContext, cancelWork := context.WithCancel(ctx)
	defer cancelWork()

	var firstErr error

	for result := range scanRun.startProbes(workContext, jobs) {
		if firstErr != nil {
			continue
		}

		if err := scanRun.record(ctx, result); err != nil {
			firstErr = err

			cancelWork()
		}
	}

	if firstErr != nil {
		return firstErr
	}

	// The feeder stops handing out jobs when ctx ends, so a cancelled scan may end here
	// without any failed result.
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("probe files of library %s: %w", scanRun.library.ID, err)
	}

	return nil
}

// startProbes starts the workers and a feeder. The returned channel is closed after every
// worker has returned, so draining it guarantees that no probe goroutine outlives the scan.
func (scanRun *run) startProbes(ctx context.Context, jobs []probeJob) <-chan probeResult {
	pending := make(chan probeJob)
	results := make(chan probeResult)

	var workers sync.WaitGroup

	for range MaxConcurrentProbes {
		workers.Go(func() {
			for job := range pending {
				results <- scanRun.probe(ctx, job)
			}
		})
	}

	go func() {
		feed(ctx, jobs, pending)
		close(pending)
		workers.Wait()
		close(results)
	}()

	return results
}

func feed(ctx context.Context, jobs []probeJob, pending chan<- probeJob) {
	for _, job := range jobs {
		select {
		case pending <- job:
		case <-ctx.Done():
			return
		}
	}
}

// probe resolves and probes one file in one of the service-wide probe slots and classifies
// the result.
func (scanRun *run) probe(ctx context.Context, job probeJob) probeResult {
	releaseSlot, err := scanRun.service.acquireProbeSlot(ctx)
	if err != nil {
		return probeResult{job: job, err: err}
	}

	defer releaseSlot()

	absolutePath, err := scanRun.service.resolver.Resolve(scanRun.library.Root, job.state.RelativePath)
	if err != nil {
		return resolveFailure(job, err)
	}

	description, err := scanRun.service.prober.Probe(ctx, absolutePath)
	if err != nil {
		return probeFailure(ctx, job, err)
	}

	if err := description.Validate(); err != nil {
		return probeResult{job: job, outcome: outcomeProbeFailed, probeError: media.ProbeErrorFailed, reason: err.Error()}
	}

	if _, hasVideo := description.PrimaryVideo(); !hasVideo {
		return probeResult{job: job, outcome: outcomeNoVideo}
	}

	return probeResult{job: job, outcome: outcomeProbed, description: description}
}

// acquireProbeSlot waits for one of the service-wide probe slots and returns the function
// that frees it. It gives up when ctx ends.
func (service *Service) acquireProbeSlot(ctx context.Context) (func(), error) {
	select {
	case service.probeSlots <- struct{}{}:
		return func() { <-service.probeSlots }, nil
	case <-ctx.Done():
		return nil, fmt.Errorf("wait for a probe slot: %w", ctx.Err())
	}
}

// resolveFailure ends the scan when the root itself is gone. Any other resolve error only
// affects this file: it is stored as probe_failed, so it is listed as unavailable and a
// changed file does not keep its old, playable description.
func resolveFailure(job probeJob, err error) probeResult {
	reason := reasonUnresolvable

	switch {
	case errors.Is(err, domain.ErrUnavailable):
		return probeResult{job: job, err: fail(ErrorCodeRootUnavailable, causeRootUnavailable,
			fmt.Errorf("resolve file: %w", err))}
	case errors.Is(err, domain.ErrNotFound):
		reason = reasonNotFound
	case errors.Is(err, domain.ErrForbidden):
		reason = reasonForbidden
	}

	return probeResult{job: job, outcome: outcomeProbeFailed, probeError: media.ProbeErrorFailed, reason: reason}
}

// probeFailure applies the prober's error contract: a shutdown stops the scan without
// recording anything, a missing probing tool fails the scan instead of marking every file,
// a timeout or any other error is stored as the file's probe error.
func probeFailure(ctx context.Context, job probeJob, err error) probeResult {
	switch {
	case ctx.Err() != nil:
		return probeResult{job: job, err: fmt.Errorf("probe file: %w", ctx.Err())}
	case errors.Is(err, context.Canceled):
		return probeResult{job: job, err: fmt.Errorf("probe file: %w", err)}
	case errors.Is(err, domain.ErrUnavailable):
		return probeResult{job: job, err: fail(ErrorCodeProberUnavailable, causeProberUnavailable,
			fmt.Errorf("probe file: %w", err))}
	case errors.Is(err, context.DeadlineExceeded):
		return probeResult{job: job, outcome: outcomeProbeFailed, probeError: media.ProbeErrorTimedOut, reason: err.Error()}
	default:
		return probeResult{job: job, outcome: outcomeProbeFailed, probeError: media.ProbeErrorFailed, reason: err.Error()}
	}
}
