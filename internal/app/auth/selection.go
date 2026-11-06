package auth

import (
	"context"
	"errors"
	"fmt"
	"time"

	appaccess "github.com/magpeek/mivo/internal/app/access"
	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/access"
	"github.com/magpeek/mivo/internal/domain/account"
)

// SelectionLapse ends a profile selection made without remember after this long without
// requests (ADR 0032 §4, US-03); Resolve checks it.
const SelectionLapse = 4 * time.Hour

// Me is the signed-in account with its selected profile and the picker list.
type Me struct {
	Account account.Account
	// Profile is the selected profile when ProfileSelected is true.
	Profile         account.Profile
	ProfileSelected bool
	// Profiles is the picker list: the primary profile first, then the managed ones by name.
	Profiles []PickerProfile
}

// PickerProfile is a profile in the picker; Profile carries HasPIN and Primary.
type PickerProfile struct {
	Profile account.Profile
	// Admin is true when the profile carries admin powers (account.IsAdminViewer).
	Admin bool
}

// ProfileSelection is a request to select a profile for the current session.
type ProfileSelection struct {
	ProfileID string
	// PIN is needed when the profile has one.
	PIN string
	// Remember keeps the selection beyond SelectionLapse; nil means false for a profile with
	// a PIN and true otherwise.
	Remember *bool
}

// RequireProfile returns the viewer of ctx when it has a selected profile. Without a viewer
// it returns an error wrapping domain.ErrUnauthenticated, without a profile
// ErrProfileRequired. Catalog, media and watch use cases call it first (M7-20).
func RequireProfile(ctx context.Context) (access.Viewer, error) {
	viewer, ok := appaccess.ViewerFrom(ctx)
	if !ok || viewer.AccountID == "" {
		return access.Viewer{}, fmt.Errorf("require profile: %w", domain.ErrUnauthenticated)
	}

	if viewer.ProfileID == "" {
		return access.Viewer{}, ErrProfileRequired
	}

	return viewer, nil
}

// Me describes the session's account (from the viewer in ctx), its selected profile and the
// picker list. It works without a selected profile.
func (service *Service) Me(ctx context.Context) (Me, error) {
	viewer, err := sessionViewer(ctx)
	if err != nil {
		return Me{}, fmt.Errorf("me: %w", err)
	}

	owner, profiles, err := service.accountProfiles(ctx, viewer.AccountID)
	if err != nil {
		return Me{}, fmt.Errorf("me: %w", err)
	}

	current, selected := findProfile(profiles, viewer.AccountID, viewer.ProfileID)

	return Me{Account: owner, Profile: current, ProfileSelected: selected, Profiles: pickerProfiles(owner, profiles)}, nil
}

// SelectProfile selects a profile of the session's account. An unknown profile or one of
// another account wraps domain.ErrNotFound; a profile with a PIN needs it (ErrPINInvalid,
// *ProfileLockedError after PINFailuresBeforeLock wrong ones). It returns the new Me.
func (service *Service) SelectProfile(ctx context.Context, selection ProfileSelection) (Me, error) {
	viewer, err := sessionViewer(ctx)
	if err != nil {
		return Me{}, fmt.Errorf("select profile: %w", err)
	}

	owner, profiles, err := service.accountProfiles(ctx, viewer.AccountID)
	if err != nil {
		return Me{}, fmt.Errorf("select profile: %w", err)
	}

	target, ok := findProfile(profiles, viewer.AccountID, selection.ProfileID)
	if !ok {
		return Me{}, fmt.Errorf("select profile %s: %w", selection.ProfileID, domain.ErrNotFound)
	}

	hasPIN, err := service.checkProfilePIN(ctx, target.ID, selection.PIN)
	if err != nil {
		return Me{}, fmt.Errorf("select profile %s: %w", target.ID, err)
	}

	remember := !hasPIN
	if selection.Remember != nil {
		remember = *selection.Remember
	}

	if err := service.storeSelection(ctx, viewer.AuthSessionID, target.ID, remember); err != nil {
		return Me{}, fmt.Errorf("select profile %s: %w", target.ID, err)
	}

	return Me{Account: owner, Profile: target, ProfileSelected: true, Profiles: pickerProfiles(owner, profiles)}, nil
}

// ClearProfile leaves the selected profile of the session ("Switch profile"); the session
// stays signed in.
func (service *Service) ClearProfile(ctx context.Context) error {
	viewer, err := sessionViewer(ctx)
	if err != nil {
		return fmt.Errorf("clear profile: %w", err)
	}

	if err := service.storeSelection(ctx, viewer.AuthSessionID, "", false); err != nil {
		return fmt.Errorf("clear profile: %w", err)
	}

	return nil
}

// sessionViewer returns the viewer of ctx or an error wrapping domain.ErrUnauthenticated.
func sessionViewer(ctx context.Context) (access.Viewer, error) {
	viewer, ok := appaccess.ViewerFrom(ctx)
	if !ok || viewer.AccountID == "" || viewer.AuthSessionID == "" {
		return access.Viewer{}, domain.ErrUnauthenticated
	}

	return viewer, nil
}

// accountProfiles loads the enabled account and its profiles. A gone or disabled account
// wraps domain.ErrUnauthenticated, because the cached viewer may predate the change.
func (service *Service) accountProfiles(ctx context.Context, accountID string) (account.Account, []account.Profile, error) {
	inputs, err := service.deps.Profiles.ViewerInputs(ctx, accountID, "")
	if errors.Is(err, domain.ErrNotFound) {
		return account.Account{}, nil, fmt.Errorf("account gone: %w", domain.ErrUnauthenticated)
	}

	if err != nil {
		return account.Account{}, nil, fmt.Errorf("load account: %w", err)
	}

	if inputs.Account.ID != accountID || inputs.Account.Disabled {
		return account.Account{}, nil, fmt.Errorf("account disabled: %w", domain.ErrUnauthenticated)
	}

	profiles, err := service.deps.Profiles.Profiles(ctx, accountID)
	if err != nil {
		return account.Account{}, nil, fmt.Errorf("load profiles: %w", err)
	}

	return inputs.Account, profiles, nil
}

// storeSelection writes the selection and drops the cached resolution, so the next request
// sees it. A gone session wraps domain.ErrUnauthenticated.
func (service *Service) storeSelection(ctx context.Context, sessionID, profileID string, remember bool) error {
	err := service.deps.Selector.SetSessionProfile(ctx, sessionID, profileID, remember)

	// Evicted even after a failed write: the session may be gone, and the cache must not
	// keep serving it.
	service.EvictSession(sessionID)

	if errors.Is(err, domain.ErrNotFound) {
		return fmt.Errorf("session gone: %w", domain.ErrUnauthenticated)
	}

	if err != nil {
		return fmt.Errorf("store selection: %w", err)
	}

	return nil
}
