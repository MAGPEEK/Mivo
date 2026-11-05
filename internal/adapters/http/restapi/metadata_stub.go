package restapi

import (
	"context"
	"net/http"

	"github.com/magpeek/mivo/internal/adapters/http/apigen"
)

// CodeNotImplemented is the problem code of operations or parameter values whose contract
// exists but whose handler is not built yet (online metadata until M6-13; the rows endpoints of
// an API wired without a rows service). It is published since API 1.5.0 so
// clients can show "not available on this server" instead of a generic error.
const CodeNotImplemented = "not_implemented"

// notImplementedProblem is the 501 answer of the contract stubs.
func notImplementedProblem() apigen.Problem {
	return NewProblem(http.StatusNotImplemented, CodeNotImplemented)
}

// SearchIdentifyCandidates is a stub until M6-13.
func (api *API) SearchIdentifyCandidates(_ context.Context, _ apigen.SearchIdentifyCandidatesRequestObject) (apigen.SearchIdentifyCandidatesResponseObject, error) {
	problem := notImplementedProblem()

	return apigen.SearchIdentifyCandidatesdefaultApplicationProblemPlusJSONResponse{Body: problem, StatusCode: int(problem.Status)}, nil
}

// IdentifyItem is a stub until M6-13.
func (api *API) IdentifyItem(_ context.Context, _ apigen.IdentifyItemRequestObject) (apigen.IdentifyItemResponseObject, error) {
	problem := notImplementedProblem()

	return apigen.IdentifyItemdefaultApplicationProblemPlusJSONResponse{Body: problem, StatusCode: int(problem.Status)}, nil
}

// RemoveItemMatch is a stub until M6-13.
func (api *API) RemoveItemMatch(_ context.Context, _ apigen.RemoveItemMatchRequestObject) (apigen.RemoveItemMatchResponseObject, error) {
	problem := notImplementedProblem()

	return apigen.RemoveItemMatchdefaultApplicationProblemPlusJSONResponse{Body: problem, StatusCode: int(problem.Status)}, nil
}

// RefreshItemMetadata is a stub until M6-13.
func (api *API) RefreshItemMetadata(_ context.Context, _ apigen.RefreshItemMetadataRequestObject) (apigen.RefreshItemMetadataResponseObject, error) {
	problem := notImplementedProblem()

	return apigen.RefreshItemMetadatadefaultApplicationProblemPlusJSONResponse{Body: problem, StatusCode: int(problem.Status)}, nil
}

// RefreshLibraryMetadata is a stub until M6-13.
func (api *API) RefreshLibraryMetadata(_ context.Context, _ apigen.RefreshLibraryMetadataRequestObject) (apigen.RefreshLibraryMetadataResponseObject, error) {
	problem := notImplementedProblem()

	return apigen.RefreshLibraryMetadatadefaultApplicationProblemPlusJSONResponse{Body: problem, StatusCode: int(problem.Status)}, nil
}

// GetMetadataSettings is a stub until M6-13.
func (api *API) GetMetadataSettings(_ context.Context, _ apigen.GetMetadataSettingsRequestObject) (apigen.GetMetadataSettingsResponseObject, error) {
	problem := notImplementedProblem()

	return apigen.GetMetadataSettingsdefaultApplicationProblemPlusJSONResponse{Body: problem, StatusCode: int(problem.Status)}, nil
}

// UpdateMetadataSettings is a stub until M6-13.
func (api *API) UpdateMetadataSettings(_ context.Context, _ apigen.UpdateMetadataSettingsRequestObject) (apigen.UpdateMetadataSettingsResponseObject, error) {
	problem := notImplementedProblem()

	return apigen.UpdateMetadataSettingsdefaultApplicationProblemPlusJSONResponse{Body: problem, StatusCode: int(problem.Status)}, nil
}

// GetMetadataStatus is a stub until M6-13.
func (api *API) GetMetadataStatus(_ context.Context, _ apigen.GetMetadataStatusRequestObject) (apigen.GetMetadataStatusResponseObject, error) {
	problem := notImplementedProblem()

	return apigen.GetMetadataStatusdefaultApplicationProblemPlusJSONResponse{Body: problem, StatusCode: int(problem.Status)}, nil
}
