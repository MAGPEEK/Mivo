package users_test

import (
	"context"
	"fmt"
	"slices"
	"testing"

	"github.com/magpeek/mivo/internal/app/users"
	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/access"
	"github.com/magpeek/mivo/internal/domain/account"
)

func TestListProfiles(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		ctx       context.Context
		accountID string
		want      error
		wantCount int
	}{
		"owner":                  {ctx: ownerContext(), accountID: ownerID, wantCount: 2},
		"admin":                  {ctx: adminContext(), accountID: otherID, wantCount: 2},
		"owner, other account":   {ctx: ownerContext(), accountID: otherID, want: domain.ErrForbidden},
		"owner, managed profile": {ctx: ownerManagedContext(), accountID: ownerID, want: domain.ErrForbidden},
		"admin, missing account": {ctx: adminContext(), accountID: "missing", want: domain.ErrNotFound},
		"no viewer":              {ctx: context.Background(), accountID: ownerID, want: domain.ErrUnauthenticated},
		"viewer account gone": {
			ctx:       withViewer(context.Background(), access.Viewer{AccountID: "gone", ProfileID: ownerPrimaryID}),
			accountID: ownerID, want: domain.ErrNotFound,
		},
		"viewer profile gone": {
			ctx:       withViewer(context.Background(), access.Viewer{AccountID: ownerID, ProfileID: "gone"}),
			accountID: ownerID, want: domain.ErrNotFound,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			fixture := newFixture(t)

			profiles, err := fixture.service.ListProfiles(test.ctx, test.accountID)
			if test.want != nil {
				wantError(t, err, test.want)

				return
			}

			wantNoError(t, err)

			if len(profiles) != test.wantCount || !profiles[0].Profile.Primary {
				t.Fatalf("ListProfiles() = %+v", profiles)
			}
		})
	}
}

func TestCreateProfileByOwner(t *testing.T) {
	t.Parallel()

	fixture := newFixture(t)
	limit, err := access.MaxAgeLimit(access.AgeAll, true)
	wantNoError(t, err)

	created, err := fixture.service.CreateProfile(ownerContext(), ownerID, users.CreateProfileInput{
		Name: "Kid", PIN: validPIN, Libraries: access.OnlyLibraries(libraryShows), Limit: limit,
	})
	wantNoError(t, err)

	profile := created.Profile
	if profile.Primary || profile.AccountID != ownerID || profile.AvatarColor != account.DefaultAvatarColor || !profile.HasPIN {
		t.Fatalf("CreateProfile() = %+v", profile)
	}

	if fixture.store.pins[profile.ID] != fakeHashPrefix+validPIN || fixture.store.profiles[profile.ID].Limit != limit {
		t.Fatalf("stored pin %q, record %+v", fixture.store.pins[profile.ID], fixture.store.profiles[profile.ID])
	}

	if len(fixture.cache.evicted) != 0 {
		t.Fatalf("create evicted %v", fixture.cache.evicted)
	}
}

func TestCreateProfileRefusals(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		ctx       context.Context
		accountID string
		input     users.CreateProfileInput
		want      error
	}{
		"other account":   {ctx: ownerContext(), accountID: otherID, input: users.CreateProfileInput{Name: "Kid"}, want: domain.ErrForbidden},
		"from managed":    {ctx: ownerManagedContext(), accountID: ownerID, input: users.CreateProfileInput{Name: "Kid"}, want: domain.ErrForbidden},
		"library outside": {ctx: adminContext(), accountID: ownerID, input: users.CreateProfileInput{Name: "Kid", Libraries: access.OnlyLibraries(libraryMusic)}, want: users.ErrLibraryAccess},
		"invalid name":    {ctx: ownerContext(), accountID: ownerID, input: users.CreateProfileInput{Name: ""}, want: domain.ErrInvalid},
		"invalid pin":     {ctx: ownerContext(), accountID: ownerID, input: users.CreateProfileInput{Name: "Kid", PIN: "12a4"}, want: domain.ErrInvalid},
		"invalid colour":  {ctx: ownerContext(), accountID: ownerID, input: users.CreateProfileInput{Name: "Kid", AvatarColor: "plaid"}, want: domain.ErrInvalid},
		"missing account": {ctx: adminContext(), accountID: "missing", input: users.CreateProfileInput{Name: "Kid"}, want: domain.ErrNotFound},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			fixture := newFixture(t)
			before := len(fixture.store.profiles)

			_, err := fixture.service.CreateProfile(test.ctx, test.accountID, test.input)
			wantError(t, err, test.want)

			if len(fixture.store.profiles) != before {
				t.Fatalf("profiles = %d, want %d", len(fixture.store.profiles), before)
			}
		})
	}
}

