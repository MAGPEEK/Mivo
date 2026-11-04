package scan

import (
	"context"
	"path"
	"slices"

	"github.com/magpeek/mivo/internal/domain/library"
)

// noRefresh is the refresher used until M3-10b provides one.
type noRefresh struct{}

func (noRefresh) RefreshItems(context.Context, RefreshRequest) error { return nil }

// refreshSet collects the IDs of the items to refresh.
type refreshSet struct {
	ids map[string]struct{}
}

func newRefreshSet() *refreshSet { return &refreshSet{ids: map[string]struct{}{}} }

func (set *refreshSet) add(itemIDs ...string) {
	for _, id := range itemIDs {
		set.ids[id] = struct{}{}
	}
}

// sorted returns the IDs in a stable order.
func (set *refreshSet) sorted() []string {
	result := make([]string, 0, len(set.ids))
	for id := range set.ids {
		result = append(result, id)
	}

	slices.Sort(result)

	return result
}

// addBelowChangedSidecars adds the chains of the files a changed sidecar can describe: those
// in its folder and, except for a sidecar in the library root, those below it (a tvshow.nfo
// describes every episode below its folder).
//
// The changed folders are collected once and each file walks up its own folders, so the cost
// is files x folder depth, not files x sidecars.
func (set *refreshSet) addBelowChangedSidecars(changed []string, chains map[string][]string) {
	if len(changed) == 0 {
		return
	}

	changedDirectories := make(map[string]bool, len(changed))
	for _, sidecarPath := range changed {
		changedDirectories[path.Dir(sidecarPath)] = true
	}

	for filePath, chain := range chains {
		if coveredByChanged(path.Dir(filePath), changedDirectories) {
			set.add(chain...)
		}
	}
}

// coveredByChanged reports whether fileDirectory or one of its ancestors holds a changed
// sidecar; a sidecar in the root covers only files directly in the root.
func coveredByChanged(fileDirectory string, changedDirectories map[string]bool) bool {
	for directory := fileDirectory; ; directory = path.Dir(directory) {
		if directory == "." {
			return changedDirectories["."] && fileDirectory == "."
		}

		if changedDirectories[directory] {
			return true
		}
	}
}

// sidecarDiff is the difference between the stored sidecars and a completed walk.
type sidecarDiff struct {
	upserts  []library.FileState
	removals []string
}

// changedPaths lists the paths of new, changed and removed sidecars.
func (diff sidecarDiff) changedPaths() []string {
	paths := make([]string, 0, len(diff.upserts)+len(diff.removals))
	for _, state := range diff.upserts {
		paths = append(paths, state.RelativePath)
	}

	return append(paths, diff.removals...)
}

// diffSidecars compares the walked sidecars with the stored ones. Stored sidecars below a
// path the walker could not read are kept.
func diffSidecars(stored, seen []library.FileState, skipped []string) sidecarDiff {
	storedByPath := make(map[string]library.FileState, len(stored))
	for _, state := range stored {
		storedByPath[state.RelativePath] = state
	}

	var diff sidecarDiff

	seenPaths := make(map[string]bool, len(seen))

	for _, state := range seen {
		if seenPaths[state.RelativePath] {
			continue
		}

		seenPaths[state.RelativePath] = true

		previous, found := storedByPath[state.RelativePath]
		if !found || library.Changed(previous, state) {
			diff.upserts = append(diff.upserts, state)
		}
	}

	for _, state := range stored {
		if !seenPaths[state.RelativePath] && !belowAny(state.RelativePath, skipped) {
			diff.removals = append(diff.removals, state.RelativePath)
		}
	}

	return diff
}
