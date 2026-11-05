package restapi

import (
	"context"
	"errors"
	"fmt"

	"github.com/magpeek/mivo/internal/adapters/http/apigen"
)

// The Browse contract (task M5-10) is declared before the catalog can sort, filter, count
// or browse folders (M5-11). Until then the handlers check every parameter against the
// contract (400 "invalid" with Problem.field) and answer a valid request that needs the
// missing query with 501 "not_implemented", as the contract allows.

// Operation names of the Browse endpoints, for logs.
const (
	operationListLibraryFacets  = "listLibraryFacets"
	operationListLibraryLetters = "listLibraryLetters"
	operationListLibraryFolders = "listLibraryFolders"
)

// listingOptions are the sort and filter parameters of listLibraryItems.
type listingOptions struct {
	sort    *apigen.ItemSort
	order   *apigen.SortOrder
	seed    *int32
	filters listingFilters
}

func listingOptionsOf(params apigen.ListLibraryItemsParams) listingOptions {
	return listingOptions{
		sort:  params.Sort,
		order: params.Order,
		seed:  params.Seed,
		filters: listingFilters{
			kind: params.Type, quick: params.Quick, genre: params.Genre, year: params.Year,
			decade: params.Decade, contentRating: params.ContentRating, resolution: params.Resolution,
			videoCodec: params.VideoCodec, audioCodec: params.AudioCodec, container: params.Container,
			audioLanguage: params.AudioLanguage, subtitleLanguage: params.SubtitleLanguage,
		},
	}
}

func (options listingOptions) validate() error {
	var seedErr error
	if options.seed != nil && *options.seed < 0 {
		seedErr = invalidParameter(fieldSeed, "negative seed")
	}

	return errors.Join(checkEnum(fieldSort, options.sort), checkSortOrder(options.order), seedErr, options.filters.validate())
}

// customized reports whether the listing differs from the default order (title ascending)
// over the whole library, which the catalog cannot serve before M5-11. The seed alone does
// not count: it is ignored for every sort but "random".
func (options listingOptions) customized() bool {
	customSort := options.sort != nil && *options.sort != apigen.ItemSortTitle
	customOrder := options.order != nil && *options.order != apigen.SortOrderAsc

	return customSort || customOrder || options.filters.active()
}

// notImplementedListing is the 501 answer of a listing the catalog cannot sort or filter yet.
func notImplementedListing() apigen.ListLibraryItemsResponseObject {
	problem := notImplementedProblem()

	return apigen.ListLibraryItemsdefaultApplicationProblemPlusJSONResponse{Body: problem, StatusCode: int(problem.Status)}
}

// ListLibraryFacets checks the request and answers 501 until M5-11.
func (api *API) ListLibraryFacets(_ context.Context, request apigen.ListLibraryFacetsRequestObject) (apigen.ListLibraryFacetsResponseObject, error) {
	params := request.Params
	filters := listingFilters{
		kind: params.Type, quick: params.Quick, genre: params.Genre, year: params.Year,
		decade: params.Decade, contentRating: params.ContentRating, resolution: params.Resolution,
		videoCodec: params.VideoCodec, audioCodec: params.AudioCodec, container: params.Container,
		audioLanguage: params.AudioLanguage, subtitleLanguage: params.SubtitleLanguage,
	}

	err := errors.Join(checkEnums(fieldFacetField, params.Field, maxFacetFields), filters.validate())
	if err != nil {
		return nil, fmt.Errorf("%s: %w", operationListLibraryFacets, err)
	}

	problem := notImplementedProblem()

	return apigen.ListLibraryFacetsdefaultApplicationProblemPlusJSONResponse{Body: problem, StatusCode: int(problem.Status)}, nil
}

// ListLibraryLetters checks the request and answers 501 until M5-11.
func (api *API) ListLibraryLetters(_ context.Context, request apigen.ListLibraryLettersRequestObject) (apigen.ListLibraryLettersResponseObject, error) {
	params := request.Params
	filters := listingFilters{
		kind: params.Type, quick: params.Quick, genre: params.Genre, year: params.Year,
		decade: params.Decade, contentRating: params.ContentRating, resolution: params.Resolution,
		videoCodec: params.VideoCodec, audioCodec: params.AudioCodec, container: params.Container,
		audioLanguage: params.AudioLanguage, subtitleLanguage: params.SubtitleLanguage,
	}

	err := errors.Join(checkSortOrder(params.Order), filters.validate())
	if err != nil {
		return nil, fmt.Errorf("%s: %w", operationListLibraryLetters, err)
	}

	problem := notImplementedProblem()

	return apigen.ListLibraryLettersdefaultApplicationProblemPlusJSONResponse{Body: problem, StatusCode: int(problem.Status)}, nil
}

// ListLibraryFolders checks the path (traversal answers 400) and the paging parameters and
// answers 501 until M5-11.
func (api *API) ListLibraryFolders(_ context.Context, request apigen.ListLibraryFoldersRequestObject) (apigen.ListLibraryFoldersResponseObject, error) {
	err := checkLibraryPath(request.Params.Path)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", operationListLibraryFolders, err)
	}

	_, err = pageRequest(request.Params.Limit, request.Params.Cursor)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", operationListLibraryFolders, err)
	}

	problem := notImplementedProblem()

	return apigen.ListLibraryFoldersdefaultApplicationProblemPlusJSONResponse{Body: problem, StatusCode: int(problem.Status)}, nil
}
