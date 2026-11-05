package scan

import (
	"context"
	"maps"
	"slices"
	"strings"

	"github.com/magpeek/mivo/internal/domain/naming"
)

// musicAlbumLevel is the position of the album in a track's item chain (artist, album, track).
const musicAlbumLevel = 1

// readForRefresh reads the unchanged tracks the refresher needs. A track's placement comes
// from its tags, which an unchanged file does not have read, so the refresher would not know
// its items. They are read only for items due for a refresh:
//   - every track of a due album, because its cover may come from its first track's picture;
//   - one track of any other due item (an artist, a track), which gives its title and folder.
//
// So a rescan without changes reads nothing, and a change reads at most the albums it touches.
func (scanRun *musicRun) readForRefresh(ctx context.Context) error {
	jobs := scanRun.refreshReads()
	if len(jobs) == 0 {
		return nil
	}

	batch := jobBatch[musicJob, musicResult]{jobs: jobs, work: scanRun.readTrack, record: scanRun.recordRefreshRead}

	return scanRun.base.runBatch(ctx, batch.run)
}

// refreshReads picks the unchanged files to read, in folder and path order. Items still
// pending from an earlier scan (failed refresh, unavailable folder) are due as well: without
// their tracks in the request the refresher would drop them and they would never be pended
// again.
func (scanRun *musicRun) refreshReads() []musicJob {
	due := maps.Clone(scanRun.refresh.ids)
	for _, id := range scanRun.base.service.peekPendingRefresh(scanRun.base.library.ID) {
		due[id] = struct{}{}
	}

	if len(due) == 0 {
		return nil
	}

	covered := map[string]bool{}

	for _, track := range scanRun.wanted {
		for _, id := range scanRun.chains[keyOf(track.state)] {
			covered[id] = true
		}
	}

	ranks := folderRanks(scanRun.base.library.Folders)
	unchanged := slices.Clone(scanRun.unchanged)
	slices.SortStableFunc(unchanged, func(first, second PlacedFileState) int {
		if byFolder := ranks[first.State.RootID] - ranks[second.State.RootID]; byFolder != 0 {
			return byFolder
		}

		return strings.Compare(first.State.RelativePath, second.State.RelativePath)
	})

	var jobs []musicJob

	for _, file := range unchanged {
		if !needsRead(file.ItemIDs, due, covered) {
			continue
		}

		for _, id := range file.ItemIDs {
			covered[id] = true
		}

		jobs = append(jobs, musicJob{state: file.State, placed: &file})
	}

	return jobs
}

// needsRead reports whether a file's chain holds a due album or a due item no read file
// covers yet.
func needsRead(chain []string, due map[string]struct{}, covered map[string]bool) bool {
	for position, id := range chain {
		if _, isDue := due[id]; !isDue {
			continue
		}

		if position == musicAlbumLevel || !covered[id] {
			return true
		}
	}

	return false
}

// recordRefreshRead makes a read unchanged file known to the refresher. Its stored chain is
// only used when the tags still lead to the stored placement; a probe failure leaves its
// pictures unknown, so the refresher keeps what it had selected from it.
func (scanRun *musicRun) recordRefreshRead(ctx context.Context, result musicResult) error {
	if result.probe.err != nil {
		return result.probe.err
	}

	job := result.job

	placement, err := naming.PlaceMusic(job.state.PathForParsing(), result.tags)
	if err != nil || job.placed == nil || placement.Key() != job.placed.PlacementKey {
		scanRun.base.logPath(ctx, "scan left an unchanged track out of the refresh", job.state.RelativePath)

		return nil
	}

	if result.probe.outcome == outcomeProbed {
		scanRun.pictures[keyOf(job.state)] = attachedPictures(result.probe.description)
	}

	scanRun.wanted = append(scanRun.wanted, placedFile{state: job.state, placement: placement})

	return nil
}
