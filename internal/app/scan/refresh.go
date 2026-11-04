package scan

import (
	"context"
	"slices"

	"github.com/magpeek/mivo/internal/domain/library"
	"github.com/magpeek/mivo/internal/domain/media"
)

// refreshFiles describes every placed file that has an item chain, for the refresher.
func (scanRun *runV2) refreshFiles() []RefreshFile {
	files := make([]RefreshFile, 0, len(scanRun.wanted))

	for _, video := range scanRun.wanted {
		chain, placed := scanRun.chains[keyOf(video.state)]
		if !placed || len(chain) != len(video.placement.Levels) {
			continue
		}

		pictures, known := scanRun.pictures[keyOf(video.state)]
		files = append(files, RefreshFile{
			State: video.state, Placement: video.placement, ItemIDs: chain,
			EmbeddedPictures: pictures, PicturesKnown: known,
		})
	}

	return files
}

// attachedPictures lists the stream indexes of the attached pictures (cover art).
func attachedPictures(description media.Description) []int {
	var indexes []int

	for _, stream := range description.Streams {
		if stream.AttachedPicture {
			indexes = append(indexes, stream.Index)
		}
	}

	return indexes
}

// noRefresh is the refresher used when none is wired.
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
func (set *refreshSet) addBelowChangedSidecars(changed []fileKey, chains map[fileKey][]string) {
	if len(changed) == 0 {
		return
	}

	changedDirectories := make(map[fileKey]bool, len(changed))
	for _, sidecar := range changed {
		changedDirectories[sidecar.directory()] = true
	}

	for file, chain := range chains {
		if coveredByChanged(file.directory(), changedDirectories) {
			set.add(chain...)
		}
	}
}

// coveredByChanged reports whether fileDirectory or one of its ancestors holds a changed
// sidecar; a sidecar in the root covers only files directly in the root. Folders are kept
// apart: a sidecar in one library folder never covers a file of another.
func coveredByChanged(fileDirectory fileKey, changedDirectories map[fileKey]bool) bool {
	for directory := fileDirectory; ; directory = directory.directory() {
		if directory.path == "." {
			return changedDirectories[directory] && fileDirectory.path == "."
		}

		if changedDirectories[directory] {
			return true
		}
	}
}

// sidecarDiff is the difference between the stored sidecars and a completed walk.
type sidecarDiff struct {
	upserts  []library.FileState
	removals []library.FileState
}

// changedSidecars lists the keys of new, changed and removed sidecars.
func (diff sidecarDiff) changedSidecars() []fileKey {
	keys := make([]fileKey, 0, len(diff.upserts)+len(diff.removals))

	for _, state := range diff.upserts {
		keys = append(keys, keyOf(state))
	}

	for _, state := range diff.removals {
		keys = append(keys, keyOf(state))
	}

	return keys
}

// diffSidecars compares the walked sidecars with the stored ones. Stored sidecars below a
// path the walker could not read, or in a folder that was unavailable, are kept.
func diffSidecars(stored, seen []library.FileState, skipped skipSet) sidecarDiff {
	storedByKey := make(map[fileKey]library.FileState, len(stored))
	for _, state := range stored {
		storedByKey[keyOf(state)] = state
	}

	var diff sidecarDiff

	seenKeys := make(map[fileKey]bool, len(seen))

	for _, state := range seen {
		if seenKeys[keyOf(state)] {
			continue
		}

		seenKeys[keyOf(state)] = true

		previous, found := storedByKey[keyOf(state)]
		if !found || library.Changed(previous, state) {
			diff.upserts = append(diff.upserts, state)
		}
	}

	for _, state := range stored {
		if !seenKeys[keyOf(state)] && !skipped.covers(state) {
			diff.removals = append(diff.removals, state)
		}
	}

	return diff
}
