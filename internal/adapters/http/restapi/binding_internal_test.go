package restapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"uuid"

	"github.com/magpeek/mivo/internal/adapters/http/apigen"
)

// recordingAPI serves the contract like API but records the library ID the generated code
// bound, so a test can see the value that reaches a handler.
type recordingAPI struct {
	*API

	boundLibraryID chan uuid.UUID
}

func (recorder recordingAPI) GetLibrary(ctx context.Context, request apigen.GetLibraryRequestObject) (apigen.GetLibraryResponseObject, error) {
	recorder.boundLibraryID <- request.LibraryID

	return recorder.API.GetLibrary(ctx, request)
}

// TestUUIDPathParameterBinding proves that the oapi-codegen runtime binds path parameters
// into the standard library uuid.UUID of the type mapping in oapi-codegen.yaml: it goes
// through encoding.TextUnmarshaler, not encoding/json like the bodies.
func TestUUIDPathParameterBinding(t *testing.T) {
	t.Parallel()

	const libraryID = "3f6c1d2a-9b8e-4f70-8a1c-2d3e4f5a6b7c"

	tests := []struct {
		name string
		text string
	}{
		{name: "lower case", text: libraryID},
		{name: "upper case", text: strings.ToUpper(libraryID)},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			api, _ := newInternalTestAPI(t)
			recorder := recordingAPI{API: api, boundLibraryID: make(chan uuid.UUID, 1)}

			mux := http.NewServeMux()
			strict := apigen.NewStrictHandlerWithOptions(recorder, nil, api.strictOptions())
			apigen.HandlerWithOptions(strict, api.routerOptions(mux))

			response := httptest.NewRecorder()
			mux.ServeHTTP(response, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/v1/libraries/"+test.text, nil))

			select {
			case bound := <-recorder.boundLibraryID:
				if bound != uuid.MustParse(libraryID) {
					t.Errorf("bound library id = %s, want %s", bound, libraryID)
				}
			default:
				t.Fatalf("handler not reached; status %d, body %s", response.Code, response.Body)
			}
		})
	}
}
