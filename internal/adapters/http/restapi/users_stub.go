package restapi

import (
	"context"

	"github.com/magpeek/mivo/internal/adapters/http/apigen"
)

// The user administration operations of ADR 0032 answer 501 until their handlers exist
// (task M7-22), so the admin screens can be built against the contract while the server
// keeps compiling.

// ListUsers is a stub until M7-22.
func (api *API) ListUsers(_ context.Context, _ apigen.ListUsersRequestObject) (apigen.ListUsersResponseObject, error) {
	problem := notImplementedProblem()

	return apigen.ListUsersdefaultApplicationProblemPlusJSONResponse{Body: problem, StatusCode: int(problem.Status)}, nil
}

// CreateUser is a stub until M7-22.
func (api *API) CreateUser(_ context.Context, _ apigen.CreateUserRequestObject) (apigen.CreateUserResponseObject, error) {
	problem := notImplementedProblem()

	return apigen.CreateUserdefaultApplicationProblemPlusJSONResponse{Body: problem, StatusCode: int(problem.Status)}, nil
}

// GetUser is a stub until M7-22.
func (api *API) GetUser(_ context.Context, _ apigen.GetUserRequestObject) (apigen.GetUserResponseObject, error) {
	problem := notImplementedProblem()

	return apigen.GetUserdefaultApplicationProblemPlusJSONResponse{Body: problem, StatusCode: int(problem.Status)}, nil
}

// UpdateUser is a stub until M7-22.
func (api *API) UpdateUser(_ context.Context, _ apigen.UpdateUserRequestObject) (apigen.UpdateUserResponseObject, error) {
	problem := notImplementedProblem()

	return apigen.UpdateUserdefaultApplicationProblemPlusJSONResponse{Body: problem, StatusCode: int(problem.Status)}, nil
}

// DeleteUser is a stub until M7-22.
func (api *API) DeleteUser(_ context.Context, _ apigen.DeleteUserRequestObject) (apigen.DeleteUserResponseObject, error) {
	problem := notImplementedProblem()

	return apigen.DeleteUserdefaultApplicationProblemPlusJSONResponse{Body: problem, StatusCode: int(problem.Status)}, nil
}

// ResetUserPassword is a stub until M7-22.
func (api *API) ResetUserPassword(_ context.Context, _ apigen.ResetUserPasswordRequestObject) (apigen.ResetUserPasswordResponseObject, error) {
	problem := notImplementedProblem()

	return apigen.ResetUserPassworddefaultApplicationProblemPlusJSONResponse{Body: problem, StatusCode: int(problem.Status)}, nil
}

// ListUserProfiles is a stub until M7-22.
func (api *API) ListUserProfiles(_ context.Context, _ apigen.ListUserProfilesRequestObject) (apigen.ListUserProfilesResponseObject, error) {
	problem := notImplementedProblem()

	return apigen.ListUserProfilesdefaultApplicationProblemPlusJSONResponse{Body: problem, StatusCode: int(problem.Status)}, nil
}

// CreateProfile is a stub until M7-22.
func (api *API) CreateProfile(_ context.Context, _ apigen.CreateProfileRequestObject) (apigen.CreateProfileResponseObject, error) {
	problem := notImplementedProblem()

	return apigen.CreateProfiledefaultApplicationProblemPlusJSONResponse{Body: problem, StatusCode: int(problem.Status)}, nil
}

// UpdateProfile is a stub until M7-22.
func (api *API) UpdateProfile(_ context.Context, _ apigen.UpdateProfileRequestObject) (apigen.UpdateProfileResponseObject, error) {
	problem := notImplementedProblem()

	return apigen.UpdateProfiledefaultApplicationProblemPlusJSONResponse{Body: problem, StatusCode: int(problem.Status)}, nil
}

// DeleteProfile is a stub until M7-22.
func (api *API) DeleteProfile(_ context.Context, _ apigen.DeleteProfileRequestObject) (apigen.DeleteProfileResponseObject, error) {
	problem := notImplementedProblem()

	return apigen.DeleteProfiledefaultApplicationProblemPlusJSONResponse{Body: problem, StatusCode: int(problem.Status)}, nil
}

// ListDevices is a stub until M7-22.
func (api *API) ListDevices(_ context.Context, _ apigen.ListDevicesRequestObject) (apigen.ListDevicesResponseObject, error) {
	problem := notImplementedProblem()

	return apigen.ListDevicesdefaultApplicationProblemPlusJSONResponse{Body: problem, StatusCode: int(problem.Status)}, nil
}

// RevokeDevice is a stub until M7-22.
func (api *API) RevokeDevice(_ context.Context, _ apigen.RevokeDeviceRequestObject) (apigen.RevokeDeviceResponseObject, error) {
	problem := notImplementedProblem()

	return apigen.RevokeDevicedefaultApplicationProblemPlusJSONResponse{Body: problem, StatusCode: int(problem.Status)}, nil
}
