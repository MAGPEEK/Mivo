package restapi_test

import (
	"net/http"
	"testing"

	"github.com/magpeek/mivo/internal/adapters/http/restapi"
)

// TestMalformedParametersAnswerBadRequest covers the oapi-codegen runtime binding of path
// and query parameters: a value that does not bind is a 400 problem that repeats neither
// the value nor the binder's error text.
func TestMalformedParametersAnswerBadRequest(t *testing.T) {
	t.Parallel()

	handler, logs := newTestHandlerWithOptions(t, restapi.Options{})

	tests := []struct {
		name   string
		method string
		target string
		input  string
	}{
		{name: "library id", method: http.MethodGet, target: "/api/v1/libraries/not-a-uuid", input: "not-a-uuid"},
		{name: "library id of scan", method: http.MethodPost, target: "/api/v1/libraries/not-a-uuid/scan", input: "not-a-uuid"},
		{name: "item id", method: http.MethodGet, target: "/api/v1/items/12345", input: "12345"},
		{name: "truncated session id", method: http.MethodGet, target: sessionsPath + "/" + testSessionID[:35], input: testSessionID[:35]},
		{name: "session id with extra text", method: http.MethodDelete, target: sessionsPath + "/" + testSessionID + "x", input: testSessionID + "x"},
		{name: "limit not a number", method: http.MethodGet, target: "/api/v1/libraries/" + testLibraryID + "/items?limit=many", input: "many"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			got := serve(t, handler, test.method, test.target)
			assertProblem(t, got, http.StatusBadRequest, restapi.CodeBadRequest)
			assertNoInternalText(t, got, test.input)
		})
	}

	t.Cleanup(func() {
		assertLogged(t, logs.String(), "rejected malformed request")
	})
}
