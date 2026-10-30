package restapi

import (
	"fmt"
	"time"
	"uuid"

	"github.com/magpeek/mivo/internal/adapters/http/apigen"
	applibrary "github.com/magpeek/mivo/internal/app/library"
	appscan "github.com/magpeek/mivo/internal/app/scan"
	"github.com/magpeek/mivo/internal/domain/library"
)

// timestampPrecision is the precision of every timestamp the API sends. JavaScript dates
// hold milliseconds, and the catalog stores modification times in milliseconds.
const timestampPrecision = time.Millisecond

// libraryResponse combines a library with the status of its scan. The root path is left
// out on purpose: the API never exposes file system paths (ADR 0022).
func libraryResponse(overview applibrary.Overview, status appscan.Status) (apigen.Library, error) {
	id, err := parseStoredID("library", overview.Library.ID)
	if err != nil {
		return apigen.Library{}, err
	}

	kind, err := libraryKindResponse(overview.Library.Kind)
	if err != nil {
		return apigen.Library{}, fmt.Errorf("library %s: %w", overview.Library.ID, err)
	}

	scan, err := scanStatusResponse(status)
	if err != nil {
		return apigen.Library{}, fmt.Errorf("library %s: %w", overview.Library.ID, err)
	}

	return apigen.Library{
		ID:        id,
		Name:      overview.Library.Name,
		Kind:      kind,
		Available: overview.Available,
		ItemCount: overview.ItemCount,
		Scan:      scan,
	}, nil
}

// libraryKindResponse rejects kinds the contract does not list rather than sending them.
func libraryKindResponse(kind library.Kind) (apigen.LibraryKind, error) {
	switch kind {
	case library.KindMovies:
		return apigen.LibraryKindMovies, nil
	case library.KindShows:
		return apigen.LibraryKindShows, nil
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

// apiTimestamp converts a time to what the contract sends: UTC with millisecond precision,
// encoded as RFC 3339 by encoding/json.
func apiTimestamp(moment time.Time) time.Time {
	return moment.UTC().Truncate(timestampPrecision)
}

// optionalTimestamp omits the zero time, which means "not yet" in the use cases.
func optionalTimestamp(moment time.Time) *time.Time {
	if moment.IsZero() {
		return nil
	}

	converted := apiTimestamp(moment)

	return &converted
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
