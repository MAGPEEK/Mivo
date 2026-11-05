package restapi

import (
	"fmt"
	"uuid"

	"github.com/magpeek/mivo/internal/adapters/http/apigen"
	applibrary "github.com/magpeek/mivo/internal/app/library"
	appscan "github.com/magpeek/mivo/internal/app/scan"
	"github.com/magpeek/mivo/internal/domain/library"
)

// libraryResponse combines a library with the status of its scan. The folder paths are the
// admin's own input; absolute paths become admin-only when accounts arrive (M7), while the
// folder browser keeps its paths root-relative.
func libraryResponse(overview applibrary.ManagedOverview, status appscan.Status) (apigen.Library, error) {
	managed := overview.Library

	id, err := parseStoredID("library", managed.ID)
	if err != nil {
		return apigen.Library{}, err
	}

	kind, err := libraryKindResponse(managed.Kind)
	if err != nil {
		return apigen.Library{}, fmt.Errorf("library %s: %w", managed.ID, err)
	}

	scan, err := scanStatusResponse(status)
	if err != nil {
		return apigen.Library{}, fmt.Errorf("library %s: %w", managed.ID, err)
	}

	folders, err := folderResponses(overview.Folders)
	if err != nil {
		return apigen.Library{}, fmt.Errorf("library %s: %w", managed.ID, err)
	}

	return apigen.Library{
		ID:                 id,
		Name:               managed.Name,
		Kind:               kind,
		Available:          overview.Available,
		ItemCount:          overview.ItemCount,
		Scan:               scan,
		Folders:            folders,
		MetadataLanguage:   managed.MetadataLanguage,
		Options:            optionsResponse(managed.Options),
		ImportedFromConfig: managed.ImportedFromConfig,
	}, nil
}

// folderResponses never returns nil: clients iterate the array.
func folderResponses(folders []applibrary.FolderOverview) ([]apigen.LibraryFolder, error) {
	responses := make([]apigen.LibraryFolder, 0, len(folders))

	for _, folder := range folders {
		id, err := parseStoredID("folder", folder.Folder.ID)
		if err != nil {
			return nil, err
		}

		responses = append(responses, apigen.LibraryFolder{ID: id, Path: folder.Folder.Path, Available: folder.Available})
	}

	return responses, nil
}

// onlineMetadataDefault is the per-library online switch reported until it is stored
// (ADR 0031 section 2: on by default; persisted from task M6-03 on).
const onlineMetadataDefault = true

func optionsResponse(options library.Options) apigen.LibraryOptions {
	onlineMetadata := onlineMetadataDefault

	return apigen.LibraryOptions{
		ScanExtras: options.ScanExtras, ReadNfo: options.ReadNFO, LocalArtwork: options.LocalArtwork,
		OnlineMetadata: &onlineMetadata,
	}
}

// libraryKindResponse rejects kinds the contract does not list rather than sending them.
func libraryKindResponse(kind library.Kind) (apigen.LibraryKind, error) {
	switch kind {
	case library.KindMovies:
		return apigen.LibraryKindMovies, nil
	case library.KindShows:
		return apigen.LibraryKindShows, nil
	case library.KindMusic:
		return "", fmt.Errorf("music library kind %q not in the contract yet", kind)
	case library.KindPhotos:
		return apigen.LibraryKindPhotos, nil
	case library.KindAudiobooks:
		return apigen.LibraryKindAudiobooks, nil
	default:
		return "", fmt.Errorf("unknown library kind %q", kind)
	}
}

func scanStatusResponse(status appscan.Status) (apigen.ScanStatus, error) {
	state, err := scanStateResponse(status.State)
	if err != nil {
		return apigen.ScanStatus{}, err
	}

	return apigen.ScanStatus{
		State:      state,
		StartedAt:  optionalTimestamp(status.StartedAt),
		FinishedAt: optionalTimestamp(status.FinishedAt),
		ErrorCode:  optionalString(string(status.ErrorCode)),
		Counts: apigen.ScanCounts{
			Added:     status.Counts.Added,
			Updated:   status.Counts.Updated,
			Removed:   status.Counts.Removed,
			Unchanged: status.Counts.Unchanged,
			Failed:    status.Counts.Failed,
			Skipped:   status.Counts.Skipped,
		},
	}, nil
}

func scanStateResponse(state appscan.State) (apigen.ScanState, error) {
	switch state {
	case appscan.StateIdle:
		return apigen.ScanStateIdle, nil
	case appscan.StateRunning:
		return apigen.ScanStateRunning, nil
	case appscan.StateFailed:
		return apigen.ScanStateFailed, nil
	default:
		return "", fmt.Errorf("unknown scan state %q", state)
	}
}

// parseStoredID converts a stored canonical UUID text. A malformed one is broken stored
// data, which the handlers answer with 500.
func parseStoredID(entity, text string) (uuid.UUID, error) {
	id, err := uuid.Parse(text)
	if err != nil {
		return uuid.UUID{}, fmt.Errorf("convert stored %s id %q: %w", entity, text, err)
	}

	return id, nil
}
