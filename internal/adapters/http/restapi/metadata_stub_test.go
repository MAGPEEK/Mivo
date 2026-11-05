package restapi

import (
	"context"
	"net/http"
	"testing"

	"github.com/magpeek/mivo/internal/adapters/http/apigen"
)

func TestMetadataStubsAnswerNotImplemented(t *testing.T) {
	t.Parallel()

	api := &API{}
	ctx := context.Background()

	tests := map[string]func() (int, error){
		"searchIdentifyCandidates": func() (int, error) {
			response, err := api.SearchIdentifyCandidates(ctx, apigen.SearchIdentifyCandidatesRequestObject{})
			problem, _ := response.(apigen.SearchIdentifyCandidatesdefaultApplicationProblemPlusJSONResponse)

			return problem.StatusCode, err
		},
		"identifyItem": func() (int, error) {
			response, err := api.IdentifyItem(ctx, apigen.IdentifyItemRequestObject{})
			problem, _ := response.(apigen.IdentifyItemdefaultApplicationProblemPlusJSONResponse)

			return problem.StatusCode, err
		},
		"removeItemMatch": func() (int, error) {
			response, err := api.RemoveItemMatch(ctx, apigen.RemoveItemMatchRequestObject{})
			problem, _ := response.(apigen.RemoveItemMatchdefaultApplicationProblemPlusJSONResponse)

			return problem.StatusCode, err
		},
		"refreshItemMetadata": func() (int, error) {
			response, err := api.RefreshItemMetadata(ctx, apigen.RefreshItemMetadataRequestObject{})
			problem, _ := response.(apigen.RefreshItemMetadatadefaultApplicationProblemPlusJSONResponse)

			return problem.StatusCode, err
		},
		"refreshLibraryMetadata": func() (int, error) {
			response, err := api.RefreshLibraryMetadata(ctx, apigen.RefreshLibraryMetadataRequestObject{})
			problem, _ := response.(apigen.RefreshLibraryMetadatadefaultApplicationProblemPlusJSONResponse)

			return problem.StatusCode, err
		},
		"getMetadataSettings": func() (int, error) {
			response, err := api.GetMetadataSettings(ctx, apigen.GetMetadataSettingsRequestObject{})
			problem, _ := response.(apigen.GetMetadataSettingsdefaultApplicationProblemPlusJSONResponse)

			return problem.StatusCode, err
		},
		"updateMetadataSettings": func() (int, error) {
			response, err := api.UpdateMetadataSettings(ctx, apigen.UpdateMetadataSettingsRequestObject{})
			problem, _ := response.(apigen.UpdateMetadataSettingsdefaultApplicationProblemPlusJSONResponse)

			return problem.StatusCode, err
		},
		"getMetadataStatus": func() (int, error) {
			response, err := api.GetMetadataStatus(ctx, apigen.GetMetadataStatusRequestObject{})
			problem, _ := response.(apigen.GetMetadataStatusdefaultApplicationProblemPlusJSONResponse)

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
