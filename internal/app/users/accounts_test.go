package users_test

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/magpeek/mivo/internal/app/users"
	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/access"
	"github.com/magpeek/mivo/internal/domain/account"
)

func TestNewRequiresEveryDependency(t *testing.T) {
	t.Parallel()

	complete := fixtureOptions(newFakeStore(), &fakeIDs{}, &fakeSessions{}, &fakeCache{})
	removals := map[string]func(*users.Options){
		"Accounts": func(options *users.Options) { options.Accounts = nil },
		"Writer":   func(options *users.Options) { options.Writer = nil },
		"Password": func(options *users.Options) { options.Password = nil },
		"Profiles": func(options *users.Options) { options.Profiles = nil },
		"Profile":  func(options *users.Options) { options.Profile = nil },
		"Hasher":   func(options *users.Options) { options.Hasher = nil },
		"IDs":      func(options *users.Options) { options.IDs = nil },
		"Sessions": func(options *users.Options) { options.Sessions = nil },
		"Cache":    func(options *users.Options) { options.Cache = nil },
		"Viewers":  func(options *users.Options) { options.Viewers = nil },
		"Compose":  func(options *users.Options) { options.Compose = nil },
	}

	for name, remove := range removals {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			options := complete
			remove(&options)

			if _, err := users.New(options); err == nil {
				t.Fatalf("New() without %s succeeded", name)
			}
		})
	}
}

func TestAdminOperationsRefuseOthers(t *testing.T) {
	t.Parallel()

	operations := map[string]func(*users.Service, context.Context) error{
		"list": func(service *users.Service, ctx context.Context) error {
			_, err := service.ListAccounts(ctx)

			return err
		},
		"get": func(service *users.Service, ctx context.Context) error {
			_, err := service.GetAccount(ctx, otherID)

			return err
		},
		"create": func(service *users.Service, ctx context.Context) error {
			_, err := service.CreateAccount(ctx, users.CreateAccountInput{
				Name: "New", Password: validPassword, Role: account.RoleUser,
			})

			return err
		},
		"update": func(service *users.Service, ctx context.Context) error {
			_, err := service.UpdateAccount(ctx, otherID, users.AccountPatch{Disabled: pointer(true)})

			return err
		},
		"delete": func(service *users.Service, ctx context.Context) error {
			return service.DeleteAccount(ctx, otherID)
		},
		"reset password": func(service *users.Service, ctx context.Context) error {
			return service.ResetPassword(ctx, otherID, validPassword)
		},
	}

	viewers := map[string]struct {
		ctx  context.Context
		want error
	}{
		"no viewer":     {ctx: context.Background(), want: domain.ErrUnauthenticated},
		"account owner": {ctx: ownerContext(), want: domain.ErrForbidden},
	}

	for operationName, operation := range operations {
		for viewerName, viewer := range viewers {
			t.Run(operationName+"/"+viewerName, func(t *testing.T) {
				t.Parallel()

				fixture := newFixture(t)
				wantError(t, operation(fixture.service, viewer.ctx), viewer.want)

				if len(fixture.sessions.revoked) != 0 || len(fixture.cache.evicted) != 0 {
					t.Fatalf("refused call revoked %v, evicted %v", fixture.sessions.revoked, fixture.cache.evicted)
				}
			})
		}
	}
}

func TestListAndGetAccounts(t *testing.T) {
	t.Parallel()

	fixture := newFixture(t)

	summaries, err := fixture.service.ListAccounts(adminContext())
	wantNoError(t, err)

	if len(summaries) != 3 || summaries[2].Account.ID != ownerID || summaries[2].ProfileCount != 2 {
		t.Fatalf("ListAccounts() = %+v", summaries)
	}

	record, err := fixture.service.GetAccount(adminContext(), ownerID)
	wantNoError(t, err)

	if record.Account.Name != "Owner" {
		t.Fatalf("GetAccount() = %+v", record)
	}

	_, err = fixture.service.GetAccount(adminContext(), "missing")
	wantError(t, err, domain.ErrNotFound)

	fixture.store.failList = true
	_, err = fixture.service.ListAccounts(adminContext())
	wantError(t, err, errFake)
}

