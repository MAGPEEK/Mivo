package library

import (
	"context"
	"fmt"
	"slices"

	"github.com/magpeek/mivo/internal/domain/library"
)

// UpdateLibrary applies the patch. Changed folders or options cancel a running scan, store,
// announce the change and start a scan; a change of name or language only stores and
// announces. An unknown ID wraps domain.ErrNotFound.
func (manager *Manager) UpdateLibrary(ctx context.Context, id string, patch UpdatePatch,
) (library.ManagedLibrary, error) {
	manager.mutex.Lock()
	defer manager.mutex.Unlock()

	current, err := manager.deps.Catalog.GetManaged(ctx, id)
	if err != nil {
		return library.ManagedLibrary{}, fmt.Errorf("update library %q: %w", id, err)
	}

	updated, err := manager.applyPatch(ctx, current, patch)
	if err != nil {
		return library.ManagedLibrary{}, fmt.Errorf("update library %q: %w", id, err)
	}

	if err := manager.validate(ctx, updated); err != nil {
		return library.ManagedLibrary{}, fmt.Errorf("update library %q: %w", id, err)
	}

	rescan := needsRescan(current, updated)
	if rescan {
		if err := manager.stopScan(ctx, id); err != nil {
			return library.ManagedLibrary{}, fmt.Errorf("update library %q: %w", id, err)
		}
	}

	if err := manager.deps.Writer.UpdateLibrary(ctx, updated); err != nil {
		manager.resumeAfterRejectedUpdate(ctx, id, rescan)

		return library.ManagedLibrary{}, fmt.Errorf("update library %q: %w", id, storeWriteError(err))
	}

	if manager.deps.Events != nil {
		manager.deps.Events.Changed(id)
	}

	if rescan {
		manager.startScan(ctx, id)
	}

	stored, err := manager.deps.Catalog.GetManaged(ctx, id)
	if err != nil {
		return library.ManagedLibrary{}, fmt.Errorf("update library %q: reload: %w", id, err)
	}

	return stored, nil
}

// resumeAfterRejectedUpdate undoes the stop of a rescanning update the store rejected: the
// scan was cancelled and the trigger suspended for nothing, so the trigger is told to resume
// at once instead of after its hold timeout, and the scan is brought back.
func (manager *Manager) resumeAfterRejectedUpdate(ctx context.Context, id string, rescan bool) {
	if !rescan {
		return
	}

	if manager.deps.Events != nil {
		manager.deps.Events.Changed(id)
	}

	manager.startScan(ctx, id)
}

func (manager *Manager) applyPatch(ctx context.Context, current library.ManagedLibrary,
	patch UpdatePatch,
) (library.ManagedLibrary, error) {
	updated := current
	updated.Folders = slices.Clone(current.Folders)

	if patch.Name != nil {
		updated.Name = *patch.Name
	}

	if patch.MetadataLanguage != nil {
		updated.MetadataLanguage = *patch.MetadataLanguage
	}

	if patch.Options != nil {
		updated.Options = *patch.Options
	}

	if patch.Folders != nil {
		folders, err := manager.resolveFolders(ctx, *patch.Folders, current.Folders)
		if err != nil {
			return library.ManagedLibrary{}, err
		}

		updated.Folders = folders
	}

	return updated, nil
}

// needsRescan is true when the scan would see something different: another folder set or
// other options.
func needsRescan(before, after library.ManagedLibrary) bool {
	return before.Options != after.Options || !slices.Equal(before.Folders, after.Folders)
}
