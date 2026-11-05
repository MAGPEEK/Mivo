package restapi

import (
	"context"
	"fmt"
	"net/http"

	"github.com/magpeek/mivo/internal/adapters/http/apigen"
	applibrary "github.com/magpeek/mivo/internal/app/library"
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
	// It is checked in bytes: a cursor is base64 text, where bytes and characters agree, and
	// for anything else the byte count is the stricter bound.
	maxCursorLength = 2048
)

// operationListItemChildren is the operation name of the children endpoint, for logs.
const operationListItemChildren = "listItemChildren"

// ListLibraryItems answers one page of a library's items with the total count. A limit,
// cursor, sort or filter outside the contract and a type that is not the library's top-level
// kind answer 400 "invalid", an unknown library 404. Every other failure is a 500.
func (api *API) ListLibraryItems(ctx context.Context, request apigen.ListLibraryItemsRequestObject) (apigen.ListLibraryItemsResponseObject, error) {
	page, err := pageRequest(request.Params.Limit, request.Params.Cursor)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", operationListLibraryItems, err)
	}

	options := listingOptionsOf(request.Params)

	err = options.validate()
	if err != nil {
		return nil, fmt.Errorf("%s: %w", operationListLibraryItems, err)
	}

	items, err := api.items.ListItems(ctx, request.LibraryID.String(), page, options.query())
	if err != nil {
		return nil, operationFailure(operationListLibraryItems, err, http.StatusBadRequest, http.StatusNotFound)
	}

	response, err := itemPageResponse(items)
	if err != nil {
		return nil, operationFailure(operationListLibraryItems, err)
	}

	api.applySummaryMatches(ctx, response.Items)

	return apigen.ListLibraryItems200JSONResponse(response), nil
}

// GetItem answers an item with its files and streams. Only an unknown ID is a client
// mistake (404); every other failure is a 500.
func (api *API) GetItem(ctx context.Context, request apigen.GetItemRequestObject) (apigen.GetItemResponseObject, error) {
	detail, err := api.items.GetItem(ctx, request.ItemID.String())
	if err != nil {
		return nil, operationFailure(operationGetItem, err, http.StatusNotFound)
	}

	response, err := itemDetailResponse(detail)
	if err != nil {
		return nil, operationFailure(operationGetItem, err)
	}

	api.applyDetailMatch(ctx, &response)

	return apigen.GetItem200JSONResponse(response), nil
}

// ListItemChildren answers one page of the seasons of a show or the episodes of a season. A
// bad limit, cursor or order answers 400 "invalid", an unknown item 404; every other failure
// is a 500.
func (api *API) ListItemChildren(ctx context.Context, request apigen.ListItemChildrenRequestObject) (apigen.ListItemChildrenResponseObject, error) {
	page, err := pageRequest(request.Params.Limit, request.Params.Cursor)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", operationListItemChildren, err)
	}

	descending, err := descendingOrder(request.Params.Order)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", operationListItemChildren, err)
	}

	children, err := api.items.ListChildren(ctx, request.ItemID.String(),
		applibrary.ChildPageRequest{Page: page, Descending: descending})
	if err != nil {
		return nil, operationFailure(operationListItemChildren, err, http.StatusBadRequest, http.StatusNotFound)
	}

	response, err := itemPageResponse(children)
	if err != nil {
		return nil, operationFailure(operationListItemChildren, err)
	}

	api.applySummaryMatches(ctx, response.Items)

	return apigen.ListItemChildren200JSONResponse(response), nil
}

// descendingOrder reads the order parameter; omitted means ascending. The generated server
// binds the string but does not check the enum.
func descendingOrder(order *apigen.SortOrder) (bool, error) {
	if order == nil {
		return false, nil
	}

	switch *order {
	case apigen.SortOrderAsc:
		return false, nil
	case apigen.SortOrderDesc:
		return true, nil
	default:
		return false, fmt.Errorf("order not asc or desc: %w", domain.ErrInvalid)
	}
}

// pageRequest checks the paging parameters against the contract. Errors wrap
// domain.ErrInvalid and do not quote the cursor, which may be long. A present but empty
// cursor is rejected: clients omit the first page's cursor.
func pageRequest(limitParam *int32, cursorParam *string) (library.PageRequest, error) {
	var page library.PageRequest

	if limitParam != nil {
		limit := int(*limitParam)
		if limit < minPageLimit || limit > library.MaxPageLimit {
			return library.PageRequest{}, fmt.Errorf("limit %d not between %d and %d: %w",
				limit, minPageLimit, library.MaxPageLimit, domain.ErrInvalid)
		}

		page.Limit = limit
	}

	if cursorParam != nil {
		if *cursorParam == "" || len(*cursorParam) > maxCursorLength {
			return library.PageRequest{}, fmt.Errorf("cursor empty or longer than %d bytes: %w",
				maxCursorLength, domain.ErrInvalid)
		}

		page.Cursor = *cursorParam
	}

	return page, nil
}
