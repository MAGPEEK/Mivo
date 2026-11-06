package auth

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/access"
	"github.com/magpeek/mivo/internal/domain/account"
)

// DeviceView is a signed-in device (one session) as the device lists show it (US-25). It
// never carries the token hash.
type DeviceView struct {
	// ID is the session ID; RevokeDevice takes it.
	ID        string
	AccountID string
	// AccountName is set by ListAllDevices only.
	AccountName  string
	Device       Device
	LastSeenAt   time.Time
	AddressClass AddressClass
	// Current marks the caller's own session.
	Current bool
}

// ListDevices returns the devices of the session's account, most recently seen first.
// Expired sessions that the periodic purge has not deleted yet are left out.
func (service *Service) ListDevices(ctx context.Context) ([]DeviceView, error) {
	viewer, err := sessionViewer(ctx)
	if err != nil {
		return nil, fmt.Errorf("list devices: %w", err)
	}

	sessions, err := service.deps.Devices.AccountSessions(ctx, viewer.AccountID)
	if err != nil {
		return nil, fmt.Errorf("list devices: %w", err)
	}

	listed := make([]AccountSession, 0, len(sessions))
	for _, session := range sessions {
		// The account check guards against a faulty store handing out other devices.
		if session.AccountID == viewer.AccountID {
			listed = append(listed, AccountSession{Session: session})
		}
	}

	return service.deviceViews(viewer, listed), nil
}

// ListAllDevices returns the devices of every account, most recently seen first. It needs
// admin powers (domain.ErrForbidden otherwise), checked against the store.
func (service *Service) ListAllDevices(ctx context.Context) ([]DeviceView, error) {
	viewer, err := service.adminViewer(ctx)
	if err != nil {
		return nil, fmt.Errorf("list all devices: %w", err)
	}

	sessions, err := service.deps.Devices.AllSessions(ctx)
	if err != nil {
		return nil, fmt.Errorf("list all devices: %w", err)
	}

	return service.deviceViews(viewer, sessions), nil
}

// RevokeOwnDevice signs a device of the session's account out. A device of another account
// or an unknown ID wraps domain.ErrNotFound. signedOut reports that the caller's own session
// ended, so the HTTP adapter clears the cookie.
func (service *Service) RevokeOwnDevice(ctx context.Context, deviceID string) (signedOut bool, err error) {
	viewer, err := sessionViewer(ctx)
	if err != nil {
		return false, fmt.Errorf("revoke device: %w", err)
	}

	return service.revoke(ctx, viewer, deviceID, false)
}

// RevokeDevice signs any account's device out, the administration operation of ADR 0032 §9.
// It needs admin powers (domain.ErrForbidden otherwise, checked against the store); an
// unknown ID wraps domain.ErrNotFound. signedOut is as in RevokeOwnDevice.
func (service *Service) RevokeDevice(ctx context.Context, deviceID string) (signedOut bool, err error) {
	viewer, err := service.adminViewer(ctx)
	if err != nil {
		return false, fmt.Errorf("revoke device: %w", err)
	}

	return service.revoke(ctx, viewer, deviceID, true)
}

func (service *Service) revoke(ctx context.Context, viewer access.Viewer, deviceID string, anyAccount bool) (bool, error) {
	session, err := service.deps.Devices.SessionByID(ctx, deviceID)
	if err != nil {
		return false, fmt.Errorf("revoke device %s: %w", deviceID, err)
	}

	if session.ID != deviceID || (session.AccountID != viewer.AccountID && !anyAccount) {
		return false, fmt.Errorf("revoke device %s: %w", deviceID, domain.ErrNotFound)
	}

	err = service.deps.Revoker.DeleteSession(ctx, session.ID)
	service.EvictSession(session.ID)

	if err != nil {
		return false, fmt.Errorf("revoke device %s: %w", deviceID, err)
	}

	return session.ID == viewer.AuthSessionID, nil
}

// adminViewer returns the viewer of ctx when it has admin powers.
func (service *Service) adminViewer(ctx context.Context) (access.Viewer, error) {
	viewer, err := sessionViewer(ctx)
	if err != nil {
		return access.Viewer{}, err
	}

	admin, err := service.isAdmin(ctx, viewer)
	if err != nil {
		return access.Viewer{}, err
	}

	if !admin {
		return access.Viewer{}, fmt.Errorf("admin powers needed: %w", domain.ErrForbidden)
	}

	return viewer, nil
}

// isAdmin reads the account and profile from the store instead of trusting viewer.Admin, so
// a demotion within the cache window already counts.
func (service *Service) isAdmin(ctx context.Context, viewer access.Viewer) (bool, error) {
	if viewer.ProfileID == "" {
		return false, nil
	}

	inputs, err := service.deps.Profiles.ViewerInputs(ctx, viewer.AccountID, viewer.ProfileID)
	if errors.Is(err, domain.ErrNotFound) {
		return false, nil
	}

	if err != nil {
		return false, fmt.Errorf("check admin: %w", err)
	}

	if inputs.Account.ID != viewer.AccountID || inputs.Profile.ID != viewer.ProfileID {
		return false, nil
	}

	return account.IsAdminViewer(inputs.Account, inputs.Profile), nil
}

// deviceViews drops expired sessions and sorts the rest by last use, newest first.
func (service *Service) deviceViews(viewer access.Viewer, sessions []AccountSession) []DeviceView {
	now := service.deps.Clock()
	views := make([]DeviceView, 0, len(sessions))

	for _, listed := range sessions {
		session := listed.Session
		if expired(session, now) {
			continue
		}

		views = append(views, DeviceView{
			ID: session.ID, AccountID: session.AccountID, AccountName: listed.AccountName,
			Device: session.Device, LastSeenAt: session.LastSeenAt, AddressClass: session.AddressClass,
			Current: session.ID == viewer.AuthSessionID,
		})
	}

	slices.SortFunc(views, func(left, right DeviceView) int {
		return cmp.Or(right.LastSeenAt.Compare(left.LastSeenAt), cmp.Compare(left.ID, right.ID))
	})

	return views
}
