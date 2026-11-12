package sqlite_test

import (
	"database/sql"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/magpeek/mivo/internal/adapters/sqlite"
	"github.com/magpeek/mivo/internal/app/auth"
	"github.com/magpeek/mivo/internal/app/users"
	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/account"
)

// The store implements the session ports of app/auth and app/users.
var (
	_ auth.SessionStore    = (*sqlite.SessionStore)(nil)
	_ auth.SessionRevoker  = (*sqlite.SessionStore)(nil)
	_ auth.ProfileSelector = (*sqlite.SessionStore)(nil)
	_ auth.DeviceStore     = (*sqlite.SessionStore)(nil)
	_ users.SessionRevoker = (*sqlite.SessionStore)(nil)
)

// sessionFixture is a migrated database with accounts Anna (admin) and Ben (user, with a
// managed profile Kid), a session store and an inspection connection.
type sessionFixture struct {
	sessions   *sqlite.SessionStore
	accounts   *sqlite.AccountStore
	inspection *sql.DB
	clock      *testClock
}

const benKidProfileID = "profile-Ben-Kid"

func newSessionFixture(t *testing.T) sessionFixture {
	t.Helper()

	db, path := openMigratedTestDB(t)
	fixture := sessionFixture{
		sessions:   sqlite.NewSessionStore(db),
		accounts:   sqlite.NewAccountStore(db, nil),
		inspection: openInspectionDB(t, path),
		clock:      newTestClock(),
	}

	for name, role := range map[string]account.Role{"Anna": account.RoleAdmin, "Ben": account.RoleUser} {
		if err := fixture.accounts.CreateAccount(t.Context(), newAccount(name, role)); err != nil {
			t.Fatalf("CreateAccount(%s) error = %v", name, err)
		}
	}

	mustCreateProfile(t, fixture.accounts, managedWrite("Ben", "Kid"))

	return fixture
}

// newSession returns a session of "account-<owner>" whose ids derive from id, created and last
// seen at the fixture clock's time.
func (fixture sessionFixture) newSession(id, owner, profileID string) auth.Session {
	now := fixture.clock.Now()

	return auth.Session{
		ID: id, TokenHash: "hash-" + id, AccountID: "account-" + owner, ProfileID: profileID,
		Remember:  profileID != "",
		Device:    auth.Device{Name: "Laptop " + id, Client: auth.ClientWeb, Platform: "Windows"},
		CreatedAt: now, LastSeenAt: now, AddressClass: auth.AddressLAN,
	}
}

func (fixture sessionFixture) mustCreate(t *testing.T, session auth.Session) {
	t.Helper()

	if err := fixture.sessions.CreateSession(t.Context(), session); err != nil {
		t.Fatalf("CreateSession(%s) error = %v", session.ID, err)
	}
}

func (fixture sessionFixture) mustResolve(t *testing.T, tokenHash string) sqlite.SessionDetails {
	t.Helper()

	details, err := fixture.sessions.SessionDetailsByTokenHash(t.Context(), tokenHash)
	if err != nil {
		t.Fatalf("SessionDetailsByTokenHash error = %v", err)
	}

	return details
}

func TestSessionCreateAndResolveRoundTrip(t *testing.T) {
	t.Parallel()

	fixture := newSessionFixture(t)
	want := fixture.newSession("s1", "Ben", benKidProfileID)
	fixture.mustCreate(t, want)

	details := fixture.mustResolve(t, want.TokenHash)

	if !sessionsEqual(details.Session, want) {
		t.Errorf("session = %+v, want %+v", details.Session, want)
	}

	wantAccount := account.Account{ID: "account-Ben", Name: "Ben", Role: account.RoleUser}
	if details.Account != wantAccount {
		t.Errorf("account = %+v, want %+v", details.Account, wantAccount)
	}

	if !details.ProfileSelected || details.Profile.ID != benKidProfileID || details.Profile.Name != "Kid" ||
		details.Profile.Primary || details.Profile.AccountID != "account-Ben" ||
		details.Profile.AvatarColor != account.AvatarColorTeal {
		t.Errorf("profile = %+v (selected %t), want Ben's Kid", details.Profile, details.ProfileSelected)
	}

	plain, err := fixture.sessions.SessionByTokenHash(t.Context(), want.TokenHash)
	if err != nil || !sessionsEqual(plain, want) {
		t.Errorf("SessionByTokenHash = %+v, %v, want %+v", plain, err, want)
	}
}

