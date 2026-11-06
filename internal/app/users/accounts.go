package users

import (
	"context"
	"fmt"

	"github.com/magpeek/mivo/internal/domain/access"
	"github.com/magpeek/mivo/internal/domain/account"
)

// CreateAccountInput is a new account as the admin enters it. Libraries is ignored for admins,
// who always see every library.
type CreateAccountInput struct {
	Name         string
	Password     string
	Role         account.Role
	Libraries    access.LibraryAccess
	PrimaryLimit access.ContentLimit
}

// AccountPatch lists the account fields to change; nil fields stay.
type AccountPatch struct {
	Name         *string
	Role         *account.Role
	Disabled     *bool
	Libraries    *access.LibraryAccess
	PrimaryLimit *access.ContentLimit
}

// affectsSessions reports whether signed-in sessions must re-resolve the account.
func (patch AccountPatch) affectsSessions() bool {
	return patch.Role != nil || patch.Disabled != nil || patch.Libraries != nil || patch.PrimaryLimit != nil
}

// ListAccounts returns every account for the admin's users list.
func (s *Service) ListAccounts(ctx context.Context) ([]AccountSummary, error) {
	if err := s.requireAdmin(ctx); err != nil {
		return nil, err
	}

	summaries, err := s.options.Accounts.ListAccounts(ctx)
	if err != nil {
		return nil, fmt.Errorf("list accounts: %w", err)
	}

	return summaries, nil
}

// GetAccount returns one account for the admin.
func (s *Service) GetAccount(ctx context.Context, accountID string) (AccountRecord, error) {
	if err := s.requireAdmin(ctx); err != nil {
		return AccountRecord{}, err
	}

	record, err := s.options.Accounts.GetAccount(ctx, accountID)
	if err != nil {
		return AccountRecord{}, fmt.Errorf("get account %s: %w", accountID, err)
	}

	return record, nil
}

// CreateAccount creates an account with its primary profile and initial password.
func (s *Service) CreateAccount(ctx context.Context, input CreateAccountInput) (AccountRecord, error) {
	if err := s.requireAdmin(ctx); err != nil {
		return AccountRecord{}, err
	}

	record, err := s.newAccountRecord(ctx, input)
	if err != nil {
		return AccountRecord{}, err
	}

	if err := s.options.Writer.CreateAccount(ctx, record); err != nil {
		return AccountRecord{}, fmt.Errorf("create account %q: %w", record.Account.Account.Name, err)
	}

	return record.Account, nil
}

// newAccountRecord validates the input and builds the account, primary profile and hash.
func (s *Service) newAccountRecord(ctx context.Context, input CreateAccountInput) (NewAccountRecord, error) {
	name, err := s.normalizeName(input.Name)
	if err != nil {
		return NewAccountRecord{}, err
	}

	if err := account.ValidatePassword(input.Password); err != nil {
		return NewAccountRecord{}, fmt.Errorf("create account: %w", err)
	}

	created, err := s.newIdentity(name, input.Role)
	if err != nil {
		return NewAccountRecord{}, err
	}

	primary, err := s.newPrimaryProfile(created)
	if err != nil {
		return NewAccountRecord{}, err
	}

	passwordHash, err := s.hash(ctx, input.Password)
	if err != nil {
		return NewAccountRecord{}, err
	}

	record := AccountRecord{
		Account:      created,
		Libraries:    librariesForRole(created.Role, input.Libraries),
		PrimaryLimit: input.PrimaryLimit,
	}

	return NewAccountRecord{Account: record, Primary: primary, PasswordHash: passwordHash}, nil
}

// newIdentity assigns an id and validates the account fields.
func (s *Service) newIdentity(name string, role account.Role) (account.Account, error) {
	accountID, err := s.newID()
	if err != nil {
		return account.Account{}, err
	}

	created, err := account.NewAccount(account.Account{ID: accountID, Name: name, Role: role})
	if err != nil {
		return account.Account{}, fmt.Errorf("create account: %w", err)
	}

	return created, nil
}

// newPrimaryProfile builds the primary profile, which carries the account name.
func (s *Service) newPrimaryProfile(owner account.Account) (account.Profile, error) {
	profileID, err := s.newID()
	if err != nil {
		return account.Profile{}, err
	}

	primary, err := account.NewProfile(account.Profile{
		ID: profileID, AccountID: owner.ID, Name: owner.Name, Primary: true,
		AvatarColor: account.DefaultAvatarColor,
	})
	if err != nil {
		return account.Profile{}, fmt.Errorf("create primary profile: %w", err)
	}

	return primary, nil
}

