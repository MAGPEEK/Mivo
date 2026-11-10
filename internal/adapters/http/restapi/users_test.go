package restapi_test

import (
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/magpeek/mivo/internal/adapters/http/apigen"
	"github.com/magpeek/mivo/internal/adapters/http/restapi"
	appaccess "github.com/magpeek/mivo/internal/app/access"
	"github.com/magpeek/mivo/internal/app/users"
	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/access"
	"github.com/magpeek/mivo/internal/domain/account"
)

// TestUserOperationsWithoutServicesAnswerNotImplemented: an API wired without the user
// services still routes every operation of the M7-02 contract and answers 501.
func TestUserOperationsWithoutServicesAnswerNotImplemented(t *testing.T) {
	t.Parallel()

	handler, _ := newTestHandlerWithOptions(t, restapi.Options{})

	for _, operation := range userOperations() {
		t.Run(operation.method+" "+operation.target, func(t *testing.T) {
			t.Parallel()

			got := serveOperation(t, handler, operation)
			assertProblem(t, got, http.StatusNotImplemented, "not_implemented")
		})
	}
}

// TestUserAndProfileAndDeviceFailuresMapToProblems runs every operation against every
// failure the use cases report and checks the status, code and field of the contract.
func TestUserAndProfileAndDeviceFailuresMapToProblems(t *testing.T) {
	t.Parallel()

	failures := []struct {
		name       string
		err        error
		wantStatus int
		wantCode   string
		wantField  string
	}{
		{name: "unauthenticated", err: domain.ErrUnauthenticated, wantStatus: http.StatusUnauthorized, wantCode: "unauthenticated"},
		{name: "forbidden", err: domain.ErrForbidden, wantStatus: http.StatusForbidden, wantCode: "forbidden"},
		{name: "not found", err: domain.ErrNotFound, wantStatus: http.StatusNotFound, wantCode: "not_found"},
		{name: "last admin", err: account.ErrLastAdmin, wantStatus: http.StatusConflict, wantCode: "last_admin"},
		{name: "name taken", err: account.ErrNameTaken, wantStatus: http.StatusConflict, wantCode: "name_taken"},
		{name: "profile limit", err: account.ErrProfileLimit, wantStatus: http.StatusConflict, wantCode: "profile_limit"},
		{name: "primary profile", err: domain.ErrConflict, wantStatus: http.StatusConflict, wantCode: "conflict"},
		{
			name: "invalid name", err: &account.InvalidFieldError{Field: account.FieldName, Reason: "too long"},
			wantStatus: http.StatusBadRequest, wantCode: "invalid", wantField: "name",
		},
		{
			name: "invalid pin", err: &account.InvalidFieldError{Field: account.FieldPIN, Reason: "not 4 digits"},
			wantStatus: http.StatusBadRequest, wantCode: "invalid", wantField: "pin",
		},
		{
			name: "library access", err: users.ErrLibraryAccess,
			wantStatus: http.StatusBadRequest, wantCode: "invalid", wantField: "libraryAccess",
		},
		{name: "invalid", err: domain.ErrInvalid, wantStatus: http.StatusBadRequest, wantCode: "invalid"},
		{name: "store failure", err: errors.New("disk /secret/path broke"), wantStatus: http.StatusInternalServerError, wantCode: "internal_error"},
	}

	for _, failure := range failures {
		for _, operation := range userOperations() {
			t.Run(failure.name+"/"+operation.method+" "+operation.target, func(t *testing.T) {
				t.Parallel()

				fake := newFakeUsers(t)
				fake.err = fmt.Errorf("use case: %w", failure.err)

				got := serveOperation(t, newUsersHandler(t, fake), operation)
				problem := assertProblem(t, got, failure.wantStatus, failure.wantCode)

				if field := valueOr(problem.Field, ""); field != failure.wantField {
					t.Errorf("problem field = %q, want %q", field, failure.wantField)
				}

				assertNoInternalText(t, got, "/secret/path")
			})
		}
	}
}

