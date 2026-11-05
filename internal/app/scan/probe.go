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
func (scanRun *run) probeAll(ctx context.Context, jobs []probeJob, record func(context.Context, probeResult) error) error {
	batch := jobBatch[probeJob, probeResult]{jobs: jobs, work: scanRun.probe, record: record}

	return scanRun.runBatch(ctx, batch.run)
}

// runBatch runs a job batch and reports a cancellation that ended it early: the feeder stops
// handing out jobs when ctx ends, so a cancelled scan may end without any failed result.
func (scanRun *run) runBatch(ctx context.Context, runJobs func(context.Context) error) error {
	if err := runJobs(ctx); err != nil {
		return err
	}

	if err := ctx.Err(); err != nil {
		return fmt.Errorf("probe files of library %s: %w", scanRun.library.ID, err)
	}

	return nil
}

// jobBatch is a set of per-file jobs whose work runs on workers goroutines (zero means
// MaxConcurrentProbes) and whose results are recorded on the calling goroutine. Work must
// take a service-wide slot (probe or photo read) itself.
type jobBatch[Job, Result any] struct {
	jobs    []Job
	work    func(context.Context, Job) Result
	record  func(context.Context, Result) error
	workers int
}

// run works off the jobs. The first error that ends the scan cancels the workers; the
// remaining results are drained, not recorded.
func (batch jobBatch[Job, Result]) run(ctx context.Context) error {
	workContext, cancelWork := context.WithCancel(ctx)
	defer cancelWork()

	var firstErr error

	for result := range batch.start(workContext) {
		if firstErr != nil {
			continue
		}

		if err := batch.record(ctx, result); err != nil {
			firstErr = err

			cancelWork()
		}
	}

	return firstErr
}

// start starts the workers and a feeder. The returned channel is closed after every worker
// has returned, so draining it guarantees that no worker goroutine outlives the scan.
func (batch jobBatch[Job, Result]) start(ctx context.Context) <-chan Result {
	pending := make(chan Job)
	results := make(chan Result)

	var workers sync.WaitGroup

	workerCount := batch.workers
	if workerCount <= 0 {
		workerCount = MaxConcurrentProbes
	}

	for range workerCount {
		workers.Go(func() {
			for job := range pending {
				results <- batch.work(ctx, job)
			}
		})
	}

	go func() {
		feed(ctx, batch.jobs, pending)
		close(pending)
		workers.Wait()
		close(results)
	}()

	return results
}

func feed[Job any](ctx context.Context, jobs []Job, pending chan<- Job) {
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

	result, _ := scanRun.describe(ctx, job)
	if result.outcome != outcomeProbed || result.err != nil {
		return result
	}

	if _, hasVideo := result.description.PrimaryVideo(); !hasVideo {
		return probeResult{job: job, outcome: outcomeNoVideo}
	}

	return result
}

// describe resolves and probes one file without any rule about its streams; the caller holds
// a probe slot. The absolute path is empty when the file could not be resolved.
func (scanRun *run) describe(ctx context.Context, job probeJob) (probeResult, string) {
	if job.state.Layout.IsDisc() {
		return scanRun.describeDisc(ctx, job)
	}

	absolutePath, failure := scanRun.resolve(job)
	if failure != nil {
		return *failure, ""
	}

	description, err := scanRun.service.prober.Probe(ctx, absolutePath)
	if err != nil {
		return probeFailure(ctx, job, err), absolutePath
	}

	return validatedDescription(job, description), absolutePath
}

// validatedDescription is the result of a successful probe, or probe_failed when the prober
// returned a description the domain rejects.
func validatedDescription(job probeJob, description media.Description) probeResult {
	if err := description.Validate(); err != nil {
		return probeResult{job: job, outcome: outcomeProbeFailed, probeError: media.ProbeErrorFailed, reason: err.Error()}
	}

	return probeResult{job: job, outcome: outcomeProbed, description: description}
}

// resolve turns the file's root-relative path into the absolute path inside its library
// folder. A file that cannot be resolved returns the result to record instead.
func (scanRun *run) resolve(job probeJob) (string, *probeResult) {
	root, failure := scanRun.folderRootOf(job)
	if failure != nil {
		return "", failure
	}

	absolutePath, err := scanRun.service.resolver.Resolve(root, job.state.RelativePath)
	if err != nil {
		failure := resolveFailure(job, err)

		return "", &failure
	}

	return absolutePath, nil
}

// acquireProbeSlot waits for one of the service-wide probe slots and returns the function
// that frees it. It gives up when ctx ends.
func (service *Service) acquireProbeSlot(ctx context.Context) (func(), error) {
	return acquireSlot(ctx, service.probeSlots, "probe")
}

// acquirePhotoReadSlot is acquireProbeSlot for the photo metadata reads.
func (service *Service) acquirePhotoReadSlot(ctx context.Context) (func(), error) {
	return acquireSlot(ctx, service.photoReadSlots, "photo read")
}

func acquireSlot(ctx context.Context, slots chan struct{}, purpose string) (func(), error) {
	select {
	case slots <- struct{}{}:
		return func() { <-slots }, nil
	case <-ctx.Done():
		return nil, fmt.Errorf("wait for a %s slot: %w", purpose, ctx.Err())
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