// UpdateAccount changes an account; the last-admin rule runs inside the store transaction.
func (s *Service) UpdateAccount(ctx context.Context, accountID string, patch AccountPatch) (AccountRecord, error) {
	if err := s.requireAdmin(ctx); err != nil {
		return AccountRecord{}, err
	}

	if patch.Name != nil {
		name, err := s.normalizeName(*patch.Name)
		if err != nil {
			return AccountRecord{}, err
		}

		patch.Name = &name
	}

	updated, err := s.options.Writer.UpdateAccount(ctx, accountID,
		func(current AccountRecord, enabledAdmins int) (AccountRecord, error) {
			return applyAccountPatch(current, patch, enabledAdmins)
		})
	if err != nil {
		return AccountRecord{}, fmt.Errorf("update account %s: %w", accountID, err)
	}

	if patch.affectsSessions() {
		s.options.Cache.EvictAccount(accountID)
	}

	return updated, nil
}

// applyAccountPatch returns the patched account after the domain rules allowed the change.
func applyAccountPatch(current AccountRecord, patch AccountPatch, enabledAdmins int) (AccountRecord, error) {
	next := current

	if patch.Name != nil {
		next.Account.Name = *patch.Name
	}

	if patch.Role != nil {
		next.Account.Role = *patch.Role
	}

	if patch.Disabled != nil {
		next.Account.Disabled = *patch.Disabled
	}

	if patch.Libraries != nil {
		next.Libraries = *patch.Libraries
	}

	if patch.PrimaryLimit != nil {
		next.PrimaryLimit = *patch.PrimaryLimit
	}

	validated, err := account.NewAccount(next.Account)
	if err != nil {
		return AccountRecord{}, fmt.Errorf("validate account: %w", err)
	}

	if err := checkAdminLoss(current.Account, validated, enabledAdmins); err != nil {
		return AccountRecord{}, err
	}

	next.Account = validated
	next.Libraries = librariesForRole(validated.Role, next.Libraries)

	return next, nil
}

// checkAdminLoss applies the last-admin rule when an enabled admin is demoted or disabled.
func checkAdminLoss(current, next account.Account, enabledAdmins int) error {
	if !current.IsEnabledAdmin() || next.IsEnabledAdmin() {
		return nil
	}

	change := account.AdminChangeDisable
	if next.Role != account.RoleAdmin {
		change = account.AdminChangeDemote
	}

	if err := account.CheckAdminChange(current, change, enabledAdmins); err != nil {
		return fmt.Errorf("change account: %w", err)
	}

	return nil
}

// DeleteAccount removes an account unless it is the last enabled admin.
func (s *Service) DeleteAccount(ctx context.Context, accountID string) error {
	if err := s.requireAdmin(ctx); err != nil {
		return err
	}

	err := s.options.Writer.DeleteAccount(ctx, accountID,
		func(current account.Account, enabledAdmins int) error {
			return account.CheckAdminChange(current, account.AdminChangeDelete, enabledAdmins)
		})
	if err != nil {
		return fmt.Errorf("delete account %s: %w", accountID, err)
	}

	s.options.Cache.EvictAccount(accountID)

	return nil
}

// ResetPassword sets a new password chosen by the admin and signs the account out everywhere.
func (s *Service) ResetPassword(ctx context.Context, accountID, password string) error {
	if err := s.requireAdmin(ctx); err != nil {
		return err
	}

	if _, err := s.options.Accounts.GetAccount(ctx, accountID); err != nil {
		return fmt.Errorf("reset password of %s: %w", accountID, err)
	}

	return s.setPassword(ctx, accountID, password)
}

// setPassword validates and stores a password, then revokes every session of the account.
func (s *Service) setPassword(ctx context.Context, accountID, password string) error {
	if err := account.ValidatePassword(password); err != nil {
		return fmt.Errorf("reset password: %w", err)
	}

	passwordHash, err := s.hash(ctx, password)
	if err != nil {
		return err
	}

	if err := s.options.Password.SetPasswordHash(ctx, accountID, passwordHash); err != nil {
		return fmt.Errorf("store password of %s: %w", accountID, err)
	}

	if err := s.options.Sessions.RevokeAccountSessions(ctx, accountID); err != nil {
		return fmt.Errorf("revoke sessions of %s: %w", accountID, err)
	}

	s.options.Cache.EvictAccount(accountID)

	return nil
}

// librariesForRole gives admins every library (ADR 0032 §7) and keeps the choice otherwise.
func librariesForRole(role account.Role, libraries access.LibraryAccess) access.LibraryAccess {
	if role == account.RoleAdmin {
		return access.AllLibraries()
	}

	return libraries
}
