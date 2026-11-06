package restapi

import (
	"context"
	"net/http"
	"testing"

	"github.com/magpeek/mivo/internal/adapters/http/apigen"
)

func TestAuthStubsAnswerNotImplemented(t *testing.T) {
	t.Parallel()

	api := &API{}
	ctx := context.Background()

	tests := map[string]func() (int, error){
		"getSetupState": func() (int, error) {
			response, err := api.GetSetupState(ctx, apigen.GetSetupStateRequestObject{})
			problem, _ := response.(apigen.GetSetupStatedefaultApplicationProblemPlusJSONResponse)

			return problem.StatusCode, err
		},
		"completeSetup": func() (int, error) {
			response, err := api.CompleteSetup(ctx, apigen.CompleteSetupRequestObject{})
			problem, _ := response.(apigen.CompleteSetupdefaultApplicationProblemPlusJSONResponse)

			return problem.StatusCode, err
		},
		"createSession": func() (int, error) {
			response, err := api.CreateSession(ctx, apigen.CreateSessionRequestObject{})
			problem, _ := response.(apigen.CreateSessiondefaultApplicationProblemPlusJSONResponse)

			return problem.StatusCode, err
		},
		"deleteSession": func() (int, error) {
			response, err := api.DeleteSession(ctx, apigen.DeleteSessionRequestObject{})
			problem, _ := response.(apigen.DeleteSessiondefaultApplicationProblemPlusJSONResponse)

			return problem.StatusCode, err
		},
		"getMe": func() (int, error) {
			response, err := api.GetMe(ctx, apigen.GetMeRequestObject{})
			problem, _ := response.(apigen.GetMedefaultApplicationProblemPlusJSONResponse)

			return problem.StatusCode, err
		},
		"selectProfile": func() (int, error) {
			response, err := api.SelectProfile(ctx, apigen.SelectProfileRequestObject{})
			problem, _ := response.(apigen.SelectProfiledefaultApplicationProblemPlusJSONResponse)

			return problem.StatusCode, err
		},
		"clearProfileSelection": func() (int, error) {
			response, err := api.ClearProfileSelection(ctx, apigen.ClearProfileSelectionRequestObject{})
			problem, _ := response.(apigen.ClearProfileSelectiondefaultApplicationProblemPlusJSONResponse)

			return problem.StatusCode, err
		},
		"changeOwnPassword": func() (int, error) {
			response, err := api.ChangeOwnPassword(ctx, apigen.ChangeOwnPasswordRequestObject{})
			problem, _ := response.(apigen.ChangeOwnPassworddefaultApplicationProblemPlusJSONResponse)

			return problem.StatusCode, err
		},
		"listOwnDevices": func() (int, error) {
			response, err := api.ListOwnDevices(ctx, apigen.ListOwnDevicesRequestObject{})
			problem, _ := response.(apigen.ListOwnDevicesdefaultApplicationProblemPlusJSONResponse)

			return problem.StatusCode, err
		},
		"revokeOwnDevice": func() (int, error) {
			response, err := api.RevokeOwnDevice(ctx, apigen.RevokeOwnDeviceRequestObject{})
			problem, _ := response.(apigen.RevokeOwnDevicedefaultApplicationProblemPlusJSONResponse)

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