func valueOr[T any](pointer *T, fallback T) T {
	if pointer == nil {
		return fallback
	}

	return *pointer
}

func TestListUsersAnswersAccountsWithDerivedPreset(t *testing.T) {
	t.Parallel()

	got := serveOperation(t, newUsersHandler(t, newFakeUsers(t)), userOperation{method: http.MethodGet, target: usersTarget, body: ""})
	if got.status != http.StatusOK {
		t.Fatalf("status = %d, body %s", got.status, got.body)
	}

	list := decode[apigen.UserList](t, got.body)
	if len(list.Users) != 2 {
		t.Fatalf("users = %+v, want Anna and Ben", list.Users)
	}

	anna, ben := list.Users[0], list.Users[1]
	if anna.ID.String() != annaID || anna.Name != "Anna" || anna.Role != apigen.AccountRoleAdmin ||
		anna.Status != apigen.UserStatusActive || anna.ProfileCount != 1 || !anna.CreatedAt.Equal(createdAt()) ||
		anna.LastActiveAt == nil || !anna.LibraryAccess.All || anna.LibraryAccess.LibraryIds != nil {
		t.Errorf("anna = %+v", anna)
	}

	if anna.ContentLimit.Preset != apigen.ContentLimitPresetAdults || anna.ContentLimit.MaxAge != nil || anna.ContentLimit.HideUnrated {
		t.Errorf("anna content limit = %+v, want adults without a max age", anna.ContentLimit)
	}

	if ben.Status != apigen.UserStatusDisabled || ben.Role != apigen.AccountRoleUser || ben.LastActiveAt != nil ||
		ben.ProfileCount != 2 || ben.LibraryAccess.All || ben.LibraryAccess.LibraryIds == nil ||
		len(*ben.LibraryAccess.LibraryIds) != 1 || (*ben.LibraryAccess.LibraryIds)[0].String() != moviesID {
		t.Errorf("ben = %+v", ben)
	}

	// 12 without hiding unrated titles is still "teens": the preset follows maxAge alone.
	if ben.ContentLimit.Preset != apigen.ContentLimitPresetTeens || valueOr(ben.ContentLimit.MaxAge, -1) != 12 || ben.ContentLimit.HideUnrated {
		t.Errorf("ben content limit = %+v, want teens, 12, unrated shown", ben.ContentLimit)
	}

	if strings.Contains(string(got.body), `"lastActiveAt":null`) {
		t.Errorf("body carries a null lastActiveAt: %s", got.body)
	}
}

func TestGetUserAnswersTheAccount(t *testing.T) {
	t.Parallel()

	got := serveOperation(t, newUsersHandler(t, newFakeUsers(t)), userOperation{method: http.MethodGet, target: benTarget, body: ""})
	if got.status != http.StatusOK {
		t.Fatalf("status = %d, body %s", got.status, got.body)
	}

	if user := decode[apigen.User](t, got.body); user.ID.String() != benID || user.ProfileCount != 2 {
		t.Errorf("user = %+v, want Ben with 2 profiles", user)
	}
}

func TestCreateUserMapsBodyAndAnswersLocation(t *testing.T) {
	t.Parallel()

	fake := newFakeUsers(t)
	body := `{"name":"Cleo","password":"long enough","role":"user",` +
		`"libraryAccess":{"all":false,"libraryIds":["` + moviesID + `","` + showsID + `"]},` +
		`"contentLimit":{"preset":"custom","maxAge":12,"hideUnrated":true}}`

	got := serveOperation(t, newUsersHandler(t, fake), userOperation{method: http.MethodPost, target: usersTarget, body: body})
	if got.status != http.StatusCreated {
		t.Fatalf("status = %d, body %s", got.status, got.body)
	}

	if location := got.header.Get("Location"); location != usersTarget+"/"+cleoID {
		t.Errorf("Location = %q, want %q", location, usersTarget+"/"+cleoID)
	}

	input := fake.createdAccount
	if input.Name != "Cleo" || input.Password != validPassword || input.Role != account.RoleUser ||
		input.Libraries.IsAll() || !slices.Equal(input.Libraries.IDs(), []string{moviesID, showsID}) ||
		input.PrimaryLimit != mustLimit(t, access.Age12, true) {
		t.Errorf("CreateAccount input = %+v", input)
	}

	// The request said custom; the answer carries the preset derived from maxAge.
	user := decode[apigen.User](t, got.body)
	if user.Name != "Cleo" || user.ProfileCount != 1 || user.ContentLimit.Preset != apigen.ContentLimitPresetTeens {
		t.Errorf("user = %+v", user)
	}

	if strings.Contains(string(got.body), "password") || strings.Contains(string(got.body), validPassword) {
		t.Errorf("response carries the password: %s", got.body)
	}
}

