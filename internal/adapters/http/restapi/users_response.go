package restapi

import (
	"math"
	"uuid"

	"github.com/magpeek/mivo/internal/adapters/http/apigen"
	"github.com/magpeek/mivo/internal/app/auth"
	"github.com/magpeek/mivo/internal/app/users"
	"github.com/magpeek/mivo/internal/domain/access"
)

// Entity names in the errors of unreadable stored IDs.
const (
	entityAccount = "account"
	entityProfile = "profile"
	entityDevice  = "device"
	entityLibrary = "library"
)

// userResponse converts an account of the users list. The password is not part of the
// summary, so it cannot leak here.
func userResponse(summary users.AccountSummary) (apigen.User, error) {
	id, err := parseStoredID(entityAccount, summary.Account.ID)
	if err != nil {
		return apigen.User{}, err
	}

	libraries, err := libraryAccessResponse(summary.Libraries)
	if err != nil {
		return apigen.User{}, err
	}

	user := apigen.User{
		ID:            id,
		Name:          summary.Account.Name,
		Role:          apigen.AccountRole(summary.Account.Role),
		Status:        apigen.UserStatusActive,
		CreatedAt:     summary.CreatedAt,
		LibraryAccess: libraries,
		ContentLimit:  contentLimitResponse(summary.PrimaryLimit),
		ProfileCount:  int32(min(summary.ProfileCount, math.MaxInt32)), //nolint:gosec // bounded above
	}

	if summary.Account.Disabled {
		user.Status = apigen.UserStatusDisabled
	}

	if !summary.LastActiveAt.IsZero() {
		user.LastActiveAt = &summary.LastActiveAt
	}

	return user, nil
}

func userListResponse(summaries []users.AccountSummary) (apigen.UserList, error) {
	list := apigen.UserList{Users: make([]apigen.User, 0, len(summaries))}

	for _, summary := range summaries {
		user, err := userResponse(summary)
		if err != nil {
			return apigen.UserList{}, err
		}

		list.Users = append(list.Users, user)
	}

	return list, nil
}

// profileResponse converts a profile. Only HasPIN exists outside the store, so neither the
// PIN nor its hash can appear in a response.
func profileResponse(record users.ProfileRecord) (apigen.Profile, error) {
	id, err := parseStoredID(entityProfile, record.Profile.ID)
	if err != nil {
		return apigen.Profile{}, err
	}

	userID, err := parseStoredID(entityAccount, record.Profile.AccountID)
	if err != nil {
		return apigen.Profile{}, err
	}

	libraries, err := libraryAccessResponse(record.Libraries)
	if err != nil {
		return apigen.Profile{}, err
	}

	return apigen.Profile{
		ID:            id,
		UserID:        userID,
		Name:          record.Profile.Name,
		IsPrimary:     record.Profile.Primary,
		AvatarColor:   apigen.AvatarColor(record.Profile.AvatarColor),
		HasPin:        record.Profile.HasPIN,
		ContentLimit:  contentLimitResponse(record.Limit),
		LibraryAccess: libraries,
	}, nil
}

func profileListResponse(records []users.ProfileRecord) (apigen.ProfileList, error) {
	list := apigen.ProfileList{Profiles: make([]apigen.Profile, 0, len(records))}

	for _, record := range records {
		profile, err := profileResponse(record)
		if err != nil {
			return apigen.ProfileList{}, err
		}

		list.Profiles = append(list.Profiles, profile)
	}

	return list, nil
}

// libraryAccessResponse converts a library access: all true carries no list, a list (also
// an empty one, which allows nothing) is always written out.
func libraryAccessResponse(libraries access.LibraryAccess) (apigen.LibraryAccess, error) {
	if libraries.IsAll() {
		return apigen.LibraryAccess{All: true}, nil
	}

	stored := libraries.IDs()
	ids := make([]uuid.UUID, 0, len(stored))

	for _, text := range stored {
		id, err := parseStoredID(entityLibrary, text)
		if err != nil {
			return apigen.LibraryAccess{}, err
		}

		ids = append(ids, id)
	}

	return apigen.LibraryAccess{LibraryIds: &ids}, nil
}

// contentLimitResponse converts a content limit. The preset is derived from maxAge on every
// answer and never stored (ADR 0032 §7), so preset and maxAge cannot disagree.
func contentLimitResponse(limit access.ContentLimit) apigen.ContentLimit {
	response := apigen.ContentLimit{
		Preset:      apigen.ContentLimitPreset(presetOf(limit)),
		HideUnrated: limit.HideUnrated(),
	}

	if maxAge, ok := limit.MaxAge(); ok {
		age := apigen.AgeRating(maxAge) //nolint:gosec // an age bucket of 0 to 18
		response.MaxAge = &age
	}

	return response
}

func accountDeviceListResponse(views []auth.DeviceView) (apigen.AccountDeviceList, error) {
	list := apigen.AccountDeviceList{Devices: make([]apigen.AccountDevice, 0, len(views))}

	for _, view := range views {
		device, err := accountDeviceResponse(view)
		if err != nil {
			return apigen.AccountDeviceList{}, err
		}

		list.Devices = append(list.Devices, device)
	}

	return list, nil
}

func accountDeviceResponse(view auth.DeviceView) (apigen.AccountDevice, error) {
	id, err := parseStoredID(entityDevice, view.ID)
	if err != nil {
		return apigen.AccountDevice{}, err
	}

	userID, err := parseStoredID(entityAccount, view.AccountID)
	if err != nil {
		return apigen.AccountDevice{}, err
	}

	device := apigen.AccountDevice{
		ID:           id,
		UserID:       userID,
		UserName:     view.AccountName,
		Name:         view.Device.Name,
		Client:       apigen.ClientKind(view.Device.Client),
		LastSeenAt:   view.LastSeenAt,
		AddressClass: apigen.AddressClass(view.AddressClass),
		Current:      view.Current,
	}

	if view.Device.Platform != "" {
		device.Platform = &view.Device.Platform
	}

	return device, nil
}
