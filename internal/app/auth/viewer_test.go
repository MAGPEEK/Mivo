package auth

import (
	"slices"
	"testing"

	"github.com/magpeek/mivo/internal/domain/access"
	"github.com/magpeek/mivo/internal/domain/account"
)

func mustLimit(t *testing.T, maxAge access.AgeBucket, hideUnrated bool) access.ContentLimit {
	t.Helper()

	limit, err := access.MaxAgeLimit(maxAge, hideUnrated)
	if err != nil {
		t.Fatalf("MaxAgeLimit(%d): %v", maxAge, err)
	}

	return limit
}

func TestBuildViewer(t *testing.T) {
	t.Parallel()

	user := account.Account{ID: "acc", Name: "Alice", Role: account.RoleUser}
	admin := account.Account{ID: "acc", Name: "Root", Role: account.RoleAdmin}
	primary := account.Profile{ID: "primary", AccountID: "acc", Primary: true}
	managed := account.Profile{ID: "kid", AccountID: "acc"}

	type want struct {
		admin       bool
		allLibs     bool
		libraries   []string
		maxAge      access.AgeBucket
		hasMaxAge   bool
		hideUnrated bool
	}

	tests := []struct {
		name   string
		inputs ViewerInputs
		want   want
	}{
		{
			name:   "admin primary sees everything",
			inputs: ViewerInputs{Account: admin, Profile: primary, AccountLibraries: access.OnlyLibraries("lib-1")},
			want:   want{admin: true, allLibs: true},
		},
		{
			name: "managed profile of an admin is no admin and keeps its own set",
			inputs: ViewerInputs{
				Account: admin, Profile: managed,
				ProfileLibraries: access.OnlyLibraries("lib-2"),
			},
			want: want{libraries: []string{"lib-2"}},
		},
		{
			name: "user primary gets the account set and its own limit",
			inputs: ViewerInputs{
				Account: user, Profile: primary,
				AccountLibraries: access.OnlyLibraries("lib-1", "lib-2"),
				ProfileLibraries: access.OnlyLibraries("ignored"),
				ProfileLimit:     mustLimit(t, access.Age16, false),
			},
			want: want{libraries: []string{"lib-1", "lib-2"}, maxAge: access.Age16, hasMaxAge: true},
		},
		{
			name: "managed profile intersects with the account set",
			inputs: ViewerInputs{
				Account: user, Profile: managed,
				AccountLibraries: access.OnlyLibraries("lib-1", "lib-2"),
				ProfileLibraries: access.OnlyLibraries("lib-2", "lib-3"),
			},
			want: want{libraries: []string{"lib-2"}},
		},
		{
			name: "managed profile without own limit inherits the primary limit",
			inputs: ViewerInputs{
				Account: user, Profile: managed,
				AccountLibraries: access.AllLibraries(), ProfileLibraries: access.AllLibraries(),
				ProfileLimit: access.NoLimit(), PrimaryLimit: mustLimit(t, access.Age12, true),
			},
			want: want{allLibs: true, maxAge: access.Age12, hasMaxAge: true, hideUnrated: true},
		},
		{
			name: "managed profile cannot weaken the primary limit",
			inputs: ViewerInputs{
				Account: user, Profile: managed,
				AccountLibraries: access.AllLibraries(), ProfileLibraries: access.AllLibraries(),
				ProfileLimit: mustLimit(t, access.Age18, false), PrimaryLimit: mustLimit(t, access.Age6, false),
			},
			want: want{allLibs: true, maxAge: access.Age6, hasMaxAge: true},
		},
		{
			name: "managed profile keeps a stricter own limit",
			inputs: ViewerInputs{
				Account: user, Profile: managed,
				AccountLibraries: access.AllLibraries(), ProfileLibraries: access.AllLibraries(),
				ProfileLimit: mustLimit(t, access.AgeAll, true), PrimaryLimit: access.NoLimit(),
			},
			want: want{allLibs: true, maxAge: access.AgeAll, hasMaxAge: true, hideUnrated: true},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			viewer, err := buildViewer("session", test.inputs, true)
			if err != nil {
				t.Fatalf("buildViewer: %v", err)
			}

			checkViewer(t, viewer, test.inputs.Profile.ID)

			restriction := viewer.Restriction()
			maxAge, hasMaxAge := restriction.MaxAge()

			got := want{
				admin: viewer.Admin, allLibs: restriction.AllLibraries(), libraries: restriction.LibraryIDs(),
				maxAge: maxAge, hasMaxAge: hasMaxAge, hideUnrated: restriction.HideUnrated(),
			}
			if got.admin != test.want.admin || got.allLibs != test.want.allLibs ||
				!slices.Equal(got.libraries, test.want.libraries) || got.maxAge != test.want.maxAge ||
				got.hasMaxAge != test.want.hasMaxAge || got.hideUnrated != test.want.hideUnrated {
				t.Fatalf("viewer = %+v, want %+v", got, test.want)
			}
		})
	}
}

func checkViewer(t *testing.T, viewer access.Viewer, profileID string) {
	t.Helper()

	if viewer.AuthSessionID != "session" || viewer.AccountID != "acc" || viewer.ProfileID != profileID {
		t.Fatalf("viewer identity = %+v", viewer)
	}
}

func TestBuildViewerWithoutProfileFailsClosed(t *testing.T) {
	t.Parallel()

	admin := account.Account{ID: "acc", Role: account.RoleAdmin}
	viewer, err := buildViewer("session", ViewerInputs{Account: admin, AccountLibraries: access.AllLibraries()}, false)
	if err != nil {
		t.Fatalf("buildViewer: %v", err)
	}

	restriction := viewer.Restriction()
	maxAge, hasMaxAge := restriction.MaxAge()

	if viewer.Admin || viewer.ProfileID != "" || !restriction.DeniesAll() ||
		!hasMaxAge || maxAge != access.AgeAll || !restriction.HideUnrated() {
		t.Fatalf("viewer without profile = %+v", viewer)
	}
}

func TestAutoSelectProfile(t *testing.T) {
	t.Parallel()

	primary := account.Profile{ID: "p", Primary: true}
	withPIN := account.Profile{ID: "p", Primary: true, HasPIN: true}
	managed := account.Profile{ID: "m"}

	tests := []struct {
		name     string
		profiles []account.Profile
		want     bool
	}{
		{"only the primary", []account.Profile{primary}, true},
		{"primary with PIN", []account.Profile{withPIN}, false},
		{"managed after primary", []account.Profile{primary, managed}, false},
		{"managed before primary", []account.Profile{managed, primary}, false},
		{"none", nil, false},
	}

	for _, test := range tests {
		got, ok := autoSelectProfile(test.profiles)
		if ok != test.want || (ok && got.ID != primary.ID) {
			t.Errorf("%s: autoSelectProfile = %+v, %v; want %v", test.name, got, ok, test.want)
		}
	}
}
