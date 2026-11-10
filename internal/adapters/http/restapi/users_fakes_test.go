package restapi_test

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/magpeek/mivo/internal/adapters/http/restapi"
	appaccess "github.com/magpeek/mivo/internal/app/access"
	"github.com/magpeek/mivo/internal/app/auth"
	"github.com/magpeek/mivo/internal/app/users"
	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/access"
	"github.com/magpeek/mivo/internal/domain/account"
)

// IDs of the user administration tests.
const (
	annaID        = "1a2b3c4d-5e6f-4a7b-8c9d-0e1f2a3b4c5d"
	benID         = "2b3c4d5e-6f7a-4b8c-9d0e-1f2a3b4c5d6e"
	cleoID        = "5b8e3f4c-1d0a-4b92-8c3e-4f5a6b7c8d9e"
	benPrimaryID  = "6f5e4d3c-2b1a-4f0e-9d8c-7b6a5f4e3d2c"
	miaID         = "7a6b5c4d-3e2f-4a1b-8c9d-0e1f2a3b4c5d"
	deviceID      = "8e7d6c5b-4a3f-4e2d-9c1b-0a9f8e7d6c5b"
	moviesID      = "3f6c1d2a-9b8e-4f70-8a1c-2d3e4f5a6b7c"
	showsID       = "4a7d2e3b-0c9f-4a81-9b2d-3e4f5a6b7c8d"
	usersTarget   = "/api/v1/users"
	annaTarget    = usersTarget + "/" + annaID
	benTarget     = usersTarget + "/" + benID
	miaTarget     = "/api/v1/profiles/" + miaID
	devicesTarget = "/api/v1/devices"
	validPassword = "long enough"
)

// fakeUsers is an in-memory app/users.Service and device administration: it records what the
// handlers pass and answers the configured accounts, profiles, devices or failure.
type fakeUsers struct {
	mutex sync.Mutex
	// err fails every call when set.
	err       error
	summaries []users.AccountSummary
	profiles  []users.ProfileRecord
	devices   []auth.DeviceView

	createdAccount  users.CreateAccountInput
	accountPatch    users.AccountPatch
	createdProfile  users.CreateProfileInput
	profilePatch    users.ProfilePatch
	passwords       map[string]string
	deletedAccounts []string
	deletedProfiles []string
	revokedDevices  []string
}

func (fake *fakeUsers) ListAccounts(context.Context) ([]users.AccountSummary, error) {
	fake.mutex.Lock()
	defer fake.mutex.Unlock()

	return fake.summaries, fake.err
}

func (fake *fakeUsers) AccountSummary(_ context.Context, accountID string) (users.AccountSummary, error) {
	fake.mutex.Lock()
	defer fake.mutex.Unlock()

	if fake.err != nil {
		return users.AccountSummary{}, fake.err
	}

	return fake.summary(accountID)
}

func (fake *fakeUsers) summary(accountID string) (users.AccountSummary, error) {
	for _, summary := range fake.summaries {
		if summary.Account.ID == accountID {
			return summary, nil
		}
	}

	return users.AccountSummary{}, fmt.Errorf("account %s: %w", accountID, domain.ErrNotFound)
}

func (fake *fakeUsers) CreateAccount(_ context.Context, input users.CreateAccountInput) (users.AccountRecord, error) {
	fake.mutex.Lock()
	defer fake.mutex.Unlock()

	if fake.err != nil {
		return users.AccountRecord{}, fake.err
	}

	fake.createdAccount = input
	record := users.AccountRecord{
		Account:   account.Account{ID: cleoID, Name: input.Name, Role: input.Role},
		Libraries: input.Libraries, PrimaryLimit: input.PrimaryLimit,
	}
	fake.summaries = append(fake.summaries, users.AccountSummary{
		AccountRecord: record, ProfileCount: 1, CreatedAt: createdAt(),
	})

	return record, nil
}

func (fake *fakeUsers) UpdateAccount(_ context.Context, accountID string, patch users.AccountPatch) (users.AccountRecord, error) {
	fake.mutex.Lock()
	defer fake.mutex.Unlock()

	if fake.err != nil {
		return users.AccountRecord{}, fake.err
	}

	summary, err := fake.summary(accountID)
	if err != nil {
		return users.AccountRecord{}, err
	}

	fake.accountPatch = patch

	return summary.AccountRecord, nil
}

