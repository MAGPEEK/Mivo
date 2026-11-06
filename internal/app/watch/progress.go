package watch

import (
	"context"
	"fmt"
	"math"

	"github.com/magpeek/mivo/internal/app/access"
	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/watch"
)

// ReportProgress records that the viewer's profile (from ctx) played the item up to position
// seconds (domain/watch ApplyProgress with the catalog duration). Players call it every 10 s,
// so it costs one item lookup and one store update, nothing more.
//
// Errors: no viewer wraps domain.ErrUnauthenticated, no selected profile
// access.ErrProfileRequired, a NaN or infinite position domain.ErrInvalid, an unknown or
// hidden item domain.ErrNotFound and a kind without watch state ErrNoWatchState.
func (service *Service) ReportProgress(ctx context.Context, itemID string, position float64) error {
	viewer, err := access.ProfileViewerFrom(ctx)
	if err != nil {
		return fmt.Errorf("report progress of %q: %w", itemID, err)
	}

	if math.IsNaN(position) || math.IsInf(position, 0) {
		return fmt.Errorf("report progress of %q: position %v is not finite: %w", itemID, position, domain.ErrInvalid)
	}

	target, err := service.stateTarget(ctx, itemID)
	if err != nil {
		return fmt.Errorf("report progress: %w", err)
	}

	now := service.deps.Clock()
	update := StateUpdate{
		ProfileID: viewer.ProfileID,
		Keys:      []StateKey{keyOf(target)},
		Change: func(state watch.State) watch.State {
			return watch.ApplyProgress(state, position, target.DurationSeconds, now)
		},
		Now: now,
	}

	if err := service.deps.Store.UpdateStates(ctx, update); err != nil {
		return fmt.Errorf("report progress of %q: %w", itemID, err)
	}

	return nil
}

// stateTarget looks the item up for the viewer and checks that its kind carries watch state.
func (service *Service) stateTarget(ctx context.Context, itemID string) (Target, error) {
	target, err := service.deps.Items.LookupWatchTarget(ctx, itemID)
	if err != nil {
		return Target{}, fmt.Errorf("look up item %q: %w", itemID, err)
	}

	if !watch.SupportsWatchState(target.Kind) {
		return Target{}, fmt.Errorf("item %q of kind %q: %w", itemID, target.Kind, ErrNoWatchState)
	}

	return target, nil
}

func keyOf(target Target) StateKey {
	return StateKey{ItemID: target.ItemID, AttachKey: target.AttachKey}
}
