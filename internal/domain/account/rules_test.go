package account_test

import (
	"errors"
	"testing"

	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/account"
)

func TestCheckAdminChange(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		target        account.Account
		change        account.AdminChange
		enabledAdmins int
		want          error
	}{
		{name: "delete last admin", target: adminAccount(), change: account.AdminChangeDelete, enabledAdmins: 1, want: domain.ErrConflict},
		{name: "disable last admin", target: adminAccount(), change: account.AdminChangeDisable, enabledAdmins: 1, want: domain.ErrConflict},
		{name: "demote last admin", target: adminAccount(), change: account.AdminChangeDemote, enabledAdmins: 1, want: domain.ErrConflict},
		{name: "inconsistent zero count still protects", target: adminAccount(), change: account.AdminChangeDelete, enabledAdmins: 0, want: domain.ErrConflict},
		{name: "delete one of two admins", target: adminAccount(), change: account.AdminChangeDelete, enabledAdmins: 2},
		{name: "demote one of two admins", target: adminAccount(), change: account.AdminChangeDemote, enabledAdmins: 2},
		{name: "delete disabled admin", target: disabled(adminAccount()), change: account.AdminChangeDelete, enabledAdmins: 1},
		{name: "delete user", target: userAccount(), change: account.AdminChangeDelete, enabledAdmins: 1},
		{name: "disable user", target: userAccount(), change: account.AdminChangeDisable, enabledAdmins: 1},
		{name: "unknown change", target: userAccount(), change: account.AdminChange(99), enabledAdmins: 1, want: domain.ErrInvalid},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			err := account.CheckAdminChange(test.target, test.change, test.enabledAdmins)
			if test.want == nil {
				if err != nil {
					t.Errorf("CheckAdminChange() error = %v, want nil", err)
				}

				return
			}

			if !errors.Is(err, test.want) {
				t.Errorf("CheckAdminChange() error = %v, want %v", err, test.want)
			}
		})
	}
}

func TestCountEnabledAdmins(t *testing.T) {
	t.Parallel()

	second := adminAccount()
	second.ID = "second"

	accounts := []account.Account{adminAccount(), second, disabled(adminAccount()), userAccount()}
	if got := account.CountEnabledAdmins(accounts); got != 2 {
		t.Errorf("CountEnabledAdmins() = %d, want 2", got)
	}

	if got := account.CountEnabledAdmins(nil); got != 0 {
		t.Errorf("CountEnabledAdmins(nil) = %d, want 0", got)
	}
}

func TestCheckManagedProfileLimit(t *testing.T) {
	t.Parallel()

	tests := []struct {
		existing int
		want     error
	}{
		{existing: 0},
		{existing: account.MaxManagedProfiles - 1},
		{existing: account.MaxManagedProfiles, want: domain.ErrConflict},
		{existing: account.MaxManagedProfiles + 1, want: domain.ErrConflict},
	}

	for _, test := range tests {
		err := account.CheckManagedProfileLimit(test.existing)
		if (test.want == nil && err != nil) || (test.want != nil && !errors.Is(err, test.want)) {
			t.Errorf("CheckManagedProfileLimit(%d) error = %v, want %v", test.existing, err, test.want)
		}
	}

	if account.MaxManagedProfiles != 10 {
		t.Errorf("MaxManagedProfiles = %d, want 10 (ADR 0032 §1)", account.MaxManagedProfiles)
	}
}

func TestCheckProfileDeletion(t *testing.T) {
	t.Parallel()

	if err := account.CheckProfileDeletion(primaryOf(userAccount())); !errors.Is(err, domain.ErrConflict) {
		t.Errorf("CheckProfileDeletion(primary) error = %v, want ErrConflict", err)
	}

	if err := account.CheckProfileDeletion(managedOf(userAccount())); err != nil {
		t.Errorf("CheckProfileDeletion(managed) error = %v, want nil", err)
	}
}

func TestIsAdminViewer(t *testing.T) {
	t.Parallel()

	otherPrimary := primaryOf(userAccount())

	tests := []struct {
		name    string
		account account.Account
		profile account.Profile
		want    bool
	}{
		{name: "primary profile of an enabled admin", account: adminAccount(), profile: primaryOf(adminAccount()), want: true},
		{name: "managed profile of an admin", account: adminAccount(), profile: managedOf(adminAccount())},
		{name: "primary profile of a disabled admin", account: disabled(adminAccount()), profile: primaryOf(adminAccount())},
		{name: "primary profile of a user", account: userAccount(), profile: primaryOf(userAccount())},
		{name: "primary profile of another account", account: adminAccount(), profile: otherPrimary},
		{name: "admin without id fails closed", account: account.Account{Role: account.RoleAdmin}, profile: account.Profile{Primary: true}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			if got := account.IsAdminViewer(test.account, test.profile); got != test.want {
				t.Errorf("IsAdminViewer() = %v, want %v", got, test.want)
			}
		})
	}
}

func TestCanManageProfile(t *testing.T) {
	t.Parallel()

	admin, user := adminAccount(), userAccount()

	tests := []struct {
		name   string
		actor  account.Actor
		target account.Profile
		want   bool
	}{
		{name: "admin manages another account's profile", actor: account.Actor{Account: admin, Profile: primaryOf(admin)}, target: managedOf(user), want: true},
		{name: "owner manages own managed profile", actor: account.Actor{Account: user, Profile: primaryOf(user)}, target: managedOf(user), want: true},
		{name: "owner manages own primary profile", actor: account.Actor{Account: user, Profile: primaryOf(user)}, target: primaryOf(user), want: true},
		{name: "user cannot manage another account", actor: account.Actor{Account: user, Profile: primaryOf(user)}, target: managedOf(admin)},
		{name: "managed profile cannot manage siblings", actor: account.Actor{Account: user, Profile: managedOf(user)}, target: managedOf(user)},
		{name: "admin on a managed profile is no admin", actor: account.Actor{Account: admin, Profile: managedOf(admin)}, target: managedOf(user)},
		{name: "disabled owner", actor: account.Actor{Account: disabled(user), Profile: primaryOf(user)}, target: managedOf(user)},
		{name: "disabled admin on primary manages another account", actor: account.Actor{Account: disabled(admin), Profile: primaryOf(admin)}, target: managedOf(user)},
		{name: "zero-value actor and target", actor: account.Actor{Account: account.Account{Role: account.RoleAdmin}, Profile: account.Profile{Primary: true}}, target: account.Profile{}},
		{name: "profile of another account selected", actor: account.Actor{Account: user, Profile: primaryOf(admin)}, target: managedOf(user)},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			if got := account.CanManageProfile(test.actor, test.target); got != test.want {
				t.Errorf("CanManageProfile() = %v, want %v", got, test.want)
			}
		})
	}
}
