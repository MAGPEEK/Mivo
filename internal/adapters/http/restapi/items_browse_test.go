package restapi

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/magpeek/mivo/internal/adapters/http/apigen"
)

func pointer[T any](value T) *T {
	return &value
}

// listingFailure runs ListLibraryItems on an API without services and returns the status
// and field of its problem; the checked requests never reach a use case.
func listingFailure(t *testing.T, params apigen.ListLibraryItemsParams) (int32, string) {
	t.Helper()

	response, err := (&API{}).ListLibraryItems(context.Background(), apigen.ListLibraryItemsRequestObject{Params: params})

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
			t.Fatalf("response = %#v, want a problem", response)
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

func TestListLibraryItemsAnswersNotImplementedForNonDefaults(t *testing.T) {
	t.Parallel()

	tests := map[string]apigen.ListLibraryItemsParams{
		"sort year":        {Sort: pointer(apigen.ItemSortYear)},
		"descending title": {Order: pointer(apigen.SortOrderDesc)},
		"random":           {Sort: pointer(apigen.ItemSortRandom), Seed: pointer(int32(7))},
		"type":             {Type: pointer(apigen.ItemKindMovie)},
		"quick hdr":        {Quick: pointer(apigen.ItemQuickFilterHdr)},
		"genre":            {Genre: &[]string{"Drama"}},
		"decade":           {Decade: &[]int32{1990}},
		"resolution":       {Resolution: &[]apigen.VideoResolution{apigen.VideoResolutionN4K}},
	}

	for name, params := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			status, _ := listingFailure(t, params)
			if status != http.StatusNotImplemented {
				t.Errorf("status = %d, want 501", status)
			}
		})
	}
}

func TestListLibraryFacetsStub(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		params     apigen.ListLibraryFacetsParams
		wantStatus int32
		wantField  string
	}{
		"unknown field":   {apigen.ListLibraryFacetsParams{Field: &[]apigen.FacetField{"actor"}}, http.StatusBadRequest, "field"},
		"too many fields": {apigen.ListLibraryFacetsParams{Field: pointer(make([]apigen.FacetField, 21))}, http.StatusBadRequest, "field"},
		"bad filter":      {apigen.ListLibraryFacetsParams{Quick: pointer(apigen.ItemQuickFilter("x"))}, http.StatusBadRequest, "quick"},
		"valid":           {apigen.ListLibraryFacetsParams{Field: &[]apigen.FacetField{apigen.FacetFieldGenre}, Year: &[]int32{2008}}, http.StatusNotImplemented, ""},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			response, err := (&API{}).ListLibraryFacets(context.Background(), apigen.ListLibraryFacetsRequestObject{Params: test.params})

			status, field := problemOf(t, response, err)
			if status != test.wantStatus || field != test.wantField {
				t.Errorf("problem = %d field %q, want %d field %q", status, field, test.wantStatus, test.wantField)
			}
		})
	}
}

func TestListLibraryLettersStub(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		params     apigen.ListLibraryLettersParams
		wantStatus int32
		wantField  string
	}{
		"unknown order": {apigen.ListLibraryLettersParams{Order: pointer(apigen.SortOrder("x"))}, http.StatusBadRequest, "order"},
		"bad filter":    {apigen.ListLibraryLettersParams{Resolution: &[]apigen.VideoResolution{"x"}}, http.StatusBadRequest, "resolution"},
		"valid":         {apigen.ListLibraryLettersParams{Order: pointer(apigen.SortOrderDesc)}, http.StatusNotImplemented, ""},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			response, err := (&API{}).ListLibraryLetters(context.Background(), apigen.ListLibraryLettersRequestObject{Params: test.params})

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
		"top":             {"", http.StatusNotImplemented},
		"folder":          {"6a7b8c9d-0e1f-4a2b-8c3d-4e5f6a7b8c9d/Animation/Pixar", http.StatusNotImplemented},
		"dots in a name":  {"6a7b8c9d/..hidden/a..b", http.StatusNotImplemented},
		"parent":          {"6a7b8c9d/../other", http.StatusBadRequest},
		"only parent":     {"..", http.StatusBadRequest},
		"trailing parent": {"6a7b8c9d/..", http.StatusBadRequest},
		"current":         {"6a7b8c9d/./Animation", http.StatusBadRequest},
		"absolute":        {"/etc/passwd", http.StatusBadRequest},
		"backslash":       {`6a7b8c9d\..\x`, http.StatusBadRequest},
		"empty segment":   {"6a7b8c9d//Animation", http.StatusBadRequest},
		"trailing slash":  {"6a7b8c9d/", http.StatusBadRequest},
		"too long":        {strings.Repeat("a", 4097), http.StatusBadRequest},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			request := apigen.ListLibraryFoldersRequestObject{Params: apigen.ListLibraryFoldersParams{Path: &test.path}}
			response, err := (&API{}).ListLibraryFolders(context.Background(), request)

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
	response, err := (&API{}).ListLibraryFolders(context.Background(), request)

	if status, _ := problemOf(t, response, err); status != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", status)
	}
}
