package restapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/magpeek/mivo/internal/adapters/http/apigen"
	applibrary "github.com/magpeek/mivo/internal/app/library"
	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/library"
)

func pointer[T any](value T) *T {
	return &value
}

// listingFailure runs ListLibraryItems on an API without services and returns the status
// and field of its problem; the checked requests never reach a use case.
func listingFailure(t *testing.T, params apigen.ListLibraryItemsParams) (int32, string) {
	t.Helper()

	response, err := (&API{items: okBrowsing{}}).ListLibraryItems(context.Background(), apigen.ListLibraryItemsRequestObject{Params: params})

	return problemOf(t, response, err)
}

// problemOf returns status and field of a handler failure: a returned error goes through
// ProblemForError like in the router, a default response is read directly.
func problemOf(t *testing.T, response any, err error) (int32, string) {
	t.Helper()

	var problem apigen.Problem

	switch typed := response.(type) {
	case apigen.ListLibraryItemsdefaultApplicationProblemPlusJSONResponse:
		problem = typed.Body
	case apigen.ListLibraryFacetsdefaultApplicationProblemPlusJSONResponse:
		problem = typed.Body
	case apigen.ListLibraryLettersdefaultApplicationProblemPlusJSONResponse:
		problem = typed.Body
	case apigen.ListLibraryFoldersdefaultApplicationProblemPlusJSONResponse:
		problem = typed.Body
	default:
		if err == nil {
			return http.StatusOK, ""
		}

		problem = ProblemForError(err)
	}

	field := ""
	if problem.Field != nil {
		field = *problem.Field
	}

	return problem.Status, field
}

func TestListLibraryItemsRejectsUnknownValues(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		params    apigen.ListLibraryItemsParams
		wantField string
	}{
		"sort":              {apigen.ListLibraryItemsParams{Sort: pointer(apigen.ItemSort("rating"))}, "sort"},
		"order":             {apigen.ListLibraryItemsParams{Order: pointer(apigen.SortOrder("up"))}, "order"},
		"negative seed":     {apigen.ListLibraryItemsParams{Seed: pointer(int32(-1))}, "seed"},
		"type":              {apigen.ListLibraryItemsParams{Type: pointer(apigen.ItemKind("song"))}, "type"},
		"non-top-level":     {apigen.ListLibraryItemsParams{Type: pointer(apigen.ItemKindEpisode)}, "type"},
		"quick":             {apigen.ListLibraryItemsParams{Quick: pointer(apigen.ItemQuickFilter("sdr"))}, "quick"},
		"resolution":        {apigen.ListLibraryItemsParams{Resolution: &[]apigen.VideoResolution{"1080p", "2k"}}, "resolution"},
		"year too small":    {apigen.ListLibraryItemsParams{Year: &[]int32{999}}, "year"},
		"decade not 10":     {apigen.ListLibraryItemsParams{Decade: &[]int32{1995}}, "decade"},
		"empty genre":       {apigen.ListLibraryItemsParams{Genre: &[]string{""}}, "genre"},
		"long codec":        {apigen.ListLibraryItemsParams{VideoCodec: &[]string{strings.Repeat("h", 65)}}, "videoCodec"},
		"too many ratings":  {apigen.ListLibraryItemsParams{ContentRating: pointer(make([]string, 51))}, "contentRating"},
		"long language":     {apigen.ListLibraryItemsParams{SubtitleLanguage: &[]string{strings.Repeat("e", 36)}}, "subtitleLanguage"},
		"empty container":   {apigen.ListLibraryItemsParams{Container: &[]string{""}}, "container"},
		"empty audio codec": {apigen.ListLibraryItemsParams{AudioCodec: &[]string{""}}, "audioCodec"},
		"empty audio lang":  {apigen.ListLibraryItemsParams{AudioLanguage: &[]string{""}}, "audioLanguage"},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			status, field := listingFailure(t, test.params)
			if status != http.StatusBadRequest || field != test.wantField {
				t.Errorf("problem = %d field %q, want 400 field %q", status, field, test.wantField)
			}
		})
	}
}

// okBrowsing answers every listing and browse call successfully and records the query.
type okBrowsing struct {
	ItemCatalog

	queries *[]library.ListingQuery
}

func (browsing okBrowsing) ListItems(_ context.Context, _ string, _ library.PageRequest, query library.ListingQuery) (applibrary.ItemPage, error) {
	if browsing.queries != nil {
		*browsing.queries = append(*browsing.queries, query)
	}

	return applibrary.ItemPage{Counted: true, TotalCount: 3}, nil
}

