package restapi

import (
	"context"

	"github.com/magpeek/mivo/internal/adapters/http/apigen"
)

// The watch state operations of ADR 0032 section 8 answer 501 until their handlers exist
// (task M7-23), so the player and the detail and Home screens can be built against the
// contract while the server keeps compiling.

// ReportItemProgress is a stub until M7-23.
func (api *API) ReportItemProgress(_ context.Context, _ apigen.ReportItemProgressRequestObject) (apigen.ReportItemProgressResponseObject, error) {
	problem := notImplementedProblem()

	return apigen.ReportItemProgressdefaultApplicationProblemPlusJSONResponse{Body: problem, StatusCode: int(problem.Status)}, nil
}

// MarkItemWatched is a stub until M7-23.
func (api *API) MarkItemWatched(_ context.Context, _ apigen.MarkItemWatchedRequestObject) (apigen.MarkItemWatchedResponseObject, error) {
	problem := notImplementedProblem()

	return apigen.MarkItemWatcheddefaultApplicationProblemPlusJSONResponse{Body: problem, StatusCode: int(problem.Status)}, nil
}

// MarkItemUnwatched is a stub until M7-23.
func (api *API) MarkItemUnwatched(_ context.Context, _ apigen.MarkItemUnwatchedRequestObject) (apigen.MarkItemUnwatchedResponseObject, error) {
	problem := notImplementedProblem()

	return apigen.MarkItemUnwatcheddefaultApplicationProblemPlusJSONResponse{Body: problem, StatusCode: int(problem.Status)}, nil
}

// HideFromContinueWatching is a stub until M7-23.
func (api *API) HideFromContinueWatching(_ context.Context, _ apigen.HideFromContinueWatchingRequestObject) (apigen.HideFromContinueWatchingResponseObject, error) {
	problem := notImplementedProblem()

	return apigen.HideFromContinueWatchingdefaultApplicationProblemPlusJSONResponse{Body: problem, StatusCode: int(problem.Status)}, nil
}
