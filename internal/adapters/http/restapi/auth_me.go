package restapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/magpeek/mivo/internal/adapters/http/apigen"
	"github.com/magpeek/mivo/internal/app/auth"
	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/account"
)

// fieldNewPassword is the PasswordChange property an invalid new password points at.
const fieldNewPassword = "newPassword"

// GetMe describes the signed-in account, its selected profile and the picker list.
func (api *API) GetMe(ctx context.Context, _ apigen.GetMeRequestObject) (apigen.GetMeResponseObject, error) {
	me, err := api.auth.Me(ctx)
	if err != nil {
		return nil, operationFailure(operationGetMe, err, http.StatusUnauthorized)
	}

	response, err := meResponse(me)
	if err != nil {
		return nil, operationFailure(operationGetMe, err)
	}

	return apigen.GetMe200JSONResponse(response), nil
}

// SelectProfile selects a profile for this session; the use case checks the PIN.
func (api *API) SelectProfile(ctx context.Context, request apigen.SelectProfileRequestObject) (apigen.SelectProfileResponseObject, error) {
	if request.Body == nil {
		return nil, fmt.Errorf("%s: missing request body: %w", operationSelectProfile, domain.ErrInvalid)
	}

	me, err := api.auth.SelectProfile(ctx, auth.ProfileSelection{
		ProfileID: request.Body.ProfileID.String(),
		PIN:       textOrEmpty(request.Body.Pin),
		Remember:  request.Body.Remember,
	})
	if err != nil {
		return nil, operationFailure(operationSelectProfile, err,
			http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound, http.StatusTooManyRequests)
	}

	response, err := meResponse(me)
	if err != nil {
		return nil, operationFailure(operationSelectProfile, err)
	}

	return apigen.SelectProfile200JSONResponse(response), nil
}

// ClearProfileSelection leaves the selected profile; the session stays signed in.
func (api *API) ClearProfileSelection(ctx context.Context, _ apigen.ClearProfileSelectionRequestObject) (apigen.ClearProfileSelectionResponseObject, error) {
	err := api.auth.ClearProfile(ctx)
	if err != nil {
		return nil, operationFailure(operationClearProfile, err, http.StatusUnauthorized)
	}

	return apigen.ClearProfileSelection204Response{}, nil
}

// ChangeOwnPassword replaces the password of the signed-in account.
func (api *API) ChangeOwnPassword(ctx context.Context, request apigen.ChangeOwnPasswordRequestObject) (apigen.ChangeOwnPasswordResponseObject, error) {
	if request.Body == nil {
		return nil, fmt.Errorf("%s: missing request body: %w", operationChangeOwnPassword, domain.ErrInvalid)
	}

	err := api.auth.ChangePassword(ctx, auth.PasswordChange{
		Current: textOrEmpty(request.Body.CurrentPassword),
		New:     textOrEmpty(request.Body.NewPassword),
		Peer:    PeerFrom(ctx),
	})

	switch {
	case err == nil:
		return apigen.ChangeOwnPassword204Response{}, nil
	case errors.Is(err, auth.ErrInvalidCredentials):
		// 403, not the 401 of a failed sign-in: the session stays signed in, and clients
		// treat 401 as "signed out".
		return apigen.ChangeOwnPassword403ApplicationProblemPlusJSONResponse(
			NewProblem(http.StatusForbidden, CodeInvalidCredentials)), nil
	default:
		return nil, operationFailure(operationChangeOwnPassword, newPasswordField(err),
			http.StatusBadRequest, http.StatusUnauthorized, http.StatusForbidden, http.StatusTooManyRequests)
	}
}

// newPasswordField renames the field of an invalid password to the property of the
// PasswordChange body, where the password rule applies to newPassword.
func newPasswordField(err error) error {
	var fieldErr *account.InvalidFieldError
	if errors.As(err, &fieldErr) && fieldErr.Field == account.FieldPassword {
		return &account.InvalidFieldError{Field: fieldNewPassword, Reason: fieldErr.Reason}
	}

	return err
}

// ListOwnDevices lists the devices signed in to the account.
func (api *API) ListOwnDevices(ctx context.Context, _ apigen.ListOwnDevicesRequestObject) (apigen.ListOwnDevicesResponseObject, error) {
	devices, err := api.auth.ListDevices(ctx)
	if err != nil {
		return nil, operationFailure(operationListOwnDevices, err, http.StatusUnauthorized)
	}

	response, err := deviceListResponse(devices)
	if err != nil {
		return nil, operationFailure(operationListOwnDevices, err)
	}

	return apigen.ListOwnDevices200JSONResponse(response), nil
}

// RevokeOwnDevice signs a device of the account out. Revoking the caller's own device equals
// signing out, so the cookie is cleared then.
func (api *API) RevokeOwnDevice(ctx context.Context, request apigen.RevokeOwnDeviceRequestObject) (apigen.RevokeOwnDeviceResponseObject, error) {
	signedOut, err := api.auth.RevokeOwnDevice(ctx, request.DeviceID.String())
	if err != nil {
		return nil, operationFailure(operationRevokeOwnDevice, err, http.StatusUnauthorized, http.StatusNotFound)
	}

	if signedOut {
		return signedOutResponse{secure: clientRequestFrom(ctx).secure}, nil
	}

	return apigen.RevokeOwnDevice204Response{}, nil
}
