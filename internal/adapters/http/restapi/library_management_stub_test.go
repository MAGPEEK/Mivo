package restapi

import (
	"context"
	"net/http"
	"testing"

	"github.com/magpeek/mivo/internal/adapters/http/apigen"
)

func TestLibraryManagementStubsAnswerNotImplemented(t *testing.T) {
	t.Parallel()

	api := &API{}
	ctx := context.Background()

	tests := map[string]func() (int, error){
		"createLibrary": func() (int, error) {
			response, err := api.CreateLibrary(ctx, apigen.CreateLibraryRequestObject{})
			problem, _ := response.(apigen.CreateLibrarydefaultApplicationProblemPlusJSONResponse)

			return problem.StatusCode, err
		},
		"updateLibrary": func() (int, error) {
			response, err := api.UpdateLibrary(ctx, apigen.UpdateLibraryRequestObject{})
			problem, _ := response.(apigen.UpdateLibrarydefaultApplicationProblemPlusJSONResponse)

			return problem.StatusCode, err
		},
		"deleteLibrary": func() (int, error) {
			response, err := api.DeleteLibrary(ctx, apigen.DeleteLibraryRequestObject{})
			problem, _ := response.(apigen.DeleteLibrarydefaultApplicationProblemPlusJSONResponse)

			return problem.StatusCode, err
		},
		"listFolderRoots": func() (int, error) {
			response, err := api.ListFolderRoots(ctx, apigen.ListFolderRootsRequestObject{})
			problem, _ := response.(apigen.ListFolderRootsdefaultApplicationProblemPlusJSONResponse)

			return problem.StatusCode, err
		},
		"listFolders": func() (int, error) {
			response, err := api.ListFolders(ctx, apigen.ListFoldersRequestObject{})
			problem, _ := response.(apigen.ListFoldersdefaultApplicationProblemPlusJSONResponse)

			return problem.StatusCode, err
		},
	}

	for name, call := range tests {
		status, err := call()
		if err != nil || status != http.StatusNotImplemented {
			t.Errorf("%s = %d, %v, want 501", name, status, err)
		}
	}
}
