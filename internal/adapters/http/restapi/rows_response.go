package restapi

import (
	"fmt"
	"math"

	"github.com/magpeek/mivo/internal/adapters/http/apigen"
	approws "github.com/magpeek/mivo/internal/app/rows"
	"github.com/magpeek/mivo/internal/domain/library"
	"github.com/magpeek/mivo/internal/domain/media"
	"github.com/magpeek/mivo/internal/domain/rows"
)

// The row enums share their values with the contract. A value the contract does not list
// would be a server bug; it is reported as a server fault instead of being sent.

// sentEnum returns value when the contract lists it.
func sentEnum[T validEnum](name string, value T) (T, error) {
	if !value.Valid() {
		return value, fmt.Errorf("%s %v is not in the contract", name, value)
	}

	return value, nil
}

func homeRowsResponse(home approws.Home) (apigen.HomeRows, error) {
	rowList, err := rowsResponse(home.Rows)
	if err != nil {
		return apigen.HomeRows{}, err
	}

	hero := make([]apigen.HeroItem, 0, len(home.Hero))

	for _, item := range home.Hero {
		entry, err := heroItemResponse(item)
		if err != nil {
			return apigen.HomeRows{}, err
		}

		hero = append(hero, entry)
	}

	hint, err := hintResponse(home.Hint)
	if err != nil {
		return apigen.HomeRows{}, err
	}

	return apigen.HomeRows{Rows: rowList, Hero: hero, Hint: hint}, nil
}

func libraryRowsResponse(libraryRows approws.LibraryRows) (apigen.LibraryRows, error) {
	libraryID, err := parseStoredID("library", libraryRows.LibraryID)
	if err != nil {
		return apigen.LibraryRows{}, err
	}

	rowList, err := rowsResponse(libraryRows.Rows)
	if err != nil {
		return apigen.LibraryRows{}, err
	}

	hint, err := hintResponse(libraryRows.Hint)
	if err != nil {
		return apigen.LibraryRows{}, err
	}

	return apigen.LibraryRows{LibraryID: libraryID, Rows: rowList, Hint: hint}, nil
}

func hintResponse(hint rows.Hint) (*apigen.RowsHint, error) {
	if hint == rows.HintNone {
		return nil, nil //nolint:nilnil // nil pointer is the "absent" value of an optional field
	}

	value, err := sentEnum("rows hint", apigen.RowsHint(hint))

	return &value, err
}

func rowsResponse(rowList []approws.Row) ([]apigen.Row, error) {
	responses := make([]apigen.Row, 0, len(rowList))

	for _, row := range rowList {
		response, err := rowResponse(row)
		if err != nil {
			return nil, fmt.Errorf("row %s: %w", row.ID, err)
		}

		responses = append(responses, response)
	}

	return responses, nil
}

func rowResponse(row approws.Row) (apigen.Row, error) {
	kind, err := sentEnum("row kind", apigen.RowKind(row.Kind))
	if err != nil {
		return apigen.Row{}, err
	}

	cardFormat, err := sentEnum("card format", apigen.CardFormat(row.CardFormat))
	if err != nil {
		return apigen.Row{}, err
	}

	libraryID, err := optionalStoredID("library", row.LibraryID)
	if err != nil {
		return apigen.Row{}, err
	}

	seeAll, err := browseQueryResponse(row.SeeAll)
	if err != nil {
		return apigen.Row{}, err
	}

	items, err := rowItemsResponse(row.Items)
	if err != nil {
		return apigen.Row{}, err
	}

	response := apigen.Row{
		ID: row.ID, Kind: kind, TitleKey: row.TitleKey, LibraryID: libraryID, Genre: optionalString(row.Genre),
		CardFormat: cardFormat, SeeAllQuery: &seeAll, Items: items,
	}

	if len(row.TitleParams) > 0 {
		params := row.TitleParams
		response.TitleParams = &params
	}

	return response, nil
}

// browseQueryResponse maps the parts of a listing query a row sets: sort, direction, seed,
// type and genres. Ascending is the default and is omitted.
func browseQueryResponse(query library.ListingQuery) (apigen.BrowseQuery, error) {
	sort, err := sentEnum("sort", apigen.ItemSort(query.Order.Key()))
	if err != nil {
		return apigen.BrowseQuery{}, err
	}

	response := apigen.BrowseQuery{Sort: &sort, Genre: optionalStrings(query.Filter.Genres)}

	if query.Order.IsDescending() {
		order := apigen.SortOrderDesc
		response.Order = &order
	}

	if query.Order.HasSeed && query.Order.Seed >= 0 && query.Order.Seed <= math.MaxInt32 {
		seed := int32(query.Order.Seed)
		response.Seed = &seed
	}

	if query.Filter.Kind != "" {
		kind, err := itemKindResponse(query.Filter.Kind)
		if err != nil {
			return apigen.BrowseQuery{}, err
		}

		response.Type = &kind
	}

	return response, nil
}

func rowItemsResponse(items []approws.Item) ([]apigen.ItemSummary, error) {
	responses := make([]apigen.ItemSummary, 0, len(items))

	for _, item := range items {
		response, err := itemSummaryResponse(item.Summary)
		if err != nil {
			return nil, err
		}

		response.SeasonNumber = optionalNumber(item.SeasonNumber)
		response.ShowTitle = optionalString(item.ShowTitle)
		response.NewEpisodeCount = optionalInt32(item.NewEpisodeCount)

		if item.ViewerState != nil {
			response.ViewerState = viewerStateResponse(*item.ViewerState)
		}

		responses = append(responses, response)
	}

	return responses, nil
}

// rowSummaries lists the cards of all rows, so the viewer states of a response come from one
// batch.
func rowSummaries(rowList []apigen.Row) []*apigen.ItemSummary {
	var summaries []*apigen.ItemSummary

	for index := range rowList {
		summaries = append(summaries, pointersTo(rowList[index].Items)...)
	}

	return summaries
}

func heroItemResponse(item approws.HeroItem) (apigen.HeroItem, error) {
	identity, err := itemIdentity(library.Item{ID: item.ItemID, LibraryID: item.LibraryID, Kind: item.Kind})
	if err != nil {
		return apigen.HeroItem{}, err
	}

	backdrop, err := imageRefResponse(&item.Backdrop)
	if err != nil {
		return apigen.HeroItem{}, fmt.Errorf("hero %s: backdrop: %w", item.ItemID, err)
	}

	logo, err := imageRefResponse(item.Logo)
	if err != nil {
		return apigen.HeroItem{}, fmt.Errorf("hero %s: logo: %w", item.ItemID, err)
	}

	return apigen.HeroItem{
		ItemID: identity.id, LibraryID: identity.libraryID, Kind: identity.kind, Title: item.Title,
		Year: optionalNumber(item.Year), SummaryLine: optionalString(item.SummaryLine), Backdrop: *backdrop, Logo: logo,
		Genres: optionalStrings(item.Genres), Media: heroMediaResponse(item.PrimaryFile),
	}, nil
}

// heroMediaResponse describes a movie's primary file; nil for a show, which owns no file.
func heroMediaResponse(file media.File) *apigen.MediaSummary {
	if file.ID == "" {
		return nil
	}

	return mediaSummaryResponse(file)
}