func (fake *fakeUsers) DeleteAccount(_ context.Context, accountID string) error {
	fake.mutex.Lock()
	defer fake.mutex.Unlock()

	if fake.err != nil {
		return fake.err
	}

	fake.deletedAccounts = append(fake.deletedAccounts, accountID)

	return nil
}

func (fake *fakeUsers) ResetPassword(_ context.Context, accountID, password string) error {
	fake.mutex.Lock()
	defer fake.mutex.Unlock()

	if fake.err != nil {
		return fake.err
	}

	if fake.passwords == nil {
		fake.passwords = map[string]string{}
	}

	fake.passwords[accountID] = password

	return nil
}

func (fake *fakeUsers) ListProfiles(_ context.Context, accountID string) ([]users.ProfileRecord, error) {
	fake.mutex.Lock()
	defer fake.mutex.Unlock()

	if fake.err != nil {
		return nil, fake.err
	}

	var owned []users.ProfileRecord

	for _, profile := range fake.profiles {
		if profile.Profile.AccountID == accountID {
			owned = append(owned, profile)
		}
	}

	return owned, nil
}

func (fake *fakeUsers) CreateProfile(_ context.Context, accountID string, input users.CreateProfileInput) (users.ProfileRecord, error) {
	fake.mutex.Lock()
	defer fake.mutex.Unlock()

	if fake.err != nil {
		return users.ProfileRecord{}, fake.err
	}

	fake.createdProfile = input

	color := input.AvatarColor
	if color == "" {
		color = account.DefaultAvatarColor
	}

	return users.ProfileRecord{
		Profile: account.Profile{
			ID: miaID, AccountID: accountID, Name: input.Name, AvatarColor: color, HasPIN: input.PIN != "",
		},
		Libraries: input.Libraries, Limit: input.Limit,
	}, nil
}

// UpdateProfile applies the ownership rule of the use case (ADR 0032 §1): an admin viewer
// or the owning account with its primary profile selected.
func (fake *fakeUsers) UpdateProfile(ctx context.Context, profileID string, patch users.ProfilePatch) (users.ProfileRecord, error) {
	fake.mutex.Lock()
	defer fake.mutex.Unlock()

	if fake.err != nil {
		return users.ProfileRecord{}, fake.err
	}

	for _, profile := range fake.profiles {
		if profile.Profile.ID != profileID {
			continue
		}

		if err := fake.mayManage(ctx, profile.Profile); err != nil {
			return users.ProfileRecord{}, err
		}

		fake.profilePatch = patch
		if patch.Name != nil {
			profile.Profile.Name = *patch.Name
		}

		return profile, nil
	}

	return users.ProfileRecord{}, fmt.Errorf("profile %s: %w", profileID, domain.ErrNotFound)
}

func (fake *fakeUsers) mayManage(ctx context.Context, target account.Profile) error {
	viewer, ok := appaccess.ViewerFrom(ctx)
	if !ok {
		return nil
	}

	if viewer.Admin || (viewer.AccountID == target.AccountID && fake.isPrimary(viewer.ProfileID)) {
		return nil
	}

	return fmt.Errorf("viewer %s may not manage profile %s: %w", viewer.AccountID, target.ID, domain.ErrForbidden)
}

func (fake *fakeUsers) isPrimary(profileID string) bool {
	for _, profile := range fake.profiles {
		if profile.Profile.ID == profileID {
			return profile.Profile.Primary
		}
	}

	return false
}

func (fake *fakeUsers) DeleteProfile(_ context.Context, profileID string) error {
	fake.mutex.Lock()
	defer fake.mutex.Unlock()

	if fake.err != nil {
		return fake.err
	}

	fake.deletedProfiles = append(fake.deletedProfiles, profileID)

	return nil
}

func (fake *fakeUsers) ListAllDevices(context.Context) ([]auth.DeviceView, error) {
	fake.mutex.Lock()
	defer fake.mutex.Unlock()

	return fake.devices, fake.err
}

func (fake *fakeUsers) RevokeDevice(_ context.Context, id string) (bool, error) {
	fake.mutex.Lock()
	defer fake.mutex.Unlock()

	if fake.err != nil {
		return false, fake.err
	}

	fake.revokedDevices = append(fake.revokedDevices, id)

	return false, nil
}

