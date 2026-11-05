package identify

import (
	"context"
	"fmt"

	"github.com/magpeek/mivo/internal/app/enrich"
	"github.com/magpeek/mivo/internal/domain"
)

// RefreshItem queues an online refresh of the item and, for a show, of its seasons and
// episodes, and returns at once; the queue retries while the provider is unreachable. It
// refuses with domain.ErrConflict when online metadata is switched off globally, for the
// item's library or for its provider; a kind without provider wraps domain.ErrInvalid, an
// unknown item domain.ErrNotFound.
func (service *Service) RefreshItem(ctx context.Context, itemID string) error {
	current, err := service.loadOnlineTarget(ctx, itemID)
	if err != nil {
		return fmt.Errorf("refresh item %s: %w", itemID, err)
	}

	if err := service.enqueueWithChildren(ctx, current, enrich.ReasonRefresh); err != nil {
		return fmt.Errorf("refresh item %s: %w", itemID, err)
	}

	return nil
}

// RefreshLibrary queues an online refresh of every item of the library whose match is not
// disabled and returns at once. It refuses with domain.ErrConflict when online metadata is
// switched off globally or for the library; an unknown library wraps domain.ErrNotFound.
func (service *Service) RefreshLibrary(ctx context.Context, libraryID string) error {
	settings, err := service.deps.Settings.OnlineSettings(ctx)
	if err != nil {
		return fmt.Errorf("refresh library %s: read online settings: %w", libraryID, err)
	}

	libraryOnline, err := service.deps.Libraries.LibraryOnlineEnabled(ctx, libraryID)
	if err != nil {
		return fmt.Errorf("refresh library %s: %w", libraryID, err)
	}

	if !settings.Enabled || !libraryOnline {
		return fmt.Errorf("refresh library %s: online metadata is switched off: %w", libraryID, domain.ErrConflict)
	}

	itemIDs, err := service.deps.Libraries.MatchableItems(ctx, libraryID)
	if err != nil {
		return fmt.Errorf("refresh library %s: list items: %w", libraryID, err)
	}

	if err := service.enqueueAll(ctx, itemIDs, enrich.ReasonRefresh); err != nil {
		return fmt.Errorf("refresh library %s: %w", libraryID, err)
	}

	return nil
}
