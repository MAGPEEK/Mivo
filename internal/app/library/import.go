package library

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"

	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/library"
)

// ImportResult says what ImportConfigured did.
type ImportResult struct {
	// Created are the libraries made from configured roots.
	Created []library.ManagedLibrary
	// Skipped are the roots that were recorded without creating a library because another
	// library already uses the folder or its name.
	Skipped []string
}

// ImportConfigured turns configured library definitions into managed libraries, once per
// root (ADR 0028). It never changes or deletes existing libraries. Invalid definitions fail
// before anything is written; every handled root is recorded in the ledger.
func (manager *Manager) ImportConfigured(ctx context.Context, definitions []library.Definition,
) (ImportResult, error) {
	if err := validateDefinitions(definitions); err != nil {
		return ImportResult{}, fmt.Errorf("import configured libraries: %w", err)
	}

	manager.mutex.Lock()
	defer manager.mutex.Unlock()

	roots := make([]string, 0, len(definitions))
	for _, definition := range definitions {
		roots = append(roots, definition.Root)
	}

	pending, err := manager.deps.Ledger.NeverImported(ctx, roots)
	if err != nil {
		return ImportResult{}, fmt.Errorf("import configured libraries: ledger: %w", err)
	}

	others, err := manager.deps.Catalog.ListManaged(ctx)
	if err != nil {
		return ImportResult{}, fmt.Errorf("import configured libraries: list libraries: %w", err)
	}

	result, err := manager.importPending(ctx, definitions, pending, others)
	if err != nil {
		return result, err
	}

	if err := manager.recordRoots(ctx, pending); err != nil {
		return result, err
	}

	return result, nil
}

// importPending imports the definitions whose root is pending, in order.
func (manager *Manager) importPending(ctx context.Context, definitions []library.Definition,
	pending []string, others []library.ManagedLibrary,
) (ImportResult, error) {
	var result ImportResult

	for _, definition := range definitions {
		if !slices.Contains(pending, definition.Root) {
			continue
		}

		created, skipped, err := manager.importOne(ctx, definition, others)
		if err != nil {
			return result, fmt.Errorf("import configured library %q: %w", definition.Name, err)
		}

		if skipped {
			result.Skipped = append(result.Skipped, definition.Root)
			continue
		}

		others = append(others, created)
		result.Created = append(result.Created, created)
	}

	return result, nil
}

// importOne creates the library or reports it skipped. Roots used by another library and
// names already taken are skipped with a log entry; any other failure aborts the import.
func (manager *Manager) importOne(ctx context.Context, definition library.Definition,
	others []library.ManagedLibrary,
) (created library.ManagedLibrary, skipped bool, err error) {
	candidate := library.FromDefinition(definition)

	if err := manager.validateAgainst(candidate, others); err != nil {
		// A taken name is logged at warn, because the configured library would otherwise vanish
		// silently. The root is still recorded in the ledger, so renaming the definition later
		// does not import it; the user adds the library in the UI instead.
		if errors.Is(err, domain.ErrConflict) {
			manager.logSkipped(ctx, slog.LevelWarn, definition, err)

			return library.ManagedLibrary{}, true, nil
		}

		if hasFolderReason(err, library.ReasonOverlap) {
			manager.logSkipped(ctx, slog.LevelInfo, definition, err)

			return library.ManagedLibrary{}, true, nil
		}

		return library.ManagedLibrary{}, false, err
	}

	created, err = manager.deps.Writer.CreateLibrary(ctx, candidate)
	if err != nil {
		return library.ManagedLibrary{}, false, fmt.Errorf("store: %w", err)
	}

	return created, false, nil
}

func (manager *Manager) logSkipped(ctx context.Context, level slog.Level, definition library.Definition, reason error) {
	manager.deps.Logger.Log(ctx, level, "configured library not imported",
		slog.String("name", definition.Name), slog.String("root", definition.Root),
		slog.Any("reason", reason))
}

func (manager *Manager) recordRoots(ctx context.Context, roots []string) error {
	if len(roots) == 0 {
		return nil
	}

	if err := manager.deps.Ledger.RecordImported(ctx, roots); err != nil {
		return fmt.Errorf("import configured libraries: record roots: %w", err)
	}

	return nil
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
