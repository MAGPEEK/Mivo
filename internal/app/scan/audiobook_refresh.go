package scan

import (
	"context"
	"maps"
)

// readForRefresh reads the files of unchanged folders that belong to a book due for a
// refresh (a changed sidecar, a changed library option, a refresh left pending): without
// their tags the refresher would not know the book's files, author and narrator and would
// merge the people away. Every file of such a book is read, as the cover may come from the
// first part's picture and the author from the first tagged part. A rescan without changes
// reads nothing.
func (scanRun *audiobookRun) readForRefresh(ctx context.Context) error {
	jobs := scanRun.refreshReads()
	if len(jobs) == 0 {
		return nil
	}

	results, err := scanRun.readParts(ctx, jobs)
	if err != nil {
		return err
	}

	byRoot := map[string]map[string]audiobookResult{}

	for _, result := range results {
		scanRun.read[keyOf(result.job.state)] = true

		if result.probe.outcome != outcomeProbed || result.tagErr != nil {
			scanRun.base.logPath(ctx, "scan left an unreadable unchanged file out of the refresh", result.job.state.RelativePath)

			continue
		}

		addByRoot(byRoot, result)
	}

	for _, grouped := range scanRun.groupRoots(byRoot) {
		scanRun.rememberRefreshBook(ctx, grouped)
	}

	return nil
}

// refreshReads lists the unread files whose book is due, including books left pending by an
// earlier scan (BACKLOG, m3-13e notes: without their files the refresher would drop them).
func (scanRun *audiobookRun) refreshReads() []audiobookJob {
	due := maps.Clone(scanRun.refresh.ids)
	for _, id := range scanRun.base.service.peekPendingRefresh(scanRun.base.library.ID) {
		due[id] = struct{}{}
	}

	var jobs []audiobookJob

	for _, file := range scanRun.unread {
		if len(file.ItemIDs) == 0 || scanRun.read[keyOf(file.State)] {
			continue
		}

		if _, isDue := due[file.ItemIDs[0]]; isDue {
			jobs = append(jobs, audiobookJob{state: file.State, placed: &file})
		}
	}

	return jobs
}

// rememberRefreshBook makes the read files of a due book known to the refresher. A file is
// only used when its tags still lead to its stored placement.
func (scanRun *audiobookRun) rememberRefreshBook(ctx context.Context, grouped groupedBook) {
	for index, part := range grouped.parts {
		placement := grouped.book.PartPlacement(index)
		state := part.job.state

		if placement.Key() != part.job.placed.PlacementKey {
			scanRun.base.logPath(ctx, "scan left an unchanged file out of the refresh", state.RelativePath)

			continue
		}

		scanRun.pictures[keyOf(state)] = attachedPictures(part.probe.description)
		scanRun.wanted = append(scanRun.wanted, placedFile{state: state, placement: placement})
	}

	scanRun.rememberBook(grouped)
}