func TestCreateUserDefaultsToAllLibrariesAndNoLimit(t *testing.T) {
	t.Parallel()

	fake := newFakeUsers(t)

	got := serveOperation(t, newUsersHandler(t, fake), userOperation{method: http.MethodPost, target: usersTarget, body: `{"name":"Dora","role":"admin","password":"long enough"}`})
	if got.status != http.StatusCreated {
		t.Fatalf("status = %d, body %s", got.status, got.body)
	}

	if input := fake.createdAccount; !input.Libraries.IsAll() || input.PrimaryLimit != access.NoLimit() || input.Role != account.RoleAdmin {
		t.Errorf("CreateAccount input = %+v, want all libraries and no limit", input)
	}
}

// TestUserAndProfileBodiesAreRejectedWithTheirField covers the rules the adapter checks
// itself because only it sees the request shape: the library access form, an admin with a
// library list and a preset that contradicts maxAge.
func TestUserAndProfileBodiesAreRejectedWithTheirField(t *testing.T) {
	t.Parallel()

	const list = `{"all":false,"libraryIds":["` + moviesID + `"]}`

	tests := []struct {
		name      string
		method    string
		target    string
		body      string
		wantField string
	}{
		{
			name: "admin with a library list", method: http.MethodPost, target: usersTarget,
			body: `{"name":"Cleo","password":"long enough","role":"admin","libraryAccess":` + list + `}`, wantField: "libraryAccess",
		},
		{
			name: "all with library ids", method: http.MethodPost, target: usersTarget,
			body:      `{"name":"Cleo","password":"long enough","role":"user","libraryAccess":{"all":true,"libraryIds":["` + moviesID + `"]}}`,
			wantField: "libraryAccess",
		},
		{
			name: "preset contradicts max age", method: http.MethodPost, target: usersTarget,
			body:      `{"name":"Cleo","password":"long enough","role":"user","contentLimit":{"preset":"children","maxAge":12,"hideUnrated":true}}`,
			wantField: "contentLimit",
		},
		{
			name: "adults with a max age", method: http.MethodPatch, target: benTarget,
			body: `{"contentLimit":{"preset":"adults","maxAge":18,"hideUnrated":false}}`, wantField: "contentLimit",
		},
		{
			name: "unknown preset", method: http.MethodPatch, target: miaTarget,
			body: `{"contentLimit":{"preset":"toddlers","maxAge":0,"hideUnrated":true}}`, wantField: "contentLimit",
		},
		{
			name: "max age outside the buckets", method: http.MethodPost, target: benTarget + "/profiles",
			body: `{"name":"Kid","contentLimit":{"preset":"custom","maxAge":7,"hideUnrated":true}}`, wantField: "contentLimit",
		},
		{
			name: "promotion with a library list", method: http.MethodPatch, target: benTarget,
			body: `{"role":"admin","libraryAccess":` + list + `}`, wantField: "libraryAccess",
		},
		{
			name: "library list for an admin account", method: http.MethodPatch, target: annaTarget,
			body: `{"libraryAccess":` + list + `}`, wantField: "libraryAccess",
		},
		{
			name: "unknown status", method: http.MethodPatch, target: benTarget,
			body: `{"status":"asleep"}`, wantField: "status",
		},
		{
			name: "profile all with library ids", method: http.MethodPatch, target: miaTarget,
			body: `{"libraryAccess":{"all":true,"libraryIds":["` + moviesID + `"]}}`, wantField: "libraryAccess",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			fake := newFakeUsers(t)

			got := serveOperation(t, newUsersHandler(t, fake), userOperation{method: test.method, target: test.target, body: test.body})
			problem := assertProblem(t, got, http.StatusBadRequest, "invalid")

			if field := valueOr(problem.Field, ""); field != test.wantField {
				t.Errorf("problem field = %q, want %q", field, test.wantField)
			}

			if fake.createdAccount.Name != "" || fake.createdProfile.Name != "" || fake.accountPatch != (users.AccountPatch{}) ||
				fake.profilePatch != (users.ProfilePatch{}) {
				t.Error("the use case was called for a rejected body")
			}
		})
	}
}