func TestCreateProfileCap(t *testing.T) {
	t.Parallel()

	fixture := newFixture(t)

	for index := 1; index < account.MaxManagedProfiles; index++ {
		fixture.store.addManaged(fmt.Sprintf("extra-%d", index), ownerID)
	}

	_, err := fixture.service.CreateProfile(ownerContext(), ownerID, users.CreateProfileInput{Name: "Eleventh"})
	wantError(t, err, domain.ErrConflict)

	wantNoError(t, fixture.service.DeleteProfile(ownerContext(), ownerManagedID))

	_, err = fixture.service.CreateProfile(ownerContext(), ownerID, users.CreateProfileInput{Name: "Tenth"})
	wantNoError(t, err)
}

func TestAdminManagesOwnManagedProfiles(t *testing.T) {
	t.Parallel()

	fixture := newFixture(t)

	created, err := fixture.service.CreateProfile(adminContext(), adminID, users.CreateProfileInput{
		Name: "Guest room", AvatarColor: account.AvatarColorTeal, Libraries: access.OnlyLibraries(libraryMusic),
	})
	wantNoError(t, err)

	if created.Profile.AvatarColor != account.AvatarColorTeal || !created.Libraries.Allows(libraryMusic) {
		t.Fatalf("CreateProfile() = %+v", created)
	}
}

func TestUpdateProfile(t *testing.T) {
	t.Parallel()

	fixture := newFixture(t)
	limit, err := access.MaxAgeLimit(access.Age12, false)
	wantNoError(t, err)

	updated, err := fixture.service.UpdateProfile(ownerContext(), ownerManagedID, users.ProfilePatch{
		Name: pointer("Little " + decomposedAccent), AvatarColor: pointer(account.AvatarColorPink),
		SetPIN: pointer(validPIN), Libraries: pointer(access.OnlyLibraries(libraryMovies)), Limit: pointer(limit),
	})
	wantNoError(t, err)

	profile := updated.Profile
	if profile.Name != "Little "+composedAccent || profile.AvatarColor != account.AvatarColorPink || !profile.HasPIN ||
		updated.Limit != limit || !updated.Libraries.Allows(libraryMovies) || updated.Libraries.Allows(libraryShows) {
		t.Fatalf("UpdateProfile() = %+v", updated)
	}

	if fixture.store.pins[ownerManagedID] != fakeHashPrefix+validPIN {
		t.Fatalf("stored pin = %q", fixture.store.pins[ownerManagedID])
	}

	if !slices.Equal(fixture.cache.evicted, []string{ownerID}) {
		t.Fatalf("evicted = %v", fixture.cache.evicted)
	}
}

func TestUpdateProfileClearPIN(t *testing.T) {
	t.Parallel()

	fixture := newFixture(t)

	_, err := fixture.service.UpdateProfile(ownerContext(), ownerPrimaryID, users.ProfilePatch{SetPIN: pointer(validPIN)})
	wantNoError(t, err)

	cleared, err := fixture.service.UpdateProfile(ownerContext(), ownerPrimaryID, users.ProfilePatch{ClearPIN: true})
	wantNoError(t, err)

	if cleared.Profile.HasPIN {
		t.Fatal("HasPIN after clear")
	}

	if _, ok := fixture.store.pins[ownerPrimaryID]; ok {
		t.Fatal("PIN hash still stored")
	}

	if !slices.Equal(fixture.cache.evicted, []string{ownerID, ownerID}) {
		t.Fatalf("evicted = %v", fixture.cache.evicted)
	}
}

func TestUpdateProfileCosmeticKeepsCache(t *testing.T) {
	t.Parallel()

	fixture := newFixture(t)

	_, err := fixture.service.UpdateProfile(ownerContext(), ownerPrimaryID, users.ProfilePatch{
		Name: pointer("Me"), AvatarColor: pointer(account.AvatarColorAmber),
	})
	wantNoError(t, err)

	if len(fixture.cache.evicted) != 0 {
		t.Fatalf("cosmetic update evicted %v", fixture.cache.evicted)
	}
}

func TestAdminChangesPrimaryLimit(t *testing.T) {
	t.Parallel()

	fixture := newFixture(t)
	limit, err := access.MaxAgeLimit(access.Age16, true)
	wantNoError(t, err)

	updated, err := fixture.service.UpdateProfile(adminContext(), ownerPrimaryID, users.ProfilePatch{Limit: pointer(limit)})
	wantNoError(t, err)

	if updated.Limit != limit {
		t.Fatalf("limit = %+v, want %+v", updated.Limit, limit)
	}
}

