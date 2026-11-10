package restapi

import (
	"github.com/magpeek/mivo/internal/adapters/http/apigen"
	"github.com/magpeek/mivo/internal/app/users"
	"github.com/magpeek/mivo/internal/domain/access"
	"github.com/magpeek/mivo/internal/domain/account"
)

// createAccountInput converts a UserCreate body. libraryAccess defaults to all libraries
// and contentLimit to no limit; a missing password stays empty and is refused by the use
// case with its field.
func createAccountInput(body apigen.UserCreate) (users.CreateAccountInput, error) {
	libraries, err := optionalLibraryAccess(body.LibraryAccess)
	if err != nil {
		return users.CreateAccountInput{}, err
	}

	role := account.Role(body.Role)
	if err := checkRoleLibraries(role, libraries); err != nil {
		return users.CreateAccountInput{}, err
	}

	limit, err := optionalContentLimit(body.ContentLimit)
	if err != nil {
		return users.CreateAccountInput{}, err
	}

	input := users.CreateAccountInput{
		Name: body.Name, Password: valueOrZero(body.Password), Role: role,
		Libraries: access.AllLibraries(), PrimaryLimit: access.NoLimit(),
	}

	if libraries != nil {
		input.Libraries = *libraries
	}

	if limit != nil {
		input.PrimaryLimit = *limit
	}

	return input, nil
}

// accountPatch converts a UserUpdate body; omitted properties stay nil.
func accountPatch(body apigen.UserUpdate) (users.AccountPatch, error) {
	libraries, err := optionalLibraryAccess(body.LibraryAccess)
	if err != nil {
		return users.AccountPatch{}, err
	}

	limit, err := optionalContentLimit(body.ContentLimit)
	if err != nil {
		return users.AccountPatch{}, err
	}

	disabled, err := optionalDisabled(body.Status)
	if err != nil {
		return users.AccountPatch{}, err
	}

	patch := users.AccountPatch{Name: body.Name, Disabled: disabled, Libraries: libraries, PrimaryLimit: limit}

	if body.Role != nil {
		role := account.Role(*body.Role)
		patch.Role = &role
	}

	return patch, nil
}

// optionalDisabled converts a status; a value outside the contract is refused here because
// the domain knows only the disabled flag.
func optionalDisabled(status *apigen.UserStatus) (*bool, error) {
	if status == nil {
		return nil, nil //nolint:nilnil // an omitted property is no error and changes nothing
	}

	var disabled bool

	switch *status {
	case apigen.UserStatusActive:
	case apigen.UserStatusDisabled:
		disabled = true
	default:
		return nil, invalidParameter(fieldStatus, "unknown status")
	}

	return &disabled, nil
}

// checkRoleLibraries refuses a library list for an admin account, which always has every
// library: answering 400 instead of storing "all" silently keeps the admin from believing
// the list applies (api/openapi.yaml, createUser).
func checkRoleLibraries(role account.Role, libraries *access.LibraryAccess) error {
	if role == account.RoleAdmin && libraries != nil && !libraries.IsAll() {
		return invalidParameter(fieldLibraryAccess, "an admin account has all libraries")
	}

	return nil
}

// createProfileInput converts a ProfileCreate body. An omitted avatar colour and PIN stay
// empty, which the use case reads as the default colour and no PIN.
func createProfileInput(body apigen.ProfileCreate) (users.CreateProfileInput, error) {
	libraries, err := optionalLibraryAccess(body.LibraryAccess)
	if err != nil {
		return users.CreateProfileInput{}, err
	}

	limit, err := optionalContentLimit(body.ContentLimit)
	if err != nil {
		return users.CreateProfileInput{}, err
	}

	input := users.CreateProfileInput{
		Name: body.Name, AvatarColor: account.AvatarColor(valueOrZero(body.AvatarColor)), PIN: valueOrZero(body.Pin),
		Libraries: access.AllLibraries(), Limit: access.NoLimit(),
	}

	if libraries != nil {
		input.Libraries = *libraries
	}

	if limit != nil {
		input.Limit = *limit
	}

	return input, nil
}

// profilePatch converts a ProfileUpdate body; omitted properties stay nil. An omitted pin
// keeps the PIN, null removes it and a string replaces it.
func profilePatch(body apigen.ProfileUpdate) (users.ProfilePatch, error) {
	libraries, err := optionalLibraryAccess(body.LibraryAccess)
	if err != nil {
		return users.ProfilePatch{}, err
	}

	limit, err := optionalContentLimit(body.ContentLimit)
	if err != nil {
		return users.ProfilePatch{}, err
	}

	patch := users.ProfilePatch{Name: body.Name, Libraries: libraries, Limit: limit}

	if body.AvatarColor != nil {
		color := account.AvatarColor(*body.AvatarColor)
		patch.AvatarColor = &color
	}

	switch {
	case !body.Pin.IsSpecified():
	case body.Pin.IsNull():
		patch.ClearPIN = true
	default:
		pin := body.Pin.MustGet()
		patch.SetPIN = &pin
	}

	return patch, nil
}

// optionalLibraryAccess converts a LibraryAccess; nil stays nil. all true together with
// library IDs is contradictory and refused (api/openapi.yaml, LibraryAccess.all).
func optionalLibraryAccess(body *apigen.LibraryAccess) (*access.LibraryAccess, error) {
	if body == nil {
		return nil, nil //nolint:nilnil // an omitted property is no error and changes nothing
	}

	var ids []string
	if body.LibraryIds != nil {
		for _, id := range *body.LibraryIds {
			ids = append(ids, id.String())
		}
	}

	if body.All && len(ids) > 0 {
		return nil, invalidParameter(fieldLibraryAccess, "all libraries together with a list")
	}

	libraries := access.OnlyLibraries(ids...)
	if body.All {
		libraries = access.AllLibraries()
	}

	return &libraries, nil
}

// optionalContentLimit converts a ContentLimit; nil stays nil. Only maxAge and hideUnrated
// are passed on: the preset is a label that is never stored, but one that contradicts
// maxAge is refused so a client bug cannot set another limit than the one it shows.
func optionalContentLimit(body *apigen.ContentLimit) (*access.ContentLimit, error) {
	if body == nil {
		return nil, nil //nolint:nilnil // an omitted property is no error and changes nothing
	}

	limit := access.NoLimit().HidingUnrated(body.HideUnrated)

	if body.MaxAge != nil {
		aged, err := access.MaxAgeLimit(access.AgeBucket(*body.MaxAge), body.HideUnrated)
		if err != nil {
			return nil, invalidParameter(fieldContentLimit, "unknown max age")
		}

		limit = aged
	}

	preset := access.Preset(body.Preset)
	if preset != access.PresetCustom && preset != presetOf(limit) {
		return nil, invalidParameter(fieldContentLimit, "preset does not match the max age")
	}

	return &limit, nil
}

// presetOf derives the preset from the maximum age alone (youngChildren 0, children 6,
// teens 12, adults none, else custom). hideUnrated is independent of the preset in the
// contract, so it is set to what the domain's fixed presets carry before asking the domain.
func presetOf(limit access.ContentLimit) access.Preset {
	_, hasMaxAge := limit.MaxAge()

	return limit.HidingUnrated(hasMaxAge).Preset()
}

// valueOrZero returns the pointed-to value or the zero value for nil.
func valueOrZero[T any](pointer *T) T {
	var zero T
	if pointer == nil {
		return zero
	}

	return *pointer
}
