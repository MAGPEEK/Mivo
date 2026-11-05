package restapi_test

import (
	"net/http"
	"testing"
)

// TestBrowseRoutes runs the Browse parameters through the generated router, so the binding
// of repeated query parameters and the problem body are covered (task M5-10).
func TestBrowseRoutes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		target     string
		wantStatus int
		wantCode   string
		wantField  string
	}{
		{target: itemsPath + "?sort=title&order=asc&quick=all&seed=3", wantStatus: http.StatusOK},
		{target: itemsPath + "?sort=bogus", wantStatus: http.StatusBadRequest, wantCode: "invalid", wantField: "sort"},
		{target: itemsPath + "?genre=Drama&genre=", wantStatus: http.StatusBadRequest, wantCode: "invalid", wantField: "genre"},
		{target: itemsPath + "?year=abc", wantStatus: http.StatusBadRequest, wantCode: "bad_request"},
		{target: itemsPath + "?genre=Drama&genre=Comedy&decade=1990", wantStatus: http.StatusNotImplemented, wantCode: "not_implemented"},
		{target: libraryPath + "/facets?field=genre&field=year", wantStatus: http.StatusNotImplemented, wantCode: "not_implemented"},
		{target: libraryPath + "/facets?field=people", wantStatus: http.StatusBadRequest, wantCode: "invalid", wantField: "field"},
		{target: libraryPath + "/letters?resolution=4k", wantStatus: http.StatusNotImplemented, wantCode: "not_implemented"},
		{target: libraryPath + "/folders?path=a%2F..%2Fb", wantStatus: http.StatusBadRequest, wantCode: "invalid", wantField: "path"},
		{target: libraryPath + "/folders?path=a%2Fb", wantStatus: http.StatusNotImplemented, wantCode: "not_implemented"},
	}

	for _, test := range tests {
		t.Run(test.target, func(t *testing.T) {
			t.Parallel()

			handler, _ := newLibraryHandler(t, &fakeLibraryService{}, &fakeScanService{})

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
