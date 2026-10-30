package scan

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"path"

	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/library"
	"github.com/magpeek/mivo/internal/domain/media"
)

// run is one scan of one library.
type run struct {
	service  *Service
	library  library.Library
	tracker  *tracker
	itemKind library.ItemKind
	// withoutVideo collects the files of this scan without a video stream. Only the goroutine
	// that records results touches it.
	withoutVideo map[string]library.FileState
}

func (service *Service) newRun(scanned library.Library, scanTracker *tracker) *run {
	return &run{service: service, library: scanned, tracker: scanTracker}
}

// execute walks the root, compares it with the catalog, removes vanished files and probes
// new and changed ones. Nothing is removed unless the walk completed: a lost root or a
// cancelled walk must never empty the catalog.
func (scanRun *run) execute(ctx context.Context) error {
	itemKind, err := library.ItemKindForLibrary(scanRun.library.Kind)
	if err != nil {
		return fail(ErrorCodeFailed, err.Error(), fmt.Errorf("scan library %s: %w", scanRun.library.ID, err))
	}

	scanRun.itemKind = itemKind

	seen, skipped, err := scanRun.walk(ctx)
	if err != nil {
		return err
	}

	stored, err := scanRun.service.catalog.FileStates(ctx, scanRun.library.ID)
	if err != nil {
		return fail(ErrorCodeCatalogFailed, "load file states: "+err.Error(),
			fmt.Errorf("load file states of library %s: %w", scanRun.library.ID, err))
	}

	plan := planChanges(changeInput{
		stored: stored, seen: seen, skipped: skipped,
		withoutVideo: scanRun.service.filesWithoutVideo(scanRun.library.ID),
	})
	scanRun.withoutVideo = plan.withoutVideo

	// Remembered even when the scan stops early, so files kept from earlier scans are not
	// forgotten; files not probed yet are simply probed next time.
	defer scanRun.service.rememberFilesWithoutVideo(scanRun.library.ID, scanRun.withoutVideo)

	scanRun.tracker.count(func(counts *Counts) {
		counts.Unchanged += plan.unchanged
		counts.Skipped += int64(len(plan.withoutVideo))
	})

	if err := scanRun.remove(ctx, plan.removals); err != nil {
		return err
	}

	return scanRun.probeAll(ctx, plan.probes)
}

func (scanRun *run) walk(ctx context.Context) ([]library.FileState, []string, error) {
	var seen []library.FileState

	skipped, err := scanRun.service.walker.Walk(ctx, scanRun.library.Root, func(state library.FileState) error {
		seen = append(seen, state)

		return nil
	})
	if err != nil {
		err = fmt.Errorf("walk library %s: %w", scanRun.library.ID, err)
		if errors.Is(err, domain.ErrUnavailable) {
			return nil, nil, fail(ErrorCodeRootUnavailable, causeRootUnavailable, err)
		}

		return nil, nil, fail(ErrorCodeFailed, causeWalkFailed, err)
	}

	return seen, skipped, nil
}

func (scanRun *run) remove(ctx context.Context, fileIDs []string) error {
	if len(fileIDs) == 0 {
		return nil
	}

	if err := scanRun.service.catalog.RemoveFiles(ctx, scanRun.library.ID, fileIDs); err != nil {
		return fail(ErrorCodeCatalogFailed, fmt.Sprintf("remove %d files: %v", len(fileIDs), err),
			fmt.Errorf("remove %d files of library %s: %w", len(fileIDs), scanRun.library.ID, err))
	}

	scanRun.tracker.count(func(counts *Counts) { counts.Removed += int64(len(fileIDs)) })

	return nil
}

// record stores the result of one probe job and counts it. Per-file problems are logged at
// debug level with the root-relative path only.
func (scanRun *run) record(ctx context.Context, result probeResult) error {
	if result.err != nil {
		return result.err
	}

	switch result.outcome {
	case outcomeNoVideo:
		return scanRun.recordNoVideo(ctx, result.job)
	case outcomeProbeFailed:
		return scanRun.recordProbeFailure(ctx, result)
	case outcomeProbed:
		return scanRun.recordProbed(ctx, result)
	default:
		return fmt.Errorf("record %s: unknown outcome %d", result.job.state.RelativePath, result.outcome)
	}
}

