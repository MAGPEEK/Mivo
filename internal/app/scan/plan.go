package scan

import (
	"strings"

	"github.com/magpeek/mivo/internal/domain/library"
)

// relativePathSeparator separates the segments of a root-relative catalog path.
const relativePathSeparator = "/"

// probeJob is one file to probe. stored is nil for a new file and the catalog entry for a
// changed one.
type probeJob struct {
	state  library.FileState
	stored *CatalogFile
}

// changeInput is what planChanges compares.
type changeInput struct {
	// stored are the catalog files of the library.
	stored []CatalogFile
	// seen are the files of the completed walk, skipped the paths it could not read.
	seen    []library.FileState
	skipped []string
	// withoutVideo are the files an earlier scan found without a video stream.
	withoutVideo map[string]library.FileState
}

// changePlan is the difference between the catalog and a completed walk.
type changePlan struct {
	probes    []probeJob
	removals  []string
	unchanged int64
	// withoutVideo are the remembered files without a video stream that are still there and
	// unchanged; they count as skipped without a probe.
	withoutVideo map[string]library.FileState
}

// planChanges compares the walked files with the catalog: new and changed files are probed
// in walk order, unchanged ones counted, unchanged files known to have no video stream are
// kept as skipped, and catalog files the walk did not see are removed unless they lie at or
// below a path the walker could not read.
func planChanges(input changeInput) changePlan {
	storedByPath := make(map[string]CatalogFile, len(input.stored))
	for _, file := range input.stored {
		storedByPath[file.State.RelativePath] = file
	}

	plan := changePlan{withoutVideo: map[string]library.FileState{}}
	seenPaths := make(map[string]bool, len(input.seen))

	for _, state := range input.seen {
		if seenPaths[state.RelativePath] {
			continue
		}

		seenPaths[state.RelativePath] = true
		plan.add(state, storedByPath, input.withoutVideo)
	}

	for _, file := range input.stored {
		if !seenPaths[file.State.RelativePath] && !belowAny(file.State.RelativePath, input.skipped) {
			plan.removals = append(plan.removals, file.ID)
		}
	}

	return plan
}

// add sorts one walked file into the plan.
func (plan *changePlan) add(state library.FileState, storedByPath map[string]CatalogFile,
	withoutVideo map[string]library.FileState,
) {
	previous, found := storedByPath[state.RelativePath]
	if found {
		if library.Changed(previous.State, state) {
			plan.probes = append(plan.probes, probeJob{state: state, stored: &previous})
		} else {
			plan.unchanged++
		}

		return
	}

	if known, remembered := withoutVideo[state.RelativePath]; remembered && !library.Changed(known, state) {
		plan.withoutVideo[state.RelativePath] = state

		return
	}

	plan.probes = append(plan.probes, probeJob{state: state})
}

// belowAny reports whether relativePath equals one of the skipped paths or lies below one.
// Trailing separators of a skipped path are ignored ("Locked/" protects "Locked/a.mkv"); an
// empty or "." skipped path stands for the whole root.
func belowAny(relativePath string, skipped []string) bool {
	for _, skippedPath := range skipped {
		skippedPath = strings.TrimRight(skippedPath, relativePathSeparator)

		if skippedPath == "" || skippedPath == "." || relativePath == skippedPath ||
			strings.HasPrefix(relativePath, skippedPath+relativePathSeparator) {
			return true
		}
	}

	return false
}
