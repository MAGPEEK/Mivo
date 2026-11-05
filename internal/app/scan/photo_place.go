package scan

import (
	"path"

	"github.com/magpeek/mivo/internal/domain/library"
	"github.com/magpeek/mivo/internal/domain/naming"
	"github.com/magpeek/mivo/internal/domain/photo"
)

// photoPlacement is the placement of the walked files of a photos library.
type photoPlacement struct {
	placed []placedFile
	// alternates marks the files that are no primary of their item (the RAW of a pair).
	alternates map[fileKey]bool
	// ignored are files that are no catalog entry (unplaceable paths).
	ignored []library.FileState
}

// placePhotos groups the files of each folder (photo.GroupFiles) and places the primary
// file of each group by its path (ADR 0029). Alternates share the placement of their
// primary, so the catalog keeps them as further files of the same photo item. The pairing
// depends on the neighbours in the folder, which is why every walked file is placed, not
// only the new ones.
func placePhotos(files []library.FileState) photoPlacement {
	byDirectory := map[fileKey][]library.FileState{}

	var directories []fileKey

	for _, state := range files {
		directory := fileKey{rootID: state.RootID, path: path.Dir(state.PathForParsing())}
		if _, known := byDirectory[directory]; !known {
			directories = append(directories, directory)
		}

		byDirectory[directory] = append(byDirectory[directory], state)
	}

	result := photoPlacement{alternates: map[fileKey]bool{}}
	for _, directory := range directories {
		result.addDirectory(byDirectory[directory])
	}

	return result
}

// addDirectory places the files of one folder.
func (result *photoPlacement) addDirectory(states []library.FileState) {
	byName := make(map[string]library.FileState, len(states))
	names := make([]string, 0, len(states))

	for _, state := range states {
		name := path.Base(state.PathForParsing())
		if _, duplicate := byName[name]; !duplicate {
			names = append(names, name)
		}

		byName[name] = state
	}

	grouped := make(map[string]bool, len(names))

	for _, group := range photo.GroupFiles(names) {
		grouped[group.Primary] = true
		for _, alternate := range group.Alternates {
			grouped[alternate] = true
		}

		result.addGroup(group, byName)
	}

	for _, name := range names {
		if !grouped[name] {
			result.ignored = append(result.ignored, byName[name])
		}
	}
}

// addGroup places the primary of a group and gives its alternates the same placement.
func (result *photoPlacement) addGroup(group photo.Group, byName map[string]library.FileState) {
	primary := byName[group.Primary]

	placed, err := naming.Place(library.KindPhotos, naming.FileContext{Path: primary.PathForParsing()})
	if err != nil || placed.Outcome != naming.OutcomePlaced {
		result.ignored = append(result.ignored, primary)
		for _, alternate := range group.Alternates {
			result.ignored = append(result.ignored, byName[alternate])
		}

		return
	}

	result.placed = append(result.placed, placedFile{state: primary, placement: placed.Placement})

	for _, name := range group.Alternates {
		alternate := byName[name]
		result.placed = append(result.placed, placedFile{state: alternate, placement: placed.Placement})
		result.alternates[keyOf(alternate)] = true
	}
}
