package restapi

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/magpeek/mivo/internal/adapters/http/apigen"
	appsearch "github.com/magpeek/mivo/internal/app/search"
)

// SearchService searches the catalog. Implemented by app/search.Service.
type SearchService interface {
	// Search returns one page of hits; a query, type or cursor it rejects wraps
	// appsearch.ErrQueryTooShort, ErrUnknownType or ErrInvalidCursor.
	Search(ctx context.Context, request appsearch.Request) (appsearch.Page, error)
}

// operationSearch is the operation name of the search endpoint, for logs.
const operationSearch = "search"

// Query parameters of search that Problem.field names.
const (
	fieldSearchText = "q"
	fieldLimit      = "limit"
	fieldCursor     = "cursor"
)

// Bounds of the search parameters (paths./api/v1/search). The lower bound of q is
// appsearch.MinQueryLength, the upper bound of limit appsearch.MaxPageLimit.
const (
	maxSearchTextLength = 200
	minSearchLimit      = 1
)

// searchRequest checks the parameters against the contract and converts them. q is counted
// in characters without surrounding spaces, as the use case reads it; an omitted type is
// "top" and an omitted limit 0, the use case's default.
func searchRequest(params apigen.SearchParams) (appsearch.Request, error) {
	request := appsearch.Request{Query: strings.TrimSpace(params.Q), Type: appsearch.TypeTop}

	length := utf8.RuneCountInString(request.Query)
	if length < appsearch.MinQueryLength || length > maxSearchTextLength {
		return appsearch.Request{}, invalidParameter(fieldSearchText,
			fmt.Sprintf("not between %d and %d characters", appsearch.MinQueryLength, maxSearchTextLength))
	}

	if err := checkEnum(fieldType, params.Type); err != nil {
		return appsearch.Request{}, err
	}

	if params.Type != nil {
		request.Type = appsearch.Type(*params.Type)
	}

	if params.Limit != nil {
		if *params.Limit < minSearchLimit || *params.Limit > appsearch.MaxPageLimit {
			return appsearch.Request{}, invalidParameter(fieldLimit,
				fmt.Sprintf("not between %d and %d", minSearchLimit, appsearch.MaxPageLimit))
		}

		request.Limit = int(*params.Limit)
	}

	if params.Cursor != nil {
		if *params.Cursor == "" || len(*params.Cursor) > maxCursorLength {
			return appsearch.Request{}, invalidParameter(fieldCursor, "empty or too long")
		}

		request.Cursor = *params.Cursor
	}

	return request, nil
}

// catalogSearchFailure names the parameter of a request the use case rejected, so the problem
// carries it as field; every other failure is a server fault (500).
func catalogSearchFailure(err error) error {
	rejections := []struct {
		target error
		field  string
	}{
		{target: appsearch.ErrQueryTooShort, field: fieldSearchText},
		{target: appsearch.ErrUnknownType, field: fieldType},
		{target: appsearch.ErrInvalidCursor, field: fieldCursor},
	}

	for _, rejection := range rejections {
		if errors.Is(err, rejection.target) {
			return fmt.Errorf("%s: %w", operationSearch, invalidParameter(rejection.field, err.Error()))
		}
	}

	return operationFailure(operationSearch, err)
}

// Search answers the search dropdown and the pivots of the results page. A parameter outside
// the contract answers 400 "invalid" with its name as field; every other failure is a 500.
func (api *API) Search(ctx context.Context, request apigen.SearchRequestObject) (apigen.SearchResponseObject, error) {
	searchParams, err := searchRequest(request.Params)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", operationSearch, err)
	}

	if api.search == nil {
		problem := notImplementedProblem()

		return apigen.SearchdefaultApplicationProblemPlusJSONResponse{Body: problem, StatusCode: int(problem.Status)}, nil
	}

	page, err := api.search.Search(ctx, searchParams)
	if err != nil {
		return nil, catalogSearchFailure(err)
	}

	response, err := searchResultsResponse(page)
	if err != nil {
		return nil, operationFailure(operationSearch, err)
	}

	return apigen.Search200JSONResponse(response), nil
}

func searchResultsResponse(page appsearch.Page) (apigen.SearchResults, error) {
	items := make([]apigen.ItemSummary, 0, len(page.Items))

	for _, summary := range page.Items {
		item, err := itemSummaryResponse(summary)
		if err != nil {
			return apigen.SearchResults{}, err
		}

		items = append(items, item)
	}

	return apigen.SearchResults{Items: items, NextCursor: optionalString(page.NextCursor)}, nil
}
