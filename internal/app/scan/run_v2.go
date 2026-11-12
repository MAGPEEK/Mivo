package scan

import (
	"context"
	"fmt"
	"log/slog"
	"slices"
	"time"

	"github.com/magpeek/mivo/internal/domain/library"
	"github.com/magpeek/mivo/internal/domain/media"
	"github.com/magpeek/mivo/internal/domain/metadata"
)

// walked is what a completed walk found.
type walked struct {
	// mediaFiles are the videos, or the audio files of an audio library.
	mediaFiles []library.FileState
	sidecars   []library.FileState
	// skipped are the paths the walk could not read (unreadable or unavailable).
	skipped skipSet
	// protectedDiscs are the discs skipped because the disc ports are missing (discProtected).
	// Like skipped paths, their stored entries survive; unlike them, they are readable, so
	// they neither keep orphan extras nor block the refresh of items (protectOrphans,
	// carryOverUnreadable).
	protectedDiscs skipSet
}

// kept returns the paths whose stored entries the scan keeps although the walk did not
// report them: the unreadable paths and the discs waiting for the disc ports.
func (files walked) kept() skipSet {
	return files.skipped.with(files.protectedDiscs)
}

// runV2 is one scan of one library with the v2 pipeline (placement, versions, extras,
// sidecars). It reuses the probe workers and the status tracker of run.
type runV2 struct {
	base         *run
	catalog      CatalogV2
	refresh      *refreshSet
	chains       map[fileKey][]string
	withoutVideo map[fileKey]library.FileState
	// optionsChanged is set when the library's options differ from those of its last finished
	// scan: every item is then refreshed once, not only the changed ones.
	optionsChanged bool
	// blocked holds the IDs of items that have stored files in an unreadable or unavailable
	// path; they are not refreshed by this scan but kept pending for the next one.
	blocked map[string]bool
	// refreshed is set once the refresher has been called.
	refreshed bool
	// sidecars are the sidecar files of the completed walk.
	sidecars []library.FileState
	// wanted are the placed media files of the walk; pictures holds the attached-picture stream
	// indexes of the files probed by this scan.
	wanted   []placedFile
	pictures map[fileKey][]int
	// beforeRefresh, when set, runs once the items to refresh are known and before the
	// refresher is called; the music pipeline reads unchanged tracks there (music_refresh.go).
	beforeRefresh func(ctx context.Context) error
	// embedded holds, per item ID, the record of the container tags (RefreshRequest.Embedded).
	embedded map[string]metadata.Record
}

// refreshGrace bounds the refresh of an early-ended scan, which runs after cancellation.
const refreshGrace = 30 * time.Second

func (service *Service) newRunV2(scanned library.ManagedLibrary, scanTracker *tracker) *runV2 {
	return &runV2{
		base: service.newRun(scanned, scanTracker), catalog: service.catalogV2,
		refresh: newRefreshSet(), chains: map[fileKey][]string{},
		pictures:       map[fileKey][]int{},
		optionsChanged: service.optionsChanged(scanned.ID, scanned.Options),
	}
}

// execute walks the root, places every video, applies the difference to the catalog and
// hands the affected items to the refresher. Nothing is removed unless the walk completed.
//
// A scan that ends early still refreshes the items it already touched: saved files are
// unchanged on the next scan, so their new items would otherwise never be refreshed.
func (scanRun *runV2) execute(ctx context.Context) error {
	return scanRun.executeSteps(ctx, scanRun.steps)
}

// executeSteps runs the steps of a pipeline and refreshes what an early end touched.
func (scanRun *runV2) executeSteps(ctx context.Context, steps func(context.Context) error) error {
	err := steps(ctx)
	if err != nil && !scanRun.refreshed && len(scanRun.refresh.ids) > 0 {
		// Best effort on a context that survives cancellation but is bounded by refreshGrace;
		// Close waits for this scan, so it is bounded by the same grace. The scan's own error
		// wins. The refresher gets the walked sidecars: without them it would merge
		// filename-only data over NFO-derived metadata that no later scan repairs. When the
		// walk failed nothing was touched, so this branch is not reached. A pipeline with a
		// beforeRefresh hook (it reads files for the due items) did not read for the items left
		// pending by earlier scans, so they stay pending instead of being refreshed without
		// their files and dropped (BACKLOG, m3-13e review 2).
		graceContext, cancel := context.WithTimeout(context.WithoutCancel(ctx), refreshGrace)
		defer cancel()

		_ = scanRun.refreshTouched(graceContext, scanRun.sidecars, scanRun.beforeRefresh == nil)
	}

	return err
}

