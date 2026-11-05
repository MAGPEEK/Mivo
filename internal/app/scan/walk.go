package scan

import (
	"context"
	"errors"
	"fmt"

	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/library"
)

// walkAll walks every folder of the library and sorts the files into videos and sidecars,
// each carrying the ID of its folder.
//
// A folder that is missing or lost during its walk contributes nothing and is marked as
// skipped as a whole, so the plan keeps everything the catalog holds for it (ADR 0022). Only
// when every folder is unavailable the scan fails, as a one-folder library always did.
func (scanRun *run) walkAll(ctx context.Context) (walked, error) {
	files := walked{skipped: skipSet{}}

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
	var (
		found     walked
		discPaths []string
	)

	skipped, err := scanRun.service.walker.Walk(ctx, folder.Path, scanRun.library.Kind, func(state library.FileState) error {
		state.RootID = folder.ID

		if state.Layout.IsDisc() {
			// Temporary guard, removed by M3-16h: until discs are placed and probed, a disc
			// is neither a video nor a failure, and its stored entries at or below the path
			// (the disc's files from older scans) must survive.
			discPaths = append(discPaths, state.RelativePath)

			return nil
		}

		if state.Class == library.FileClassPhoto {
			// Until the photo pipeline exists (M3-15f), a photos library treats its images
			// as image sidecars, as it did before the walker classified by library kind.
			state.Class = library.FileClassImage
		}

		if isMediaClass(state.Class) {
			found.mediaFiles = append(found.mediaFiles, state)
		} else {
			found.sidecars = append(found.sidecars, state)
		}

		return nil
	})
	if err != nil {
		return walked{}, nil, fmt.Errorf("walk folder %s: %w", folder.ID, err)
	}

	return found, append(skipped, discPaths...), nil
}

// isMediaClass reports the classes a pipeline places and probes; the walker only reports
// the ones the library kind allows, so a video never reaches the music pipeline.
func isMediaClass(class library.FileClass) bool {
	return class == library.FileClassVideo || class == library.FileClassAudio
}
