package restapi

import (
	"context"
	"fmt"
	"path"

	"github.com/magpeek/mivo/internal/adapters/http/apigen"
	"github.com/magpeek/mivo/internal/domain"
)

// Operation names in the errors and logs of the profile and device handlers.
const (
	operationListUserProfiles = "list user profiles"
	operationCreateProfile    = "create profile"
	operationUpdateProfile    = "update profile"
	operationDeleteProfile    = "delete profile"
	operationListDevices      = "list devices"
	operationRevokeDevice     = "revoke device"
)

// ListUserProfiles answers the profiles of an account, the primary first.
func (api *API) ListUserProfiles(ctx context.Context, request apigen.ListUserProfilesRequestObject) (apigen.ListUserProfilesResponseObject, error) {
	profiles, err := api.userProfiles()
	if err != nil {
		return nil, fmt.Errorf("%s: %w", operationListUserProfiles, err)
	}

	records, err := profiles.ListProfiles(ctx, request.UserID.String())
	if err != nil {
		return nil, fmt.Errorf("%s: %w", operationListUserProfiles, err)
	}

	response, err := profileListResponse(records)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", operationListUserProfiles, err)
	}

	return apigen.ListUserProfiles200JSONResponse(response), nil
}

// CreateProfile adds a managed profile to an account and answers it with its location.
func (api *API) CreateProfile(ctx context.Context, request apigen.CreateProfileRequestObject) (apigen.CreateProfileResponseObject, error) {
	profiles, err := api.userProfiles()
	if err != nil {
		return nil, fmt.Errorf("%s: %w", operationCreateProfile, err)
	}

	if request.Body == nil {
		return nil, fmt.Errorf("%s: missing request body: %w", operationCreateProfile, domain.ErrInvalid)
	}

	input, err := createProfileInput(*request.Body)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", operationCreateProfile, err)
	}

	created, err := profiles.CreateProfile(ctx, request.UserID.String(), input)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", operationCreateProfile, err)
	}

	response, err := profileResponse(created)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", operationCreateProfile, err)
	}

	location := path.Join(profilesPath, response.ID.String())

	return apigen.CreateProfile201JSONResponse{
		Body:    response,
		Headers: apigen.CreateProfile201ResponseHeaders{Location: &location},
	}, nil
}

// UpdateProfile changes the given properties of a profile and answers it as it is stored
// now. An admin and the owning account may call it; the use case tells them apart.
func (api *API) UpdateProfile(ctx context.Context, request apigen.UpdateProfileRequestObject) (apigen.UpdateProfileResponseObject, error) {
	profiles, err := api.userProfiles()
	if err != nil {
		return nil, fmt.Errorf("%s: %w", operationUpdateProfile, err)
	}

	if request.Body == nil {
		return nil, fmt.Errorf("%s: missing request body: %w", operationUpdateProfile, domain.ErrInvalid)
	}

	patch, err := profilePatch(*request.Body)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", operationUpdateProfile, err)
	}

	updated, err := profiles.UpdateProfile(ctx, request.ProfileID.String(), patch)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", operationUpdateProfile, err)
	}

	response, err := profileResponse(updated)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", operationUpdateProfile, err)
	}

	return apigen.UpdateProfile200JSONResponse(response), nil
}

// DeleteProfile removes a managed profile.
func (api *API) DeleteProfile(ctx context.Context, request apigen.DeleteProfileRequestObject) (apigen.DeleteProfileResponseObject, error) {
	profiles, err := api.userProfiles()
	if err != nil {
		return nil, fmt.Errorf("%s: %w", operationDeleteProfile, err)
	}

	err = profiles.DeleteProfile(ctx, request.ProfileID.String())
	if err != nil {
		return nil, fmt.Errorf("%s: %w", operationDeleteProfile, err)
	}

	return apigen.DeleteProfile204Response{}, nil
}

// ListDevices answers the devices signed in to any account.
func (api *API) ListDevices(ctx context.Context, _ apigen.ListDevicesRequestObject) (apigen.ListDevicesResponseObject, error) {
	devices, err := api.deviceAdministration()
	if err != nil {
		return nil, fmt.Errorf("%s: %w", operationListDevices, err)
	}

	views, err := devices.ListAllDevices(ctx)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", operationListDevices, err)
	}

	response, err := accountDeviceListResponse(views)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", operationListDevices, err)
	}

	return apigen.ListDevices200JSONResponse(response), nil
}

// RevokeDevice signs any account's device out. Revoking the caller's own device needs no
// cookie handling here: its session is gone, so the next request answers 401 and the client
// returns to the sign-in screen.
func (api *API) RevokeDevice(ctx context.Context, request apigen.RevokeDeviceRequestObject) (apigen.RevokeDeviceResponseObject, error) {
	devices, err := api.deviceAdministration()
	if err != nil {
		return nil, fmt.Errorf("%s: %w", operationRevokeDevice, err)
	}

	_, err = devices.RevokeDevice(ctx, request.DeviceID.String())
	if err != nil {
		return nil, fmt.Errorf("%s: %w", operationRevokeDevice, err)
	}

	return apigen.RevokeDevice204Response{}, nil
}
