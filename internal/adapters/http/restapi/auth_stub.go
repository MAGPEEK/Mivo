package restapi

import (
	"context"

	"github.com/magpeek/mivo/internal/adapters/http/apigen"
)

// The account operations of ADR 0032 answer 501 until their handlers exist (task M7-21),
// so the contract can be built against while the server keeps compiling.

// GetSetupState is a stub until M7-21.
func (api *API) GetSetupState(_ context.Context, _ apigen.GetSetupStateRequestObject) (apigen.GetSetupStateResponseObject, error) {
	problem := notImplementedProblem()

	return apigen.GetSetupStatedefaultApplicationProblemPlusJSONResponse{Body: problem, StatusCode: int(problem.Status)}, nil
}

// CompleteSetup is a stub until M7-21.
func (api *API) CompleteSetup(_ context.Context, _ apigen.CompleteSetupRequestObject) (apigen.CompleteSetupResponseObject, error) {
	problem := notImplementedProblem()

	return apigen.CompleteSetupdefaultApplicationProblemPlusJSONResponse{Body: problem, StatusCode: int(problem.Status)}, nil
}

// CreateSession is a stub until M7-21.
func (api *API) CreateSession(_ context.Context, _ apigen.CreateSessionRequestObject) (apigen.CreateSessionResponseObject, error) {
	problem := notImplementedProblem()

	return apigen.CreateSessiondefaultApplicationProblemPlusJSONResponse{Body: problem, StatusCode: int(problem.Status)}, nil
}

// DeleteSession is a stub until M7-21.
func (api *API) DeleteSession(_ context.Context, _ apigen.DeleteSessionRequestObject) (apigen.DeleteSessionResponseObject, error) {
	problem := notImplementedProblem()

	return apigen.DeleteSessiondefaultApplicationProblemPlusJSONResponse{Body: problem, StatusCode: int(problem.Status)}, nil
}

// GetMe is a stub until M7-21.
func (api *API) GetMe(_ context.Context, _ apigen.GetMeRequestObject) (apigen.GetMeResponseObject, error) {
	problem := notImplementedProblem()

	return apigen.GetMedefaultApplicationProblemPlusJSONResponse{Body: problem, StatusCode: int(problem.Status)}, nil
}

// SelectProfile is a stub until M7-21.
func (api *API) SelectProfile(_ context.Context, _ apigen.SelectProfileRequestObject) (apigen.SelectProfileResponseObject, error) {
	problem := notImplementedProblem()

	return apigen.SelectProfiledefaultApplicationProblemPlusJSONResponse{Body: problem, StatusCode: int(problem.Status)}, nil
}

// ClearProfileSelection is a stub until M7-21.
func (api *API) ClearProfileSelection(_ context.Context, _ apigen.ClearProfileSelectionRequestObject) (apigen.ClearProfileSelectionResponseObject, error) {
	problem := notImplementedProblem()

	return apigen.ClearProfileSelectiondefaultApplicationProblemPlusJSONResponse{Body: problem, StatusCode: int(problem.Status)}, nil
}

// ChangeOwnPassword is a stub until M7-21.
func (api *API) ChangeOwnPassword(_ context.Context, _ apigen.ChangeOwnPasswordRequestObject) (apigen.ChangeOwnPasswordResponseObject, error) {
	problem := notImplementedProblem()

	return apigen.ChangeOwnPassworddefaultApplicationProblemPlusJSONResponse{Body: problem, StatusCode: int(problem.Status)}, nil
}

// ListOwnDevices is a stub until M7-21.
func (api *API) ListOwnDevices(_ context.Context, _ apigen.ListOwnDevicesRequestObject) (apigen.ListOwnDevicesResponseObject, error) {
	problem := notImplementedProblem()

	return apigen.ListOwnDevicesdefaultApplicationProblemPlusJSONResponse{Body: problem, StatusCode: int(problem.Status)}, nil
}

// RevokeOwnDevice is a stub until M7-21.
func (api *API) RevokeOwnDevice(_ context.Context, _ apigen.RevokeOwnDeviceRequestObject) (apigen.RevokeOwnDeviceResponseObject, error) {
	problem := notImplementedProblem()

	return apigen.RevokeOwnDevicedefaultApplicationProblemPlusJSONResponse{Body: problem, StatusCode: int(problem.Status)}, nil
}
