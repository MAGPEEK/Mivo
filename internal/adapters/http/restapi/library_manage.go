package restapi

import (
	"context"
	"fmt"
	"net/http"
	"path"

	"github.com/magpeek/mivo/internal/adapters/http/apigen"
	applibrary "github.com/magpeek/mivo/internal/app/library"
	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/library"
)

// Operation names of the library management endpoints, as in api/openapi.yaml, for logs.
const (
	operationCreateLibrary   = "createLibrary"
	operationUpdateLibrary   = "updateLibrary"
	operationDeleteLibrary   = "deleteLibrary"
	operationListFolderRoots = "listFolderRoots"
	operationListFolders     = "listFolders"
)

// librariesPath is where a created library is located: Location is librariesPath + "/" + ID.
const librariesPath = "/api/v1/libraries"

// LibraryManager is the port that creates, changes and removes libraries, implemented by
// app/library.Manager. Its errors carry the typed folder and field errors that
// ProblemForError turns into problem codes.
type LibraryManager interface {
	// CreateLibrary validates and stores a library and starts its scan.
	CreateLibrary(ctx context.Context, request applibrary.CreateRequest) (library.ManagedLibrary, error)
	// UpdateLibrary applies the patch; an unknown ID wraps domain.ErrNotFound.
	UpdateLibrary(ctx context.Context, id string, patch applibrary.UpdatePatch) (library.ManagedLibrary, error)
	// RemoveLibrary deletes a library with its catalog; an unknown ID wraps domain.ErrNotFound.
	RemoveLibrary(ctx context.Context, id string) error
}

// errManagementNotWired answers the management operations of a server whose composition
// root passes no manager yet; M3-19l makes the manager required.
func errManagementNotWired(operation string) error {
	return fmt.Errorf("%s: library management is not configured: %w", operation, domain.ErrUnavailable)
}

// managementStatuses are the problem statuses createLibrary and updateLibrary document: the
// folder and field codes (400), an unknown library (404), a taken name or an overlapping
// folder (409) and a scan that does not stop in time (503).
func managementStatuses() []int32 {
	return []int32{
		http.StatusBadRequest, http.StatusNotFound, http.StatusConflict, http.StatusServiceUnavailable,
	}
}

// CreateLibrary adds a library and answers 201 with its location. The scan status in the
// body already shows the scan the manager started.
func (api *API) CreateLibrary(ctx context.Context, request apigen.CreateLibraryRequestObject) (apigen.CreateLibraryResponseObject, error) {
	if api.manager == nil {
		return nil, errManagementNotWired(operationCreateLibrary)
	}

	if request.Body == nil {
		return nil, fmt.Errorf("%s: missing request body: %w", operationCreateLibrary, domain.ErrInvalid)
	}

	stored, err := api.manager.CreateLibrary(ctx, createRequest(*request.Body))
	if err != nil {
		return nil, operationFailure(operationCreateLibrary, err, managementStatuses()...)
	}

	response, err := api.managedResponse(ctx, stored)
	if err != nil {
		return nil, operationFailure(operationCreateLibrary, err)
	}

	location := path.Join(librariesPath, response.ID.String())

	return apigen.CreateLibrary201JSONResponse{
		Body:    response,
		Headers: apigen.CreateLibrary201ResponseHeaders{Location: &location},
	}, nil
}

// UpdateLibrary changes the given properties of a library and answers the library as it is
// stored now.
func (api *API) UpdateLibrary(ctx context.Context, request apigen.UpdateLibraryRequestObject) (apigen.UpdateLibraryResponseObject, error) {
	if api.manager == nil {
		return nil, errManagementNotWired(operationUpdateLibrary)
	}

	if request.Body == nil {
		return nil, fmt.Errorf("%s: missing request body: %w", operationUpdateLibrary, domain.ErrInvalid)
	}

	stored, err := api.manager.UpdateLibrary(ctx, request.LibraryID.String(), updatePatch(*request.Body))
	if err != nil {
		return nil, operationFailure(operationUpdateLibrary, err, managementStatuses()...)
	}

	response, err := api.managedResponse(ctx, stored)
	if err != nil {
		return nil, operationFailure(operationUpdateLibrary, err)
	}

	return apigen.UpdateLibrary200JSONResponse(response), nil
}

// DeleteLibrary removes a library and answers 204. Media files are never touched.
func (api *API) DeleteLibrary(ctx context.Context, request apigen.DeleteLibraryRequestObject) (apigen.DeleteLibraryResponseObject, error) {
	if api.manager == nil {
		return nil, errManagementNotWired(operationDeleteLibrary)
	}

	err := api.manager.RemoveLibrary(ctx, request.LibraryID.String())
	if err != nil {
		return nil, operationFailure(operationDeleteLibrary, err, http.StatusNotFound, http.StatusServiceUnavailable)
	}

	return apigen.DeleteLibrary204Response{}, nil
}

// managedResponse describes a stored library and adds the status of its scan.
func (api *API) managedResponse(ctx context.Context, stored library.ManagedLibrary) (apigen.Library, error) {
	overview, err := api.libraries.Describe(ctx, stored)
	if err != nil {
		return apigen.Library{}, fmt.Errorf("describe library %s: %w", stored.ID, err)
	}

	return libraryResponse(overview, api.scan.Status(stored.ID))
}
