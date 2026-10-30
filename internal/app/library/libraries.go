package library

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/library"
)

// Overview is a library as clients see it: the stored summary and whether its root is
// reachable now. Scan status is added by the HTTP adapter from the scan service.
type Overview struct {
	library.Summary
	Available bool
}

// SyncLibraries validates every definition and makes the catalog match them (ADR 0022). It
// returns all configured libraries ordered by name. Each library the sync removed, with its
// catalog, is logged at warn level, so a typo in the configuration or an empty library list
// leaves a trace. An invalid definition and a root configured twice wrap domain.ErrInvalid
// and name the index; the catalog is not touched then.
func (service *Service) SyncLibraries(ctx context.Context, definitions []library.Definition) ([]library.Library, error) {
	if err := validateDefinitions(definitions); err != nil {
		return nil, fmt.Errorf("sync libraries: %w", err)
	}

	result, err := service.libraries.SyncLibraries(ctx, definitions, service.ids.NewID)
	if err != nil {
		// The store names the operation itself ("sync libraries: …").
		return nil, fmt.Errorf("library store: %w", err)
	}

	for _, removed := range result.Removed {
		service.logger.WarnContext(ctx, "library removed from the catalog because its folder is no longer configured",
			slog.String("library_id", removed.Library.ID),
			slog.String("name", removed.Library.Name),
			slog.String("root", removed.Library.Root),
			slog.Int64("item_count", removed.ItemCount))
	}

	return result.Libraries, nil
}

// validateDefinitions compares roots exactly as given: the configuration has cleaned them
// and rejected duplicates that differ only in case on Windows.
func validateDefinitions(definitions []library.Definition) error {
	firstIndexByRoot := make(map[string]int, len(definitions))

	for index, definition := range definitions {
		if err := definition.Validate(); err != nil {
			return fmt.Errorf("definition %d: %w", index, err)
		}

		if first, seen := firstIndexByRoot[definition.Root]; seen {
			return fmt.Errorf("definition %d: root %q is already used by definition %d: %w",
				index, definition.Root, first, domain.ErrInvalid)
		}

		firstIndexByRoot[definition.Root] = index
	}

	return nil
}

// ListLibraries returns every library ordered by name, each marked available when its root
// answers within the root check timeout. The list is never nil.
func (service *Service) ListLibraries(ctx context.Context) ([]Overview, error) {
	summaries, err := service.libraries.ListLibraries(ctx)
	if err != nil {
		return nil, fmt.Errorf("library store: %w", storedDataError(err))
	}

	roots := make([]string, 0, len(summaries))
	for _, summary := range summaries {
		roots = append(roots, summary.Library.Root)
	}

	available := service.roots.availableAll(ctx, roots)

	// A caller that gave up would otherwise get every library reported unavailable.
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("list libraries: %w", err)
	}

	overviews := make([]Overview, 0, len(summaries))
	for index, summary := range summaries {
		overviews = append(overviews, Overview{Summary: summary, Available: available[index]})
	}

	return overviews, nil
}

// GetLibrary returns one library with its availability. An unknown ID wraps
// domain.ErrNotFound.
func (service *Service) GetLibrary(ctx context.Context, id string) (Overview, error) {
	summary, err := service.libraries.GetLibrary(ctx, id)
	if err != nil {
		return Overview{}, fmt.Errorf("library store: %w", storedDataError(err))
	}

	available := service.roots.available(ctx, summary.Library.Root)

	if err := ctx.Err(); err != nil {
		return Overview{}, fmt.Errorf("get library %q: %w", id, err)
	}

	return Overview{Summary: summary, Available: available}, nil
}

// storedDataError hides domain.ErrInvalid from a library store read. No client value reaches
// those reads (an unknown ID is ErrNotFound), so ErrInvalid can only mean a stored row the
// domain cannot read: a server fault that must not turn into a "bad request" for the client.
// The message is kept for the log; every other error passes through unchanged.
func storedDataError(err error) error {
	if !errors.Is(err, domain.ErrInvalid) {
		return err
	}

	return errors.New("unreadable stored library data: " + err.Error())
}
