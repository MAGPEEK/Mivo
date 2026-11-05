package scan

import (
	"context"
	"fmt"

	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/library"
)

// causeMusicUnconfigured is logged when a music library is scanned without the music ports.
const causeMusicUnconfigured = "music scanning is not configured"

// musicRun is one scan of a music library (ADR 0027 §10). It shares the walk, the probe
// workers, the catalog writes, the sidecars and the refresh with the v2 pipeline, but places
// a file only after reading its tags, so its plan cannot be computed before probing:
//   - new and changed files are probed and their tags read, then placed and saved;
//   - unchanged files (size and modification time) keep their stored placement and are not
//     read, unless an item they belong to has to be refreshed (music_refresh.go);
//   - stored files that are gone are removed.
//
// Unlike the video pipeline, a file without a video stream is no reason to skip it.
type musicRun struct {
	*runV2

	ports MusicPorts
	// unchanged are the stored files the walk found unchanged, with their walked state.
	unchanged []PlacedFileState
}

func (service *Service) newMusicRun(scanned library.ManagedLibrary, scanTracker *tracker) *musicRun {
	scanRun := &musicRun{runV2: service.newRunV2(scanned, scanTracker), ports: service.music}
	scanRun.withoutVideo = map[fileKey]library.FileState{}
	scanRun.beforeRefresh = scanRun.readForRefresh

	return scanRun
}

func (scanRun *musicRun) execute(ctx context.Context) error {
	return scanRun.executeSteps(ctx, scanRun.musicSteps)
}

func (scanRun *musicRun) musicSteps(ctx context.Context) error {
	base := scanRun.base
	if !scanRun.ports.complete() {
		return fail(ErrorCodeFailed, causeMusicUnconfigured,
			fmt.Errorf("scan library %s: %s: %w", base.library.ID, causeMusicUnconfigured, domain.ErrUnavailable))
	}

	base.itemKind = library.ItemKindTrack

	files, err := base.walkAll(ctx)
	if err != nil {
		return err
	}

	scanRun.sidecars = files.sidecars

	plan, err := scanRun.planMusic(ctx, files)
	if err != nil {
		return err
	}

	batch := jobBatch[musicJob, musicResult]{jobs: plan.reads, work: scanRun.readTrack, record: scanRun.recordTrack}
	if err := base.runBatch(ctx, batch.run); err != nil {
		return err
	}

	if err := scanRun.remove(ctx, plan.removals); err != nil {
		return err
	}

	return scanRun.finishSidecars(ctx, files)
}

// musicPlan is the difference between the catalog and a completed walk of a music library.
type musicPlan struct {
	reads    []musicJob
	removals []PlacedFileState
}

// planMusic sorts the walked audio files into reads (new or changed) and unchanged ones;
// stored files the walk did not find are removed unless they lie in a path it could not read.
func (scanRun *musicRun) planMusic(ctx context.Context, files walked) (musicPlan, error) {
	stored, err := scanRun.catalog.PlacedFiles(ctx, scanRun.base.library.ID)
	if err != nil {
		return musicPlan{}, catalogFailure("load placed files", err)
	}

	storedByPath := make(map[fileKey]PlacedFileState, len(stored))
	for _, file := range stored {
		storedByPath[keyOf(file.State)] = file
	}

	var plan musicPlan

	seen := make(map[fileKey]bool, len(files.mediaFiles))

	for _, state := range files.mediaFiles {
		if seen[keyOf(state)] {
			continue
		}

		seen[keyOf(state)] = true
		plan.reads = scanRun.planFile(state, storedByPath, plan.reads)
	}

	for _, file := range stored {
		if !seen[keyOf(file.State)] && !files.skipped.covers(file.State) {
			plan.removals = append(plan.removals, file)
		}
	}

	scanRun.carryOverUnreadable(files.skipped, stored)

	return plan, nil
}

// planFile adds a new or changed file to the reads and remembers an unchanged one.
func (scanRun *musicRun) planFile(state library.FileState, storedByPath map[fileKey]PlacedFileState, reads []musicJob) []musicJob {
	previous, found := storedByPath[keyOf(state)]
	if !found {
		return append(reads, musicJob{state: state})
	}

	if library.Changed(previous.State, state) {
		return append(reads, musicJob{state: state, placed: &previous})
	}

	previous.State = state
	scanRun.unchanged = append(scanRun.unchanged, previous)
	scanRun.chains[keyOf(state)] = previous.ItemIDs
	scanRun.base.tracker.count(func(counts *Counts) { counts.Unchanged++ })

	return reads
}