func (scanRun *runV2) steps(ctx context.Context) error {
	scanned := scanRun.base.library

	itemKind, err := libraryItemKind(scanned)
	if err != nil {
		return err
	}

	scanRun.base.itemKind = itemKind

	files, err := scanRun.base.walkAll(ctx)
	if err != nil {
		return err
	}

	scanRun.sidecars = files.sidecars

	plan, err := scanRun.plan(ctx, files)
	if err != nil {
		return err
	}

	defer scanRun.base.service.rememberFilesWithoutVideo(scanned.ID, scanRun.withoutVideo)

	if err := scanRun.applyPlan(ctx, plan); err != nil {
		return err
	}

	if err := scanRun.scanSubtitles(ctx, files); err != nil {
		return err
	}

	return scanRun.finishSidecars(ctx, files)
}

// plan loads the catalog, places the walked videos and compares them.
func (scanRun *runV2) plan(ctx context.Context, files walked) (placementPlan, error) {
	base := scanRun.base

	stored, err := scanRun.catalog.PlacedFiles(ctx, base.library.ID)
	if err != nil {
		return placementPlan{}, catalogFailure("load placed files", err)
	}

	placed, ignored := placeVideos(base.library.Kind, files.mediaFiles)

	if !base.library.Options.ScanExtras {
		var extras []library.FileState

		placed, extras = withoutExtras(placed)
		ignored = append(ignored, extras...)
	}

	wanted, orphans := dropOrphanExtras(placed)

	for _, state := range slices.Concat(ignored, orphans) {
		base.logPath(ctx, "scan skipped file that is no catalog entry", state.RelativePath)
	}

	plan := planPlacements(placementInput{
		stored: stored, wanted: wanted, skipped: protectOrphans(files.skipped, orphans).with(files.protectedDiscs),
		withoutVideo:     base.service.filesWithoutVideo(base.library.ID),
		unsupportedDiscs: base.service.unsupportedDiscsOf(base.library.ID),
	})
	scanRun.withoutVideo = plan.withoutVideo
	scanRun.carryOverUnreadable(files.skipped, stored)
	scanRun.chains = plan.chains
	scanRun.wanted = wanted

	base.tracker.count(func(counts *Counts) {
		counts.Unchanged += plan.unchanged
		counts.Skipped += int64(len(plan.withoutVideo) + len(ignored) + len(orphans))
	})

	return plan, nil
}

// carryOverUnreadable handles what lies in unreadable or unavailable paths. Files remembered
// as having no video stream are kept, so they are not probed again when the path returns.
// Items of stored files there are marked blocked: their NFO and artwork sources are not
// visible to this scan, so refreshing them now would overwrite good metadata with
// file-name data.
func (scanRun *runV2) carryOverUnreadable(skipped skipSet, stored []PlacedFileState) {
	if len(skipped) == 0 {
		return
	}

	for key, state := range scanRun.base.service.filesWithoutVideo(scanRun.base.library.ID) {
		if _, present := scanRun.withoutVideo[key]; !present && skipped.covers(state) {
			scanRun.withoutVideo[key] = state
		}
	}

	scanRun.blocked = map[string]bool{}

	for _, file := range stored {
		if skipped.covers(file.State) {
			for _, id := range file.ItemIDs {
				scanRun.blocked[id] = true
			}
		}
	}
}

// protectOrphans keeps stored orphan extras when the walk skipped something: the video that
// owns the extra may lie in the unreadable part, so an orphan is no proof that the extra is
// gone. Without skipped paths the walk was complete and orphans are removed.
func protectOrphans(skipped skipSet, orphans []library.FileState) skipSet {
	if len(skipped) == 0 {
		return skipped
	}

	protected := skipSet{}
	for rootID, paths := range skipped {
		protected[rootID] = slices.Clone(paths)
	}

	for _, orphan := range orphans {
		protected.add(orphan.RootID, orphan.RelativePath)
	}

	return protected
}

// applyPlan moves, saves and only then removes: a renamed file is saved before its old path
// is removed, so the items they share keep their IDs and everything attached to them.
func (scanRun *runV2) applyPlan(ctx context.Context, plan placementPlan) error {
	if err := scanRun.move(ctx, plan.moves); err != nil {
		return err
	}

	if err := scanRun.base.probeAll(ctx, plan.probes, scanRun.record); err != nil {
		return err
	}

	return scanRun.remove(ctx, plan.removals)
}

// finishSidecars stores the sidecar differences and refreshes the affected items. The
// refresh runs first, so a failed refresh is retried by the next scan.
func (scanRun *runV2) finishSidecars(ctx context.Context, files walked) error {
	base := scanRun.base

	storedSidecars, err := scanRun.catalog.SidecarStates(ctx, base.library.ID)
	if err != nil {
		return catalogFailure("load sidecar states", err)
	}

	diff := diffSidecars(storedSidecars, files.sidecars, files.kept())
	scanRun.refresh.addBelowChangedSidecars(diff.changedSidecars(), scanRun.chains)

	if scanRun.optionsChanged {
		for _, chain := range scanRun.chains {
			scanRun.refresh.add(chain...)
		}
	}

	if err := scanRun.prepareAndRefresh(ctx, files.sidecars); err != nil {
		return err
	}

	if len(diff.upserts) == 0 && len(diff.removals) == 0 {
		return nil
	}

	if err := scanRun.catalog.SaveSidecarStates(ctx, base.library.ID, diff.upserts, diff.removals); err != nil {
		return catalogFailure("save sidecar states", err)
	}

	return nil
}

