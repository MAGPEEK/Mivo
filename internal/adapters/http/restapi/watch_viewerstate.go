package restapi

import (
	"context"
	"fmt"
	"math"
	"net/http"

	"github.com/magpeek/mivo/internal/adapters/http/apigen"
	appwatch "github.com/magpeek/mivo/internal/app/watch"
	"github.com/magpeek/mivo/internal/domain/library"
)

// ViewerStates reads the signed-in profile's watch state of many items at once. Implemented
// by app/watch.Service.
type ViewerStates interface {
	// States returns the state of every item that has one, keyed by item ID.
	States(ctx context.Context, items []appwatch.ItemRef) (map[string]appwatch.ItemState, error)
}

// viewerStateResponse maps a state to the contract. A leaf item without stored state sends
// none ("absent means not started and never watched"), and so does a show or season without
// episodes.
func viewerStateResponse(state appwatch.ItemState) *apigen.ViewerState {
	response := apigen.ViewerState{
		Watched:         state.State.Watched,
		PositionSeconds: state.State.PositionSeconds,
		PlayCount:       clampInt32(state.State.PlayCount),
	}

	if !state.State.LastPlayed.IsZero() {
		lastPlayed := state.State.LastPlayed.UTC()
		response.LastPlayedAt = &lastPlayed
	}

	if state.Episodes.Total > 0 {
		unwatched := clampInt32(state.Episodes.Unwatched)
		response.UnwatchedCount = &unwatched

		return &response
	}

	if response == (apigen.ViewerState{}) {
		return nil
	}

	return &response
}

func clampInt32(value int) int32 {
	return int32(min(max(value, 0), math.MaxInt32))
}

// applyViewerStates adds the viewer state to every summary with one States call; a response
// of 100 items costs one lookup. Without a watch service the summaries stay plain. A state
// that is already set, by the watch rows, is kept.
func (api *API) applyViewerStates(ctx context.Context, operation string, summaries []*apigen.ItemSummary) error {
	if api.watch == nil {
		return nil
	}

	refs := make([]appwatch.ItemRef, 0, len(summaries))

	for _, summary := range summaries {
		if summary.ViewerState == nil {
			refs = append(refs, appwatch.ItemRef{ID: summary.ID.String(), Kind: library.ItemKind(summary.Kind)})
		}
	}

	if len(refs) == 0 {
		return nil
	}

	states, err := api.watch.States(ctx, refs)
	if err != nil {
		return viewerStateFailure(operation, err)
	}

	for _, summary := range summaries {
		if state, found := states[summary.ID.String()]; found && summary.ViewerState == nil {
			summary.ViewerState = viewerStateResponse(state)
		}
	}

	return nil
}

// applyDetailViewerState adds the viewer state to an item detail.
func (api *API) applyDetailViewerState(ctx context.Context, operation string, detail *apigen.ItemDetail) error {
	if api.watch == nil {
		return nil
	}

	id := detail.ID.String()

	states, err := api.watch.States(ctx, []appwatch.ItemRef{{ID: id, Kind: library.ItemKind(detail.Kind)}})
	if err != nil {
		return viewerStateFailure(operation, err)
	}

	if state, found := states[id]; found {
		detail.ViewerState = viewerStateResponse(state)
	}

	return nil
}

// viewerStateFailure keeps a missing session or profile (401, 403) as the client's problem.
func viewerStateFailure(operation string, err error) error {
	return operationFailure(operation, fmt.Errorf("viewer states: %w", err),
		http.StatusUnauthorized, http.StatusForbidden)
}

// pointersTo returns pointers to the elements, so one batch can decorate several slices.
func pointersTo(items []apigen.ItemSummary) []*apigen.ItemSummary {
	pointers := make([]*apigen.ItemSummary, len(items))
	for index := range items {
		pointers[index] = &items[index]
	}

	return pointers
}
