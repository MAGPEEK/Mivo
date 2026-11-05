package scan

import "context"

// photoJob is one file of a photos library to read and save.
type photoJob struct {
	probeJob
	// alternate marks the RAW of a pair: it is saved without reading, the details of its
	// photo come from the primary file.
	alternate bool
}

// photoPlan is the difference between the catalog and a completed walk of a photos library.
type photoPlan struct {
	reads    []photoJob
	moves    []moveJob
	removals []PlacedFileState
}

// planPhotos places the walked files and compares them with the catalog (planPlacements).
// Moves of primaries become reads, and unchanged primaries whose details are due again
// (detailRetriesOf) are read as well.
func (scanRun *photoRun) planPhotos(ctx context.Context, files walked) (photoPlan, error) {
	base := scanRun.base

	stored, err := scanRun.catalog.PlacedFiles(ctx, base.library.ID)
	if err != nil {
		return photoPlan{}, catalogFailure("load placed files", err)
	}

	placement := placePhotos(files.mediaFiles)
	for _, state := range placement.ignored {
		base.logPath(ctx, "scan skipped file that is no catalog entry", state.RelativePath)
	}

	plan := planPlacements(placementInput{
		stored: stored, wanted: placement.placed, skipped: files.skipped,
		withoutVideo: base.service.filesWithoutVideo(base.library.ID),
	})
	scanRun.withoutVideo = plan.withoutVideo
	scanRun.carryOverUnreadable(files.skipped, stored)
	scanRun.chains = plan.chains
	scanRun.wanted = placement.placed
	scanRun.alternates = placement.alternates

	photos := photoPlan{removals: plan.removals}
	for _, job := range plan.probes {
		photos.reads = append(photos.reads, photoJob{probeJob: job, alternate: placement.alternates[keyOf(job.state)]})
	}

	for _, move := range plan.moves {
		photos.addMove(move, placement.alternates)
	}

	retried := scanRun.addRetries(&photos, stored)

	base.tracker.count(func(counts *Counts) {
		counts.Unchanged += plan.unchanged - retried
		counts.Skipped += int64(len(plan.withoutVideo) + len(placement.ignored))
	})

	return photos, nil
}

// addMove keeps the move of an alternate; a primary that changes its placement is the
// primary of a new photo item, which needs its details, so it is read and saved instead.
func (plan *photoPlan) addMove(move moveJob, alternates map[fileKey]bool) {
	if alternates[keyOf(move.stored.State)] {
		plan.moves = append(plan.moves, move)

		return
	}

	stored := move.stored
	plan.reads = append(plan.reads, photoJob{probeJob: probeJob{state: stored.State, placement: move.placement, placed: &stored}})
}

// addRetries adds a read for every unchanged primary whose details could not be written by
// an earlier scan and returns how many it added.
func (scanRun *photoRun) addRetries(plan *photoPlan, stored []PlacedFileState) int64 {
	base := scanRun.base

	retries := base.service.detailRetriesOf(base.library.ID)
	if len(retries) == 0 {
		return 0
	}

	storedByPath := make(map[fileKey]PlacedFileState, len(stored))
	for _, file := range stored {
		storedByPath[keyOf(file.State)] = file
	}

	var added int64

	for _, file := range scanRun.wanted {
		key := keyOf(file.state)
		if _, due := retries[key]; !due {
			continue
		}

		previous, found := storedByPath[key]
		_, unchanged := scanRun.chains[key]

		switch {
		case scanRun.alternates[key]:
			base.service.setDetailRetry(base.library.ID, key, false)
		case found && unchanged:
			previous.State = file.state
			plan.reads = append(plan.reads, photoJob{probeJob: probeJob{state: file.state, placement: file.placement, placed: &previous}})
			added++
		}
	}

	return added
}
