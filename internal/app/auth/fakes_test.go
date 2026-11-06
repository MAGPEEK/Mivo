package auth

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/access"
	"github.com/magpeek/mivo/internal/domain/account"
)

var errStoreDown = errors.New("store down")

// fakeStore is an in-memory implementation of every store port.
type fakeStore struct {
	mutex          sync.Mutex
	credentials    map[string]Credentials // by account ID
	profiles       map[string][]account.Profile
	accountAccess  map[string]access.LibraryAccess
	profileAccess  map[string]access.LibraryAccess
	profileLimits  map[string]access.ContentLimit
	sessions       map[string]Session // by token hash
	sessionLookups int
	touches        []Seen
	purgeCutoffs   [2]time.Time
	failLookup     error
	failSetHash    error
	failTouch      error
}

func newFakeStore() *fakeStore {
	return &fakeStore{
		credentials:   map[string]Credentials{},
		profiles:      map[string][]account.Profile{},
		accountAccess: map[string]access.LibraryAccess{},
		profileAccess: map[string]access.LibraryAccess{},
		profileLimits: map[string]access.ContentLimit{},
		sessions:      map[string]Session{},
	}
}

func (store *fakeStore) HasAccounts(context.Context) (bool, error) {
	store.mutex.Lock()
	defer store.mutex.Unlock()

	return len(store.credentials) > 0, nil
}

func (store *fakeStore) CreateFirstAdmin(_ context.Context, admin FirstAdmin) error {
	store.mutex.Lock()
	defer store.mutex.Unlock()

	if len(store.credentials) > 0 {
		return fmt.Errorf("users not empty: %w", domain.ErrConflict)
	}

	store.credentials[admin.Account.ID] = Credentials{Account: admin.Account, PasswordHash: admin.PasswordHash}
	store.profiles[admin.Account.ID] = []account.Profile{admin.Profile}
	store.accountAccess[admin.Account.ID] = access.AllLibraries()

	return nil
}

func (store *fakeStore) CredentialsByName(_ context.Context, nameKey string) (Credentials, error) {
	store.mutex.Lock()
	defer store.mutex.Unlock()

	if store.failLookup != nil {
		return Credentials{}, store.failLookup
	}

	for _, credentials := range store.credentials {
		if account.NameKey(credentials.Account.Name) == nameKey {
			return credentials, nil
		}
	}

	return Credentials{}, fmt.Errorf("name: %w", domain.ErrNotFound)
}

func (store *fakeStore) CredentialsByID(_ context.Context, accountID string) (Credentials, error) {
	store.mutex.Lock()
	defer store.mutex.Unlock()

	credentials, ok := store.credentials[accountID]
	if !ok {
		return Credentials{}, fmt.Errorf("account %s: %w", accountID, domain.ErrNotFound)
	}

	return credentials, nil
}

func (store *fakeStore) SetPasswordHash(_ context.Context, accountID, passwordHash string) error {
	store.mutex.Lock()
	defer store.mutex.Unlock()

	if store.failSetHash != nil {
		return store.failSetHash
	}

	credentials := store.credentials[accountID]
	credentials.PasswordHash = passwordHash
	store.credentials[accountID] = credentials

	return nil
}

func (store *fakeStore) Profiles(_ context.Context, accountID string) ([]account.Profile, error) {
	store.mutex.Lock()
	defer store.mutex.Unlock()

	return append([]account.Profile(nil), store.profiles[accountID]...), nil
}

func (store *fakeStore) ViewerInputs(_ context.Context, accountID, profileID string) (ViewerInputs, error) {
	store.mutex.Lock()
	defer store.mutex.Unlock()

	credentials, ok := store.credentials[accountID]
	if !ok {
		return ViewerInputs{}, fmt.Errorf("account %s: %w", accountID, domain.ErrNotFound)
	}

	inputs := ViewerInputs{Account: credentials.Account, AccountLibraries: store.accountAccess[accountID]}

	for _, profile := range store.profiles[accountID] {
		if profile.Primary {
			inputs.PrimaryLimit = store.profileLimits[profile.ID]
		}

		if profile.ID == profileID {
			inputs.Profile = profile
			inputs.ProfileLibraries = store.profileAccess[profile.ID]
			inputs.ProfileLimit = store.profileLimits[profile.ID]
		}
	}

	if profileID != "" && inputs.Profile.ID == "" {
		return ViewerInputs{}, fmt.Errorf("profile %s: %w", profileID, domain.ErrNotFound)
	}

	return inputs, nil
}

