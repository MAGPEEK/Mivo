package users_test

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/magpeek/mivo/internal/app/users"
	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/access"
	"github.com/magpeek/mivo/internal/domain/account"
)

const (
	adminID          = "admin-account"
	adminPrimaryID   = "admin-primary"
	ownerID          = "owner-account"
	ownerPrimaryID   = "owner-primary"
	ownerManagedID   = "owner-managed"
	otherID          = "other-account"
	otherPrimaryID   = "other-primary"
	otherManagedID   = "other-managed"
	libraryMovies    = "library-movies"
	libraryShows     = "library-shows"
	libraryMusic     = "library-music"
	validPassword    = "correct horse battery"
	validPIN         = "1234"
	fakeHashPrefix   = "hash:"
	decomposedAccent = "e" + string(rune(combiningAcute))
	combiningAcute   = 0x0301
	zeroWidthSpace   = 0x200b
	composedAccent   = "é"
)

// errFake is returned by fakes told to fail.
var errFake = errors.New("fake failure")

// fakeStore is an in-memory account and profile store; its write methods run the callbacks
// under one lock, as the SQLite store runs them inside one transaction.
type fakeStore struct {
	mutex     sync.Mutex
	accounts  map[string]users.AccountRecord
	profiles  map[string]users.ProfileRecord
	passwords map[string]string
	pins      map[string]string
	failList  bool
}

func newFakeStore() *fakeStore {
	return &fakeStore{
		accounts:  map[string]users.AccountRecord{},
		profiles:  map[string]users.ProfileRecord{},
		passwords: map[string]string{},
		pins:      map[string]string{},
	}
}

func (store *fakeStore) addAccount(record users.AccountRecord, primaryID string) {
	store.accounts[record.Account.ID] = record
	store.profiles[primaryID] = users.ProfileRecord{
		Profile: account.Profile{
			ID: primaryID, AccountID: record.Account.ID, Name: record.Account.Name, Primary: true,
			AvatarColor: account.DefaultAvatarColor,
		},
		Libraries: access.AllLibraries(),
		Limit:     record.PrimaryLimit,
	}
}

func (store *fakeStore) addManaged(profileID, accountID string) {
	store.profiles[profileID] = users.ProfileRecord{
		Profile: account.Profile{
			ID: profileID, AccountID: accountID, Name: "Kid " + profileID,
			AvatarColor: account.AvatarColorBlue,
		},
		Libraries: access.AllLibraries(),
		Limit:     access.NoLimit(),
	}
}

func (store *fakeStore) ListAccounts(context.Context) ([]users.AccountSummary, error) {
	store.mutex.Lock()
	defer store.mutex.Unlock()

	if store.failList {
		return nil, errFake
	}

	summaries := make([]users.AccountSummary, 0, len(store.accounts))
	for _, record := range store.accounts {
		summaries = append(summaries, users.AccountSummary{
			AccountRecord: record, ProfileCount: store.countProfiles(record.Account.ID),
		})
	}

	slices.SortFunc(summaries, func(first, second users.AccountSummary) int {
		return strings.Compare(first.Account.Name, second.Account.Name)
	})

	return summaries, nil
}

func (store *fakeStore) GetAccount(_ context.Context, accountID string) (users.AccountRecord, error) {
	store.mutex.Lock()
	defer store.mutex.Unlock()

	record, ok := store.accounts[accountID]
	if !ok {
		return users.AccountRecord{}, fmt.Errorf("account %s: %w", accountID, domain.ErrNotFound)
	}

	return record, nil
}

func (store *fakeStore) FindAccountByNameKey(_ context.Context, nameKey string) (users.AccountRecord, error) {
	store.mutex.Lock()
	defer store.mutex.Unlock()

	for _, record := range store.accounts {
		if account.NameKey(record.Account.Name) == nameKey {
			return record, nil
		}
	}

	return users.AccountRecord{}, fmt.Errorf("account name: %w", domain.ErrNotFound)
}

func (store *fakeStore) CreateAccount(_ context.Context, record users.NewAccountRecord) error {
	store.mutex.Lock()
	defer store.mutex.Unlock()

	if store.nameTaken(record.Account.Account) {
		return fmt.Errorf("name: %w", domain.ErrConflict)
	}

	store.accounts[record.Account.Account.ID] = record.Account
	store.profiles[record.Primary.ID] = users.ProfileRecord{
		Profile: record.Primary, Libraries: access.AllLibraries(), Limit: record.Account.PrimaryLimit,
	}
	store.passwords[record.Account.Account.ID] = record.PasswordHash

	return nil
}

