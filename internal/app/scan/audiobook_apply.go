package scan

import (
	"context"
	"maps"
	"slices"

	"github.com/magpeek/mivo/internal/domain/library"
	"github.com/magpeek/mivo/internal/domain/naming"
)

// groupedBook is one book of the read folders with its read files in play order.
type groupedBook struct {
	book  naming.Audiobook
	parts []audiobookResult
}

// applyBooks groups the read files into books and applies the result: moves and kept
// placements first, then saves, so a renamed part is saved after its book exists and the
// book keeps its ID. Each book's embedded record is remembered for the refresh.
func (scanRun *audiobookRun) applyBooks(ctx context.Context, results []audiobookResult) error {
	books := scanRun.groupResults(ctx, results)
	// Deferred, so a scan that ends during the apply still hands the books it touched to the
	// grace refresh with their people instead of merging them away.
	defer scanRun.rememberBooks(books)

	if err := forEachPart(books, false, func(part audiobookResult, placement library.Placement) error {
		return scanRun.keepOrMove(ctx, part, placement)
	}); err != nil {
		return err
	}

	if err := forEachPart(books, true, func(part audiobookResult, placement library.Placement) error {
		return scanRun.savePart(ctx, part, placement)
	}); err != nil {
		return err
	}

	return nil
}

// forEachPart calls apply for every part whose changed flag equals changed, with its
// placement, and stops at the first error.
func forEachPart(books []groupedBook, changed bool, apply func(audiobookResult, library.Placement) error) error {
	for _, grouped := range books {
		for index, part := range grouped.parts {
			if part.job.changed != changed {
				continue
			}

			if err := apply(part, grouped.book.PartPlacement(index)); err != nil {
				return err
			}
		}
	}

	return nil
}

// groupResults groups the read files of every library folder, in folder order. An
// unchanged file that could not be read keeps its stored placement: its tags are unknown,
// so grouping it by name could move it out of its book because of a passing read error.
func (scanRun *audiobookRun) groupResults(ctx context.Context, results []audiobookResult) []groupedBook {
	byRoot := map[string]map[string]audiobookResult{}

	for _, result := range results {
		state := result.job.state
		scanRun.read[keyOf(state)] = true

		if !result.job.changed && (result.probe.outcome != outcomeProbed || result.tagErr != nil) {
			scanRun.base.logPath(ctx, "scan kept the placement of an unreadable unchanged file", state.RelativePath)
			kept := *result.job.placed
			kept.State = state
			scanRun.keepUnread(kept, state)
			scanRun.keptUnreadable = append(scanRun.keptUnreadable, kept)

			continue
		}

		if result.tagErr != nil {
			scanRun.warn(ctx, "scan could not read audio tags, using the folder names", state, result.tagErr)
		}

		addByRoot(byRoot, result)
	}

	return scanRun.groupRoots(byRoot)
}

// addByRoot files a read result under its library folder and parsing path.
func addByRoot(byRoot map[string]map[string]audiobookResult, result audiobookResult) {
	state := result.job.state
	if byRoot[state.RootID] == nil {
		byRoot[state.RootID] = map[string]audiobookResult{}
	}

	byRoot[state.RootID][state.PathForParsing()] = result
}

// groupRoots groups the read files of each library folder, in folder order.
func (scanRun *audiobookRun) groupRoots(byRoot map[string]map[string]audiobookResult) []groupedBook {
	ranks := folderRanks(scanRun.base.library.Folders)
	roots := slices.SortedFunc(maps.Keys(byRoot), func(first, second string) int { return ranks[first] - ranks[second] })

	var books []groupedBook

	for _, rootID := range roots {
		books = append(books, groupFolder(byRoot[rootID])...)
	}

	return books
}

// groupFolder groups the read files of one library folder, keyed by their parsing path.
func groupFolder(byPath map[string]audiobookResult) []groupedBook {
	files := make([]naming.AudiobookFile, 0, len(byPath))
	for parsingPath, result := range byPath {
		files = append(files, naming.NewAudiobookFile(parsingPath, result.tags))
	}

	books := naming.GroupAudiobooks(files)
	grouped := make([]groupedBook, 0, len(books))

	for _, book := range books {
		parts := make([]audiobookResult, 0, len(book.Parts))
		for _, part := range book.Parts {
			parts = append(parts, byPath[part.Path])
		}

		grouped = append(grouped, groupedBook{book: book, parts: parts})
	}

	return grouped
}

// keepOrMove keeps an unchanged file whose placement did not change and moves one whose
// placement did (a new part order, a split or merged book).
func (scanRun *audiobookRun) keepOrMove(ctx context.Context, part audiobookResult, placement library.Placement) error {
	stored := *part.job.placed
	stored.State = part.job.state
	scanRun.pictures[keyOf(stored.State)] = attachedPictures(part.probe.description)
	scanRun.wanted = append(scanRun.wanted, placedFile{state: stored.State, placement: placement})

	if placement.Key() != stored.PlacementKey {
		return scanRun.move(ctx, []moveJob{{stored: stored, placement: placement}})
	}

	scanRun.chains[keyOf(stored.State)] = stored.ItemIDs
	scanRun.base.tracker.count(func(counts *Counts) { counts.Unchanged++ })

	return nil
}

// savePart saves a new or changed file with its placement, then its chapters. The file is
// marked for a chapter retry until they are written, so an end in between (failure or
// cancel) leaves it to be read again by the next scan.
func (scanRun *audiobookRun) savePart(ctx context.Context, part audiobookResult, placement library.Placement) error {
	base := scanRun.base
	job := probeJob{state: part.job.state, placement: placement, placed: part.job.placed}

	fileID, err := scanRun.fileID(job)
	if err != nil {
		return fail(ErrorCodeFailed, err.Error(), err)
	}

	job.fileID = fileID
	key := keyOf(job.state)
	base.service.setChapterRetry(base.library.ID, key, true)

	if err := scanRun.saveProbed(ctx, job, part.probe); err != nil {
		return err
	}

	if err := scanRun.ports.Catalog.ReplaceFileChapters(ctx, fileID, scanRun.partChapters(ctx, part)); err != nil {
		return catalogFailure("save chapters of "+job.state.RelativePath, err)
	}

	base.service.setChapterRetry(base.library.ID, key, false)
	scanRun.wanted = append(scanRun.wanted, placedFile{state: job.state, placement: placement})

	return nil
}
