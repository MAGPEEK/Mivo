package scan

import (
	"context"
	"fmt"

	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/library"
	"github.com/magpeek/mivo/internal/domain/metadata"
)

// causeAudiobooksUnconfigured is logged when an audiobooks library is scanned without the
// audiobook ports.
const causeAudiobooksUnconfigured = "audiobook scanning is not configured"

// audiobookRun is one scan of an audiobooks library (ADR 0027 §10, M3-14). Books are
// grouped per folder from the tags of all audio files there (naming.GroupAudiobooks), so a
// file's placement depends on its neighbours and the folder is the unit of work:
//   - a folder with a new, changed or removed file is read as a whole (probe and tags of
//     every file, chapters of the new and changed ones) and grouped again; unchanged files
//     whose placement changed are moved, new and changed ones saved with their chapters;
//   - files in other folders keep their stored placement and are not read, unless a book
//     they belong to has to be refreshed (audiobook_refresh.go);
//   - stored files that are gone are removed.
type audiobookRun struct {
	*runV2

	ports AudiobookPorts
	// unread are the stored files of unchanged folders, with their walked state.
	unread []PlacedFileState
	// keptUnreadable are the unchanged files of read folders that could not be read; they keep
	// their stored placement.
	keptUnreadable []PlacedFileState
	// read marks the files whose tags this scan has read.
	read map[fileKey]bool
}

func (service *Service) newAudiobookRun(scanned library.ManagedLibrary, scanTracker *tracker) *audiobookRun {
	scanRun := &audiobookRun{runV2: service.newRunV2(scanned, scanTracker), ports: service.audiobook, read: map[fileKey]bool{}}
	scanRun.withoutVideo = map[fileKey]library.FileState{}
	scanRun.embedded = map[string]metadata.Record{}
	scanRun.beforeRefresh = scanRun.readForRefresh

	return scanRun
}

func (scanRun *audiobookRun) execute(ctx context.Context) error {
	return scanRun.executeSteps(ctx, scanRun.audiobookSteps)
}

func (scanRun *audiobookRun) audiobookSteps(ctx context.Context) error {
	base := scanRun.base
	if !scanRun.ports.complete() {
		return fail(ErrorCodeFailed, causeAudiobooksUnconfigured,
			fmt.Errorf("scan library %s: %s: %w", base.library.ID, causeAudiobooksUnconfigured, domain.ErrUnavailable))
	}

	base.itemKind = library.ItemKindAudiobook

	files, err := base.walkAll(ctx)
	if err != nil {
		return err
	}

	scanRun.sidecars = files.sidecars

	plan, err := scanRun.planBooks(ctx, files)
	if err != nil {
		return err
	}

	results, err := scanRun.readParts(ctx, plan.reads)
	if err != nil {
		return err
	}

	if err := scanRun.applyBooks(ctx, results); err != nil {
		return err
	}

	if err := scanRun.remove(ctx, plan.removals); err != nil {
		return err
	}

	for _, removed := range plan.removals {
		base.service.setDetailRetry(base.library.ID, keyOf(removed.State), false)
	}

	return scanRun.finishSidecars(ctx, files)
}
