package restapi

import (
	"context"
	"fmt"

	"github.com/magpeek/mivo/internal/adapters/http/apigen"
)

// The rows contract (task M5-12) is declared before the server can generate rows (M5-13).
// Until then the handlers check the parameters against the contract (400 "invalid" with
// Problem.field) and answer a valid request with 501 "not_implemented", as the contract
// allows.

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

// checkRowItemLimit checks the optional limit against the contract bounds.
func checkRowItemLimit(limit *int32) error {
	if limit != nil && (*limit < minRowItemLimit || *limit > maxRowItemLimit) {
		return invalidParameter(fieldRowItemLimit, fmt.Sprintf("not between %d and %d", minRowItemLimit, maxRowItemLimit))
	}

	return nil
}

// GetHomeRows checks the request and answers 501 until M5-13.
func (api *API) GetHomeRows(_ context.Context, request apigen.GetHomeRowsRequestObject) (apigen.GetHomeRowsResponseObject, error) {
	err := checkRowItemLimit(request.Params.Limit)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", operationGetHomeRows, err)
	}

	problem := notImplementedProblem()

	return apigen.GetHomeRowsdefaultApplicationProblemPlusJSONResponse{Body: problem, StatusCode: int(problem.Status)}, nil
}

// ListLibraryRows checks the request and answers 501 until M5-13.
func (api *API) ListLibraryRows(_ context.Context, request apigen.ListLibraryRowsRequestObject) (apigen.ListLibraryRowsResponseObject, error) {
	err := checkRowItemLimit(request.Params.Limit)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", operationListLibraryRows, err)
	}

	problem := notImplementedProblem()

	return apigen.ListLibraryRowsdefaultApplicationProblemPlusJSONResponse{Body: problem, StatusCode: int(problem.Status)}, nil
}