// prepareAndRefresh runs the pipeline's beforeRefresh hook, then the refresh.
func (scanRun *runV2) prepareAndRefresh(ctx context.Context, sidecars []library.FileState) error {
	if scanRun.beforeRefresh != nil {
		if err := scanRun.beforeRefresh(ctx); err != nil {
			return err
		}
	}

	return scanRun.refreshTouched(ctx, sidecars, true)
}

// refreshTouched hands the collected items, and with withPending those left pending by
// earlier scans, to the refresher.
func (scanRun *runV2) refreshTouched(ctx context.Context, sidecars []library.FileState, withPending bool) error {
	scanRun.refreshed = true
	base := scanRun.base
	service := base.service

	var pending []string
	if withPending {
		pending = service.takePendingRefresh(base.library.ID)
	}

	ready, deferred := scanRun.splitBlocked(slices.Concat(scanRun.refresh.sorted(), pending))
	request := RefreshRequest{
		Library: base.library, ItemIDs: ready, Sidecars: sidecars, Files: scanRun.refreshFiles(),
		Embedded: scanRun.embedded,
		CountMetadataWarnings: func(count int) {
			base.tracker.count(func(counts *Counts) { counts.MetadataWarnings += int64(count) })
		},
	}

	if err := service.refresher.RefreshItems(ctx, request); err != nil {
		// Everything that was due stays due.
		service.addPendingRefresh(base.library.ID, slices.Concat(deferred, ready))

		return fail(ErrorCodeFailed, "refresh items", fmt.Errorf("refresh items of library %s: %w", base.library.ID, err))
	}

	service.addPendingRefresh(base.library.ID, deferred)
	scanRun.requestEnrichment(ctx, ready)

	return nil
}

// requestEnrichment queues the refreshed items for online enrichment. A failure only costs
// the provider data until the next scan or refresh, so it is logged and never fails the scan.
func (scanRun *runV2) requestEnrichment(ctx context.Context, itemIDs []string) {
	service := scanRun.base.service
	if service.enrich == nil || len(itemIDs) == 0 {
		return
	}

	if err := service.enrich.EnqueueScanned(ctx, itemIDs); err != nil {
		service.logger.LogAttrs(ctx, slog.LevelWarn, "could not queue online metadata",
			slog.String("library_id", scanRun.base.library.ID), slog.String("error", err.Error()))
	}
}

// splitBlocked separates the due items (sorted, unique) into those that can be refreshed now
// and those that wait until their unavailable path is back.
func (scanRun *runV2) splitBlocked(due []string) (ready, deferred []string) {
	slices.Sort(due)

	for _, id := range slices.Compact(due) {
		if scanRun.blocked[id] {
			deferred = append(deferred, id)
		} else {
			ready = append(ready, id)
		}
	}

	return ready, deferred
}

func (scanRun *runV2) remove(ctx context.Context, removals []PlacedFileState) error {
	if len(removals) == 0 {
		return nil
	}

	ids := make([]string, 0, len(removals))
	for _, file := range removals {
		ids = append(ids, file.ID)
		scanRun.refresh.add(file.ItemIDs...)
	}

	if err := scanRun.catalog.RemovePlacedFiles(ctx, scanRun.base.library.ID, ids); err != nil {
		return catalogFailure(fmt.Sprintf("remove %d files", len(ids)), err)
	}

	scanRun.base.tracker.count(func(counts *Counts) { counts.Removed += int64(len(ids)) })

	return nil
}

func (scanRun *runV2) move(ctx context.Context, moves []moveJob) error {
	for _, job := range moves {
		chain, err := scanRun.catalog.MoveFile(ctx, MoveRequest{
			LibraryID: scanRun.base.library.ID, FileID: job.stored.ID, Placement: job.placement, NewID: scanRun.newID,
		})
		if err != nil {
			return catalogFailure("move "+job.stored.State.RelativePath, err)
		}

		scanRun.refresh.add(job.stored.ItemIDs...)
		scanRun.refresh.add(chain...)
		scanRun.chains[keyOf(job.stored.State)] = chain
		scanRun.base.tracker.count(func(counts *Counts) { counts.Updated++ })
	}

	return nil
}