func TestUpdateUserMapsOnlyTheSentProperties(t *testing.T) {
	t.Parallel()

	fake := newFakeUsers(t)
	handler := newUsersHandler(t, fake)

	got := serveOperation(t, handler, userOperation{method: http.MethodPatch, target: benTarget, body: `{"status":"active"}`})
	if got.status != http.StatusOK {
		t.Fatalf("status = %d, body %s", got.status, got.body)
	}

	patch := fake.accountPatch
	if patch.Disabled == nil || *patch.Disabled || patch.Name != nil || patch.Role != nil || patch.Libraries != nil || patch.PrimaryLimit != nil {
		t.Errorf("patch = %+v, want only Disabled false", patch)
	}

	if user := decode[apigen.User](t, got.body); user.ID.String() != benID || user.ProfileCount != 2 {
		t.Errorf("user = %+v, want Ben as the list shows him", user)
	}

	body := `{"name":"Benno","role":"user","status":"disabled","libraryAccess":{"all":true},` +
		`"contentLimit":{"preset":"adults","maxAge":null,"hideUnrated":true}}`

	got = serveOperation(t, handler, userOperation{method: http.MethodPatch, target: annaTarget, body: body})
	if got.status != http.StatusOK {
		t.Fatalf("status = %d, body %s", got.status, got.body)
	}

	patch = fake.accountPatch
	if valueOr(patch.Name, "") != "Benno" || valueOr(patch.Role, "") != account.RoleUser || !valueOr(patch.Disabled, false) ||
		patch.Libraries == nil || !patch.Libraries.IsAll() ||
		patch.PrimaryLimit == nil || *patch.PrimaryLimit != access.NoLimit().HidingUnrated(true) {
		t.Errorf("patch = %+v, want every property", patch)
	}
}

func TestDeleteUserAndResetUserPassword(t *testing.T) {
	t.Parallel()

	fake := newFakeUsers(t)
	handler := newUsersHandler(t, fake)

	got := serveOperation(t, handler, userOperation{method: http.MethodPut, target: benTarget + "/password", body: `{"password":"another long one"}`})
	if got.status != http.StatusNoContent || len(got.body) != 0 {
		t.Fatalf("reset: status = %d, body %s", got.status, got.body)
	}

	if fake.passwords[benID] != "another long one" {
		t.Errorf("passwords = %v, want Ben's replaced", fake.passwords)
	}

	got = serveOperation(t, handler, userOperation{method: http.MethodDelete, target: benTarget, body: ""})
	if got.status != http.StatusNoContent || len(got.body) != 0 {
		t.Fatalf("delete: status = %d, body %s", got.status, got.body)
	}

	if !slices.Equal(fake.deletedAccounts, []string{benID}) {
		t.Errorf("deleted accounts = %v, want Ben", fake.deletedAccounts)
	}
}

