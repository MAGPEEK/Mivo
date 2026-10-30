package restapi_test

import (
	"net/http"
	"testing"

	"github.com/magpeek/mivo/internal/adapters/http/restapi"
)

const (
	testLibraryID = "3f6c1d2a-9b8e-4f70-8a1c-2d3e4f5a6b7c"
	testItemID    = "8a9b0c1d-2e3f-4a5b-8c6d-7e8f9a0b1c2d"

	// notImplementedCode is internal (not in the contract) and disappears with M2-32.
	notImplementedCode = "not_implemented"
)

// TestLibraryOperationsAnswerNotImplemented covers the stubs M2-26 replaces: without a
// library service every library and item operation is routed, binds its parameters and
// answers 501.
func TestLibraryOperationsAnswerNotImplemented(t *testing.T) {
	t.Parallel()

	handler, logs := newTestHandlerWithOptions(t, restapi.Options{})

	tests := []struct {
		name   string
		method string
		target string
	}{
		{name: "list libraries", method: http.MethodGet, target: "/api/v1/libraries"},
		{name: "get library", method: http.MethodGet, target: "/api/v1/libraries/" + testLibraryID},
		{name: "start scan", method: http.MethodPost, target: "/api/v1/libraries/" + testLibraryID + "/scan"},
		{name: "list items", method: http.MethodGet, target: "/api/v1/libraries/" + testLibraryID + "/items"},
		{name: "list items with paging", method: http.MethodGet, target: "/api/v1/libraries/" + testLibraryID + "/items?limit=500&cursor=abc"},
		{name: "get item", method: http.MethodGet, target: "/api/v1/items/" + testItemID},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			got := serve(t, handler, test.method, test.target)
			assertProblem(t, got, http.StatusNotImplemented, notImplementedCode)
		})
	}

	t.Cleanup(func() {
		assertLogged(t, logs.String(), "api operation not implemented")
	})
}
