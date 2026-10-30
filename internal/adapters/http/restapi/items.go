package restapi

import (
	"context"
	"fmt"
	"net/http"

	"github.com/magpeek/mivo/internal/adapters/http/apigen"
	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/library"
)

// Bounds of the listLibraryItems query parameters (api/openapi.yaml). The generated server
// binds them but does not check the schema, so the handler does.
const (
	// minPageLimit is the smallest limit a client may send. The use case reads 0 as "use
	// the default", so an explicit 0 must be rejected here.
	minPageLimit = 1
	// maxCursorLength is the maxLength of the cursor. A cursor holds an item ID and a sort
	// title; 2048 characters cover the longest file names (decision of 2026-10-30, M2-26).
	maxCursorLength = 2048
)

// ListLibraryItems answers one page of a library's items. A limit or cursor outside the
// contract answers 400 "invalid", an unknown library 404; every other failure is a 500.
func (api *API) ListLibraryItems(ctx context.Context, request apigen.ListLibraryItemsRequestObject) (apigen.ListLibraryItemsResponseObject, error) {
	if api.library == nil {
		return apigen.ListLibraryItemsdefaultApplicationProblemPlusJSONResponse(api.notImplemented(ctx, operationListLibraryItems)), nil
	}

	page, err := pageRequest(request.Params)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", operationListLibraryItems, err)
	}

	items, err := api.library.ListItems(ctx, request.LibraryID.String(), page)
	if err != nil {
		return nil, operationFailure(operationListLibraryItems, err, http.StatusBadRequest, http.StatusNotFound)
	}

	response, err := itemPageResponse(items)
	if err != nil {
		return nil, operationFailure(operationListLibraryItems, err)
	}

	return apigen.ListLibraryItems200JSONResponse(response), nil
}

// GetItem answers an item with its files and streams. Only an unknown ID is a client
// mistake (404); every other failure is a 500.
func (api *API) GetItem(ctx context.Context, request apigen.GetItemRequestObject) (apigen.GetItemResponseObject, error) {
	if api.library == nil {
		return apigen.GetItemdefaultApplicationProblemPlusJSONResponse(api.notImplemented(ctx, operationGetItem)), nil
	}

	detail, err := api.library.GetItem(ctx, request.ItemID.String())
	if err != nil {
		return nil, operationFailure(operationGetItem, err, http.StatusNotFound)
	}

	response, err := itemDetailResponse(detail)
	if err != nil {
		return nil, operationFailure(operationGetItem, err)
	}

	return apigen.GetItem200JSONResponse(response), nil
}

// pageRequest checks the paging parameters against the contract. Errors wrap
// domain.ErrInvalid and do not quote the cursor, which may be long.
func pageRequest(params apigen.ListLibraryItemsParams) (library.PageRequest, error) {
	var page library.PageRequest

	if params.Limit != nil {
		limit := int(*params.Limit)
		if limit < minPageLimit || limit > library.MaxPageLimit {
			return library.PageRequest{}, fmt.Errorf("limit %d not between %d and %d: %w",
				limit, minPageLimit, library.MaxPageLimit, domain.ErrInvalid)
		}

		page.Limit = limit
	}

	if params.Cursor != nil {
		if len(*params.Cursor) > maxCursorLength {
			return library.PageRequest{}, fmt.Errorf("cursor longer than %d characters: %w",
				maxCursorLength, domain.ErrInvalid)
		}

		page.Cursor = *params.Cursor
	}

	return page, nil
}