func TestListUserProfilesNeverCarriesThePIN(t *testing.T) {
	t.Parallel()

	got := serveOperation(t, newUsersHandler(t, newFakeUsers(t)), userOperation{method: http.MethodGet, target: benTarget + "/profiles", body: ""})
	if got.status != http.StatusOK {
		t.Fatalf("status = %d, body %s", got.status, got.body)
	}

	list := decode[apigen.ProfileList](t, got.body)
	if len(list.Profiles) != 2 {
		t.Fatalf("profiles = %+v, want Ben and Mia", list.Profiles)
	}

	primary, mia := list.Profiles[0], list.Profiles[1]
	if !primary.IsPrimary || primary.HasPin || primary.AvatarColor != apigen.AvatarColorBlue || !primary.LibraryAccess.All ||
		primary.UserID.String() != benID {
		t.Errorf("primary = %+v", primary)
	}

	// 16 matches no fixed preset, so the derived preset is custom.
	if mia.ID.String() != miaID || mia.IsPrimary || !mia.HasPin || mia.Name != "Mia" ||
		mia.ContentLimit.Preset != apigen.ContentLimitPresetCustom || valueOr(mia.ContentLimit.MaxAge, -1) != 16 ||
		!mia.ContentLimit.HideUnrated || mia.LibraryAccess.All {
		t.Errorf("mia = %+v", mia)
	}

	if strings.Contains(string(got.body), `"pin"`) {
		t.Errorf("body carries a pin property: %s", got.body)
	}
}

func TestCreateProfileMapsBodyAndAnswersLocation(t *testing.T) {
	t.Parallel()

	fake := newFakeUsers(t)
	body := `{"name":"Mia","pin":"1234","avatarColor":"teal","libraryAccess":{"all":false,"libraryIds":[]},` +
		`"contentLimit":{"preset":"children","maxAge":6,"hideUnrated":false}}`

	got := serveOperation(t, newUsersHandler(t, fake), userOperation{method: http.MethodPost, target: benTarget + "/profiles", body: body})
	if got.status != http.StatusCreated {
		t.Fatalf("status = %d, body %s", got.status, got.body)
	}

	if location := got.header.Get("Location"); location != miaTarget {
		t.Errorf("Location = %q, want %q", location, miaTarget)
	}

	input := fake.createdProfile
	if input.Name != "Mia" || input.PIN != "1234" || input.AvatarColor != account.AvatarColorTeal ||
		input.Libraries.IsAll() || len(input.Libraries.IDs()) != 0 || input.Limit != mustLimit(t, access.Age6, false) {
		t.Errorf("CreateProfile input = %+v", input)
	}

	profile := decode[apigen.Profile](t, got.body)
	if !profile.HasPin || profile.ContentLimit.Preset != apigen.ContentLimitPresetChildren || profile.LibraryAccess.All ||
		profile.LibraryAccess.LibraryIds == nil || len(*profile.LibraryAccess.LibraryIds) != 0 {
		t.Errorf("profile = %+v", profile)
	}

	if strings.Contains(string(got.body), "1234") {
		t.Errorf("response carries the PIN: %s", got.body)
	}
}

func TestCreateProfileDefaults(t *testing.T) {
	t.Parallel()

	fake := newFakeUsers(t)

	got := serveOperation(t, newUsersHandler(t, fake), userOperation{method: http.MethodPost, target: benTarget + "/profiles", body: `{"name":"Kid"}`})
	if got.status != http.StatusCreated {
		t.Fatalf("status = %d, body %s", got.status, got.body)
	}

	if input := fake.createdProfile; input.PIN != "" || input.AvatarColor != "" || !input.Libraries.IsAll() || input.Limit != access.NoLimit() {
		t.Errorf("CreateProfile input = %+v, want no PIN, default colour, all libraries, no limit", input)
	}

	if profile := decode[apigen.Profile](t, got.body); profile.AvatarColor != apigen.AvatarColorCoral || profile.HasPin {
		t.Errorf("profile = %+v, want coral without a PIN", profile)
	}
}