func TestCreateAccount(t *testing.T) {
	t.Parallel()

	fixture := newFixture(t)
	limit, err := access.MaxAgeLimit(access.Age12, true)
	wantNoError(t, err)

	created, err := fixture.service.CreateAccount(adminContext(), users.CreateAccountInput{
		Name: "  Ren" + decomposedAccent + "e  ", Password: validPassword, Role: account.RoleUser,
		Libraries: access.OnlyLibraries(libraryMovies), PrimaryLimit: limit,
	})
	wantNoError(t, err)

	if created.Account.Name != "Ren"+composedAccent+"e" || created.Account.Role != account.RoleUser {
		t.Fatalf("CreateAccount() = %+v", created.Account)
	}

	if !created.Libraries.Allows(libraryMovies) || created.Libraries.Allows(libraryShows) {
		t.Fatalf("libraries = %v", created.Libraries.IDs())
	}

	if fixture.store.passwords[created.Account.ID] != fakeHashPrefix+validPassword {
		t.Fatalf("stored hash = %q", fixture.store.passwords[created.Account.ID])
	}

	profiles, err := fixture.service.ListProfiles(adminContext(), created.Account.ID)
	wantNoError(t, err)

	if len(profiles) != 1 || !profiles[0].Profile.Primary || profiles[0].Profile.Name != created.Account.Name ||
		profiles[0].Limit != limit {
		t.Fatalf("primary profile = %+v", profiles)
	}
}

func TestCreateAdminGetsEveryLibrary(t *testing.T) {
	t.Parallel()

	fixture := newFixture(t)

	created, err := fixture.service.CreateAccount(adminContext(), users.CreateAccountInput{
		Name: "Second Admin", Password: validPassword, Role: account.RoleAdmin,
		Libraries: access.OnlyLibraries(libraryMovies),
	})
	wantNoError(t, err)

	if !created.Libraries.IsAll() {
		t.Fatalf("admin libraries = %v, want all", created.Libraries.IDs())
	}
}

func TestCreateAccountRefusesInvalidInput(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		input users.CreateAccountInput
		fail  func(*fixture)
		want  error
	}{
		"duplicate name": {
			input: users.CreateAccountInput{Name: "OWNER", Password: validPassword, Role: account.RoleUser},
			want:  domain.ErrConflict,
		},
		"empty name": {
			input: users.CreateAccountInput{Name: "   ", Password: validPassword, Role: account.RoleUser},
			want:  domain.ErrInvalid,
		},
		"short password": {
			input: users.CreateAccountInput{Name: "New", Password: "short", Role: account.RoleUser},
			want:  domain.ErrInvalid,
		},
		"unknown role": {
			input: users.CreateAccountInput{Name: "New", Password: validPassword, Role: "guest"},
			want:  domain.ErrInvalid,
		},
		"id failure": {
			input: users.CreateAccountInput{Name: "New", Password: validPassword, Role: account.RoleUser},
			fail:  func(fixture *fixture) { fixture.ids.fail = true },
			want:  errFake,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			fixture := newFixture(t)
			if test.fail != nil {
				test.fail(fixture)
			}

			_, err := fixture.service.CreateAccount(adminContext(), test.input)
			wantError(t, err, test.want)

			if len(fixture.store.accounts) != 3 {
				t.Fatalf("accounts = %d, want 3", len(fixture.store.accounts))
			}
		})
	}
}

func TestCreateAccountHashFailure(t *testing.T) {
	t.Parallel()

	store := newFakeStore()
	store.addAccount(users.AccountRecord{
		Account: account.Account{ID: adminID, Name: "Admin", Role: account.RoleAdmin}, Libraries: access.AllLibraries(),
	}, adminPrimaryID)

	options := fixtureOptions(store, &fakeIDs{}, &fakeSessions{}, &fakeCache{})
	options.Hasher = fakeHasher{fail: true}

	service, err := users.New(options)
	wantNoError(t, err)

	_, err = service.CreateAccount(adminContext(), users.CreateAccountInput{
		Name: "New", Password: validPassword, Role: account.RoleUser,
	})
	wantError(t, err, errFake)
}

func TestUpdateAccountFields(t *testing.T) {
	t.Parallel()

	fixture := newFixture(t)
	limit, err := access.MaxAgeLimit(access.Age6, true)
	wantNoError(t, err)

	updated, err := fixture.service.UpdateAccount(adminContext(), ownerID, users.AccountPatch{
		Name: pointer("Owner" + decomposedAccent), Libraries: pointer(access.OnlyLibraries(libraryMusic)),
		PrimaryLimit: pointer(limit), Disabled: pointer(true),
	})
	wantNoError(t, err)

	if updated.Account.Name != "Owner"+composedAccent || !updated.Account.Disabled ||
		!updated.Libraries.Allows(libraryMusic) || updated.PrimaryLimit != limit {
		t.Fatalf("UpdateAccount() = %+v", updated)
	}

	if !slices.Equal(fixture.cache.evicted, []string{ownerID}) {
		t.Fatalf("evicted = %v, want [%s]", fixture.cache.evicted, ownerID)
	}
}

