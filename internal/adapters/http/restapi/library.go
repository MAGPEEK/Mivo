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

// LibraryCatalog is the port that reads libraries for the responses, implemented by
// app/library.ManagedCatalog. The handlers treat every failure but an unknown ID as a server
// fault (see operationFailure), so unreadable stored rows never become a "bad request".
type LibraryCatalog interface {
	// ListLibraries returns every library ordered by name with its availability.
	ListLibraries(ctx context.Context) ([]applibrary.ManagedOverview, error)
	// GetLibrary returns one library; an unknown ID wraps domain.ErrNotFound.
	GetLibrary(ctx context.Context, id string) (applibrary.ManagedOverview, error)
	// Describe adds availability and the item count to a library just created or changed.
	Describe(ctx context.Context, managed library.ManagedLibrary) (applibrary.ManagedOverview, error)
}

// ItemCatalog is the port of the item endpoints, the item part of app/library.Service.
type ItemCatalog interface {
	// ListItems returns one page of a library's items in the order and under the filter of
	// the query, with the total count. A bad limit or cursor wraps domain.ErrInvalid, a kind
	// other than the library's top-level kind applibrary.ErrItemKindMismatch, an unknown
	// library domain.ErrNotFound.
	ListItems(ctx context.Context, libraryID string, page library.PageRequest, query library.ListingQuery) (applibrary.ItemPage, error)
	// GetItem returns an item with its files and streams; an unknown ID wraps
	// domain.ErrNotFound.
	GetItem(ctx context.Context, itemID string) (applibrary.ItemDetail, error)
	// ListChildren returns one page of the seasons of a show or the episodes of a season. A
	// bad limit or cursor wraps domain.ErrInvalid, an unknown item domain.ErrNotFound.
	ListChildren(ctx context.Context, itemID string, request applibrary.ChildPageRequest) (applibrary.ItemPage, error)
	ItemBrowsing
}

// ItemBrowsing is the part of ItemCatalog behind the filter menu, the A–Z bar and the folder
// view (task M5-11), implemented by app/library.Service. Errors as ListItems; ListFolder also
// answers a missing folder or one outside the library folder with domain.ErrNotFound.
type ItemBrowsing interface {
	// ListFacets returns the facet counts of a library listing.
	ListFacets(ctx context.Context, libraryID string, request applibrary.FacetRequest) ([]applibrary.Facet, error)
	// ListLetters returns where each first letter starts in the title-sorted listing.
	ListLetters(ctx context.Context, libraryID string, request applibrary.LetterRequest) (applibrary.LetterIndex, error)
	// ListFolder returns one page of the children of a library folder.
	ListFolder(ctx context.Context, libraryID string, path library.FolderPath, page library.PageRequest) (applibrary.FolderView, error)
}

// ScanService is the port of the scan endpoint and the scan status of a library,
// implemented by app/scan.Service. The library endpoints need it next to LibraryCatalog.
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
	overviews, err := api.libraries.ListLibraries(ctx)
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
	libraryID := request.LibraryID.String()

	overview, err := api.libraries.GetLibrary(ctx, libraryID)
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
