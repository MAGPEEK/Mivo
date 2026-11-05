package scan

import (
	"path"

	"github.com/magpeek/mivo/internal/domain/library"
	"github.com/magpeek/mivo/internal/domain/media"
	"github.com/magpeek/mivo/internal/domain/naming"
)

// placedFile is a walked media file with the placement the name parser (or, for music, the tags) computed.
type placedFile struct {
	state     library.FileState
	placement library.Placement
}

func (video placedFile) isExtra() bool {
	levels := video.placement.Levels

	return levels[len(levels)-1].Kind == library.ItemKindExtra
}

// ownerKey identifies the top-level item an extra belongs to.
func (video placedFile) ownerKey() string {
	top := video.placement.Levels[0]

	return top.Kind.String() + "\x00" + top.MatchKey
}

// directory is the folder of the video, per library folder: two folders may hold equal
// relative directories whose videos must not be counted together.
func (video placedFile) directory() fileKey {
	return fileKey{rootID: video.state.RootID, path: path.Dir(video.state.PathForParsing())}
}

func (video placedFile) context(videoCount int) naming.FileContext {
	return naming.FileContext{Path: video.state.PathForParsing(), DirectoryVideoCount: videoCount}
}

// place runs the name parser; a disc is placed like a video named after its folder or image
// (naming.PlaceDisc), which ignores the directory's video count.
func (video placedFile) place(kind library.Kind, videoCount int) (naming.Result, error) {
	if video.state.Layout.IsDisc() {
		return naming.PlaceDisc(kind, video.state.PathForParsing())
	}

	return naming.Place(kind, video.context(videoCount))
}

// placeVideos runs the name parser over every video. The parser needs to know how many
// videos share a directory (not counting extras and samples), which it cannot see, so a first
// pass classifies each video and a second places the counted ones with the real count. The
// second result lists the videos that are no catalog files (samples, unplaceable names).
func placeVideos(kind library.Kind, videos []library.FileState) (placed []placedFile, ignored []library.FileState) {
	counts := map[fileKey]int{}
	first := make([]placedFile, 0, len(videos))

	for _, state := range videos {
		video := placedFile{state: state}

		result, err := video.place(kind, 0)
		if err != nil || result.Outcome != naming.OutcomePlaced {
			ignored = append(ignored, state)

			continue
		}

		video.placement = result.Placement
		if !video.isExtra() {
			counts[video.directory()]++
		}

		first = append(first, video)
	}

	for _, video := range first {
		if !video.isExtra() {
			again, err := video.place(kind, counts[video.directory()])
			if err != nil || again.Outcome != naming.OutcomePlaced {
				ignored = append(ignored, video.state)

				continue
			}

			video.placement = again.Placement
		}

		placed = append(placed, video)
	}

	return placed, ignored
}

// withoutExtras splits the extras off for a library that does not scan them. They are no
// catalog files then, so extras stored earlier are removed by the plan.
func withoutExtras(videos []placedFile) (kept []placedFile, extras []library.FileState) {
	for _, video := range videos {
		if video.isExtra() {
			extras = append(extras, video.state)

			continue
		}

		kept = append(kept, video)
	}

	return kept, extras
}

// dropOrphanExtras removes extras whose movie or show has no video of its own and returns
// them separately.
func dropOrphanExtras(videos []placedFile) (kept []placedFile, orphans []library.FileState) {
	owners := map[string]bool{}

	for _, video := range videos {
		if !video.isExtra() {
			owners[video.ownerKey()] = true
		}
	}

	for _, video := range videos {
		if video.isExtra() && !owners[video.ownerKey()] {
			orphans = append(orphans, video.state)

			continue
		}

		kept = append(kept, video)
	}

	return kept, orphans
}

// moveJob gives an unchanged file a new placement.
type moveJob struct {
	stored    PlacedFileState
	placement library.Placement
}

// placementPlan is the difference between the v2 catalog and a completed walk.
type placementPlan struct {
	probes    []probeJob
	moves     []moveJob
	removals  []PlacedFileState
	unchanged int64
	// chains maps the path of every unchanged file to its stored item chain.
	chains       map[fileKey][]string
	withoutVideo map[fileKey]library.FileState
}

// placementInput is what planPlacements compares.
type placementInput struct {
	stored       []PlacedFileState
	wanted       []placedFile
	skipped      skipSet
	withoutVideo map[fileKey]library.FileState
}

// planPlacements sorts the wanted files into probes (new or changed), moves (unchanged with
// a new placement key) and unchanged ones; stored files that are no longer wanted are removed
// unless they lie at or below a path the walker could not read.
func planPlacements(input placementInput) placementPlan {
	storedByPath := make(map[fileKey]PlacedFileState, len(input.stored))
	for _, file := range input.stored {
		storedByPath[keyOf(file.State)] = file
	}

	plan := placementPlan{chains: map[fileKey][]string{}, withoutVideo: map[fileKey]library.FileState{}}
	wantedPaths := make(map[fileKey]bool, len(input.wanted))

	for _, video := range input.wanted {
		if wantedPaths[keyOf(video.state)] {
			continue
		}

		wantedPaths[keyOf(video.state)] = true
		plan.add(video, storedByPath, input.withoutVideo)
	}

	for _, file := range input.stored {
		if !wantedPaths[keyOf(file.State)] && !input.skipped.covers(file.State) {
			plan.removals = append(plan.removals, file)
		}
	}

	return plan
}

func (plan *placementPlan) add(video placedFile, storedByPath map[fileKey]PlacedFileState,
	withoutVideo map[fileKey]library.FileState,
) {
	state := video.state

	if previous, found := storedByPath[keyOf(state)]; found {
		switch {
		case needsProbe(previous, state):
			plan.probes = append(plan.probes, probeJob{state: state, placement: video.placement, placed: &previous})
		case previous.PlacementKey != video.placement.Key():
			plan.moves = append(plan.moves, moveJob{stored: previous, placement: video.placement})
		default:
			plan.unchanged++
			plan.chains[keyOf(state)] = previous.ItemIDs
		}

		return
	}

	if known, remembered := withoutVideo[keyOf(state)]; remembered && !library.Changed(known, state) {
		plan.withoutVideo[keyOf(state)] = state

		return
	}

	plan.probes = append(plan.probes, probeJob{state: state, placement: video.placement})
}

// needsProbe reports a stored file that must be probed again: it changed, or it is a disc
// stored as disc_unsupported. That code also stands for an FFmpeg build without the needed
// demuxers or protocols (ADR 0029), which an update may add, and an unchanged disc would
// otherwise stay unavailable for good; the cost is one probe per such disc and scan.
func needsProbe(previous PlacedFileState, current library.FileState) bool {
	return library.Changed(previous.State, current) ||
		(current.Layout.IsDisc() && previous.ProbeError == media.ProbeErrorDiscUnsupported)
}