func (store *fakeStore) UpdateAccount(
	_ context.Context, accountID string, change users.AccountChange,
) (users.AccountRecord, error) {
	store.mutex.Lock()
	defer store.mutex.Unlock()

	current, ok := store.accounts[accountID]
	if !ok {
		return users.AccountRecord{}, domain.ErrNotFound
	}

	next, err := change(current, store.enabledAdmins())
	if err != nil {
		return users.AccountRecord{}, err
	}

	if store.nameTaken(next.Account) {
		return users.AccountRecord{}, fmt.Errorf("name: %w", domain.ErrConflict)
	}

	store.accounts[accountID] = next

	return next, nil
}

func (store *fakeStore) DeleteAccount(_ context.Context, accountID string, check users.AccountDeletionCheck) error {
	store.mutex.Lock()
	defer store.mutex.Unlock()

	current, ok := store.accounts[accountID]
	if !ok {
		return domain.ErrNotFound
	}

	if err := check(current.Account, store.enabledAdmins()); err != nil {
		return err
	}

	delete(store.accounts, accountID)

	for profileID, profile := range store.profiles {
		if profile.Profile.AccountID == accountID {
			delete(store.profiles, profileID)
		}
	}

	return nil
}

func (store *fakeStore) SetPasswordHash(_ context.Context, accountID, hash string) error {
	store.mutex.Lock()
	defer store.mutex.Unlock()

	store.passwords[accountID] = hash

	return nil
}

func (store *fakeStore) GetProfile(_ context.Context, profileID string) (users.ProfileRecord, error) {
	store.mutex.Lock()
	defer store.mutex.Unlock()

	profile, ok := store.profiles[profileID]
	if !ok {
		return users.ProfileRecord{}, fmt.Errorf("profile %s: %w", profileID, domain.ErrNotFound)
	}

	return profile, nil
}

func (store *fakeStore) ListProfiles(_ context.Context, accountID string) ([]users.ProfileRecord, error) {
	store.mutex.Lock()
	defer store.mutex.Unlock()

	var profiles []users.ProfileRecord

	for _, profile := range store.profiles {
		if profile.Profile.AccountID == accountID {
			profiles = append(profiles, profile)
		}
	}

	slices.SortFunc(profiles, func(first, second users.ProfileRecord) int {
		if first.Profile.Primary != second.Profile.Primary {
			if first.Profile.Primary {
				return -1
			}

			return 1
		}

		return strings.Compare(first.Profile.ID, second.Profile.ID)
	})

	return profiles, nil
}

func (store *fakeStore) CreateProfile(_ context.Context, write users.ProfileWrite, check users.ManagedProfileCheck) error {
	store.mutex.Lock()
	defer store.mutex.Unlock()

	if err := check(store.countProfiles(write.Record.Profile.AccountID) - 1); err != nil {
		return err
	}

	store.storeProfile(write)

	return nil
}

func (store *fakeStore) UpdateProfile(_ context.Context, write users.ProfileWrite) error {
	store.mutex.Lock()
	defer store.mutex.Unlock()

	if _, ok := store.profiles[write.Record.Profile.ID]; !ok {
		return domain.ErrNotFound
	}

	store.storeProfile(write)

	return nil
}

func (store *fakeStore) DeleteProfile(_ context.Context, profileID string) error {
	store.mutex.Lock()
	defer store.mutex.Unlock()

	delete(store.profiles, profileID)
	delete(store.pins, profileID)

	return nil
}

func (store *fakeStore) storeProfile(write users.ProfileWrite) {
	store.profiles[write.Record.Profile.ID] = write.Record

	switch write.PINAction {
	case users.PINSet:
		store.pins[write.Record.Profile.ID] = write.PINHash
	case users.PINClear:
		delete(store.pins, write.Record.Profile.ID)
	case users.PINKeep:
	}
}

func (store *fakeStore) countProfiles(accountID string) int {
	count := 0

	for _, profile := range store.profiles {
		if profile.Profile.AccountID == accountID {
			count++
		}
	}

	return count
}

func (store *fakeStore) enabledAdmins() int {
	identities := make([]account.Account, 0, len(store.accounts))
	for _, record := range store.accounts {
		identities = append(identities, record.Account)
	}

	return account.CountEnabledAdmins(identities)
}

func (store *fakeStore) nameTaken(candidate account.Account) bool {
	for _, record := range store.accounts {
		if record.Account.ID != candidate.ID && account.NameKey(record.Account.Name) == account.NameKey(candidate.Name) {
			return true
		}
	}

	return false
}