func TestUpdateProfileTellsOmittedPINFromNull(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		body      string
		wantSet   string
		wantClear bool
	}{
		{name: "omitted keeps the PIN", body: `{"name":"Mia"}`},
		{name: "null removes the PIN", body: `{"pin":null}`, wantClear: true},
		{name: "a string sets the PIN", body: `{"pin":"4321"}`, wantSet: "4321"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			fake := newFakeUsers(t)

			got := serveOperation(t, newUsersHandler(t, fake), userOperation{method: http.MethodPatch, target: miaTarget, body: test.body})
			if got.status != http.StatusOK {
				t.Fatalf("status = %d, body %s", got.status, got.body)
			}

			patch := fake.profilePatch
			if valueOr(patch.SetPIN, "") != test.wantSet || (patch.SetPIN != nil) != (test.wantSet != "") || patch.ClearPIN != test.wantClear {
				t.Errorf("patch = %+v, want set %q clear %t", patch, test.wantSet, test.wantClear)
			}
		})
	}
}

func TestUpdateProfileMapsEveryProperty(t *testing.T) {
	t.Parallel()

	fake := newFakeUsers(t)
	body := `{"name":"Mia Maria","avatarColor":"violet","libraryAccess":{"all":true},` +
		`"contentLimit":{"preset":"youngChildren","maxAge":0,"hideUnrated":true}}`

	got := serveOperation(t, newUsersHandler(t, fake), userOperation{method: http.MethodPatch, target: miaTarget, body: body})
	if got.status != http.StatusOK {
		t.Fatalf("status = %d, body %s", got.status, got.body)
	}

	patch := fake.profilePatch
	if valueOr(patch.Name, "") != "Mia Maria" || valueOr(patch.AvatarColor, "") != account.AvatarColorViolet ||
		patch.Libraries == nil || !patch.Libraries.IsAll() || patch.Limit == nil || *patch.Limit != mustLimit(t, access.AgeAll, true) ||
		patch.SetPIN != nil || patch.ClearPIN {
		t.Errorf("patch = %+v", patch)
	}

	if profile := decode[apigen.Profile](t, got.body); profile.Name != "Mia Maria" || profile.ID.String() != miaID {
		t.Errorf("profile = %+v", profile)
	}
}

// TestOwnerUpdatesOwnManagedProfile: the handler repeats no admin check, so the account
// owner with the primary profile selected changes a managed profile of the own account,
// while another account is refused by the use case (ADR 0032 §1).
func TestOwnerUpdatesOwnManagedProfile(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		viewer     access.Viewer
		wantStatus int
	}{
		{name: "owner with the primary profile", viewer: access.Viewer{AccountID: benID, ProfileID: benPrimaryID}, wantStatus: http.StatusOK},
		{name: "owner with the managed profile", viewer: access.Viewer{AccountID: benID, ProfileID: miaID}, wantStatus: http.StatusForbidden},
		{name: "another account", viewer: access.Viewer{AccountID: annaID, ProfileID: "other"}, wantStatus: http.StatusForbidden},
		{name: "admin", viewer: access.Viewer{AccountID: annaID, ProfileID: "other", Admin: true}, wantStatus: http.StatusOK},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			fake := newFakeUsers(t)
			request := jsonRequest(t, http.MethodPatch, miaTarget, `{"name":"Mia Maria","pin":"4321"}`)
			request = request.WithContext(appaccess.WithViewer(request.Context(), test.viewer))

			got := serveRequest(t, newUsersHandler(t, fake), request)
			if got.status != test.wantStatus {
				t.Fatalf("status = %d, want %d, body %s", got.status, test.wantStatus, got.body)
			}

			if test.wantStatus != http.StatusOK {
				assertProblem(t, got, http.StatusForbidden, "forbidden")

				return
			}

			if profile := decode[apigen.Profile](t, got.body); profile.Name != "Mia Maria" || profile.UserID.String() != benID {
				t.Errorf("profile = %+v, want the renamed profile of Ben", profile)
			}
		})
	}
}

func TestDeleteProfile(t *testing.T) {
	t.Parallel()

	fake := newFakeUsers(t)

	got := serveOperation(t, newUsersHandler(t, fake), userOperation{method: http.MethodDelete, target: miaTarget, body: ""})
	if got.status != http.StatusNoContent || len(got.body) != 0 {
		t.Fatalf("status = %d, body %s", got.status, got.body)
	}

	if !slices.Equal(fake.deletedProfiles, []string{miaID}) {
		t.Errorf("deleted profiles = %v, want Mia", fake.deletedProfiles)
	}
}