func TestSessionByID(t *testing.T) {
	t.Parallel()

	fixture := newSessionFixture(t)
	want := fixture.newSession("s1", "Ben", benKidProfileID)
	fixture.mustCreate(t, want)

	got, err := fixture.sessions.SessionByID(t.Context(), "s1")
	if err != nil || !sessionsEqual(got, want) {
		t.Errorf("SessionByID = %+v, %v, want %+v", got, err, want)
	}

	if _, err := fixture.sessions.SessionByID(t.Context(), "unknown"); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("SessionByID(unknown) = %v, want ErrNotFound", err)
	}
}

func TestSessionResolveWithoutProfileAndDisabledAccount(t *testing.T) {
	t.Parallel()

	fixture := newSessionFixture(t)
	fixture.mustCreate(t, fixture.newSession("s1", "Ben", ""))
	execute(t, fixture.inspection, "UPDATE users SET disabled = 1 WHERE id = 'account-Ben'")

	details := fixture.mustResolve(t, "hash-s1")

	if details.ProfileSelected || details.Profile != (account.Profile{}) || details.Session.ProfileID != "" {
		t.Errorf("profile = %+v (selected %t), want none", details.Profile, details.ProfileSelected)
	}

	if !details.Account.Disabled {
		t.Errorf("account disabled = false, want true (the caller decides)")
	}
}

func TestSessionResolveUnknownHashIsNotFound(t *testing.T) {
	t.Parallel()

	fixture := newSessionFixture(t)

	_, err := fixture.sessions.SessionDetailsByTokenHash(t.Context(), "hash-unknown")
	wantError(t, "SessionDetailsByTokenHash", err, domain.ErrNotFound)

	_, err = fixture.sessions.SessionByTokenHash(t.Context(), "hash-unknown")
	wantError(t, "SessionByTokenHash", err, domain.ErrNotFound)
}

// A profile id stored for another account (only possible by writing the row directly) reads
// as no selection.
func TestSessionResolveIgnoresAProfileOfAnotherAccount(t *testing.T) {
	t.Parallel()

	fixture := newSessionFixture(t)
	fixture.mustCreate(t, fixture.newSession("s1", "Anna", ""))
	execute(t, fixture.inspection, "UPDATE auth_sessions SET profile_id = ? WHERE id = 's1'", benKidProfileID)

	details := fixture.mustResolve(t, "hash-s1")

	if details.ProfileSelected || details.Session.ProfileID != "" {
		t.Errorf("profile = %q (selected %t), want none", details.Session.ProfileID, details.ProfileSelected)
	}
}

func TestSessionCreateRejectsInvalidReferencesAndDuplicates(t *testing.T) {
	t.Parallel()

	fixture := newSessionFixture(t)
	fixture.mustCreate(t, fixture.newSession("s1", "Ben", ""))

	duplicateToken := fixture.newSession("s2", "Ben", "")
	duplicateToken.TokenHash = "hash-s1"

	cases := []struct {
		name    string
		session auth.Session
		want    error
	}{
		{"profile of another account", fixture.newSession("s3", "Anna", benKidProfileID), domain.ErrNotFound},
		{"unknown profile", fixture.newSession("s4", "Ben", "profile-missing"), domain.ErrNotFound},
		{"unknown account", fixture.newSession("s5", "Nobody", ""), domain.ErrNotFound},
		{"duplicate id", fixture.newSession("s1", "Ben", ""), domain.ErrConflict},
		{"duplicate token hash", duplicateToken, domain.ErrConflict},
	}

	for _, testCase := range cases {
		err := fixture.sessions.CreateSession(t.Context(), testCase.session)
		wantError(t, testCase.name, err, testCase.want)
	}

	if got := countRows(t, fixture.inspection, "SELECT COUNT(*) FROM auth_sessions"); got != 1 {
		t.Errorf("stored sessions = %d, want 1", got)
	}
}