func (store *fakeStore) CreateSession(_ context.Context, session Session) error {
	store.mutex.Lock()
	defer store.mutex.Unlock()

	store.sessions[session.TokenHash] = session

	return nil
}

func (store *fakeStore) SessionByTokenHash(_ context.Context, tokenHash string) (Session, error) {
	store.mutex.Lock()
	defer store.mutex.Unlock()

	store.sessionLookups++

	session, ok := store.sessions[tokenHash]
	if !ok {
		return Session{}, fmt.Errorf("session: %w", domain.ErrNotFound)
	}

	return session, nil
}

func (store *fakeStore) TouchSession(_ context.Context, sessionID string, seen Seen) error {
	store.mutex.Lock()
	defer store.mutex.Unlock()

	if store.failTouch != nil {
		return store.failTouch
	}

	store.touches = append(store.touches, seen)

	for tokenHash, session := range store.sessions {
		if session.ID == sessionID {
			session.LastSeenAt, session.AddressClass = seen.At, seen.AddressClass
			store.sessions[tokenHash] = session
		}
	}

	return nil
}

func (store *fakeStore) DeleteSession(_ context.Context, sessionID string) error {
	store.mutex.Lock()
	defer store.mutex.Unlock()

	for tokenHash, session := range store.sessions {
		if session.ID == sessionID {
			delete(store.sessions, tokenHash)
		}
	}

	return nil
}

func (store *fakeStore) DeleteOtherSessions(_ context.Context, accountID, keepSessionID string) error {
	store.mutex.Lock()
	defer store.mutex.Unlock()

	for tokenHash, session := range store.sessions {
		if session.AccountID == accountID && session.ID != keepSessionID {
			delete(store.sessions, tokenHash)
		}
	}

	return nil
}

func (store *fakeStore) PurgeSessions(_ context.Context, idleBefore, createdBefore time.Time) (int64, error) {
	store.mutex.Lock()
	defer store.mutex.Unlock()

	store.purgeCutoffs = [2]time.Time{idleBefore, createdBefore}

	var deleted int64

	for tokenHash, session := range store.sessions {
		if !session.LastSeenAt.After(idleBefore) || !session.CreatedAt.After(createdBefore) {
			delete(store.sessions, tokenHash)
			deleted++
		}
	}

	return deleted, nil
}

func (store *fakeStore) passwordHash(accountID string) string {
	store.mutex.Lock()
	defer store.mutex.Unlock()

	return store.credentials[accountID].PasswordHash
}

func (store *fakeStore) setDisabled(accountID string, disabled bool) {
	store.mutex.Lock()
	defer store.mutex.Unlock()

	credentials := store.credentials[accountID]
	credentials.Account.Disabled = disabled
	store.credentials[accountID] = credentials
}

func (store *fakeStore) lookups() int {
	store.mutex.Lock()
	defer store.mutex.Unlock()

	return store.sessionLookups
}

// addAccount stores an account with a primary profile whose ID is accountID+"-primary".
func (store *fakeStore) addAccount(accountID, name, passwordHash string, role account.Role) account.Profile {
	store.mutex.Lock()
	defer store.mutex.Unlock()

	store.credentials[accountID] = Credentials{
		Account:      account.Account{ID: accountID, Name: name, Role: role},
		PasswordHash: passwordHash,
	}
	primary := account.Profile{
		ID: accountID + "-primary", AccountID: accountID, Name: name, Primary: true,
		AvatarColor: account.DefaultAvatarColor,
	}
	store.profiles[accountID] = []account.Profile{primary}
	store.accountAccess[accountID] = access.AllLibraries()

	return primary
}

func (store *fakeStore) addProfile(profile account.Profile) {
	store.mutex.Lock()
	defer store.mutex.Unlock()

	store.profiles[profile.AccountID] = append(store.profiles[profile.AccountID], profile)
}

// Fake hashes: "hash:<secret>" verifies, "weak:<secret>" verifies and asks for a rehash,
// anything else that does not start with a known prefix is corrupt.
const (
	hashPrefix = "hash:"
	weakPrefix = "weak:"
)

