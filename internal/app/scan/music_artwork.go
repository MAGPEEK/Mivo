package scan

import (
	"github.com/magpeek/mivo/internal/domain/artwork"
	"github.com/magpeek/mivo/internal/domain/naming"
)

// albumArtwork is the cover order of M3-13: cover, folder and front images in the album
// folders (above disc folders), then the embedded picture of the album's first track that
// has one, in folder and path order.
func (run *refreshRun) albumArtwork(item *refreshItem, state MetadataState) []artwork.Candidate {
	var candidates []artwork.Candidate

	for _, directory := range musicDirectories(item, func(folders naming.MusicFolders) string { return folders.Album }) {
		candidates = append(candidates, run.localCandidates(directory, artwork.LocalContext{Kind: artwork.ContextAlbumFolder}, nil)...)
	}

	return append(candidates, firstTrackPicture(item.below, state)...)
}

// artistArtwork takes artist images from the folder above each album folder.
func (run *refreshRun) artistArtwork(item *refreshItem) []artwork.Candidate {
	var candidates []artwork.Candidate

	for _, directory := range musicDirectories(item, func(folders naming.MusicFolders) string { return folders.Artist }) {
		candidates = append(candidates, run.localCandidates(directory, artwork.LocalContext{Kind: artwork.ContextArtistFolder}, nil)...)
	}

	return candidates
}

// musicDirectories lists the distinct folders pick selects from the files of the item, in
// file order; files for which pick finds no folder (too close to the root) add none.
func musicDirectories(item *refreshItem, pick func(naming.MusicFolders) string) []fileKey {
	var directories []fileKey

	seen := map[fileKey]bool{}

	for _, file := range item.below {
		directory := pick(naming.MusicFoldersOf(file.State.RelativePath))
		key := fileKey{rootID: file.State.RootID, path: directory}

		if directory != "" && !seen[key] {
			seen[key] = true

			directories = append(directories, key)
		}
	}

	return directories
}

// firstTrackPicture returns the embedded pictures of the first track that has one. A track
// this scan did not probe offers the picture selected from it before, if any.
func firstTrackPicture(tracks []RefreshFile, state MetadataState) []artwork.Candidate {
	for _, track := range tracks {
		if pictures := embeddedArtwork([]RefreshFile{track}, state); len(pictures) > 0 {
			return pictures
		}
	}

	return nil
}
