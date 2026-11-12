package restapi

import (
	"context"
	"fmt"
	"math"
	"net/http"

	"github.com/magpeek/mivo/internal/adapters/http/apigen"
)

// WatchActions changes the signed-in profile's watch state. Implemented by app/watch.Service.
// A failure wraps access.ErrProfileRequired (no profile selected), domain.ErrNotFound (unknown
// or hidden item) or domain.ErrInvalid (the item's kind has no watch state).
type WatchActions interface {
	// ReportProgress stores the resume position of an item.
	ReportProgress(ctx context.Context, itemID string, position float64) error
	// MarkWatched marks an item, or every episode of a show or season, as watched.
	MarkWatched(ctx context.Context, itemID string) error
	// MarkUnwatched clears the watched mark and the resume position.
	MarkUnwatched(ctx context.Context, itemID string) error
	// HideFromContinueWatching removes an in-progress item from the Continue Watching row.
	HideFromContinueWatching(ctx context.Context, itemID string) error
}

// WatchService is what the watch endpoints and the viewerState decoration need: the actions
// and the batched state lookup.
type WatchService interface {
	WatchActions
	ViewerStates
}

// Operation names of the watch endpoints, for logs, and the field of a bad progress report.
const (
	operationReportItemProgress       = "reportItemProgress"
	operationMarkItemWatched          = "markItemWatched"
	operationMarkItemUnwatched        = "markItemUnwatched"
	operationHideFromContinueWatching = "hideFromContinueWatching"
	fieldPositionSeconds              = "positionSeconds"
)

// watchFailure keeps the failures the contract publishes for the watch operations: 400 for a
// kind without watch state, 401 and 403 for a missing session or profile, 404 for an unknown
// or hidden item. Everything else is a server fault.
func watchFailure(operation string, err error) error {
	return operationFailure(operation, err,
		http.StatusBadRequest, http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound)
}

// ReportItemProgress stores the resume position. A negative position answers 400 "invalid"
// with field positionSeconds.
func (api *API) ReportItemProgress(ctx context.Context, request apigen.ReportItemProgressRequestObject) (apigen.ReportItemProgressResponseObject, error) {
	if request.Body == nil {
		return nil, fmt.Errorf("%s: %w", operationReportItemProgress, invalidParameter(fieldPositionSeconds, "missing body"))
	}

	position := request.Body.PositionSeconds
	if position < 0 || math.IsNaN(position) || math.IsInf(position, 0) {
		return nil, fmt.Errorf("%s: %w", operationReportItemProgress,
			invalidParameter(fieldPositionSeconds, "not a finite number of at least 0"))
	}

	if api.watch == nil {
		problem := notImplementedProblem()

		return apigen.ReportItemProgressdefaultApplicationProblemPlusJSONResponse{Body: problem, StatusCode: int(problem.Status)}, nil
	}

	if err := api.watch.ReportProgress(ctx, request.ItemID.String(), position); err != nil {
		return nil, watchFailure(operationReportItemProgress, err)
	}

	return apigen.ReportItemProgress204Response{}, nil
}

// MarkItemWatched marks an item, or the episodes of a show or season, as watched.
func (api *API) MarkItemWatched(ctx context.Context, request apigen.MarkItemWatchedRequestObject) (apigen.MarkItemWatchedResponseObject, error) {
	if api.watch == nil {
		problem := notImplementedProblem()

		return apigen.MarkItemWatcheddefaultApplicationProblemPlusJSONResponse{Body: problem, StatusCode: int(problem.Status)}, nil
	}

	if err := api.watch.MarkWatched(ctx, request.ItemID.String()); err != nil {
		return nil, watchFailure(operationMarkItemWatched, err)
	}

	return apigen.MarkItemWatched204Response{}, nil
}

// MarkItemUnwatched clears the watched mark and the resume position.
func (api *API) MarkItemUnwatched(ctx context.Context, request apigen.MarkItemUnwatchedRequestObject) (apigen.MarkItemUnwatchedResponseObject, error) {
	if api.watch == nil {
		problem := notImplementedProblem()

		return apigen.MarkItemUnwatcheddefaultApplicationProblemPlusJSONResponse{Body: problem, StatusCode: int(problem.Status)}, nil
	}

	if err := api.watch.MarkUnwatched(ctx, request.ItemID.String()); err != nil {
		return nil, watchFailure(operationMarkItemUnwatched, err)
	}

	return apigen.MarkItemUnwatched204Response{}, nil
}

// HideFromContinueWatching removes an in-progress item from the Continue Watching row.
func (api *API) HideFromContinueWatching(ctx context.Context, request apigen.HideFromContinueWatchingRequestObject) (apigen.HideFromContinueWatchingResponseObject, error) {
	if api.watch == nil {
		problem := notImplementedProblem()

		return apigen.HideFromContinueWatchingdefaultApplicationProblemPlusJSONResponse{Body: problem, StatusCode: int(problem.Status)}, nil
	}

	if err := api.watch.HideFromContinueWatching(ctx, request.ItemID.String()); err != nil {
		return nil, watchFailure(operationHideFromContinueWatching, err)
	}

	return apigen.HideFromContinueWatching204Response{}, nil
}
