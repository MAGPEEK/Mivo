package restapi

import (
	"context"
	"fmt"
	"net/http"

	"github.com/magpeek/mivo/internal/adapters/http/apigen"
	approws "github.com/magpeek/mivo/internal/app/rows"
)

// RowsService generates the Home and Overview rows. Implemented by app/rows.Service.
type RowsService interface {
	// Home returns the Home rows, the hero and the hint. A limit of 0 means the default.
	Home(ctx context.Context, limit int) (approws.Home, error)
	// LibraryRows returns the Overview rows of a library; an unknown library wraps
	// domain.ErrNotFound.
	LibraryRows(ctx context.Context, libraryID string, limit int) (approws.LibraryRows, error)
}

// Operation names of the rows endpoints, for logs.
const (
	operationGetHomeRows     = "getHomeRows"
	operationListLibraryRows = "listLibraryRows"
)

// fieldRowItemLimit is the query parameter that bounds the items of each row.
const fieldRowItemLimit = "limit"

// Bounds of the items per row (components.parameters.RowItemLimit).
const (
	minRowItemLimit = 1
	maxRowItemLimit = 50
)

// rowItemLimit checks the optional limit against the contract bounds; absent is 0, the
// service's default.
func rowItemLimit(limit *int32) (int, error) {
	if limit == nil {
		return 0, nil
	}

	if *limit < minRowItemLimit || *limit > maxRowItemLimit {
		return 0, invalidParameter(fieldRowItemLimit, fmt.Sprintf("not between %d and %d", minRowItemLimit, maxRowItemLimit))
	}

	return int(*limit), nil
}

// GetHomeRows answers the Home rows and hero. A limit outside the contract answers 400
// "invalid"; every other failure is a 500.
func (api *API) GetHomeRows(ctx context.Context, request apigen.GetHomeRowsRequestObject) (apigen.GetHomeRowsResponseObject, error) {
	limit, err := rowItemLimit(request.Params.Limit)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", operationGetHomeRows, err)
	}

	if api.rows == nil {
		problem := notImplementedProblem()

		return apigen.GetHomeRowsdefaultApplicationProblemPlusJSONResponse{Body: problem, StatusCode: int(problem.Status)}, nil
	}

	home, err := api.rows.Home(ctx, limit)
	if err != nil {
		return nil, operationFailure(operationGetHomeRows, err)
	}

	response, err := homeRowsResponse(home)
	if err != nil {
		return nil, operationFailure(operationGetHomeRows, err)
	}

	return apigen.GetHomeRows200JSONResponse(response), nil
}

// ListLibraryRows answers the Overview rows of a library. A limit outside the contract
// answers 400 "invalid", an unknown library 404; every other failure is a 500.
func (api *API) ListLibraryRows(ctx context.Context, request apigen.ListLibraryRowsRequestObject) (apigen.ListLibraryRowsResponseObject, error) {
	limit, err := rowItemLimit(request.Params.Limit)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", operationListLibraryRows, err)
	}

	if api.rows == nil {
		problem := notImplementedProblem()

		return apigen.ListLibraryRowsdefaultApplicationProblemPlusJSONResponse{Body: problem, StatusCode: int(problem.Status)}, nil
	}

	libraryRows, err := api.rows.LibraryRows(ctx, request.LibraryID.String(), limit)
	if err != nil {
		return nil, operationFailure(operationListLibraryRows, err, http.StatusNotFound)
	}

	response, err := libraryRowsResponse(libraryRows)
	if err != nil {
		return nil, operationFailure(operationListLibraryRows, err)
	}

	return apigen.ListLibraryRows200JSONResponse(response), nil
}