// fakeHasher marks secrets instead of hashing them.
type fakeHasher struct{ fail bool }

func (hasher fakeHasher) Hash(_ context.Context, secret string) (string, error) {
	if hasher.fail {
		return "", errFake
	}

	return fakeHashPrefix + secret, nil
}

// fakeIDs counts up.
type fakeIDs struct {
	mutex sync.Mutex
	next  int
	fail  bool
}

func (ids *fakeIDs) NewID() (string, error) {
	ids.mutex.Lock()
	defer ids.mutex.Unlock()

	if ids.fail {
		return "", errFake
	}

	ids.next++

	return fmt.Sprintf("id-%d", ids.next), nil
}

// fakeSessions records revoked accounts.
type fakeSessions struct {
	revoked []string
	fail    bool
}

func (sessions *fakeSessions) RevokeAccountSessions(_ context.Context, accountID string) error {
	if sessions.fail {
		return errFake
	}

	sessions.revoked = append(sessions.revoked, accountID)

	return nil
}

// fakeCache records evicted accounts.
type fakeCache struct{ evicted []string }

func (cache *fakeCache) EvictAccount(accountID string) {
	cache.evicted = append(cache.evicted, accountID)
}

type viewerKey struct{}

func withViewer(ctx context.Context, viewer access.Viewer) context.Context {
	return context.WithValue(ctx, viewerKey{}, viewer)
}

func viewerFrom(ctx context.Context) (access.Viewer, bool) {
	viewer, ok := ctx.Value(viewerKey{}).(access.Viewer)

	return viewer, ok
}

// composeAccents stands in for norm.NFC.String with the one composition the tests use.
func composeAccents(text string) string {
	return strings.ReplaceAll(text, decomposedAccent, composedAccent)
}

// fixture is a service over fakes with an admin, an owner (one managed profile, two
// libraries) and another user (one managed profile).
type fixture struct {
	service  *users.Service
	store    *fakeStore
	ids      *fakeIDs
	sessions *fakeSessions
	cache    *fakeCache
}

func newFixture(t *testing.T) *fixture {
	t.Helper()

	store := newFakeStore()
	store.addAccount(users.AccountRecord{
		Account:   account.Account{ID: adminID, Name: "Admin", Role: account.RoleAdmin},
		Libraries: access.AllLibraries(), PrimaryLimit: access.NoLimit(),
	}, adminPrimaryID)
	store.addAccount(users.AccountRecord{
		Account:   account.Account{ID: ownerID, Name: "Owner", Role: account.RoleUser},
		Libraries: access.OnlyLibraries(libraryMovies, libraryShows), PrimaryLimit: access.NoLimit(),
	}, ownerPrimaryID)
	store.addManaged(ownerManagedID, ownerID)
	store.addAccount(users.AccountRecord{
		Account:   account.Account{ID: otherID, Name: "Other", Role: account.RoleUser},
		Libraries: access.AllLibraries(), PrimaryLimit: access.NoLimit(),
	}, otherPrimaryID)
	store.addManaged(otherManagedID, otherID)

	ids := &fakeIDs{}
	sessions := &fakeSessions{}
	cache := &fakeCache{}

	service, err := users.New(fixtureOptions(store, ids, sessions, cache))
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	return &fixture{service: service, store: store, ids: ids, sessions: sessions, cache: cache}
}

func fixtureOptions(store *fakeStore, ids *fakeIDs, sessions *fakeSessions, cache *fakeCache) users.Options {
	return users.Options{
		Accounts: store, Writer: store, Password: store, Profiles: store, Profile: store,
		Hasher: fakeHasher{}, IDs: ids, Sessions: sessions, Cache: cache,
		Viewers: viewerFrom, Compose: composeAccents,
	}
}

func adminContext() context.Context {
	return withViewer(context.Background(), access.Viewer{
		AccountID: adminID, ProfileID: adminPrimaryID, Admin: true,
	})
}

func ownerContext() context.Context {
	return withViewer(context.Background(), access.Viewer{AccountID: ownerID, ProfileID: ownerPrimaryID})
}

func ownerManagedContext() context.Context {
	return withViewer(context.Background(), access.Viewer{AccountID: ownerID, ProfileID: ownerManagedID})
}

func wantError(t *testing.T, err, target error) {
	t.Helper()

	if !errors.Is(err, target) {
		t.Fatalf("error = %v, want %v", err, target)
	}
}

func wantNoError(t *testing.T, err error) {
	t.Helper()

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func pointer[T any](value T) *T {
	return &value
}
