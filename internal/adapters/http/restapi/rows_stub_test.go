package restapi_test

import (
	"net/http"
	"testing"
)

// homeRowsPath is the Home rows route (task M5-12).
const homeRowsPath = "/api/v1/home/rows"

// TestRowsRoutes runs the rows parameters through the generated router: invalid limits
// answer 400 with the field, valid requests 501 until M5-13.
func TestRowsRoutes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		target     string
		wantStatus int
		wantCode   string
		wantField  string
	}{
		{target: homeRowsPath, wantStatus: http.StatusNotImplemented, wantCode: "not_implemented"},
		{target: homeRowsPath + "?limit=50", wantStatus: http.StatusNotImplemented, wantCode: "not_implemented"},
		{target: homeRowsPath + "?limit=0", wantStatus: http.StatusBadRequest, wantCode: "invalid", wantField: "limit"},
		{target: homeRowsPath + "?limit=51", wantStatus: http.StatusBadRequest, wantCode: "invalid", wantField: "limit"},
		{target: homeRowsPath + "?limit=abc", wantStatus: http.StatusBadRequest, wantCode: "bad_request"},
		{target: libraryPath + "/rows?limit=1", wantStatus: http.StatusNotImplemented, wantCode: "not_implemented"},
		{target: libraryPath + "/rows?limit=-1", wantStatus: http.StatusBadRequest, wantCode: "invalid", wantField: "limit"},
		{target: "/api/v1/libraries/not-a-uuid/rows", wantStatus: http.StatusBadRequest, wantCode: "bad_request"},
	}

	for _, test := range tests {
		t.Run(test.target, func(t *testing.T) {
			t.Parallel()

			handler, _ := newLibraryHandler(t, &fakeLibraryService{}, &fakeScanService{})

			problem := assertProblem(t, serve(t, handler, http.MethodGet, test.target), test.wantStatus, test.wantCode)

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
