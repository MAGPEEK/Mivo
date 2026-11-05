package restapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/magpeek/mivo/internal/adapters/http/apigen"
	applibrary "github.com/magpeek/mivo/internal/app/library"
	"github.com/magpeek/mivo/internal/domain/library"
)

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
	// matchState filters listLibraryItems only; facets and letters do not take it.
	matchState *apigen.MatchState
}

// The generated parameter types of listLibraryItems, listLibraryFacets and
// listLibraryLetters share their filter fields but no type, so each gets its own small
// conversion into listingFilters; checking and converting is shared from there on.

func listingOptionsOf(params apigen.ListLibraryItemsParams) listingOptions {
	return listingOptions{
		sort: params.Sort, order: params.Order, seed: params.Seed, matchState: params.MatchState,
		filters: listingFilters{
			kind: params.Type, quick: params.Quick, genre: params.Genre, year: params.Year,
			decade: params.Decade, contentRating: params.ContentRating, resolution: params.Resolution,
			videoCodec: params.VideoCodec, audioCodec: params.AudioCodec, container: params.Container,
			audioLanguage: params.AudioLanguage, subtitleLanguage: params.SubtitleLanguage,
		},
	}
}

func facetFiltersOf(params apigen.ListLibraryFacetsParams) listingFilters {
	return listingFilters{
		kind: params.Type, quick: params.Quick, genre: params.Genre, year: params.Year,
		decade: params.Decade, contentRating: params.ContentRating, resolution: params.Resolution,
		videoCodec: params.VideoCodec, audioCodec: params.AudioCodec, container: params.Container,
		audioLanguage: params.AudioLanguage, subtitleLanguage: params.SubtitleLanguage,
	}
}

func letterFiltersOf(params apigen.ListLibraryLettersParams) listingFilters {
	return listingFilters{
		kind: params.Type, quick: params.Quick, genre: params.Genre, year: params.Year,
		decade: params.Decade, contentRating: params.ContentRating, resolution: params.Resolution,
		videoCodec: params.VideoCodec, audioCodec: params.AudioCodec, container: params.Container,
		audioLanguage: params.AudioLanguage, subtitleLanguage: params.SubtitleLanguage,
	}
}

func (options listingOptions) validate() error {
	var seedErr error
	if options.seed != nil && *options.seed < 0 {
		seedErr = invalidParameter(fieldSeed, "negative seed")
	}

	return errors.Join(checkEnum(fieldSort, options.sort), checkSortOrder(options.order), seedErr,
		checkEnum(fieldMatchState, options.matchState), options.filters.validate())
}

// query converts checked options to the use case's query. The seed only counts for "random".
func (options listingOptions) query() library.ListingQuery {
	query := library.ListingQuery{Filter: options.filters.domainFilter()}

	if options.matchState != nil {
		query.Filter.MatchState = library.MatchStateFilter(*options.matchState)
	}

	if options.sort != nil {
		query.Order.Sort = library.ItemSort(*options.sort)
	}

	query.Order.Descending = options.order != nil && *options.order == apigen.SortOrderDesc

	if options.seed != nil {
		query.Order.Seed, query.Order.HasSeed = int64(*options.seed), true
	}

	return query
}

// ListLibraryFacets answers the facet counts of a library listing. An unknown field or a
// malformed filter answers 400 "invalid", an unknown library 404; every other failure is a
// 500.
func (api *API) ListLibraryFacets(ctx context.Context, request apigen.ListLibraryFacetsRequestObject) (apigen.ListLibraryFacetsResponseObject, error) {
	params := request.Params
	filters := facetFiltersOf(params)

	err := errors.Join(checkEnums(fieldFacetField, params.Field, maxFacetFields), filters.validate())
	if err != nil {
		return nil, fmt.Errorf("%s: %w", operationListLibraryFacets, err)
	}

	fields := convertAll(params.Field, func(field apigen.FacetField) library.FacetField { return library.FacetField(field) })

	facets, err := api.items.ListFacets(ctx, request.LibraryID.String(), applibrary.FacetRequest{Fields: fields, Filter: filters.domainFilter()})
	if err != nil {
		return nil, operationFailure(operationListLibraryFacets, err, http.StatusBadRequest, http.StatusNotFound)
	}

	return apigen.ListLibraryFacets200JSONResponse(facetListResponse(facets)), nil
}

