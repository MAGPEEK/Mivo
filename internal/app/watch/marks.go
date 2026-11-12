package watch

import (
	"context"
	"fmt"
	"time"

	"github.com/magpeek/mivo/internal/app/access"
	domainaccess "github.com/magpeek/mivo/internal/domain/access"
	"github.com/magpeek/mivo/internal/domain/watch"
)

// MarkWatched marks the item watched for the viewer's profile (domain/watch MarkWatched). A
// show or season has no state of its own: the mark applies to each of its episodes. Errors as
// ReportProgress, without the position check; ErrNoWatchState also for kinds that cannot be
// marked.
func (service *Service) MarkWatched(ctx context.Context, itemID string) error {
	return service.applyToMarkTarget(ctx, "mark watched", itemID, func(now time.Time) StateChange {
		return func(state watch.State) watch.State { return watch.MarkWatched(state, now) }
	})
}

// MarkUnwatched marks the item unwatched for the viewer's profile (domain/watch
// MarkUnwatched), for a show or season each of its episodes. Errors as MarkWatched.
func (service *Service) MarkUnwatched(ctx context.Context, itemID string) error {
	return service.applyToMarkTarget(ctx, "mark unwatched", itemID, func(time.Time) StateChange {
		return watch.MarkUnwatched
	})
}

// HideFromContinueWatching removes the item from the profile's Continue watching until a new
// position is reported; for a show or season each of its episodes. An item that is not in
// progress is left unchanged. Errors as MarkWatched.
func (service *Service) HideFromContinueWatching(ctx context.Context, itemID string) error {
	return service.applyToMarkTarget(ctx, "hide from continue watching", itemID, func(time.Time) StateChange {
		return hide
	})
}

// applyToMarkTarget resolves the viewer and the item's state keys and stores the change the
// operation builds for the current time.
func (service *Service) applyToMarkTarget(ctx context.Context, operation, itemID string, changeAt func(time.Time) StateChange) error {
	viewer, err := access.ProfileViewerFrom(ctx)
	if err != nil {
		return fmt.Errorf("%s %q: %w", operation, itemID, err)
	}

	keys, err := service.markKeys(ctx, itemID, viewer.Restriction())
	if err != nil {
		return fmt.Errorf("%s: %w", operation, err)
	}

	if len(keys) == 0 {
		return nil
	}

	now := service.deps.Clock()
	update := StateUpdate{ProfileID: viewer.ProfileID, Keys: keys, Change: changeAt(now), Now: now}

	if err := service.deps.Store.UpdateStates(ctx, update); err != nil {
		return fmt.Errorf("%s %q: %w", operation, itemID, err)
	}

	return nil
}

// markKeys returns the state keys a mark of the item applies to: the item itself, or the
// episodes of a show or season (none when it has no episodes yet).
func (service *Service) markKeys(ctx context.Context, itemID string, restriction domainaccess.Restriction) ([]StateKey, error) {
	target, err := service.deps.Items.LookupWatchTarget(ctx, itemID)
	if err != nil {
		return nil, fmt.Errorf("look up item %q: %w", itemID, err)
	}

	if !watch.IsMarkTarget(target.Kind) {
		return nil, fmt.Errorf("item %q of kind %q: %w", itemID, target.Kind, ErrNoWatchState)
	}

	if watch.SupportsWatchState(target.Kind) {
		return []StateKey{keyOf(target)}, nil
	}

	episodes, err := service.deps.Episodes.ListEpisodes(ctx, target.ItemID, restriction)
	if err != nil {
		return nil, fmt.Errorf("list episodes of %q: %w", itemID, err)
	}

	keys := make([]StateKey, 0, len(episodes))
	for _, episode := range episodes {
		if watch.SupportsWatchState(episode.Kind) {
			keys = append(keys, keyOf(episode))
		}
	}

	return keys, nil
}

// hide sets the hidden flag of an in-progress state; other states are not in Continue
// watching, so there is nothing to hide.
func hide(state watch.State) watch.State {
	if state.InProgress() {
		state.Hidden = true
	}

	return state
}
