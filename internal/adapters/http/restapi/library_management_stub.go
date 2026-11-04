package restapi

import (
	"context"
	"net/http"

	"github.com/magpeek/mivo/internal/adapters/http/apigen"
)

// CodeNotImplemented is the problem code of operations whose contract exists but whose
// handler is not built yet (M3-19k). It is not a published code: the stubs vanish with that task.
const CodeNotImplemented = "not_implemented"

// notImplementedProblem is the 501 answer of the library management stubs.
func notImplementedProblem() apigen.Problem {
	return NewProblem(http.StatusNotImplemented, CodeNotImplemented)
}

// CreateLibrary is a stub until M3-19k.
func (api *API) CreateLibrary(_ context.Context, _ apigen.CreateLibraryRequestObject) (apigen.CreateLibraryResponseObject, error) {
	problem := notImplementedProblem()

	return apigen.CreateLibrarydefaultApplicationProblemPlusJSONResponse{Body: problem, StatusCode: int(problem.Status)}, nil
}

// UpdateLibrary is a stub until M3-19k.
func (api *API) UpdateLibrary(_ context.Context, _ apigen.UpdateLibraryRequestObject) (apigen.UpdateLibraryResponseObject, error) {
	problem := notImplementedProblem()

	return apigen.UpdateLibrarydefaultApplicationProblemPlusJSONResponse{Body: problem, StatusCode: int(problem.Status)}, nil
}

// DeleteLibrary is a stub until M3-19k.
func (api *API) DeleteLibrary(_ context.Context, _ apigen.DeleteLibraryRequestObject) (apigen.DeleteLibraryResponseObject, error) {
	problem := notImplementedProblem()

	return apigen.DeleteLibrarydefaultApplicationProblemPlusJSONResponse{Body: problem, StatusCode: int(problem.Status)}, nil
}

// ListFolderRoots is a stub until M3-19k.
func (api *API) ListFolderRoots(_ context.Context, _ apigen.ListFolderRootsRequestObject) (apigen.ListFolderRootsResponseObject, error) {
	problem := notImplementedProblem()

	return apigen.ListFolderRootsdefaultApplicationProblemPlusJSONResponse{Body: problem, StatusCode: int(problem.Status)}, nil
}

// ListFolders is a stub until M3-19k.
func (api *API) ListFolders(_ context.Context, _ apigen.ListFoldersRequestObject) (apigen.ListFoldersResponseObject, error) {
	problem := notImplementedProblem()

	return apigen.ListFoldersdefaultApplicationProblemPlusJSONResponse{Body: problem, StatusCode: int(problem.Status)}, nil
}
