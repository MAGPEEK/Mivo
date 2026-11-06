package users

import (
	"context"
	"fmt"

	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/access"
	"github.com/magpeek/mivo/internal/domain/account"
)

// CreateProfileInput is a new managed profile. An empty AvatarColor picks the default, an
// empty PIN creates the profile without one. Libraries all means the account's set.
type CreateProfileInput struct {
	Name        string
	AvatarColor account.AvatarColor
	PIN         string
	Libraries   access.LibraryAccess
	Limit       access.ContentLimit
}

// ProfilePatch lists the profile fields to change; nil fields stay. SetPIN replaces the PIN,
// ClearPIN removes it; both at once are invalid. The primary profile takes its library access
// from the account, and only an admin may change its limit, so an owner cannot lift the limit
// the admin set (ADR 0032 §7).
type ProfilePatch struct {
	Name        *string
	AvatarColor *account.AvatarColor
	SetPIN      *string
	ClearPIN    bool
	Libraries   *access.LibraryAccess
	Limit       *access.ContentLimit
}

// affectsSessions reports whether sessions with the profile must re-resolve it.
func (patch ProfilePatch) affectsSessions() bool {
	return patch.SetPIN != nil || patch.ClearPIN || patch.Libraries != nil || patch.Limit != nil
}

// ListProfiles returns the profiles of an account to an admin or the account owner.
func (s *Service) ListProfiles(ctx context.Context, accountID string) ([]ProfileRecord, error) {
	if _, err := s.authorizedAccount(ctx, accountID); err != nil {
		return nil, err
	}

	profiles, err := s.options.Profiles.ListProfiles(ctx, accountID)
	if err != nil {
		return nil, fmt.Errorf("list profiles of %s: %w", accountID, err)
	}

	return profiles, nil
}

// CreateProfile adds a managed profile; the cap of managed profiles runs inside the store
// transaction.
func (s *Service) CreateProfile(ctx context.Context, accountID string, input CreateProfileInput) (ProfileRecord, error) {
	owner, err := s.authorizedAccount(ctx, accountID)
	if err != nil {
		return ProfileRecord{}, err
	}

	write, err := s.newProfileWrite(ctx, owner, input)
	if err != nil {
		return ProfileRecord{}, err
	}

	if err := s.options.Profile.CreateProfile(ctx, write, account.CheckManagedProfileLimit); err != nil {
		return ProfileRecord{}, fmt.Errorf("create profile for %s: %w", accountID, err)
	}

	return write.Record, nil
}

// authorizedAccount loads the account whose profiles the viewer wants to manage.
func (s *Service) authorizedAccount(ctx context.Context, accountID string) (AccountRecord, error) {
	actor, err := s.actor(ctx)
	if err != nil {
		return AccountRecord{}, err
	}

	if !account.CanManageProfile(actor, account.Profile{AccountID: accountID}) {
		return AccountRecord{}, forbiddenProfiles(actor, accountID)
	}

	owner, err := s.options.Accounts.GetAccount(ctx, accountID)
	if err != nil {
		return AccountRecord{}, fmt.Errorf("load account %s: %w", accountID, err)
	}

	return owner, nil
}

// newProfileWrite validates the input and builds the managed profile to store.
func (s *Service) newProfileWrite(ctx context.Context, owner AccountRecord, input CreateProfileInput) (ProfileWrite, error) {
	name, err := s.normalizeName(input.Name)
	if err != nil {
		return ProfileWrite{}, err
	}

	if err := checkLibrarySubset(owner.Libraries, input.Libraries); err != nil {
		return ProfileWrite{}, err
	}

	profileID, err := s.newID()
	if err != nil {
		return ProfileWrite{}, err
	}

	color := input.AvatarColor
	if color == "" {
		color = account.DefaultAvatarColor
	}

	write := ProfileWrite{Record: ProfileRecord{
		Profile: account.Profile{
			ID: profileID, AccountID: owner.Account.ID, Name: name, AvatarColor: color,
		},
		Libraries: input.Libraries,
		Limit:     input.Limit,
	}}

	if input.PIN != "" {
		if write, err = s.withNewPIN(ctx, write, input.PIN); err != nil {
			return ProfileWrite{}, err
		}
	}

	return validatedWrite(write)
}

// UpdateProfile changes a profile for an admin or the owning account.
func (s *Service) UpdateProfile(ctx context.Context, profileID string, patch ProfilePatch) (ProfileRecord, error) {
	actor, current, err := s.manageableProfile(ctx, profileID)
	if err != nil {
		return ProfileRecord{}, err
	}

	owner, err := s.options.Accounts.GetAccount(ctx, current.Profile.AccountID)
	if err != nil {
		return ProfileRecord{}, fmt.Errorf("load account %s: %w", current.Profile.AccountID, err)
	}

	write, err := s.applyProfilePatch(ctx, profileChange{actor: actor, owner: owner, current: current}, patch)
	if err != nil {
		return ProfileRecord{}, err
	}

	if err := s.options.Profile.UpdateProfile(ctx, write); err != nil {
		return ProfileRecord{}, fmt.Errorf("update profile %s: %w", profileID, err)
	}

	if patch.affectsSessions() {
		s.options.Cache.EvictAccount(owner.Account.ID)
	}

	return write.Record, nil
}

// profileChange is what a profile patch is checked against.
type profileChange struct {
	actor   account.Actor
	owner   AccountRecord
	current ProfileRecord
}

