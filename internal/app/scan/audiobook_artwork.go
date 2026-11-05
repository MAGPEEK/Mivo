package scan

import (
	"cmp"
	"math"
	"slices"

	"github.com/magpeek/mivo/internal/domain/artwork"
	"github.com/magpeek/mivo/internal/domain/naming"
)

// audiobookArtwork follows the album cover rules of M3-13: cover, folder and front images
// in the book's folder, then the embedded picture of the first part (play order) that has
// one. A book that stands alone (an M4B in a shared folder, a file in the library root)
// shares its folder with other books, so it takes only images named after its file.
func (run *refreshRun) audiobookArtwork(item *refreshItem, state MetadataState) []artwork.Candidate {
	parts := slices.Clone(item.own)
	slices.SortStableFunc(parts, func(first, second RefreshFile) int {
		return cmp.Compare(playOrder(first), playOrder(second))
	})

	if len(parts) == 0 {
		return nil
	}

	first := parts[0].State
	directory := keyOf(first).directory()
	localContext := artwork.LocalContext{Kind: artwork.ContextAlbumFolder}

	if len(parts) == 1 && naming.AudiobookStandsAlone(first.PathForParsing()) {
		localContext = artwork.LocalContext{Kind: artwork.ContextLooseMovie, BaseName: baseName(first.RelativePath)}
	}

	candidates := run.localCandidates(directory, localContext, nil)

	return append(candidates, firstTrackPicture(parts, state)...)
}

// playOrder is the part number of a file; a part without one (kept unreadable) sorts last.
func playOrder(file RefreshFile) int {
	if number, ok := file.Placement.PartNumber.Get(); ok {
		return number
	}

	return math.MaxInt
}
