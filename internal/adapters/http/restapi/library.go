package restapi

import (
	"context"
	"net/http"

	"github.com/magpeek/mivo/internal/adapters/http/apigen"
	applibrary "github.com/magpeek/mivo/internal/app/library"
	appscan "github.com/magpeek/mivo/internal/app/scan"
	"github.com/magpeek/mivo/internal/domain/library"
)

// Operation names of the library and item endpoints, as in api/openapi.yaml, for logs.
const (
	operationListLibraries    = "listLibraries"
	operationGetLibrary       = "getLibrary"
	operationStartLibraryScan = "startLibraryScan"
	operationListLibraryItems = "listLibraryItems"
	operationGetItem          = "getItem"
)

// LibraryCatalog is the library part of app/library.Service.
type LibraryCatalog interface {
	// ListLibraries returns every library ordered by name with its availability.
	ListLibraries(ctx context.Context) ([]applibrary.Overview, error)
	// GetLibrary returns one library; an unknown ID wraps domain.ErrNotFound.
	GetLibrary(ctx context.Context, id string) (applibrary.Overview, error)
}

// ItemCatalog is the item part of app/library.Service.
type ItemCatalog interface {
	// ListItems returns one page of a library's items. A bad limit or cursor wraps
	// domain.ErrInvalid, an unknown library domain.ErrNotFound.
	ListItems(ctx context.Context, libraryID string, page library.PageRequest) (applibrary.ItemPage, error)
	// GetItem returns an item with its files and streams; an unknown ID wraps
	// domain.ErrNotFound.
	GetItem(ctx context.Context, itemID string) (applibrary.ItemDetail, error)
}

// LibraryService is the port of the library and item endpoints, implemented by
// app/library.Service. Handlers must go through it rather than a repository: the service
// turns unreadable stored rows into server faults, which a repository reports as
// domain.ErrInvalid. A nil service makes these operations answer 501 until M2-32 wires it.
type LibraryService interface {
	LibraryCatalog
	ItemCatalog
}

// ScanService is the port of the scan endpoint and the scan status of a library,
// implemented by app/scan.Service. The library endpoints need it next to LibraryService;
// while it is nil they answer 501, like the scan endpoint.
type ScanService interface {
	// Start starts a background scan, or returns the status of the one that runs. An
	// unknown library wraps domain.ErrNotFound, a closed service domain.ErrUnavailable.
	Start(ctx context.Context, libraryID string) (appscan.Status, error)
	// Status returns the status of the current or last scan of the library.
	Status(libraryID string) appscan.Status
}

// ListLibraries answers every library with its scan status. Every failure is a 500: the
// operation takes no client input (see operationFailure).
func (api *API) ListLibraries(ctx context.Context, _ apigen.ListLibrariesRequestObject) (apigen.ListLibrariesResponseObject, error) {
	if api.library == nil || api.scan == nil {
		return apigen.ListLibrariesdefaultApplicationProblemPlusJSONResponse(api.notImplemented(ctx, operationListLibraries)), nil
	}

	overviews, err := api.library.ListLibraries(ctx)
	if err != nil {
		return nil, operationFailure(operationListLibraries, err)
	}

	libraries := make([]apigen.Library, 0, len(overviews))

	for _, overview := range overviews {
		response, err := libraryResponse(overview, api.scan.Status(overview.Library.ID))
		if err != nil {
			return nil, operationFailure(operationListLibraries, err)
		}

		libraries = append(libraries, response)
	}

	return apigen.ListLibraries200JSONResponse{Libraries: libraries}, nil
}

// GetLibrary answers one library with its scan status. Only an unknown ID is a client
// mistake (404); every other failure is a 500.
func (api *API) GetLibrary(ctx context.Context, request apigen.GetLibraryRequestObject) (apigen.GetLibraryResponseObject, error) {
	if api.library == nil || api.scan == nil {
		return apigen.GetLibrarydefaultApplicationProblemPlusJSONResponse(api.notImplemented(ctx, operationGetLibrary)), nil
	}

	libraryID := request.LibraryID.String()

	overview, err := api.library.GetLibrary(ctx, libraryID)
	if err != nil {
		return nil, operationFailure(operationGetLibrary, err, http.StatusNotFound)
	}

	response, err := libraryResponse(overview, api.scan.Status(libraryID))
	if err != nil {
		return nil, operationFailure(operationGetLibrary, err)
	}

	return apigen.GetLibrary200JSONResponse(response), nil
}

// StartLibraryScan answers 202 with the status of the new scan, or of the one that already
// runs. An unknown ID answers 404, a server that is shutting down 503; every other failure
// is a 500.
func (api *API) StartLibraryScan(ctx context.Context, request apigen.StartLibraryScanRequestObject) (apigen.StartLibraryScanResponseObject, error) {
	if api.scan == nil {
		return apigen.StartLibraryScandefaultApplicationProblemPlusJSONResponse(api.notImplemented(ctx, operationStartLibraryScan)), nil
	}

	status, err := api.scan.Start(ctx, request.LibraryID.String())
	if err != nil {
		return nil, operationFailure(operationStartLibraryScan, err, http.StatusNotFound, http.StatusServiceUnavailable)
	}

	response, err := scanStatusResponse(status)
	if err != nil {
		return nil, operationFailure(operationStartLibraryScan, err)
	}

	return apigen.StartLibraryScan202JSONResponse(response), nil
}
