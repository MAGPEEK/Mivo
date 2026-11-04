package restapi

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/magpeek/mivo/internal/adapters/http/apigen"
	"github.com/magpeek/mivo/internal/adapters/http/requestid"
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

// codeNotImplemented answers an operation of the contract whose use case is not wired yet.
// It is internal, not a published Problem.code.
const codeNotImplemented = "not_implemented"

// ListLibraryItems answers one page of a library's items. A limit or cursor outside the
// contract answers 400 "invalid", an unknown library 404; every other failure is a 500.
func (api *API) ListLibraryItems(ctx context.Context, request apigen.ListLibraryItemsRequestObject) (apigen.ListLibraryItemsResponseObject, error) {
	page, err := pageRequest(request.Params)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", operationListLibraryItems, err)
	}

	items, err := api.items.ListItems(ctx, request.LibraryID.String(), page)
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
	detail, err := api.items.GetItem(ctx, request.ItemID.String())
	if err != nil {
		return nil, operationFailure(operationGetItem, err, http.StatusNotFound)
	}

	response, err := itemDetailResponse(detail)
	if err != nil {
		return nil, operationFailure(operationGetItem, err)
	}

	return apigen.GetItem200JSONResponse(response), nil
}

// ListItemChildren answers 501 until M3-06b implements it on the library read model v2.
func (api *API) ListItemChildren(ctx context.Context, _ apigen.ListItemChildrenRequestObject) (apigen.ListItemChildrenResponseObject, error) {
	return apigen.ListItemChildrendefaultApplicationProblemPlusJSONResponse(api.notImplemented(ctx, operationListItemChildren)), nil
}

// notImplemented answers an operation whose use case does not exist yet. The code
// "not_implemented" is internal and not published in the contract; it disappears with the
// last stub (M3-06b).
func (api *API) notImplemented(ctx context.Context, operation string) problemResponse {
	api.logger.DebugContext(ctx, "api operation not implemented",
		slog.String("operation", operation),
		slog.String("request_id", requestid.FromContext(ctx)))

	return problemResponse{
		Body:       NewProblem(http.StatusNotImplemented, codeNotImplemented),
		StatusCode: http.StatusNotImplemented,
	}
}

// problemResponse has the fields of every generated
// <Operation>defaultApplicationProblemPlusJSONResponse, so it converts to each of them.
type problemResponse struct {
	Body       apigen.Problem
	StatusCode int
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
			return library.PageRequest{}, fmt.Errorf("cursor longer than %d bytes: %w",
				maxCursorLength, domain.ErrInvalid)
		}

		page.Cursor = *params.Cursor
	}

	return page, nil
}
