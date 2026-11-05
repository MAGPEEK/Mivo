package scan

import (
	"context"
	"errors"
	"fmt"
	"path"
	"time"

	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/library"
	"github.com/magpeek/mivo/internal/domain/media"
	"github.com/magpeek/mivo/internal/domain/photo"
)

// Warnings logged instead of the reader's error, which names the absolute path.
const (
	reasonPhotoUnsupported = "format or layout not supported"
	reasonPhotoCorrupt     = "metadata is corrupt"
	reasonPhotoUnreadable  = "metadata could not be read"
)

// photoResult is what reading one file of a photos library found. probe carries the outcome
// of a video probe, or for an image the stored outcome (probed, or failed when it could not
// be resolved); probe.err ends the scan. infoErr only costs the image its metadata.
type photoResult struct {
	job     photoJob
	probe   probeResult
	info    photo.Info
	infoErr error
}

// imageDescription is the description an image is stored with: images are not probed, and
// an unknown container without streams is the valid empty description.
func imageDescription() media.Description {
	return media.Description{Container: media.ContainerUnknown}
}

// readPhoto probes a video (in a probe slot, with the video rule) or reads the metadata of a
// primary image (in a photo read slot). Alternates are not read.
func (scanRun *photoRun) readPhoto(ctx context.Context, job photoJob) photoResult {
	if job.state.Class == library.FileClassVideo {
		return photoResult{job: job, probe: scanRun.base.probe(ctx, job.probeJob)}
	}

	result := photoResult{job: job, probe: probeResult{job: job.probeJob, outcome: outcomeProbed, description: imageDescription()}}
	if job.alternate {
		return result
	}

	releaseSlot, err := scanRun.base.service.acquirePhotoReadSlot(ctx)
	if err != nil {
		result.probe.err = err

		return result
	}

	defer releaseSlot()

	absolutePath, failure := scanRun.base.resolve(job.probeJob)
	if failure != nil {
		result.probe = *failure

		return result
	}

	info, err := scanRun.ports.Metadata.ReadInfo(ctx, absolutePath)

	switch {
	case ctx.Err() != nil:
		result.probe.err = fmt.Errorf("read photo metadata: %w", ctx.Err())
	case err != nil:
		result.infoErr = err
	default:
		result.info = info
	}

	return result
}

// recordPhoto saves one read file and then the details of its photo. The file is marked for
// a details retry until they are written, so an end in between (failure or cancel) leaves it
// to be read again by the next scan.
func (scanRun *photoRun) recordPhoto(ctx context.Context, result photoResult) error {
	if result.probe.err != nil {
		return result.probe.err
	}

	job := result.job
	if result.probe.outcome == outcomeNoVideo {
		return scanRun.recordNoVideo(ctx, job.probeJob)
	}

	if job.alternate {
		return scanRun.saveProbed(ctx, job.probeJob, result.probe)
	}

	fileID, err := scanRun.fileID(job.probeJob)
	if err != nil {
		return fail(ErrorCodeFailed, err.Error(), err)
	}

	job.fileID = fileID
	base := scanRun.base
	key := keyOf(job.state)
	base.service.setDetailRetry(base.library.ID, key, true)

	if err := scanRun.saveProbed(ctx, job.probeJob, result.probe); err != nil {
		return err
	}

	if err := scanRun.saveDetails(ctx, job.probeJob, scanRun.photoDetails(ctx, result)); err != nil {
		return err
	}

	base.service.setDetailRetry(base.library.ID, key, false)

	return nil
}

// saveDetails writes the details of the photo item the saved primary file belongs to.
func (scanRun *photoRun) saveDetails(ctx context.Context, job probeJob, details PhotoDetails) error {
	chain := scanRun.chains[keyOf(job.state)]
	if len(chain) == 0 {
		return fail(ErrorCodeFailed, "save photo details",
			fmt.Errorf("photo %s has no item chain: %w", job.state.RelativePath, domain.ErrInvalid))
	}

	details.ItemID, details.PrimaryFileID = chain[len(chain)-1], job.fileID
	if err := scanRun.ports.Catalog.SavePhotoDetails(ctx, details); err != nil {
		return catalogFailure("save photo details of "+job.state.RelativePath, err)
	}

	return nil
}

// photoDetails derives the details of a saved primary file, without the IDs saveDetails
// adds. The capture time falls back to the modification time as the server's wall clock, so
// every photo sorts; TakenAtSource records which of the two it is.
func (scanRun *photoRun) photoDetails(ctx context.Context, result photoResult) PhotoDetails {
	state := result.job.state
	modified := photo.WallClock(state.ModifiedAt, scanRun.ports.Zone)

	switch {
	case result.probe.outcome == outcomeProbeFailed:
		return PhotoDetails{Info: photo.Info{TakenAt: modified}, TakenAtSource: photo.TakenAtFile, ThumbnailState: photo.ThumbnailUnsupported}
	case state.Class == library.FileClassVideo:
		return PhotoDetails{
			Info: videoInfo(result.probe.description, modified), TakenAtSource: photo.TakenAtFile, ThumbnailState: photo.ThumbnailReady,
		}
	}

	info, err := result.info, result.infoErr
	if err == nil {
		if invalid := info.Validate(); invalid != nil {
			info, err = photo.Info{}, invalid
		}
	}

	unsupported := errors.Is(err, domain.ErrUnsupported)
	thumbnail := photo.ThumbnailStateFor(path.Base(state.RelativePath), unsupported)

	// A format the reader does not parse (a PNG) is no problem of the file unless the photo
	// depends on the reader for its thumbnail (a RAW).
	if err != nil && (!unsupported || thumbnail == photo.ThumbnailUnsupported) {
		scanRun.warn(ctx, "scan could not read photo metadata, using the modification time", state,
			errors.New(metadataReason(err)))
	}

	source := photo.SourceOf(info)
	info.TakenAt = photo.CaptureTime(info, modified)

	return PhotoDetails{Info: info, TakenAtSource: source, ThumbnailState: thumbnail}
}

// videoInfo is what a video of a photos library contributes: the size of its primary video
// stream and the modification time as the capture time.
func videoInfo(description media.Description, modified time.Time) photo.Info {
	info := photo.Info{TakenAt: modified}
	if stream, found := description.PrimaryVideo(); found {
		info.Width, info.Height = stream.Width, stream.Height
	}

	return info
}

func metadataReason(err error) string {
	switch {
	case errors.Is(err, domain.ErrUnsupported):
		return reasonPhotoUnsupported
	case errors.Is(err, domain.ErrInvalid):
		return reasonPhotoCorrupt
	default:
		return reasonPhotoUnreadable
	}
}
