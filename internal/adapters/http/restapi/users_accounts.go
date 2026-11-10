package restapi

import (
	"context"
	"fmt"
	"path"

	"github.com/magpeek/mivo/internal/adapters/http/apigen"
	"github.com/magpeek/mivo/internal/app/users"
	"github.com/magpeek/mivo/internal/domain"
)

// Operation names in the errors and logs of the account handlers.
const (
	operationListUsers         = "list users"
	operationCreateUser        = "create user"
	operationGetUser           = "get user"
	operationUpdateUser        = "update user"
	operationDeleteUser        = "delete user"
	operationResetUserPassword = "reset user password"
)

// ListUsers answers every account as the users list shows it.
func (api *API) ListUsers(ctx context.Context, _ apigen.ListUsersRequestObject) (apigen.ListUsersResponseObject, error) {
	accounts, err := api.userAccounts()
	if err != nil {
		return nil, fmt.Errorf("%s: %w", operationListUsers, err)
	}

	summaries, err := accounts.ListAccounts(ctx)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", operationListUsers, err)
	}

	response, err := userListResponse(summaries)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", operationListUsers, err)
	}

	return apigen.ListUsers200JSONResponse(response), nil
}

// CreateUser creates an account and answers it with its location.
func (api *API) CreateUser(ctx context.Context, request apigen.CreateUserRequestObject) (apigen.CreateUserResponseObject, error) {
	accounts, err := api.userAccounts()
	if err != nil {
		return nil, fmt.Errorf("%s: %w", operationCreateUser, err)
	}

	if request.Body == nil {
		return nil, fmt.Errorf("%s: missing request body: %w", operationCreateUser, domain.ErrInvalid)
	}

	input, err := createAccountInput(*request.Body)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", operationCreateUser, err)
	}

	created, err := accounts.CreateAccount(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", operationCreateUser, err)
	}

	response, err := storedUser(ctx, accounts, created.Account.ID)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", operationCreateUser, err)
	}

	location := path.Join(usersPath, response.ID.String())

	return apigen.CreateUser201JSONResponse{
		Body:    response,
		Headers: apigen.CreateUser201ResponseHeaders{Location: &location},
	}, nil
}

// GetUser answers one account as the users list shows it.
func (api *API) GetUser(ctx context.Context, request apigen.GetUserRequestObject) (apigen.GetUserResponseObject, error) {
	accounts, err := api.userAccounts()
	if err != nil {
		return nil, fmt.Errorf("%s: %w", operationGetUser, err)
	}

	response, err := storedUser(ctx, accounts, request.UserID.String())
	if err != nil {
		return nil, fmt.Errorf("%s: %w", operationGetUser, err)
	}

	return apigen.GetUser200JSONResponse(response), nil
}

// UpdateUser changes the given properties of an account and answers it as it is stored now.
func (api *API) UpdateUser(ctx context.Context, request apigen.UpdateUserRequestObject) (apigen.UpdateUserResponseObject, error) {
	accounts, err := api.userAccounts()
	if err != nil {
		return nil, fmt.Errorf("%s: %w", operationUpdateUser, err)
	}

	if request.Body == nil {
		return nil, fmt.Errorf("%s: missing request body: %w", operationUpdateUser, domain.ErrInvalid)
	}

	accountID := request.UserID.String()

	patch, err := accountPatch(*request.Body)
	if err == nil {
		err = checkPatchedRoleLibraries(ctx, accounts, accountID, patch)
	}

	if err != nil {
		return nil, fmt.Errorf("%s: %w", operationUpdateUser, err)
	}

	_, err = accounts.UpdateAccount(ctx, accountID, patch)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", operationUpdateUser, err)
	}

	response, err := storedUser(ctx, accounts, accountID)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", operationUpdateUser, err)
	}

	return apigen.UpdateUser200JSONResponse(response), nil
}

// checkPatchedRoleLibraries applies checkRoleLibraries to an update. The role comes from
// the request or, when it sends none, from the stored account; reading it here is no
// authorization (the use case decides that) and a role changed in between is harmless,
// because the use case gives an admin account every library whatever the list says.
func checkPatchedRoleLibraries(ctx context.Context, accounts UserAccounts, accountID string, patch users.AccountPatch) error {
	if patch.Libraries == nil || patch.Libraries.IsAll() {
		return nil
	}

	if patch.Role != nil {
		return checkRoleLibraries(*patch.Role, patch.Libraries)
	}

	current, err := accounts.AccountSummary(ctx, accountID)
	if err != nil {
		return fmt.Errorf("read role: %w", err)
	}

	return checkRoleLibraries(current.Account.Role, patch.Libraries)
}

// DeleteUser removes an account with its profiles and sessions.
func (api *API) DeleteUser(ctx context.Context, request apigen.DeleteUserRequestObject) (apigen.DeleteUserResponseObject, error) {
	accounts, err := api.userAccounts()
	if err != nil {
		return nil, fmt.Errorf("%s: %w", operationDeleteUser, err)
	}

	err = accounts.DeleteAccount(ctx, request.UserID.String())
	if err != nil {
		return nil, fmt.Errorf("%s: %w", operationDeleteUser, err)
	}

	return apigen.DeleteUser204Response{}, nil
}

// ResetUserPassword replaces the password of an account. A missing password is passed on
// empty, so the use case refuses it with its field like every other invalid password.
func (api *API) ResetUserPassword(ctx context.Context, request apigen.ResetUserPasswordRequestObject) (apigen.ResetUserPasswordResponseObject, error) {
	accounts, err := api.userAccounts()
	if err != nil {
		return nil, fmt.Errorf("%s: %w", operationResetUserPassword, err)
	}

	if request.Body == nil {
		return nil, fmt.Errorf("%s: missing request body: %w", operationResetUserPassword, domain.ErrInvalid)
	}

	err = accounts.ResetPassword(ctx, request.UserID.String(), valueOrZero(request.Body.Password))
	if err != nil {
		return nil, fmt.Errorf("%s: %w", operationResetUserPassword, err)
	}

	return apigen.ResetUserPassword204Response{}, nil
}

// storedUser reads an account as the users list shows it: the answers of createUser,
// getUser and updateUser carry the profile count, the creation time and the last activity,
// which only the summary has.
func storedUser(ctx context.Context, accounts UserAccounts, accountID string) (apigen.User, error) {
	summary, err := accounts.AccountSummary(ctx, accountID)
	if err != nil {
		return apigen.User{}, fmt.Errorf("read account %s: %w", accountID, err)
	}

	return userResponse(summary)
}
