package scan

import (
	"context"

	"github.com/magpeek/mivo/internal/domain/library"
)

// audiobookJob is one audio file to read. placed is its stored entry, if any; changed is set
// for a new or changed file (and for one whose chapters an earlier scan failed to write),
// which is saved again with its chapters. An unchanged file is read only for the grouping
// of its folder and for the refresh.
type audiobookJob struct {
	state   library.FileState
	placed  *PlacedFileState
	changed bool
}

// audiobookPlan is the difference between the catalog and a completed walk.
type audiobookPlan struct {
	reads    []audiobookJob
	removals []PlacedFileState
}

// planBooks finds the folders with a new, changed or removed file and reads every walked
// file there; the other stored files keep their placement. Stored files the walk did not
// find are removed unless they lie in a path it could not read.
func (scanRun *audiobookRun) planBooks(ctx context.Context, files walked) (audiobookPlan, error) {
	base := scanRun.base

	stored, err := scanRun.catalog.PlacedFiles(ctx, base.library.ID)
	if err != nil {
		return audiobookPlan{}, catalogFailure("load placed files", err)
	}

	storedByKey := make(map[fileKey]PlacedFileState, len(stored))
	for _, file := range stored {
		storedByKey[keyOf(file.State)] = file
	}

	walkedFiles, dirty := scanRun.walkedJobs(files.mediaFiles, storedByKey)

	var plan audiobookPlan

	seen := make(map[fileKey]bool, len(walkedFiles))
	for _, job := range walkedFiles {
		seen[keyOf(job.state)] = true
	}

	for _, file := range stored {
		if !seen[keyOf(file.State)] && !files.skipped.covers(file.State) {
			plan.removals = append(plan.removals, file)
			dirty[keyOf(file.State).directory()] = true
		}
	}

	for _, job := range walkedFiles {
		if dirty[keyOf(job.state).directory()] {
			plan.reads = append(plan.reads, job)

			continue
		}

		scanRun.keepUnread(*job.placed, job.state)
	}

	scanRun.carryOverUnreadable(files.skipped, stored)

	return plan, nil
}

// walkedJobs turns the walked files (first occurrence wins) into jobs and marks the folders
// that hold a new or changed one.
func (scanRun *audiobookRun) walkedJobs(walkedFiles []library.FileState, storedByKey map[fileKey]PlacedFileState) ([]audiobookJob, map[fileKey]bool) {
	retries := scanRun.base.service.chapterRetriesOf(scanRun.base.library.ID)
	dirty := map[fileKey]bool{}
	seen := make(map[fileKey]bool, len(walkedFiles))
	jobs := make([]audiobookJob, 0, len(walkedFiles))

	for _, state := range walkedFiles {
		key := keyOf(state)
		if seen[key] {
			continue
		}

		seen[key] = true
		job := audiobookJob{state: state, changed: true}

		if previous, found := storedByKey[key]; found {
			job.placed = &previous
			_, retry := retries[key]
			job.changed = retry || library.Changed(previous.State, state)
		}

		if job.changed {
			dirty[key.directory()] = true
		}

		jobs = append(jobs, job)
	}

	return jobs, dirty
}

// keepUnread keeps an unchanged file of an unchanged folder with its stored placement.
func (scanRun *audiobookRun) keepUnread(previous PlacedFileState, state library.FileState) {
	previous.State = state
	scanRun.unread = append(scanRun.unread, previous)
	scanRun.chains[keyOf(state)] = previous.ItemIDs
	scanRun.base.tracker.count(func(counts *Counts) { counts.Unchanged++ })
}
