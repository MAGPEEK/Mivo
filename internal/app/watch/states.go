package watch

import (
	"context"
	"fmt"

	"github.com/magpeek/mivo/internal/app/access"
	"github.com/magpeek/mivo/internal/domain/library"
	"github.com/magpeek/mivo/internal/domain/watch"
)

// ItemRef names an item of a list to decorate: its ID and kind, as in an item summary.
type ItemRef struct {
	ID   string
	Kind library.ItemKind
}

// ItemState is the profile's watch state of a listed item.
//
// For a movie, episode or audiobook State is its stored state (zero when never played) and
// Episodes is zero. A show or season has no state of its own: Episodes counts its episodes
// and the unwatched ones, and State.Watched is true when it has episodes and all are watched.
type ItemState struct {
	State    watch.State
	Episodes EpisodeCount
}

// States returns the viewer's profile state of each listed item that can carry one, keyed by
// item ID; other kinds (photos, tracks, extras, ...) are absent. The items come from a list
// the viewer may already see, so they are not looked up again. A page costs at most two store
// calls: one for the items with their own state and one for the shows and seasons.
//
// Errors: no viewer wraps domain.ErrUnauthenticated, no selected profile
// access.ErrProfileRequired.
func (service *Service) States(ctx context.Context, items []ItemRef) (map[string]ItemState, error) {
	viewer, err := access.ProfileViewerFrom(ctx)
	if err != nil {
		return nil, fmt.Errorf("watch states: %w", err)
	}

	leafIDs, containerIDs := splitByStateSource(items)
	result := make(map[string]ItemState, len(leafIDs)+len(containerIDs))

	if err := service.addLeafStates(ctx, viewer.ProfileID, leafIDs, result); err != nil {
		return nil, fmt.Errorf("watch states: %w", err)
	}

	if err := service.addContainerStates(ctx, viewer.ProfileID, containerIDs, result); err != nil {
		return nil, fmt.Errorf("watch states: %w", err)
	}

	return result, nil
}

// splitByStateSource returns the distinct IDs of the items with their own state and of the
// shows and seasons, in list order; other kinds are dropped.
func splitByStateSource(items []ItemRef) (leafIDs, containerIDs []string) {
	seen := make(map[string]bool, len(items))

	for _, item := range items {
		if seen[item.ID] {
			continue
		}

		seen[item.ID] = true

		switch {
		case watch.SupportsWatchState(item.Kind):
			leafIDs = append(leafIDs, item.ID)
		case watch.IsMarkTarget(item.Kind):
			containerIDs = append(containerIDs, item.ID)
		}
	}

	return leafIDs, containerIDs
}

func (service *Service) addLeafStates(ctx context.Context, profileID string, itemIDs []string, result map[string]ItemState) error {
	if len(itemIDs) == 0 {
		return nil
	}

	states, err := service.deps.Store.States(ctx, profileID, itemIDs)
	if err != nil {
		return fmt.Errorf("read states of %d items: %w", len(itemIDs), err)
	}

	for _, itemID := range itemIDs {
		result[itemID] = ItemState{State: states[itemID]}
	}

	return nil
}

func (service *Service) addContainerStates(ctx context.Context, profileID string, containerIDs []string, result map[string]ItemState) error {
	if len(containerIDs) == 0 {
		return nil
	}

	counts, err := service.deps.Store.EpisodeCounts(ctx, profileID, containerIDs)
	if err != nil {
		return fmt.Errorf("count episodes of %d shows and seasons: %w", len(containerIDs), err)
	}

	for _, containerID := range containerIDs {
		count := counts[containerID]
		result[containerID] = ItemState{
			State:    watch.State{Watched: count.Total > 0 && count.Unwatched == 0},
			Episodes: count,
		}
	}

	return nil
}
