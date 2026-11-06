package account_test

import (
	"errors"
	"testing"

	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/account"
)

const (
	adminAccountID = "0b6f5c2e-6d3a-4f43-9a51-3c1f0e2d7a10"
	userAccountID  = "7d1e4a90-2b8c-4e6f-8a3d-5f9c0b1e2a34"
	primaryID      = "a3c9e1f2-4b5d-4c6e-8f70-1a2b3c4d5e6f"
	managedID      = "b4d0f2a3-5c6e-4d7f-9081-2b3c4d5e6f70"
)

func TestParseRole(t *testing.T) {
	t.Parallel()

	tests := []struct {
		text    string
		want    account.Role
		wantErr bool
	}{
		{text: "admin", want: account.RoleAdmin},
		{text: "user", want: account.RoleUser},
		{text: "Admin", wantErr: true},
		{text: "", wantErr: true},
		{text: "guest", wantErr: true},
	}

	for _, test := range tests {
		t.Run(test.text, func(t *testing.T) {
			t.Parallel()

			got, err := account.ParseRole(test.text)
			if test.wantErr {
				assertInvalidField(t, err, account.FieldRole)

				return
			}

			if err != nil || got != test.want {
				t.Errorf("ParseRole(%q) = %q, %v, want %q", test.text, got, err, test.want)
			}
		})
	}
}

func TestRoles(t *testing.T) {
	t.Parallel()

	for _, role := range account.Roles() {
		if !role.IsValid() {
			t.Errorf("Roles() contains invalid role %q", role)
		}
	}
}

func TestNewAccount(t *testing.T) {
	t.Parallel()

	valid := account.Account{ID: adminAccountID, Name: "Anna", Role: account.RoleAdmin}

	tests := []struct {
		name      string
		edit      func(*account.Account)
		wantField string
	}{
		{name: "valid admin", edit: func(*account.Account) {}},
		{name: "valid disabled user", edit: func(params *account.Account) {
			params.Role, params.Disabled = account.RoleUser, true
		}},
		{name: "missing id", edit: func(params *account.Account) { params.ID = "" }, wantField: account.FieldID},
		{name: "empty name", edit: func(params *account.Account) { params.Name = "" }, wantField: account.FieldName},
		{name: "name not trimmed", edit: func(params *account.Account) { params.Name = " Anna" }, wantField: account.FieldName},
		{name: "name with double space", edit: func(params *account.Account) { params.Name = "Anna  Lena" }, wantField: account.FieldName},
		{name: "unknown role", edit: func(params *account.Account) { params.Role = "owner" }, wantField: account.FieldRole},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			params := valid
			test.edit(&params)

			got, err := account.NewAccount(params)
			if test.wantField != "" {
				assertInvalidField(t, err, test.wantField)

				return
			}

			if err != nil {
				t.Fatalf("NewAccount(%+v) error = %v", params, err)
			}

			if got != params {
				t.Errorf("NewAccount(%+v) = %+v, want it unchanged", params, got)
			}
		})
	}
}

func TestAccountIsEnabledAdmin(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		account account.Account
		want    bool
	}{
		{name: "enabled admin", account: adminAccount(), want: true},
		{name: "disabled admin", account: disabled(adminAccount()), want: false},
		{name: "enabled user", account: userAccount(), want: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			if got := test.account.IsEnabledAdmin(); got != test.want {
				t.Errorf("IsEnabledAdmin() = %v, want %v", got, test.want)
			}
		})
	}
}

func TestParseAvatarColor(t *testing.T) {
	t.Parallel()

	for _, color := range account.AvatarColors() {
		got, err := account.ParseAvatarColor(string(color))
		if err != nil || got != color {
			t.Errorf("ParseAvatarColor(%q) = %q, %v, want %q", color, got, err, color)
		}
	}

	for _, text := range []string{"", "#ff8572", "Coral", "red"} {
		_, err := account.ParseAvatarColor(text)
		assertInvalidField(t, err, account.FieldAvatarColor)
	}

	if !account.DefaultAvatarColor.IsValid() {
		t.Errorf("DefaultAvatarColor %q is not a known token", account.DefaultAvatarColor)
	}
}

func TestNewProfile(t *testing.T) {
	t.Parallel()

	valid := account.Profile{
		ID: managedID, AccountID: userAccountID, Name: "Max", AvatarColor: account.AvatarColorBlue,
	}

	tests := []struct {
		name      string
		edit      func(*account.Profile)
		wantField string
	}{
		{name: "valid managed", edit: func(*account.Profile) {}},
		{name: "valid primary with PIN", edit: func(params *account.Profile) {
			params.Primary, params.HasPIN = true, true
		}},
		{name: "missing id", edit: func(params *account.Profile) { params.ID = "" }, wantField: account.FieldID},
		{name: "missing account id", edit: func(params *account.Profile) { params.AccountID = "" }, wantField: account.FieldAccountID},
		{name: "name too long", edit: func(params *account.Profile) {
			params.Name = "Maximilian Alexander von und zu Somewhere-Else"
		}, wantField: account.FieldName},
		{name: "name with control character", edit: func(params *account.Profile) { params.Name = "Ma\tx" }, wantField: account.FieldName},
		{name: "unknown avatar colour", edit: func(params *account.Profile) { params.AvatarColor = "#4c9aff" }, wantField: account.FieldAvatarColor},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			params := valid
			test.edit(&params)

			got, err := account.NewProfile(params)
			if test.wantField != "" {
				assertInvalidField(t, err, test.wantField)

				return
			}

			if err != nil {
				t.Fatalf("NewProfile(%+v) error = %v", params, err)
			}

			if got != params {
				t.Errorf("NewProfile(%+v) = %+v, want it unchanged", params, got)
			}
		})
	}
}

func TestInvalidFieldErrorMessage(t *testing.T) {
	t.Parallel()

	err := account.ValidatePIN("12")

	want := "pin: must be exactly 4 ASCII digits: invalid"
	if err == nil || err.Error() != want {
		t.Errorf("error = %v, want %q", err, want)
	}

	if errors.Is(err, domain.ErrConflict) {
		t.Errorf("errors.Is(%v, ErrConflict) = true, want false", err)
	}
}

func adminAccount() account.Account {
	return account.Account{ID: adminAccountID, Name: "Anna", Role: account.RoleAdmin}
}

func userAccount() account.Account {
	return account.Account{ID: userAccountID, Name: "Max", Role: account.RoleUser}
}

func disabled(subject account.Account) account.Account {
	subject.Disabled = true

	return subject
}

func primaryOf(owner account.Account) account.Profile {
	return account.Profile{
		ID: primaryID, AccountID: owner.ID, Name: owner.Name, Primary: true,
		AvatarColor: account.DefaultAvatarColor,
	}
}

func managedOf(owner account.Account) account.Profile {
	return account.Profile{
		ID: managedID, AccountID: owner.ID, Name: "Lena", AvatarColor: account.AvatarColorGreen,
	}
}