func TestUpdateAccountRenameKeepsCache(t *testing.T) {
	t.Parallel()

	fixture := newFixture(t)

	_, err := fixture.service.UpdateAccount(adminContext(), ownerID, users.AccountPatch{Name: pointer("Renamed")})
	wantNoError(t, err)

	if len(fixture.cache.evicted) != 0 {
		t.Fatalf("rename evicted %v", fixture.cache.evicted)
	}
}

func TestUpdateAccountRefusals(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		accountID string
		patch     users.AccountPatch
		want      error
	}{
		"duplicate name":     {accountID: ownerID, patch: users.AccountPatch{Name: pointer("other")}, want: domain.ErrConflict},
		"invalid name":       {accountID: ownerID, patch: users.AccountPatch{Name: pointer(string(rune(zeroWidthSpace)))}, want: domain.ErrInvalid},
		"unknown role":       {accountID: ownerID, patch: users.AccountPatch{Role: pointer(account.Role("guest"))}, want: domain.ErrInvalid},
		"demote last admin":  {accountID: adminID, patch: users.AccountPatch{Role: pointer(account.RoleUser)}, want: domain.ErrConflict},
		"disable last admin": {accountID: adminID, patch: users.AccountPatch{Disabled: pointer(true)}, want: domain.ErrConflict},
		"missing account":    {accountID: "missing", patch: users.AccountPatch{Disabled: pointer(true)}, want: domain.ErrNotFound},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			fixture := newFixture(t)

			_, err := fixture.service.UpdateAccount(adminContext(), test.accountID, test.patch)
			wantError(t, err, test.want)

			if len(fixture.cache.evicted) != 0 {
				t.Fatalf("refused update evicted %v", fixture.cache.evicted)
			}
		})
	}
}

// TestLastAdminCountedInStoreTransaction promotes a second admin, demotes the first and then
// refuses to demote the second: the count comes from the store, not from the caller.
func TestLastAdminCountedInStoreTransaction(t *testing.T) {
	t.Parallel()

	fixture := newFixture(t)

	promoted, err := fixture.service.UpdateAccount(adminContext(), otherID, users.AccountPatch{Role: pointer(account.RoleAdmin)})
	wantNoError(t, err)

	if !promoted.Libraries.IsAll() {
		t.Fatalf("promoted admin libraries = %v, want all", promoted.Libraries.IDs())
	}

	_, err = fixture.service.UpdateAccount(adminContext(), adminID, users.AccountPatch{
		Role: pointer(account.RoleUser), Libraries: pointer(access.AllLibraries()),
	})
	wantNoError(t, err)

	otherAdmin := withViewer(context.Background(), access.Viewer{AccountID: otherID, ProfileID: otherPrimaryID, Admin: true})

	_, err = fixture.service.UpdateAccount(otherAdmin, otherID, users.AccountPatch{Disabled: pointer(true)})
	wantError(t, err, domain.ErrConflict)

	wantError(t, fixture.service.DeleteAccount(otherAdmin, otherID), domain.ErrConflict)
}

// TestDemotionNeedsExplicitLibraries keeps a demoted admin from silently keeping access to
// every library.
func TestDemotionNeedsExplicitLibraries(t *testing.T) {
	t.Parallel()

	fixture := newFixture(t)

	_, err := fixture.service.UpdateAccount(adminContext(), otherID, users.AccountPatch{Role: pointer(account.RoleAdmin)})
	wantNoError(t, err)

	_, err = fixture.service.UpdateAccount(adminContext(), otherID, users.AccountPatch{Role: pointer(account.RoleUser)})
	wantError(t, err, domain.ErrInvalid)

	if fixture.store.accounts[otherID].Account.Role != account.RoleAdmin {
		t.Fatal("refused demotion changed the role")
	}

	demoted, err := fixture.service.UpdateAccount(adminContext(), otherID, users.AccountPatch{
		Role: pointer(account.RoleUser), Libraries: pointer(access.OnlyLibraries(libraryShows)),
	})
	wantNoError(t, err)

	if demoted.Libraries.IsAll() || !demoted.Libraries.Allows(libraryShows) {
		t.Fatalf("demoted libraries = %v", demoted.Libraries.IDs())
	}
}

func TestAccountWriteFailures(t *testing.T) {
	t.Parallel()

	fixture := newFixture(t)
	fixture.store.failWrite = true

	_, err := fixture.service.UpdateAccount(adminContext(), ownerID, users.AccountPatch{Disabled: pointer(true)})
	wantError(t, err, errFake)
	wantError(t, fixture.service.DeleteAccount(adminContext(), ownerID), errFake)

	if len(fixture.cache.evicted) != 0 || fixture.store.accounts[ownerID].Account.Disabled {
		t.Fatalf("failed writes evicted %v or changed the account", fixture.cache.evicted)
	}
}