// recordNoVideo skips a file without a video stream and remembers it, so it is not probed
// again while it stays unchanged. A stored file that lost its video stream is removed,
// because it is not a video any more (ADR 0022).
func (scanRun *run) recordNoVideo(ctx context.Context, job probeJob) error {
	scanRun.logFile(ctx, "scan skipped file without a video stream", job)

	if job.stored != nil {
		if err := scanRun.service.catalog.RemoveFiles(ctx, scanRun.library.ID, []string{job.stored.ID}); err != nil {
			return fail(ErrorCodeCatalogFailed, fmt.Sprintf("remove %s: %v", job.state.RelativePath, err),
				fmt.Errorf("remove %s: %w", job.state.RelativePath, err))
		}
	}

	scanRun.withoutVideo[job.state.RelativePath] = job.state
	scanRun.tracker.count(func(counts *Counts) { counts.Skipped++ })

	return nil
}

// recordProbeFailure stores the file with its probe error and no streams, so it is listed
// as unavailable; a changed file loses its old, possibly playable description.
func (scanRun *run) recordProbeFailure(ctx context.Context, result probeResult) error {
	scanRun.logFile(ctx, "scan could not probe file", result.job,
		slog.String("code", string(result.probeError)), slog.String("error", result.reason))

	if err := scanRun.save(ctx, result.job, media.Description{}, result.probeError); err != nil {
		return err
	}

	scanRun.tracker.count(func(counts *Counts) { counts.Failed++ })

	return nil
}

func (scanRun *run) recordProbed(ctx context.Context, result probeResult) error {
	if err := scanRun.save(ctx, result.job, result.description, media.ProbeErrorNone); err != nil {
		return err
	}

	scanRun.tracker.count(func(counts *Counts) {
		if result.job.stored == nil {
			counts.Added++
		} else {
			counts.Updated++
		}
	})

	return nil
}

func (scanRun *run) save(ctx context.Context, job probeJob, description media.Description, probeError media.ProbeError) error {
	record, err := scanRun.newRecord(job, description, probeError)
	if err != nil {
		return err
	}

	if err := scanRun.service.catalog.SaveProbedFile(ctx, record); err != nil {
		return fail(ErrorCodeCatalogFailed, fmt.Sprintf("save %s: %v", job.state.RelativePath, err),
			fmt.Errorf("save %s: %w", job.state.RelativePath, err))
	}

	return nil
}

// newRecord builds the catalog record of a probed file. A changed file keeps its file and
// item IDs; a new file gets new ones.
func (scanRun *run) newRecord(job probeJob, description media.Description, probeError media.ProbeError) (ProbedFile, error) {
	fileID, itemID, err := scanRun.identify(job)
	if err != nil {
		return ProbedFile{}, fail(ErrorCodeFailed, err.Error(), err)
	}

	now := scanRun.service.clock()
	title := library.TitleFromFileName(path.Base(job.state.RelativePath))

	return ProbedFile{
		Item: library.Item{
			ID: itemID, LibraryID: scanRun.library.ID, Kind: scanRun.itemKind,
			Title: title, SortTitle: library.SortTitle(title), CreatedAt: now,
		},
		File: media.File{
			ID: fileID, ItemID: itemID, LibraryID: scanRun.library.ID,
			RelativePath: job.state.RelativePath, Size: job.state.Size, ModifiedAt: job.state.ModifiedAt,
			Description: description, ProbeError: probeError,
		},
		ProbedAt: now,
	}, nil
}

func (scanRun *run) identify(job probeJob) (fileID, itemID string, err error) {
	if job.stored != nil {
		return job.stored.ID, job.stored.ItemID, nil
	}

	if fileID, err = scanRun.service.ids.NewID(); err != nil {
		return "", "", fmt.Errorf("new file id for %s: %w", job.state.RelativePath, err)
	}

	if itemID, err = scanRun.service.ids.NewID(); err != nil {
		return "", "", fmt.Errorf("new item id for %s: %w", job.state.RelativePath, err)
	}

	return fileID, itemID, nil
}

func (scanRun *run) logFile(ctx context.Context, message string, job probeJob, attributes ...slog.Attr) {
	attributes = append([]slog.Attr{
		slog.String("library", scanRun.library.ID), slog.String("path", job.state.RelativePath),
	}, attributes...)

	scanRun.service.logger.LogAttrs(ctx, slog.LevelDebug, message, attributes...)
}
