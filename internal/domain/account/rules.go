package account

import (
	"fmt"

	"github.com/magpeek/mivo/internal/domain"
)

// MaxManagedProfiles is the number of managed profiles an account may have besides its
// primary profile (ADR 0032 §1).
const MaxManagedProfiles = 10

// AdminChange is an account change that can take admin power away.
type AdminChange int

// Changes checked by CheckAdminChange.
const (
	AdminChangeDelete AdminChange = iota + 1
	AdminChangeDisable
	AdminChangeDemote
)

// Actor is the signed-in account together with its selected profile.
type Actor struct {
	Account Account
	Profile Profile
}

// CheckAdminChange refuses to delete, disable or demote the last enabled admin, so the server
// can always be administered (ADR 0032 §1). enabledAdmins is the current number of enabled
// admin accounts including target. The error wraps ErrLastAdmin, or domain.ErrInvalid
// for an unknown change.
func CheckAdminChange(target Account, change AdminChange, enabledAdmins int) error {
	switch change {
	case AdminChangeDelete, AdminChangeDisable, AdminChangeDemote:
	default:
		return fmt.Errorf("check admin change %d: unknown change: %w", change, domain.ErrInvalid)
	}

	// A count below one contradicts an enabled admin target; refusing keeps the invariant
	// even when the caller's count is stale.
	if target.IsEnabledAdmin() && enabledAdmins <= 1 {
		return fmt.Errorf("account %s is the last enabled admin: %w", target.ID, ErrLastAdmin)
	}

	return nil
}

// CountEnabledAdmins returns how many of accounts are enabled admins.
func CountEnabledAdmins(accounts []Account) int {
	count := 0

	for _, candidate := range accounts {
		if candidate.IsEnabledAdmin() {
			count++
		}
	}

	return count
}

// CheckManagedProfileLimit refuses a new managed profile when the account already has
// existingManaged of them. The error wraps ErrProfileLimit.
func CheckManagedProfileLimit(existingManaged int) error {
	if existingManaged >= MaxManagedProfiles {
		return fmt.Errorf("account has %d managed profiles, at most %d allowed: %w",
			existingManaged, MaxManagedProfiles, ErrProfileLimit)
	}

	return nil
}

// CheckProfileDeletion refuses to delete a primary profile on its own; it goes only with its
// account. The error wraps domain.ErrConflict.
func CheckProfileDeletion(profile Profile) error {
	if profile.Primary {
		return fmt.Errorf("profile %s is the primary profile of its account: %w", profile.ID, domain.ErrConflict)
	}

	return nil
}

// IsAdminViewer reports whether the viewer has admin powers: the selected profile must be the
// primary profile of an enabled admin account. A managed profile of an admin account is never
// admin (ADR 0032 §1).
func IsAdminViewer(account Account, profile Profile) bool {
	return account.IsEnabledAdmin() && isOwnPrimary(account, profile)
}

// CanManageProfile reports whether actor may create, change or delete target: an admin viewer
// may manage every profile, an enabled account owner the profiles of its own account. Owner
// rights need the primary profile selected, so a managed profile (for example a child's)
// cannot lift its own PIN or content limit.
func CanManageProfile(actor Actor, target Profile) bool {
	if IsAdminViewer(actor.Account, actor.Profile) {
		return true
	}

	return !actor.Account.Disabled &&
		isOwnPrimary(actor.Account, actor.Profile) &&
		target.AccountID == actor.Account.ID
}

// isOwnPrimary fails closed for a zero-value account: an empty ID would otherwise match a
// profile whose account ID is empty as well.
func isOwnPrimary(account Account, profile Profile) bool {
	return account.ID != "" && profile.Primary && profile.AccountID == account.ID
}
