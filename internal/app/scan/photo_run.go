package scan

import (
	"context"
	"fmt"

	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/library"
)

// causePhotosUnconfigured is logged when a photos library is scanned without the photo ports.
const causePhotosUnconfigured = "photo scanning is not configured"

// MaxConcurrentPhotoReads bounds the photo metadata reads that run at the same time across
// all scans of a Service. A read touches only the header of a file, far less than a probe,
// so more of them run at once; NAS disks still slow down with many seeking readers, so the
// number stays small. Videos of a photos library take a probe slot as everywhere else.
const MaxConcurrentPhotoReads = 4

// photoRun is one scan of a photos library (ADR 0029, M3-15). Every folder is an album and
// every image or video a photo; a RAW file joins the JPEG (or HEIC, TIFF) with its stem as an
// alternate file of that photo. The placement depends only on the names in a folder, so it
// is computed for every walked file and compared with the catalog like the video pipeline:
//   - new and changed files are read (EXIF for images, the probe for videos) and saved, and
//     the details of their photo are written;
//   - unchanged files are not read; an unchanged primary whose placement changed (its JPEG
//     is gone) is read again because its item is new, an unchanged alternate is moved;
//   - stored files that are gone are removed with their emptied photos and albums.
//
// Thumbnails are not rendered here; the image endpoint renders them on demand.
type photoRun struct {
	*runV2

	ports      PhotoPorts
	alternates map[fileKey]bool
}

func (service *Service) newPhotoRun(scanned library.ManagedLibrary, scanTracker *tracker) *photoRun {
	scanRun := &photoRun{runV2: service.newRunV2(scanned, scanTracker), ports: service.photo}
	scanRun.withoutVideo = map[fileKey]library.FileState{}

	return scanRun
}

func (scanRun *photoRun) execute(ctx context.Context) error {
	return scanRun.executeSteps(ctx, scanRun.photoSteps)
}

func (scanRun *photoRun) photoSteps(ctx context.Context) error {
	base := scanRun.base
	if !scanRun.ports.complete() {
		return fail(ErrorCodeFailed, causePhotosUnconfigured,
			fmt.Errorf("scan library %s: %s: %w", base.library.ID, causePhotosUnconfigured, domain.ErrUnavailable))
	}

	base.itemKind = library.ItemKindPhoto

	files, err := base.walkAll(ctx)
	if err != nil {
		return err
	}

	scanRun.sidecars = files.sidecars

	plan, err := scanRun.planPhotos(ctx, files)
	if err != nil {
		return err
	}

	// Deferred, so videos found without a video stream are remembered even when the scan
	// ends early and are not probed again by the next one.
	defer base.service.rememberFilesWithoutVideo(base.library.ID, scanRun.withoutVideo)

	if err := scanRun.applyPhotos(ctx, plan); err != nil {
		return err
	}

	return scanRun.finishSidecars(ctx, files)
}

// applyPhotos moves, reads and saves, and only then removes, like applyPlan.
func (scanRun *photoRun) applyPhotos(ctx context.Context, plan photoPlan) error {
	base := scanRun.base

	if err := scanRun.move(ctx, plan.moves); err != nil {
		return err
	}

	batch := jobBatch[photoJob, photoResult]{
		jobs: plan.reads, work: scanRun.readPhoto, record: scanRun.recordPhoto, workers: MaxConcurrentPhotoReads,
	}
	if err := base.runBatch(ctx, batch.run); err != nil {
		return err
	}

	if err := scanRun.remove(ctx, plan.removals); err != nil {
		return err
	}

	for _, removed := range plan.removals {
		base.service.setDetailRetry(base.library.ID, keyOf(removed.State), false)
	}

	return nil
}