// applyProfilePatch checks the patch against the profile rules and builds the write.
func (s *Service) applyProfilePatch(ctx context.Context, change profileChange, patch ProfilePatch) (ProfileWrite, error) {
	if err := checkPrimaryPatch(change, patch); err != nil {
		return ProfileWrite{}, err
	}

	write := ProfileWrite{Record: change.current}

	if err := s.applyProfileFields(&write.Record, change.owner, patch); err != nil {
		return ProfileWrite{}, err
	}

	write, err := s.applyPINPatch(ctx, write, patch)
	if err != nil {
		return ProfileWrite{}, err
	}

	return validatedWrite(write)
}

// checkPrimaryPatch refuses library changes on a primary profile and limit changes on it by
// anyone but an admin.
func checkPrimaryPatch(change profileChange, patch ProfilePatch) error {
	if !change.current.Profile.Primary {
		return nil
	}

	if patch.Libraries != nil {
		return fmt.Errorf("profile %s is primary and uses the account's library access: %w",
			change.current.Profile.ID, domain.ErrInvalid)
	}

	if patch.Limit != nil && !account.IsAdminViewer(change.actor.Account, change.actor.Profile) {
		return fmt.Errorf("only an admin may change the limit of primary profile %s: %w",
			change.current.Profile.ID, domain.ErrForbidden)
	}

	return nil
}

// applyProfileFields copies name, colour, library access and limit from the patch.
func (s *Service) applyProfileFields(record *ProfileRecord, owner AccountRecord, patch ProfilePatch) error {
	if patch.Name != nil {
		name, err := s.normalizeName(*patch.Name)
		if err != nil {
			return err
		}

		record.Profile.Name = name
	}

	if patch.AvatarColor != nil {
		record.Profile.AvatarColor = *patch.AvatarColor
	}

	if patch.Libraries != nil {
		if err := checkLibrarySubset(owner.Libraries, *patch.Libraries); err != nil {
			return err
		}

		record.Libraries = *patch.Libraries
	}

	if patch.Limit != nil {
		record.Limit = *patch.Limit
	}

	return nil
}

// applyPINPatch turns SetPIN and ClearPIN into the write's PIN action.
func (s *Service) applyPINPatch(ctx context.Context, write ProfileWrite, patch ProfilePatch) (ProfileWrite, error) {
	switch {
	case patch.SetPIN != nil && patch.ClearPIN:
		return ProfileWrite{}, fmt.Errorf("set and clear the PIN at once: %w", domain.ErrInvalid)
	case patch.SetPIN != nil:
		return s.withNewPIN(ctx, write, *patch.SetPIN)
	case patch.ClearPIN:
		write.PINAction = PINClear
		write.PINHash = ""
		write.Record.Profile.HasPIN = false
	}

	return write, nil
}

// withNewPIN validates and hashes a PIN and marks the write to store it.
func (s *Service) withNewPIN(ctx context.Context, write ProfileWrite, pin string) (ProfileWrite, error) {
	if err := account.ValidatePIN(pin); err != nil {
		return ProfileWrite{}, fmt.Errorf("set PIN: %w", err)
	}

	pinHash, err := s.hash(ctx, pin)
	if err != nil {
		return ProfileWrite{}, err
	}

	write.PINAction = PINSet
	write.PINHash = pinHash
	write.Record.Profile.HasPIN = true

	return write, nil
}

// DeleteProfile removes a managed profile; the primary profile goes only with its account.
func (s *Service) DeleteProfile(ctx context.Context, profileID string) error {
	_, current, err := s.manageableProfile(ctx, profileID)
	if err != nil {
		return err
	}

	if err := account.CheckProfileDeletion(current.Profile); err != nil {
		return fmt.Errorf("delete profile: %w", err)
	}

	if err := s.options.Profile.DeleteProfile(ctx, profileID); err != nil {
		return fmt.Errorf("delete profile %s: %w", profileID, err)
	}

	s.options.Cache.EvictAccount(current.Profile.AccountID)

	return nil
}

// manageableProfile loads a profile the viewer may manage.
func (s *Service) manageableProfile(ctx context.Context, profileID string) (account.Actor, ProfileRecord, error) {
	actor, err := s.actor(ctx)
	if err != nil {
		return account.Actor{}, ProfileRecord{}, err
	}

	current, err := s.options.Profiles.GetProfile(ctx, profileID)
	if err != nil {
		return account.Actor{}, ProfileRecord{}, fmt.Errorf("load profile %s: %w", profileID, err)
	}

	if !account.CanManageProfile(actor, current.Profile) {
		return account.Actor{}, ProfileRecord{}, forbiddenProfiles(actor, current.Profile.AccountID)
	}

	return actor, current, nil
}

// validatedWrite runs the domain profile validation on the record to store.
func validatedWrite(write ProfileWrite) (ProfileWrite, error) {
	validated, err := account.NewProfile(write.Record.Profile)
	if err != nil {
		return ProfileWrite{}, fmt.Errorf("validate profile: %w", err)
	}

	write.Record.Profile = validated

	return write, nil
}

// checkLibrarySubset refuses profile access outside the account's access (ADR 0032 §7).
func checkLibrarySubset(accountLibraries, profileLibraries access.LibraryAccess) error {
	if accountLibraries.IsAll() || profileLibraries.IsAll() {
		return nil
	}

	for _, libraryID := range profileLibraries.IDs() {
		if !accountLibraries.Allows(libraryID) {
			return fmt.Errorf("library %s is outside the account's access: %w", libraryID, domain.ErrInvalid)
		}
	}

	return nil
}

// forbiddenProfiles reports a viewer managing profiles of an account it does not own.
func forbiddenProfiles(actor account.Actor, accountID string) error {
	return fmt.Errorf("account %s may not manage profiles of %s: %w",
		actor.Account.ID, accountID, domain.ErrForbidden)
}