func TestDeleteAccount(t *testing.T) {
	t.Parallel()

	fixture := newFixture(t)

	wantNoError(t, fixture.service.DeleteAccount(adminContext(), ownerID))

	if _, ok := fixture.store.accounts[ownerID]; ok {
		t.Fatal("account still stored")
	}

	if !slices.Equal(fixture.cache.evicted, []string{ownerID}) {
		t.Fatalf("evicted = %v", fixture.cache.evicted)
	}

	wantError(t, fixture.service.DeleteAccount(adminContext(), adminID), domain.ErrConflict)
	wantError(t, fixture.service.DeleteAccount(adminContext(), "missing"), domain.ErrNotFound)
}

func TestResetPassword(t *testing.T) {
	t.Parallel()

	fixture := newFixture(t)

	wantNoError(t, fixture.service.ResetPassword(adminContext(), ownerID, validPassword))

	if fixture.store.passwords[ownerID] != fakeHashPrefix+validPassword {
		t.Fatalf("stored hash = %q", fixture.store.passwords[ownerID])
	}

	if !slices.Equal(fixture.sessions.revoked, []string{ownerID}) || !slices.Equal(fixture.cache.evicted, []string{ownerID}) {
		t.Fatalf("revoked %v, evicted %v", fixture.sessions.revoked, fixture.cache.evicted)
	}
}

func TestResetPasswordRefusals(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		accountID string
		password  string
		fail      func(*fixture)
		want      error
	}{
		"missing account": {accountID: "missing", password: validPassword, want: domain.ErrNotFound},
		"short password":  {accountID: ownerID, password: "short", want: domain.ErrInvalid},
		"store failure": {
			accountID: ownerID, password: validPassword,
			fail: func(fixture *fixture) { fixture.store.failPassword = true }, want: errFake,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			fixture := newFixture(t)
			if test.fail != nil {
				test.fail(fixture)
			}

			wantError(t, fixture.service.ResetPassword(adminContext(), test.accountID, test.password), test.want)

			if len(fixture.cache.evicted) != 0 {
				t.Fatalf("refused reset evicted %v", fixture.cache.evicted)
			}
		})
	}
}

// TestResetPasswordEvictsWhenRevokeFails keeps cached resolutions of the old sessions from
// outliving a reset whose revoke failed.
func TestResetPasswordEvictsWhenRevokeFails(t *testing.T) {
	t.Parallel()

	fixture := newFixture(t)
	fixture.sessions.fail = true

	wantError(t, fixture.service.ResetPassword(adminContext(), ownerID, validPassword), errFake)

	if !slices.Equal(fixture.cache.evicted, []string{ownerID}) {
		t.Fatalf("evicted = %v, want [%s]", fixture.cache.evicted, ownerID)
	}
}

func newSystem(t *testing.T, fixture *fixture) *users.System {
	t.Helper()

	options := fixtureOptions(fixture.store, fixture.ids, fixture.sessions, fixture.cache)
	options.Viewers = nil
	options.Cache = nil

	system, err := users.NewSystem(options)
	wantNoError(t, err)

	return system
}

func TestNewSystemRequiresStores(t *testing.T) {
	t.Parallel()

	options := fixtureOptions(newFakeStore(), &fakeIDs{}, &fakeSessions{}, &fakeCache{})
	options.Accounts = nil

	if _, err := users.NewSystem(options); err == nil {
		t.Fatal("NewSystem() without Accounts succeeded")
	}
}

func TestSystemResetPassword(t *testing.T) {
	t.Parallel()

	fixture := newFixture(t)
	system := newSystem(t, fixture)

	wantNoError(t, system.ResetPassword(context.Background(), " owner ", validPassword))

	if fixture.store.passwords[ownerID] != fakeHashPrefix+validPassword || !slices.Equal(fixture.sessions.revoked, []string{ownerID}) {
		t.Fatalf("hash %q, revoked %v", fixture.store.passwords[ownerID], fixture.sessions.revoked)
	}

	wantError(t, system.ResetPassword(context.Background(), "nobody", validPassword), domain.ErrNotFound)
	wantError(t, system.ResetPassword(context.Background(), "", validPassword), domain.ErrInvalid)
	wantError(t, system.ResetPassword(context.Background(), "Owner", "short"), domain.ErrInvalid)
}

func TestSystemListAccounts(t *testing.T) {
	t.Parallel()

	fixture := newFixture(t)

	system := newSystem(t, fixture)

	summaries, err := system.ListAccounts(context.Background())
	wantNoError(t, err)

	if len(summaries) != 3 {
		t.Fatalf("ListAccounts() = %d accounts, want 3", len(summaries))
	}

	fixture.store.failList = true

	if _, err := system.ListAccounts(context.Background()); !errors.Is(err, errFake) {
		t.Fatalf("error = %v, want %v", err, errFake)
	}
}
