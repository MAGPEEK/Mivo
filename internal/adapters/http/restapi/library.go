package restapi

import (
	"context"

	"github.com/magpeek/mivo/internal/adapters/http/apigen"
)

// Operation names of the library and item endpoints, as in api/openapi.yaml, for logs.
const (
	operationListLibraries    = "listLibraries"
	operationGetLibrary       = "getLibrary"
	operationStartLibraryScan = "startLibraryScan"
	operationListLibraryItems = "listLibraryItems"
	operationGetItem          = "getItem"
)

// LibraryService is the port of the library and item endpoints. It is a placeholder: M2-26
// declares the methods the handlers need. Until then every library and item operation
// answers 501, and afterwards a nil service keeps doing so until M2-32 wires it.
type LibraryService interface{}

// ScanService is the port of the scan endpoint and the scan status of a library. It is a
// placeholder that M2-26 fills in, like LibraryService.
type ScanService interface{}

// ListLibraries answers 501 until M2-26 implements it.
func (api *API) ListLibraries(ctx context.Context, _ apigen.ListLibrariesRequestObject) (apigen.ListLibrariesResponseObject, error) {
	return apigen.ListLibrariesdefaultApplicationProblemPlusJSONResponse(api.notImplemented(ctx, operationListLibraries)), nil
}

// GetLibrary answers 501 until M2-26 implements it.
func (api *API) GetLibrary(ctx context.Context, _ apigen.GetLibraryRequestObject) (apigen.GetLibraryResponseObject, error) {
	return apigen.GetLibrarydefaultApplicationProblemPlusJSONResponse(api.notImplemented(ctx, operationGetLibrary)), nil
}

// StartLibraryScan answers 501 until M2-26 implements it.
func (api *API) StartLibraryScan(ctx context.Context, _ apigen.StartLibraryScanRequestObject) (apigen.StartLibraryScanResponseObject, error) {
	return apigen.StartLibraryScandefaultApplicationProblemPlusJSONResponse(api.notImplemented(ctx, operationStartLibraryScan)), nil
}

// ListLibraryItems answers 501 until M2-26 implements it.
func (api *API) ListLibraryItems(ctx context.Context, _ apigen.ListLibraryItemsRequestObject) (apigen.ListLibraryItemsResponseObject, error) {
	return apigen.ListLibraryItemsdefaultApplicationProblemPlusJSONResponse(api.notImplemented(ctx, operationListLibraryItems)), nil
}

// GetItem answers 501 until M2-26 implements it.
func (api *API) GetItem(ctx context.Context, _ apigen.GetItemRequestObject) (apigen.GetItemResponseObject, error) {
	return apigen.GetItemdefaultApplicationProblemPlusJSONResponse(api.notImplemented(ctx, operationGetItem)), nil
}
