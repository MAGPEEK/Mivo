package scan

import (
	"context"
	"errors"
	"fmt"

	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/library"
)

// walkAll walks every folder of the library and sorts the files into media and sidecars,
// each carrying the ID of its folder.
//
// A folder that is missing or lost during its walk contributes nothing and is marked as
// skipped as a whole, so the plan keeps everything the catalog holds for it (ADR 0022). Only
// when every folder is unavailable the scan fails, as a one-folder library always did.
func (scanRun *run) walkAll(ctx context.Context) (walked, error) {
	files := walked{skipped: skipSet{}, protectedDiscs: skipSet{}}

	var (
		unavailable    int
		unavailableErr error
	)

	for _, folder := range scanRun.library.Folders {
		found, skipped, err := scanRun.walkFolder(ctx, folder)

		switch {
		case errors.Is(err, domain.ErrUnavailable):
			unavailable++
			unavailableErr = err

			files.skipped.add(folder.ID, wholeFolder)
		case err != nil:
			return walked{}, scanRun.walkError(err)
		default:
			files.mediaFiles = append(files.mediaFiles, found.mediaFiles...)
			files.sidecars = append(files.sidecars, found.sidecars...)
			files.skipped.add(folder.ID, skipped...)
			files.protectedDiscs.add(folder.ID, found.protectedDiscs[folder.ID]...)
		}
	}

	scanRun.tracker.setUnavailableFolders(unavailable)

	if unavailable == len(scanRun.library.Folders) {
		if unavailableErr == nil {
			unavailableErr = fmt.Errorf("library has no folders: %w", domain.ErrUnavailable)
		}

		return walked{}, scanRun.walkError(unavailableErr)
	}

	return files, nil
}

// walkFolder walks one folder. Its result is only used when the whole walk succeeded, so a
// folder lost halfway never yields a partial listing that would delete catalog entries.
func (scanRun *run) walkFolder(ctx context.Context, folder library.Folder) (walked, []string, error) {
	found := walked{protectedDiscs: skipSet{}}

	skipped, err := scanRun.service.walker.Walk(ctx, folder.Path, scanRun.library.Kind, func(state library.FileState) error {
		state.RootID = folder.ID

		switch scanRun.discHandling(state) {
		case discDropped:
		case discProtected:
			found.protectedDiscs.add(folder.ID, state.RelativePath)
		case discNone, discPlaced:
			// A disc (folder or image) is one video entry; placement and probing dispatch on
			// its layout (placedFile.place, describe).
			if isMediaClass(state.Class) {
				found.mediaFiles = append(found.mediaFiles, state)
			} else {
				found.sidecars = append(found.sidecars, state)
			}
		}

		return nil
	})
	if err != nil {
		return walked{}, nil, fmt.Errorf("walk folder %s: %w", folder.ID, err)
	}

	return found, skipped, nil
}

// discHandling says what the scan does with a walked entry that may be a disc.
type discHandling int

const (
	// discNone: no disc; handled like any walked file.
	discNone discHandling = iota
	// discPlaced: a disc in a movies or shows library with complete disc ports.
	discPlaced
	// discDropped: a disc in a library kind without discs (music, audiobooks, photos); it is
	// no catalog entry of that kind.
	discDropped
	// discProtected: a disc while the disc ports are incomplete. It is neither placed nor a
	// failure, and the entries stored at or below its path survive, so the disc is scanned
	// once the ports exist instead of being stored as unavailable.
	discProtected
)

func (scanRun *run) discHandling(state library.FileState) discHandling {
	switch {
	case !state.Layout.IsDisc():
		return discNone
	case scanRun.library.Kind != library.KindMovies && scanRun.library.Kind != library.KindShows:
		return discDropped
	case !scanRun.service.discs.complete():
		return discProtected
	default:
		return discPlaced
	}
}

// isMediaClass reports the classes a pipeline places and probes. The walker only reports the
// ones the library kind allows, so a video never reaches the music pipeline and a photo only
// the photo pipeline; discs, which the walker reports for every kind, are sorted out before
// (discHandling).
func isMediaClass(class library.FileClass) bool {
	return class == library.FileClassVideo || class == library.FileClassAudio || class == library.FileClassPhoto
}