func facetListResponse(facets []applibrary.Facet) apigen.FacetList {
	list := apigen.FacetList{Facets: make([]apigen.Facet, 0, len(facets))}

	for _, facet := range facets {
		values := make([]apigen.FacetValue, 0, len(facet.Values))
		for _, value := range facet.Values {
			values = append(values, apigen.FacetValue{Value: value.Value, Count: value.Count})
		}

		list.Facets = append(list.Facets, apigen.Facet{Field: apigen.FacetField(facet.Field), Values: values, Truncated: facet.Truncated})
	}

	return list
}

// ListLibraryLetters answers where each first letter starts in the title-sorted listing. A
// bad order or filter answers 400 "invalid", an unknown library 404; every other failure is
// a 500.
func (api *API) ListLibraryLetters(ctx context.Context, request apigen.ListLibraryLettersRequestObject) (apigen.ListLibraryLettersResponseObject, error) {
	params := request.Params
	filters := letterFiltersOf(params)

	err := errors.Join(checkSortOrder(params.Order), filters.validate())
	if err != nil {
		return nil, fmt.Errorf("%s: %w", operationListLibraryLetters, err)
	}

	index, err := api.items.ListLetters(ctx, request.LibraryID.String(), applibrary.LetterRequest{
		Descending: params.Order != nil && *params.Order == apigen.SortOrderDesc, Filter: filters.domainFilter(),
	})
	if err != nil {
		return nil, operationFailure(operationListLibraryLetters, err, http.StatusBadRequest, http.StatusNotFound)
	}

	letters := make([]apigen.LetterOffset, 0, len(index.Letters))
	for _, letter := range index.Letters {
		letters = append(letters, apigen.LetterOffset{Letter: letter.Letter, Offset: letter.Offset})
	}

	return apigen.ListLibraryLetters200JSONResponse(apigen.LetterIndex{Letters: letters, TotalCount: index.TotalCount}), nil
}

// ListLibraryFolders answers one page of the children of a library folder. A path that could
// leave the folder (400 "invalid", field "path"), a bad limit or cursor (400) and an unknown
// library or missing folder (404) are client mistakes; an offline folder answers 503; every
// other failure is a 500.
func (api *API) ListLibraryFolders(ctx context.Context, request apigen.ListLibraryFoldersRequestObject) (apigen.ListLibraryFoldersResponseObject, error) {
	path, err := libraryPathOf(request.Params.Path)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", operationListLibraryFolders, err)
	}

	page, err := pageRequest(request.Params.Limit, request.Params.Cursor)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", operationListLibraryFolders, err)
	}

	listing, err := api.items.ListFolder(ctx, request.LibraryID.String(), path, page)
	if err != nil {
		return nil, operationFailure(operationListLibraryFolders, err,
			http.StatusBadRequest, http.StatusNotFound, http.StatusServiceUnavailable)
	}

	response, err := folderListingResponse(listing)
	if err != nil {
		return nil, operationFailure(operationListLibraryFolders, err)
	}

	response.LibraryID = request.LibraryID

	return apigen.ListLibraryFolders200JSONResponse(response), nil
}

func folderListingResponse(listing applibrary.FolderView) (apigen.LibraryFolderListing, error) {
	response := apigen.LibraryFolderListing{
		Path: listing.Path.String(), Entries: make([]apigen.LibraryFolderEntry, 0, len(listing.Entries)),
		NextCursor: optionalString(listing.NextCursor),
	}

	if parent, found := listing.Path.Parent(); found {
		parentPath := parent.String()
		response.ParentPath = &parentPath
	}

	for _, entry := range listing.Entries {
		converted := apigen.LibraryFolderEntry{Kind: apigen.LibraryFolderEntryKind(entry.Kind), Name: entry.Name, Path: entry.Path.String()}

		if entry.Item != nil {
			item, err := itemSummaryResponse(*entry.Item)
			if err != nil {
				return apigen.LibraryFolderListing{}, err
			}

			converted.Item = &item
		}

		response.Entries = append(response.Entries, converted)
	}

	return response, nil
}