func (okBrowsing) ListFacets(context.Context, string, applibrary.FacetRequest) ([]applibrary.Facet, error) {
	return nil, nil
}

func (okBrowsing) ListLetters(context.Context, string, applibrary.LetterRequest) (applibrary.LetterIndex, error) {
	return applibrary.LetterIndex{}, nil
}

func (okBrowsing) ListFolder(_ context.Context, _ string, path library.FolderPath, _ library.PageRequest) (applibrary.FolderView, error) {
	return applibrary.FolderView{Path: path}, nil
}

func TestListLibraryItemsPassesTheQuery(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		params apigen.ListLibraryItemsParams
		want   library.ListingQuery
	}{
		"defaults":         {want: library.ListingQuery{}},
		"sort year":        {params: apigen.ListLibraryItemsParams{Sort: pointer(apigen.ItemSortYear)}, want: library.ListingQuery{Order: library.ListingOrder{Sort: library.SortByYear}}},
		"descending title": {params: apigen.ListLibraryItemsParams{Order: pointer(apigen.SortOrderDesc)}, want: library.ListingQuery{Order: library.ListingOrder{Descending: true}}},
		"random with seed": {
			params: apigen.ListLibraryItemsParams{Sort: pointer(apigen.ItemSortRandom), Seed: pointer(int32(7))},
			want:   library.ListingQuery{Order: library.ListingOrder{Sort: library.SortByRandom, Seed: 7, HasSeed: true}},
		},
		"filters": {
			params: apigen.ListLibraryItemsParams{
				Type: pointer(apigen.ItemKindMovie), Quick: pointer(apigen.ItemQuickFilterHdr), Genre: &[]string{"Drama"},
				Year: &[]int32{2008}, Decade: &[]int32{1990}, ContentRating: &[]string{"PG"},
				Resolution: &[]apigen.VideoResolution{apigen.VideoResolutionN4K}, VideoCodec: &[]string{"hevc"},
				AudioCodec: &[]string{"aac"}, Container: &[]string{"mkv"}, AudioLanguage: &[]string{"eng"}, SubtitleLanguage: &[]string{"ger"},
			},
			want: library.ListingQuery{Filter: library.ListingFilter{
				Kind: library.ItemKindMovie, Quick: library.QuickHDR, Genres: []string{"Drama"}, Years: []int{2008},
				Decades: []int{1990}, ContentRatings: []string{"PG"}, Resolutions: []library.Resolution{library.Resolution4K},
				VideoCodecs: []string{"hevc"}, AudioCodecs: []string{"aac"}, Containers: []string{"mkv"},
				AudioLanguages: []string{"eng"}, SubtitleLanguages: []string{"ger"},
			}},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			var queries []library.ListingQuery

			api := &API{items: okBrowsing{queries: &queries}}

			response, err := api.ListLibraryItems(context.Background(), apigen.ListLibraryItemsRequestObject{Params: test.params})
			if err != nil {
				t.Fatalf("ListLibraryItems error = %v", err)
			}

			page, ok := response.(apigen.ListLibraryItems200JSONResponse)
			if !ok || page.TotalCount == nil || *page.TotalCount != 3 {
				t.Errorf("response = %#v, want 200 with totalCount 3", response)
			}

			if len(queries) != 1 || !reflect.DeepEqual(queries[0], test.want) {
				t.Errorf("queries = %+v, want %+v", queries, test.want)
			}
		})
	}
}

func TestQueryProblemsNameTheParameter(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		err       error
		wantField string
	}{
		"kind mismatch":                          {err: fmt.Errorf("list: %w", applibrary.ErrItemKindMismatch), wantField: fieldType},
		"query field named like a library field": {err: invalidParameter(fieldName, "x"), wantField: fieldName},
	}

	for name, test := range tests {
		problem := ProblemForError(test.err)
		if problem.Status != http.StatusBadRequest || problem.Code != CodeInvalid || problem.Field == nil || *problem.Field != test.wantField {
			t.Errorf("%s: problem = %+v, want 400 invalid field %q", name, problem, test.wantField)
		}

		if !errors.Is(test.err, domain.ErrInvalid) {
			t.Errorf("%s: error does not wrap domain.ErrInvalid", name)
		}
	}
}

