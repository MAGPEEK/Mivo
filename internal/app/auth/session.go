package auth

import (
	"context"
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/account"
)

// Device label limits; M7-21 derives a default name of at most MaxDeviceNameRunes.
const (
	MaxDeviceNameRunes = 60
	MaxPlatformRunes   = 40
)

// validateDevice returns the trimmed device or an error wrapping domain.ErrInvalid.
func validateDevice(device Device) (Device, error) {
	if !device.Client.IsValid() {
		return Device{}, fmt.Errorf("device client must be web, tv or cli: %w", domain.ErrInvalid)
	}

	name, err := deviceLabel("device name", device.Name, MaxDeviceNameRunes)
	if err != nil {
		return Device{}, err
	}

	platform, err := deviceLabel("device platform", device.Platform, MaxPlatformRunes)
	if err != nil {
		return Device{}, err
	}

	return Device{Name: name, Client: device.Client, Platform: platform}, nil
}

func deviceLabel(field, text string, maxRunes int) (string, error) {
	label := strings.TrimSpace(text)
	if !utf8.ValidString(label) || utf8.RuneCountInString(label) > maxRunes ||
		strings.IndexFunc(label, unicode.IsControl) >= 0 {
		return "", fmt.Errorf("%s must be valid text of at most %d characters: %w", field, maxRunes, domain.ErrInvalid)
	}

	return label, nil
}

// sessionOrigin is what a new session needs besides the account.
type sessionOrigin struct {
	device Device
	peer   Peer
}

// startSession creates a session for credentials, selecting the primary profile when
// allowed, and returns it with the token.
func (service *Service) startSession(ctx context.Context, signedIn account.Account, origin sessionOrigin) (SignInResult, error) {
	profiles, err := service.deps.Profiles.Profiles(ctx, signedIn.ID)
	if err != nil {
		return SignInResult{}, fmt.Errorf("start session: load profiles: %w", err)
	}

	profile, selected := autoSelectProfile(profiles)

	session, token, err := service.newSession(signedIn.ID, profile.ID, origin)
	if err != nil {
		return SignInResult{}, err
	}

	if err := service.deps.Sessions.CreateSession(ctx, session); err != nil {
		return SignInResult{}, fmt.Errorf("start session: store: %w", err)
	}

	return SignInResult{
		Token: token, Session: session, Account: signedIn, Profile: profile, ProfileSelected: selected,
	}, nil
}

func (service *Service) newSession(accountID, profileID string, origin sessionOrigin) (Session, string, error) {
	sessionID, err := service.deps.IDs.NewID()
	if err != nil {
		return Session{}, "", fmt.Errorf("start session: new id: %w", err)
	}

	token, tokenHash, err := service.deps.Tokens.New()
	if err != nil {
		return Session{}, "", fmt.Errorf("start session: new token: %w", err)
	}

	now := service.deps.Clock()

	return Session{
		ID:           sessionID,
		TokenHash:    tokenHash,
		AccountID:    accountID,
		ProfileID:    profileID,
		Remember:     profileID != "",
		Device:       origin.device,
		CreatedAt:    now,
		LastSeenAt:   now,
		AddressClass: origin.peer.Class,
	}, token, nil
}