func createdAt() time.Time {
	return time.Date(2026, time.November, 1, 9, 30, 0, 0, time.UTC)
}

func mustLimit(t *testing.T, maxAge access.AgeBucket, hideUnrated bool) access.ContentLimit {
	t.Helper()

	limit, err := access.MaxAgeLimit(maxAge, hideUnrated)
	if err != nil {
		t.Fatalf("MaxAgeLimit(%d): %v", maxAge, err)
	}

	return limit
}

// newFakeUsers returns the household of the tests: the admin Anna and the user Ben, who has
// the managed profile Mia.
func newFakeUsers(t *testing.T) *fakeUsers {
	t.Helper()

	return &fakeUsers{
		summaries: []users.AccountSummary{
			{
				AccountRecord: users.AccountRecord{
					Account:   account.Account{ID: annaID, Name: "Anna", Role: account.RoleAdmin},
					Libraries: access.AllLibraries(), PrimaryLimit: access.NoLimit(),
				},
				CreatedAt: createdAt(), ProfileCount: 1, LastActiveAt: createdAt().Add(time.Hour),
			},
			{
				AccountRecord: users.AccountRecord{
					Account:   account.Account{ID: benID, Name: "Ben", Role: account.RoleUser, Disabled: true},
					Libraries: access.OnlyLibraries(moviesID), PrimaryLimit: mustLimit(t, access.Age12, false),
				},
				CreatedAt: createdAt(), ProfileCount: 2,
			},
		},
		profiles: []users.ProfileRecord{
			{
				Profile: account.Profile{
					ID: benPrimaryID, AccountID: benID, Name: "Ben", Primary: true, AvatarColor: account.AvatarColorBlue,
				},
				Libraries: access.AllLibraries(), Limit: mustLimit(t, access.Age12, false),
			},
			{
				Profile: account.Profile{
					ID: miaID, AccountID: benID, Name: "Mia", AvatarColor: account.AvatarColorCoral, HasPIN: true,
				},
				Libraries: access.OnlyLibraries(moviesID), Limit: mustLimit(t, access.Age16, true),
			},
		},
		devices: []auth.DeviceView{{
			ID: deviceID, AccountID: benID, AccountName: "Ben",
			Device:     auth.Device{Name: "Living room TV", Client: auth.ClientTV, Platform: "webOS"},
			LastSeenAt: createdAt(), AddressClass: auth.AddressLAN, Current: true,
		}},
	}
}

func newUsersHandler(t *testing.T, fake *fakeUsers) http.Handler {
	t.Helper()

	handler, _ := newTestHandlerWithOptions(t, restapi.Options{
		Users: restapi.UserServices{Accounts: fake, Profiles: fake, Devices: fake},
	})

	return handler
}

// serveOperation sends the operation with its JSON body, if any.
func serveOperation(t *testing.T, handler http.Handler, operation userOperation) response {
	t.Helper()

	return serveRequest(t, handler, jsonRequest(t, operation.method, operation.target, operation.body))
}

// userOperation is one operation of the user administration contract with a valid request.
type userOperation struct {
	method string
	target string
	body   string
}

func userOperations() []userOperation {
	return []userOperation{
		{method: http.MethodGet, target: usersTarget},
		{method: http.MethodPost, target: usersTarget, body: `{"name":"Cleo","password":"long enough","role":"user"}`},
		{method: http.MethodGet, target: benTarget},
		{method: http.MethodPatch, target: benTarget, body: `{"status":"disabled"}`},
		{method: http.MethodDelete, target: benTarget},
		{method: http.MethodPut, target: benTarget + "/password", body: `{"password":"long enough"}`},
		{method: http.MethodGet, target: benTarget + "/profiles"},
		{method: http.MethodPost, target: benTarget + "/profiles", body: `{"name":"Mia","pin":"1234"}`},
		{method: http.MethodPatch, target: miaTarget, body: `{"pin":null,"contentLimit":{"preset":"adults","maxAge":null,"hideUnrated":true}}`},
		{method: http.MethodDelete, target: miaTarget},
		{method: http.MethodGet, target: devicesTarget},
		{method: http.MethodDelete, target: devicesTarget + "/" + deviceID},
	}
}