func TestUpdateProfileRefusals(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		ctx       context.Context
		profileID string
		patch     users.ProfilePatch
		want      error
	}{
		"other account's profile": {ctx: ownerContext(), profileID: otherManagedID, patch: users.ProfilePatch{Name: pointer("Mine")}, want: domain.ErrForbidden},
		"from managed profile":    {ctx: ownerManagedContext(), profileID: ownerManagedID, patch: users.ProfilePatch{Name: pointer("Mine")}, want: domain.ErrForbidden},
		"owner lifts own limit":   {ctx: ownerContext(), profileID: ownerPrimaryID, patch: users.ProfilePatch{Limit: pointer(access.NoLimit())}, want: domain.ErrForbidden},
		"primary libraries":       {ctx: adminContext(), profileID: ownerPrimaryID, patch: users.ProfilePatch{Libraries: pointer(access.OnlyLibraries(libraryMovies))}, want: domain.ErrInvalid},
		"library outside account": {ctx: ownerContext(), profileID: ownerManagedID, patch: users.ProfilePatch{Libraries: pointer(access.OnlyLibraries(libraryMusic))}, want: domain.ErrInvalid},
		"set and clear PIN":       {ctx: ownerContext(), profileID: ownerManagedID, patch: users.ProfilePatch{SetPIN: pointer(validPIN), ClearPIN: true}, want: domain.ErrInvalid},
		"invalid PIN":             {ctx: ownerContext(), profileID: ownerManagedID, patch: users.ProfilePatch{SetPIN: pointer("12345")}, want: domain.ErrInvalid},
		"invalid name":            {ctx: ownerContext(), profileID: ownerManagedID, patch: users.ProfilePatch{Name: pointer("\n")}, want: domain.ErrInvalid},
		"invalid colour":          {ctx: ownerContext(), profileID: ownerManagedID, patch: users.ProfilePatch{AvatarColor: pointer(account.AvatarColor("plaid"))}, want: domain.ErrInvalid},
		"missing profile":         {ctx: adminContext(), profileID: "missing", patch: users.ProfilePatch{Name: pointer("X")}, want: domain.ErrNotFound},
		"no viewer":               {ctx: context.Background(), profileID: ownerManagedID, patch: users.ProfilePatch{Name: pointer("X")}, want: domain.ErrUnauthenticated},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			fixture := newFixture(t)
			before := fixture.store.profiles[test.profileID]

			_, err := fixture.service.UpdateProfile(test.ctx, test.profileID, test.patch)
			wantError(t, err, test.want)

			if fixture.store.profiles[test.profileID].Profile != before.Profile || len(fixture.cache.evicted) != 0 {
				t.Fatalf("refused update changed %+v or evicted %v", fixture.store.profiles[test.profileID], fixture.cache.evicted)
			}
		})
	}
}

func TestDeleteProfile(t *testing.T) {
	t.Parallel()

	fixture := newFixture(t)

	wantError(t, fixture.service.DeleteProfile(ownerContext(), ownerPrimaryID), domain.ErrConflict)
	wantError(t, fixture.service.DeleteProfile(ownerContext(), otherManagedID), domain.ErrForbidden)
	wantError(t, fixture.service.DeleteProfile(ownerManagedContext(), ownerManagedID), domain.ErrForbidden)
	wantError(t, fixture.service.DeleteProfile(adminContext(), "missing"), domain.ErrNotFound)

	if len(fixture.cache.evicted) != 0 {
		t.Fatalf("refused deletes evicted %v", fixture.cache.evicted)
	}

	wantNoError(t, fixture.service.DeleteProfile(ownerContext(), ownerManagedID))
	wantNoError(t, fixture.service.DeleteProfile(adminContext(), otherManagedID))

	if _, ok := fixture.store.profiles[ownerManagedID]; ok {
		t.Fatal("profile still stored")
	}

	if !slices.Equal(fixture.cache.evicted, []string{ownerID, otherID}) {
		t.Fatalf("evicted = %v", fixture.cache.evicted)
	}
}

// TestDisabledOwnerRefused checks that a viewer whose account was disabled after the session
// was resolved can no longer manage its profiles: the actor is loaded fresh from the store.
func TestDisabledOwnerRefused(t *testing.T) {
	t.Parallel()

	fixture := newFixture(t)
	owner := fixture.store.accounts[ownerID]
	owner.Account.Disabled = true
	fixture.store.accounts[ownerID] = owner

	_, err := fixture.service.ListProfiles(ownerContext(), ownerID)
	wantError(t, err, domain.ErrForbidden)

	_, err = fixture.service.CreateProfile(ownerContext(), ownerID, users.CreateProfileInput{Name: "Kid"})
	wantError(t, err, domain.ErrForbidden)

	_, err = fixture.service.UpdateProfile(ownerContext(), ownerManagedID, users.ProfilePatch{Name: pointer("Mine")})
	wantError(t, err, domain.ErrForbidden)

	wantError(t, fixture.service.DeleteProfile(ownerContext(), ownerManagedID), domain.ErrForbidden)
}
