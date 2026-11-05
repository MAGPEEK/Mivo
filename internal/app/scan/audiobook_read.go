package scan

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/media"
)

// audiobookResult is what reading one audio file found. probe.err ends the scan; tagErr and
// chapterErr only mean that tags or chapters could not be read.
type audiobookResult struct {
	job        audiobookJob
	probe      probeResult
	tags       media.AudioTags
	duration   time.Duration
	tagErr     error
	chapters   []media.Chapter
	chapterErr error
}

// readParts reads the jobs on the probe workers and collects the results; the first
// result that ends the scan stops the batch.
func (scanRun *audiobookRun) readParts(ctx context.Context, jobs []audiobookJob) ([]audiobookResult, error) {
	results := make([]audiobookResult, 0, len(jobs))
	collect := func(_ context.Context, result audiobookResult) error {
		if result.probe.err != nil {
			return result.probe.err
		}

		results = append(results, result)

		return nil
	}

	batch := jobBatch[audiobookJob, audiobookResult]{jobs: jobs, work: scanRun.readPart, record: collect}
	if err := scanRun.base.runBatch(ctx, batch.run); err != nil {
		return nil, err
	}

	return results, nil
}

// readPart probes one file and, when the probe succeeded, reads its tags and, for a file to
// save, its chapters, all in one probe slot.
func (scanRun *audiobookRun) readPart(ctx context.Context, job audiobookJob) audiobookResult {
	result := audiobookResult{job: job}

	releaseSlot, err := scanRun.base.service.acquireProbeSlot(ctx)
	if err != nil {
		result.probe = probeResult{err: err}

		return result
	}

	defer releaseSlot()

	probed, absolutePath := scanRun.base.describe(ctx, probeJob{state: job.state, placed: job.placed})

	result.probe = probed
	if probed.err != nil || probed.outcome != outcomeProbed {
		return result
	}

	tags, duration, err := scanRun.ports.Tags.ReadAudioTags(ctx, absolutePath)
	if result.probe.err, result.tagErr = toolError(ctx, "read tags", err); result.probe.err != nil {
		return result
	}

	result.tags, result.duration = tags, duration
	if !job.changed {
		return result
	}

	chapters, err := scanRun.ports.Chapters.ReadChapters(ctx, absolutePath)
	result.probe.err, result.chapterErr = toolError(ctx, "read chapters", err)
	result.chapters = chapters

	return result
}

// toolError sorts the error of a tag or chapter read: a cancelled scan and a tool that cannot
// run end the scan (fatal), any other error is a problem of the file (soft).
func toolError(ctx context.Context, step string, err error) (fatal, soft error) {
	switch {
	case ctx.Err() != nil:
		return fmt.Errorf("%s: %w", step, ctx.Err()), nil
	case errors.Is(err, domain.ErrUnavailable):
		return fail(ErrorCodeProberUnavailable, causeProberUnavailable, fmt.Errorf("%s: %w", step, err)), nil
	default:
		return nil, err
	}
}