func TestSessionTouchIsThrottled(t *testing.T) {
	t.Parallel()

	fixture := newSessionFixture(t)
	created := fixture.newSession("s1", "Ben", "")
	fixture.mustCreate(t, created)

	steps := []struct {
		name     string
		after    time.Duration
		class    auth.AddressClass
		wantSeen time.Duration
	}{
		{"inside the interval", auth.TouchInterval - time.Millisecond, auth.AddressLAN, 0},
		{"at the interval", auth.TouchInterval, auth.AddressLAN, auth.TouchInterval},
		{"again inside", auth.TouchInterval + time.Minute, auth.AddressLAN, auth.TouchInterval},
		{"class changed inside", auth.TouchInterval + 2*time.Minute, auth.AddressRemote, auth.TouchInterval + 2*time.Minute},
	}

	for _, step := range steps {
		seen := auth.Seen{At: created.CreatedAt.Add(step.after), AddressClass: step.class}
		if err := fixture.sessions.TouchSession(t.Context(), created.ID, seen); err != nil {
			t.Fatalf("%s: TouchSession error = %v", step.name, err)
		}

		got := fixture.mustResolve(t, created.TokenHash).Session
		if want := created.CreatedAt.Add(step.wantSeen); !got.LastSeenAt.Equal(want) {
			t.Errorf("%s: last seen = %v, want %v", step.name, got.LastSeenAt, want)
		}
	}

	if got := fixture.mustResolve(t, created.TokenHash).Session.AddressClass; got != auth.AddressRemote {
		t.Errorf("address class = %q, want remote", got)
	}

	err := fixture.sessions.TouchSession(t.Context(), "unknown", auth.Seen{At: created.CreatedAt, AddressClass: auth.AddressLAN})
	if err != nil {
		t.Errorf("TouchSession(unknown) error = %v, want nil", err)
	}
}

func TestSessionSelectAndClearProfile(t *testing.T) {
	t.Parallel()

	fixture := newSessionFixture(t)
	ctx := t.Context()
	fixture.mustCreate(t, fixture.newSession("s1", "Ben", ""))
	fixture.mustCreate(t, fixture.newSession("s2", "Anna", ""))

	if err := fixture.sessions.SelectProfile(ctx, "s1", benKidProfileID, true); err != nil {
		t.Fatalf("SelectProfile error = %v", err)
	}

	if got := fixture.mustResolve(t, "hash-s1"); got.Profile.ID != benKidProfileID || !got.Session.Remember {
		t.Errorf("after select: profile %q remember %t, want %s and true", got.Profile.ID, got.Session.Remember, benKidProfileID)
	}

	if err := fixture.sessions.ClearProfile(ctx, "s1"); err != nil {
		t.Fatalf("ClearProfile error = %v", err)
	}

	if got := fixture.mustResolve(t, "hash-s1"); got.ProfileSelected || got.Session.Remember {
		t.Errorf("after clear: selected %t remember %t, want false and false", got.ProfileSelected, got.Session.Remember)
	}

	wantError(t, "select another account's profile", fixture.sessions.SelectProfile(ctx, "s2", benKidProfileID, false), domain.ErrNotFound)
	wantError(t, "select unknown profile", fixture.sessions.SelectProfile(ctx, "s1", "profile-missing", false), domain.ErrNotFound)
	wantError(t, "select for unknown session", fixture.sessions.SelectProfile(ctx, "unknown", "profile-Ben", false), domain.ErrNotFound)
	wantError(t, "clear unknown session", fixture.sessions.ClearProfile(ctx, "unknown"), domain.ErrNotFound)

	if got := fixture.mustResolve(t, "hash-s2"); got.ProfileSelected {
		t.Errorf("Anna's session selected %q, want none", got.Profile.ID)
	}
}

func sessionsEqual(got, want auth.Session) bool {
	got.CreatedAt, got.LastSeenAt = got.CreatedAt.UTC(), got.LastSeenAt.UTC()
	want.CreatedAt, want.LastSeenAt = want.CreatedAt.UTC(), want.LastSeenAt.UTC()

	return got == want
}

func sessionIDs(sessions []auth.Session) []string {
	ids := make([]string, 0, len(sessions))
	for _, session := range sessions {
		ids = append(ids, session.ID)
	}

	return ids
}

func sortedHashes(hashes []string) []string {
	sorted := slices.Clone(hashes)
	slices.Sort(sorted)

	return sorted
}