type fakeHasher struct {
	mutex    sync.Mutex
	hashes   int
	verifies int
	dummies  int
	failHash error
	gate     chan struct{} // when set, Verify waits for it
}

func (hasher *fakeHasher) Hash(_ context.Context, secret string) (string, error) {
	hasher.mutex.Lock()
	defer hasher.mutex.Unlock()

	hasher.hashes++

	if hasher.failHash != nil {
		return "", hasher.failHash
	}

	return hashPrefix + secret, nil
}

func (hasher *fakeHasher) Verify(ctx context.Context, secret, encoded string) (bool, bool, error) {
	hasher.mutex.Lock()
	hasher.verifies++
	gate := hasher.gate
	hasher.mutex.Unlock()

	if gate != nil {
		<-gate
	}

	if err := ctx.Err(); err != nil {
		return false, false, err
	}

	switch {
	case strings.HasPrefix(encoded, hashPrefix):
		return encoded == hashPrefix+secret, false, nil
	case strings.HasPrefix(encoded, weakPrefix):
		matched := encoded == weakPrefix+secret

		return matched, matched, nil
	default:
		return false, false, fmt.Errorf("parse hash: %w", domain.ErrInvalid)
	}
}

func (hasher *fakeHasher) DummyVerify(ctx context.Context, _ string) error {
	hasher.mutex.Lock()
	hasher.dummies++
	hasher.mutex.Unlock()

	return ctx.Err()
}

func (hasher *fakeHasher) counts() (hashes, verifies, dummies int) {
	hasher.mutex.Lock()
	defer hasher.mutex.Unlock()

	return hasher.hashes, hasher.verifies, hasher.dummies
}

type fakeTokens struct {
	mutex sync.Mutex
	next  int
}

func (tokens *fakeTokens) New() (string, string, error) {
	tokens.mutex.Lock()
	defer tokens.mutex.Unlock()

	tokens.next++
	token := fmt.Sprintf("token-%d", tokens.next)

	return token, tokens.HashToken(token), nil
}

func (*fakeTokens) HashToken(token string) string {
	return "sha:" + token
}

type fakeIDs struct {
	mutex sync.Mutex
	next  int
}

func (ids *fakeIDs) NewID() (string, error) {
	ids.mutex.Lock()
	defer ids.mutex.Unlock()

	ids.next++

	return fmt.Sprintf("id-%d", ids.next), nil
}

type fakeClock struct {
	mutex sync.Mutex
	now   time.Time
}

func newFakeClock() *fakeClock {
	return &fakeClock{now: time.Date(2026, time.November, 6, 12, 0, 0, 0, time.UTC)}
}

func (clock *fakeClock) Now() time.Time {
	clock.mutex.Lock()
	defer clock.mutex.Unlock()

	return clock.now
}

func (clock *fakeClock) Advance(step time.Duration) {
	clock.mutex.Lock()
	defer clock.mutex.Unlock()

	clock.now = clock.now.Add(step)
}

// composeAcute composes "e" + combining acute into "é", so tests can tell whether the
// injected composer runs.
func composeAcute(text string) string {
	return strings.ReplaceAll(text, "é", "é")
}

type harness struct {
	service *Service
	store   *fakeStore
	hasher  *fakeHasher
	clock   *fakeClock
}

func newHarness(t *testing.T) harness {
	t.Helper()

	store := newFakeStore()
	hasher := &fakeHasher{}
	clock := newFakeClock()

	service, err := NewService(Dependencies{
		Setup: store, Credentials: store, Profiles: store, Sessions: store, Revoker: store,
		Hasher: hasher, Tokens: &fakeTokens{}, IDs: &fakeIDs{}, Clock: clock.Now, Compose: composeAcute,
	})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	return harness{service: service, store: store, hasher: hasher, clock: clock}
}

func lanPeer() Peer {
	return Peer{Address: netip.MustParseAddr("192.168.1.20"), Class: AddressLAN}
}

func otherPeer() Peer {
	return Peer{Address: netip.MustParseAddr("192.168.1.30"), Class: AddressLAN}
}

func webDevice() Device {
	return Device{Name: "Firefox on Windows", Client: ClientWeb, Platform: "windows"}
}

func signInRequest(name, password string) SignInRequest {
	return SignInRequest{Name: name, Password: password, Device: webDevice(), Peer: lanPeer()}
}