// newID creates an item ID for the catalog and marks the item for refresh: a new item has
// no metadata yet.
func (scanRun *runV2) newID() (string, error) {
	id, err := scanRun.base.service.ids.NewID()
	if err != nil {
		return "", fmt.Errorf("new item id: %w", err)
	}

	scanRun.refresh.add(id)

	return id, nil
}

// record stores the result of one probe job and counts it.
func (scanRun *runV2) record(ctx context.Context, result probeResult) error {
	if result.err != nil {
		return result.err
	}

	switch result.outcome {
	case outcomeNoVideo:
		return scanRun.recordNoVideo(ctx, result.job)
	case outcomeProbeFailed:
		scanRun.base.logFile(ctx, "scan could not probe file", result.job,
			slog.String("code", string(result.probeError)), slog.String("error", result.reason))

		if err := scanRun.save(ctx, result.job, media.Description{}, result.probeError); err != nil {
			return err
		}

		if result.probeError == media.ProbeErrorDiscUnsupported {
			scanRun.base.service.rememberUnsupportedDisc(scanRun.base.library.ID, result.job.state)
		}

		scanRun.base.tracker.count(func(counts *Counts) { counts.Failed++ })

		return nil
	case outcomeProbed:
		return scanRun.recordProbed(ctx, result)
	default:
		return fmt.Errorf("record %s: unknown outcome %d", result.job.state.RelativePath, result.outcome)
	}
}

func (scanRun *runV2) recordNoVideo(ctx context.Context, job probeJob) error {
	scanRun.base.logFile(ctx, "scan skipped file without a video stream", job)

	if job.placed != nil {
		if err := scanRun.remove(ctx, []PlacedFileState{*job.placed}); err != nil {
			return err
		}
	}

	scanRun.withoutVideo[keyOf(job.state)] = job.state
	scanRun.base.tracker.count(func(counts *Counts) { counts.Skipped++ })

	return nil
}

func (scanRun *runV2) recordProbed(ctx context.Context, result probeResult) error {
	if err := scanRun.save(ctx, result.job, result.description, media.ProbeErrorNone); err != nil {
		return err
	}

	scanRun.base.tracker.count(func(counts *Counts) {
		if result.job.placed == nil {
			counts.Added++
		} else {
			counts.Updated++
		}
	})

	return nil
}

// save writes one probed file with its placement. A changed file keeps its file ID.
func (scanRun *runV2) save(ctx context.Context, job probeJob, description media.Description, probeError media.ProbeError) error {
	base := scanRun.base
	scanRun.pictures[keyOf(job.state)] = attachedPictures(description)

	fileID, err := scanRun.fileID(job)
	if err != nil {
		return fail(ErrorCodeFailed, err.Error(), err)
	}

	now := base.service.clock()
	record := PlacedFile{
		RootID: job.state.RootID, Placement: job.placement,
		File: media.File{
			ID: fileID, LibraryID: base.library.ID,
			RelativePath: job.state.RelativePath, Size: job.state.Size, ModifiedAt: job.state.ModifiedAt,
			Layout: job.state.Layout, Description: description, ProbeError: probeError,
		},
		ProbedAt: now,
	}

	chain, err := scanRun.catalog.SavePlacedFile(ctx, record, scanRun.newID)
	if err != nil {
		return catalogFailure("save "+job.state.RelativePath, err)
	}

	if job.placed != nil {
		scanRun.refresh.add(job.placed.ItemIDs...)
	}

	scanRun.refresh.add(chain...)
	scanRun.chains[keyOf(job.state)] = chain

	return nil
}

func (scanRun *runV2) fileID(job probeJob) (string, error) {
	if job.placed != nil {
		return job.placed.ID, nil
	}

	if job.fileID != "" {
		return job.fileID, nil
	}

	id, err := scanRun.base.service.ids.NewID()
	if err != nil {
		return "", fmt.Errorf("new file id for %s: %w", job.state.RelativePath, err)
	}

	return id, nil
}

// catalogFailure ends the scan with the catalog code; the loggable cause names the step and
// the catalog error, which carries no absolute path.
func catalogFailure(step string, err error) error {
	return fail(ErrorCodeCatalogFailed, step+": "+err.Error(), fmt.Errorf("%s: %w", step, err))
}

func (scanRun *run) logPath(ctx context.Context, message, relativePath string) {
	scanRun.service.logger.LogAttrs(ctx, slog.LevelDebug, message,
		slog.String("library", scanRun.library.ID), slog.String("path", relativePath))
}

func libraryItemKind(scanned library.ManagedLibrary) (library.ItemKind, error) {
	itemKind, err := library.ItemKindForLibrary(scanned.Kind)
	if err != nil {
		return "", fail(ErrorCodeFailed, err.Error(), fmt.Errorf("scan library %s: %w", scanned.ID, err))
	}

	return itemKind, nil
}