func TestListDevicesAnswersEveryAccountsDevices(t *testing.T) {
	t.Parallel()

	fake := newFakeUsers(t)
	fake.devices = append(fake.devices, fake.devices[0])
	fake.devices[1].Device.Platform = ""
	fake.devices[1].Current = false

	got := serveOperation(t, newUsersHandler(t, fake), userOperation{method: http.MethodGet, target: devicesTarget, body: ""})
	if got.status != http.StatusOK {
		t.Fatalf("status = %d, body %s", got.status, got.body)
	}

	list := decode[apigen.AccountDeviceList](t, got.body)
	if len(list.Devices) != 2 {
		t.Fatalf("devices = %+v, want 2", list.Devices)
	}

	device := list.Devices[0]
	if device.ID.String() != deviceID || device.UserID.String() != benID || device.UserName != "Ben" ||
		device.Name != "Living room TV" || device.Client != apigen.ClientKindTv || valueOr(device.Platform, "") != "webOS" ||
		device.AddressClass != apigen.AddressClassLan || !device.Current || !device.LastSeenAt.Equal(createdAt()) {
		t.Errorf("device = %+v", device)
	}

	if other := list.Devices[1]; other.Platform != nil || other.Current {
		t.Errorf("second device = %+v, want no platform and not current", other)
	}
}

func TestRevokeDevice(t *testing.T) {
	t.Parallel()

	fake := newFakeUsers(t)

	got := serveOperation(t, newUsersHandler(t, fake), userOperation{method: http.MethodDelete, target: devicesTarget + "/" + deviceID, body: ""})
	if got.status != http.StatusNoContent || len(got.body) != 0 {
		t.Fatalf("status = %d, body %s", got.status, got.body)
	}

	if !slices.Equal(fake.revokedDevices, []string{deviceID}) {
		t.Errorf("revoked devices = %v, want the device", fake.revokedDevices)
	}
}

// TestUserResponsesWithUnreadableStoredIDsAreServerFaults: an id that is no UUID is broken
// stored data, never a client mistake.
func TestUserResponsesWithUnreadableStoredIDsAreServerFaults(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		corrupt func(fake *fakeUsers)
		target  string
	}{
		{name: "account id", corrupt: func(fake *fakeUsers) { fake.summaries[0].Account.ID = "anna" }, target: usersTarget},
		{
			name:    "library id",
			corrupt: func(fake *fakeUsers) { fake.summaries[1].Libraries = access.OnlyLibraries("movies") }, target: usersTarget,
		},
		{name: "profile id", corrupt: func(fake *fakeUsers) { fake.profiles[1].Profile.ID = "mia" }, target: benTarget + "/profiles"},
		{name: "device id", corrupt: func(fake *fakeUsers) { fake.devices[0].ID = "tv" }, target: devicesTarget},
		{name: "device account id", corrupt: func(fake *fakeUsers) { fake.devices[0].AccountID = "ben" }, target: devicesTarget},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			fake := newFakeUsers(t)
			test.corrupt(fake)

			got := serveOperation(t, newUsersHandler(t, fake), userOperation{method: http.MethodGet, target: test.target, body: ""})
			assertProblem(t, got, http.StatusInternalServerError, "internal_error")
		})
	}
}

func TestUserBodiesAreRequired(t *testing.T) {
	t.Parallel()

	handler := newUsersHandler(t, newFakeUsers(t))

	for _, operation := range userOperations() {
		if operation.body == "" {
			continue
		}

		t.Run(operation.method+" "+operation.target, func(t *testing.T) {
			t.Parallel()

			got := serveOperation(t, handler, userOperation{method: operation.method, target: operation.target, body: ""})
			if got.status != http.StatusBadRequest {
				t.Errorf("status = %d, want 400, body %s", got.status, got.body)
			}
		})
	}
}
