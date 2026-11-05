package restapi_test

import (
	"fmt"
	"net/http"
	"testing"

	applibrary "github.com/magpeek/mivo/internal/app/library"
	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/library"
)

// TestBrowseRoutes runs the Browse parameters through the generated router, so the binding
// of repeated query parameters and the problem body are covered (tasks M5-10, M5-11).
func TestBrowseRoutes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		target     string
		browseErr  error
		wantStatus int
		wantCode   string
		wantField  string
	}{
		{target: itemsPath + "?sort=title&order=asc&quick=all&seed=3", wantStatus: http.StatusOK},
		{target: itemsPath + "?sort=bogus", wantStatus: http.StatusBadRequest, wantCode: "invalid", wantField: "sort"},
		{target: itemsPath + "?genre=Drama&genre=", wantStatus: http.StatusBadRequest, wantCode: "invalid", wantField: "genre"},
		{target: itemsPath + "?year=abc", wantStatus: http.StatusBadRequest, wantCode: "bad_request"},
		{target: itemsPath + "?genre=Drama&genre=Comedy&decade=1990", wantStatus: http.StatusOK},
		{target: libraryPath + "/facets?field=genre&field=year", wantStatus: http.StatusOK},
		{target: libraryPath + "/facets?field=people", wantStatus: http.StatusBadRequest, wantCode: "invalid", wantField: "field"},
		{target: libraryPath + "/facets", browseErr: errLibraryNotFound(testLibraryID), wantStatus: http.StatusNotFound, wantCode: "not_found"},
		{target: libraryPath + "/facets?type=show", browseErr: fmt.Errorf("x: %w", applibrary.ErrItemKindMismatch), wantStatus: http.StatusBadRequest, wantCode: "invalid", wantField: "type"},
		{target: libraryPath + "/letters?resolution=4k", wantStatus: http.StatusOK},
		{target: libraryPath + "/letters", browseErr: errLibraryNotFound(testLibraryID), wantStatus: http.StatusNotFound, wantCode: "not_found"},
		{target: libraryPath + "/folders?path=a%2F..%2Fb", wantStatus: http.StatusBadRequest, wantCode: "invalid", wantField: "path"},
		{target: libraryPath + "/folders?path=a%2Fnul", wantStatus: http.StatusBadRequest, wantCode: "invalid", wantField: "path"},
		{target: libraryPath + "/folders?path=a%2Fb", wantStatus: http.StatusOK},
		{target: libraryPath + "/folders?path=a%2Fb", browseErr: fmt.Errorf("x: %w", domain.ErrNotFound), wantStatus: http.StatusNotFound, wantCode: "not_found"},
		{target: libraryPath + "/folders", browseErr: fmt.Errorf("x: %w", domain.ErrUnavailable), wantStatus: http.StatusServiceUnavailable, wantCode: "unavailable"},
	}

	for _, test := range tests {
		t.Run(test.target, func(t *testing.T) {
			t.Parallel()

			handler, _ := newLibraryHandler(t, &fakeLibraryService{browseErr: test.browseErr}, &fakeScanService{})

			got := serve(t, handler, http.MethodGet, test.target)
			if test.wantStatus == http.StatusOK {
				assertJSONResponse(t, got, http.StatusOK)

				return
			}

			problem := assertProblem(t, got, test.wantStatus, test.wantCode)

			field := ""
			if problem.Field != nil {
				field = *problem.Field
			}

			if field != test.wantField {
				t.Errorf("field = %q, want %q", field, test.wantField)
			}
		})
	}
}

func TestBrowseResponses(t *testing.T) {
	t.Parallel()

	item := applibrary.ItemSummary{Item: testItem(testItemID, "Up")}
	service := &fakeLibraryService{
		page:    applibrary.ItemPage{Counted: true, TotalCount: 42},
		facets:  []applibrary.Facet{{Field: library.FacetGenre, Values: []applibrary.FacetValue{{Value: "Drama", Count: 2}}, Truncated: true}},
		letters: applibrary.LetterIndex{Letters: []applibrary.LetterOffset{{Letter: "#", Offset: 0}, {Letter: "U", Offset: 3}}, TotalCount: 4},
		folder: applibrary.FolderView{NextCursor: "next", Entries: []applibrary.FolderViewEntry{
			{Kind: applibrary.FolderEntryFolder, Name: "Pixar", Path: library.FolderPath{FolderID: "f1", Segments: []string{"Animation", "Pixar"}}},
			{Kind: applibrary.FolderEntryItem, Name: "Up.mkv", Path: library.FolderPath{FolderID: "f1", Segments: []string{"Animation", "Up.mkv"}}, Item: &item},
		}},
	}
	handler, _ := newLibraryHandler(t, service, &fakeScanService{})

	tests := []struct {
		target    string
		fragments []string
	}{
		{target: itemsPath, fragments: []string{`"totalCount":42`}},
		{target: libraryPath + "/facets?field=genre", fragments: []string{`"field":"genre"`, `"value":"Drama"`, `"count":2`, `"truncated":true`}},
		{target: libraryPath + "/letters", fragments: []string{`{"letter":"#","offset":0}`, `{"letter":"U","offset":3}`, `"totalCount":4`}},
		{target: libraryPath + "/folders?path=f1%2FAnimation", fragments: []string{
			`"path":"f1/Animation"`, `"parentPath":"f1"`, `"nextCursor":"next"`, `"libraryId":"` + testLibraryID + `"`,
			`{"kind":"folder","name":"Pixar","path":"f1/Animation/Pixar"}`, `"kind":"item"`, `"path":"f1/Animation/Up.mkv"`, `"title":"Up"`,
		}},
	}

	for _, test := range tests {
		got := serve(t, handler, http.MethodGet, test.target)
		assertJSONResponse(t, got, http.StatusOK)
		assertBodyContains(t, got.body, test.fragments...)
	}

	top := serve(t, handler, http.MethodGet, libraryPath+"/folders")
	assertBodyContains(t, top.body, `"path":""`)

	if requests := service.requestedFacets; len(requests) != 1 || len(requests[0].Fields) != 1 || requests[0].Fields[0] != library.FacetGenre {
		t.Errorf("facet requests = %+v, want genre", requests)
	}
}
