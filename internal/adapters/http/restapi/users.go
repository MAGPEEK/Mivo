package restapi

import (
	"context"
	"errors"
	"net/http"

	"github.com/magpeek/mivo/internal/adapters/http/apigen"
	"github.com/magpeek/mivo/internal/app/auth"
	"github.com/magpeek/mivo/internal/app/users"
	"github.com/magpeek/mivo/internal/domain/account"
)

// The user administration operations of ADR 0032 (US-10, US-11, US-20, US-22, US-25). The
// handlers only translate between the contract and the use cases: who may do what is
// decided by the use cases and by the management guard of the server (ADR 0032 §9), never
// here.

// UserAccounts is the account part of app/users.Service.
type UserAccounts interface {
	// ListAccounts returns every account ordered by name.
	ListAccounts(ctx context.Context) ([]users.AccountSummary, error)
	// AccountSummary returns one account as the list shows it.
	AccountSummary(ctx context.Context, accountID string) (users.AccountSummary, error)
	// CreateAccount creates an account with its primary profile and initial password.
	CreateAccount(ctx context.Context, input users.CreateAccountInput) (users.AccountRecord, error)
	// UpdateAccount changes the given fields of an account.
	UpdateAccount(ctx context.Context, accountID string, patch users.AccountPatch) (users.AccountRecord, error)
	// DeleteAccount removes an account with its profiles and sessions.
	DeleteAccount(ctx context.Context, accountID string) error
	// ResetPassword replaces the password of an account.
	ResetPassword(ctx context.Context, accountID, password string) error
}

// UserProfiles is the profile part of app/users.Service.
type UserProfiles interface {
	// ListProfiles returns the profiles of an account, the primary first.
	ListProfiles(ctx context.Context, accountID string) ([]users.ProfileRecord, error)
	// CreateProfile adds a managed profile to an account.
	CreateProfile(ctx context.Context, accountID string, input users.CreateProfileInput) (users.ProfileRecord, error)
	// UpdateProfile changes the given fields of a profile.
	UpdateProfile(ctx context.Context, profileID string, patch users.ProfilePatch) (users.ProfileRecord, error)
	// DeleteProfile removes a managed profile.
	DeleteProfile(ctx context.Context, profileID string) error
}

// DeviceAdministration is the all-accounts device part of app/auth.Service.
type DeviceAdministration interface {
	// ListAllDevices returns the devices of every account, most recently seen first.
	ListAllDevices(ctx context.Context) ([]auth.DeviceView, error)
	// RevokeDevice signs any account's device out; signedOut reports the caller's own.
	RevokeDevice(ctx context.Context, deviceID string) (signedOut bool, err error)
}

// UserServices are the services of the user administration endpoints. Each is optional: an
// operation whose service is nil answers 501 "not_implemented", so servers and tests that
// have no accounts need not wire them.
type UserServices struct {
	Accounts UserAccounts
	Profiles UserProfiles
	Devices  DeviceAdministration
}

// Problem codes of the user administration (api/openapi.yaml, Problem.code, ADR 0032).
const (
	// CodeLastAdmin reports a change that would delete, disable or demote the last enabled
	// admin account (409).
	CodeLastAdmin = "last_admin"
	// CodeNameTaken reports a name another account or profile of the account has (409).
	CodeNameTaken = "name_taken"
	// CodeProfileLimit reports an account that has all its managed profiles already (409).
	CodeProfileLimit = "profile_limit"
)

// Where created accounts and profiles are located: Location is the path + "/" + ID.
const (
	usersPath    = "/api/v1/users"
	profilesPath = "/api/v1/profiles"
)

// Request properties a user administration problem can point at (Problem.field) besides the
// account.Field* names, which already are the API property names.
const (
	fieldLibraryAccess = "libraryAccess"
	fieldContentLimit  = "contentLimit"
	fieldStatus        = "status"
)

// invalidValueDetail is the fixed detail of a rejected property. The reason of the domain
// error stays in the log: Problem.field tells the client what to correct.
const invalidValueDetail = "The value is not valid."

// errNotWired reports an operation whose service was not wired (see UserServices).
var errNotWired = errors.New("service not wired")

// userProblem maps the failures of the user administration that have their own answer: the
// three conflicts with their codes, a rejected field with its name, a refused library access
// and an operation without its service. It reports false for every other error.
func userProblem(err error) (apigen.Problem, bool) {
	var fieldErr *account.InvalidFieldError

	switch {
	case errors.Is(err, account.ErrLastAdmin):
		return NewProblem(http.StatusConflict, CodeLastAdmin), true
	case errors.Is(err, account.ErrNameTaken):
		return NewProblem(http.StatusConflict, CodeNameTaken), true
	case errors.Is(err, account.ErrProfileLimit):
		return NewProblem(http.StatusConflict, CodeProfileLimit), true
	case errors.As(err, &fieldErr):
		return fieldRequestProblem(CodeInvalid, fieldErr.Field, invalidValueDetail), true
	case errors.Is(err, users.ErrLibraryAccess):
		return fieldRequestProblem(CodeInvalid, fieldLibraryAccess, invalidValueDetail), true
	case errors.Is(err, errNotWired):
		return notImplementedProblem(), true
	default:
		return apigen.Problem{}, false
	}
}

// userAccounts returns the accounts service or errNotWired.
func (api *API) userAccounts() (UserAccounts, error) {
	if api.users.Accounts == nil {
		return nil, errNotWired
	}

	return api.users.Accounts, nil
}

// userProfiles returns the profiles service or errNotWired.
func (api *API) userProfiles() (UserProfiles, error) {
	if api.users.Profiles == nil {
		return nil, errNotWired
	}

	return api.users.Profiles, nil
}

// deviceAdministration returns the devices service or errNotWired.
func (api *API) deviceAdministration() (DeviceAdministration, error) {
	if api.users.Devices == nil {
		return nil, errNotWired
	}

	return api.users.Devices, nil
}