func TestListLibraryFacetsChecksTheRequest(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		params     apigen.ListLibraryFacetsParams
		wantStatus int32
		wantField  string
	}{
		"unknown field":   {apigen.ListLibraryFacetsParams{Field: &[]apigen.FacetField{"actor"}}, http.StatusBadRequest, "field"},
		"too many fields": {apigen.ListLibraryFacetsParams{Field: pointer(make([]apigen.FacetField, 21))}, http.StatusBadRequest, "field"},
		"bad filter":      {apigen.ListLibraryFacetsParams{Quick: pointer(apigen.ItemQuickFilter("x"))}, http.StatusBadRequest, "quick"},
		"valid":           {apigen.ListLibraryFacetsParams{Field: &[]apigen.FacetField{apigen.FacetFieldGenre}, Year: &[]int32{2008}}, http.StatusOK, ""},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			response, err := (&API{items: okBrowsing{}}).ListLibraryFacets(context.Background(), apigen.ListLibraryFacetsRequestObject{Params: test.params})

			status, field := problemOf(t, response, err)
			if status != test.wantStatus || field != test.wantField {
				t.Errorf("problem = %d field %q, want %d field %q", status, field, test.wantStatus, test.wantField)
			}
		})
	}
}

func TestListLibraryLettersChecksTheRequest(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		params     apigen.ListLibraryLettersParams
		wantStatus int32
		wantField  string
	}{
		"unknown order": {apigen.ListLibraryLettersParams{Order: pointer(apigen.SortOrder("x"))}, http.StatusBadRequest, "order"},
		"bad filter":    {apigen.ListLibraryLettersParams{Resolution: &[]apigen.VideoResolution{"x"}}, http.StatusBadRequest, "resolution"},
		"valid":         {apigen.ListLibraryLettersParams{Order: pointer(apigen.SortOrderDesc)}, http.StatusOK, ""},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			response, err := (&API{items: okBrowsing{}}).ListLibraryLetters(context.Background(), apigen.ListLibraryLettersRequestObject{Params: test.params})

			status, field := problemOf(t, response, err)
			if status != test.wantStatus || field != test.wantField {
				t.Errorf("problem = %d field %q, want %d field %q", status, field, test.wantStatus, test.wantField)
			}
		})
	}
}

func TestListLibraryFoldersRejectsTraversal(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		path       string
		wantStatus int32
	}{
		"top":             {"", http.StatusOK},
		"folder":          {"6a7b8c9d-0e1f-4a2b-8c3d-4e5f6a7b8c9d/Animation/Pixar", http.StatusOK},
		"dots in a name":  {"6a7b8c9d/..hidden/a..b", http.StatusOK},
		"parent":          {"6a7b8c9d/../other", http.StatusBadRequest},
		"only parent":     {"..", http.StatusBadRequest},
		"trailing parent": {"6a7b8c9d/..", http.StatusBadRequest},
		"current":         {"6a7b8c9d/./Animation", http.StatusBadRequest},
		"absolute":        {"/etc/passwd", http.StatusBadRequest},
		"backslash":       {`6a7b8c9d\..\x`, http.StatusBadRequest},
		"empty segment":   {"6a7b8c9d//Animation", http.StatusBadRequest},
		"trailing slash":  {"6a7b8c9d/", http.StatusBadRequest},
		"too long":        {strings.Repeat("a", 4097), http.StatusBadRequest},
		"nul":             {"6a7b8c9d/a\x00b", http.StatusBadRequest},
		"control":         {"6a7b8c9d/a\nb", http.StatusBadRequest},
		"invalid utf-8":   {"6a7b8c9d/\xff", http.StatusBadRequest},
		"colon":           {"6a7b8c9d/a:b", http.StatusBadRequest},
		"reserved name":   {"6a7b8c9d/CON", http.StatusBadRequest},
		"reserved ext":    {"6a7b8c9d/lpt1.txt", http.StatusBadRequest},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			request := apigen.ListLibraryFoldersRequestObject{Params: apigen.ListLibraryFoldersParams{Path: &test.path}}
			response, err := (&API{items: okBrowsing{}}).ListLibraryFolders(context.Background(), request)

			status, field := problemOf(t, response, err)
			if status != test.wantStatus {
				t.Errorf("status = %d, want %d", status, test.wantStatus)
			}

			if test.wantStatus == http.StatusBadRequest && field != "path" {
				t.Errorf("field = %q, want path", field)
			}
		})
	}
}

func TestListLibraryFoldersChecksPaging(t *testing.T) {
	t.Parallel()

	request := apigen.ListLibraryFoldersRequestObject{Params: apigen.ListLibraryFoldersParams{Limit: pointer(int32(0))}}
	response, err := (&API{items: okBrowsing{}}).ListLibraryFolders(context.Background(), request)

	if status, _ := problemOf(t, response, err); status != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", status)
	}
}
