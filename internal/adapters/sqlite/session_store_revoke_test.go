package sqlite_test

import (
	"slices"
	"testing"
	"time"

	"github.com/magpeek/mivo/internal/app/auth"
)

// sessionAge is how long before the fixture clock a session was created and last seen.
type sessionAge struct {
	created, seen time.Duration
}

// createAged stores a session of the given age.
func (fixture sessionFixture) createAged(t *testing.T, id, owner string, age sessionAge) {
	t.Helper()

	session := fixture.newSession(id, owner, "")
	session.CreatedAt = session.CreatedAt.Add(-age.created)
	session.LastSeenAt = session.LastSeenAt.Add(-age.seen)
	fixture.mustCreate(t, session)
}

func (fixture sessionFixture) remainingSessionIDs(t *testing.T) []string {
	t.Helper()

	rows, err := fixture.inspection.QueryContext(t.Context(), "SELECT id FROM auth_sessions ORDER BY id")
	if err != nil {
		t.Fatalf("query sessions: %v", err)
	}

	defer func() { _ = rows.Close() }()

	var ids []string

	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			t.Fatalf("scan session id: %v", err)
		}

		ids = append(ids, id)
	}

	if err := rows.Err(); err != nil {
		t.Fatalf("read sessions: %v", err)
	}

	return ids
}

func TestSessionListOfAccountAndOfAllAccounts(t *testing.T) {
	t.Parallel()

	fixture := newSessionFixture(t)
	fixture.createAged(t, "ben-old", "Ben", sessionAge{time.Hour, time.Hour})
	fixture.createAged(t, "ben-new", "Ben", sessionAge{time.Hour, time.Minute})
	fixture.createAged(t, "anna", "Anna", sessionAge{time.Hour, 2 * time.Minute})

	own, err := fixture.sessions.AccountSessions(t.Context(), "account-Ben")
	if err != nil {
		t.Fatalf("AccountSessions error = %v", err)
	}

	if got, want := sessionIDs(own), []string{"ben-new", "ben-old"}; !slices.Equal(got, want) {
		t.Errorf("Ben's sessions = %v, want %v (most recent first)", got, want)
	}

	all, err := fixture.sessions.AllSessions(t.Context())
	if err != nil {
		t.Fatalf("AllSessions error = %v", err)
	}

	var got []string
	for _, listed := range all {
		got = append(got, listed.Session.ID+"/"+listed.AccountName)
	}

	if want := []string{"ben-new/Ben", "anna/Anna", "ben-old/Ben"}; !slices.Equal(got, want) {
		t.Errorf("all sessions = %v, want %v", got, want)
	}

	none, err := fixture.sessions.AccountSessions(t.Context(), "account-Nobody")
	if err != nil || len(none) != 0 {
		t.Errorf("AccountSessions(unknown) = %v, %v, want empty", none, err)
	}
}

// User A cannot delete user B's session by id; the owner and the admin path can.
func TestSessionDeleteIsScopedToTheAccount(t *testing.T) {
	t.Parallel()

	fixture := newSessionFixture(t)
	ctx := t.Context()
	fixture.mustCreate(t, fixture.newSession("ben", "Ben", ""))
	fixture.mustCreate(t, fixture.newSession("anna", "Anna", ""))

	deleted, err := fixture.sessions.DeleteAccountSession(ctx, "account-Anna", "ben")
	if err != nil || len(deleted) != 0 {
		t.Errorf("Anna deletes Ben's session = %v, %v, want nothing deleted", deleted, err)
	}

	deleted, err = fixture.sessions.DeleteAccountSession(ctx, "account-Ben", "ben")
	if err != nil || !slices.Equal(deleted, []string{"hash-ben"}) {
		t.Errorf("Ben deletes his session = %v, %v, want [hash-ben]", deleted, err)
	}

	if err := fixture.sessions.DeleteSession(ctx, "anna"); err != nil {
		t.Errorf("DeleteSession error = %v", err)
	}

	if err := fixture.sessions.DeleteSession(ctx, "unknown"); err != nil {
		t.Errorf("DeleteSession(unknown) error = %v, want nil", err)
	}

	if left := fixture.remainingSessionIDs(t); len(left) != 0 {
		t.Errorf("sessions left = %v, want none", left)
	}
}

func TestSessionDeleteOthersOfAnAccount(t *testing.T) {
	t.Parallel()

	fixture := newSessionFixture(t)
	ctx := t.Context()

	for _, id := range []string{"ben-1", "ben-2", "ben-3"} {
		fixture.mustCreate(t, fixture.newSession(id, "Ben", ""))
	}

	fixture.mustCreate(t, fixture.newSession("anna", "Anna", ""))

	deleted, err := fixture.sessions.RevokeOtherSessions(ctx, "account-Ben", "ben-2")
	if err != nil || !slices.Equal(sortedHashes(deleted), []string{"hash-ben-1", "hash-ben-3"}) {
		t.Errorf("RevokeOtherSessions = %v, %v, want hash-ben-1 and hash-ben-3", deleted, err)
	}

	if got, want := fixture.remainingSessionIDs(t), []string{"anna", "ben-2"}; !slices.Equal(got, want) {
		t.Errorf("after keeping ben-2: %v, want %v", got, want)
	}

	if err := fixture.sessions.DeleteOtherSessions(ctx, "account-Ben", ""); err != nil {
		t.Fatalf("DeleteOtherSessions error = %v", err)
	}

	if err := fixture.sessions.RevokeAccountSessions(ctx, "account-Anna"); err != nil {
		t.Fatalf("RevokeAccountSessions error = %v", err)
	}

	if left := fixture.remainingSessionIDs(t); len(left) != 0 {
		t.Errorf("sessions left = %v, want none", left)
	}
}

// The purge bounds count as expired, like the resolver does.
func TestSessionPurgeDeletesIdleAndOldSessions(t *testing.T) {
	t.Parallel()

	fixture := newSessionFixture(t)
	fixture.createAged(t, "fresh", "Ben", sessionAge{time.Hour, time.Minute})
	fixture.createAged(t, "idle-at-bound", "Ben", sessionAge{auth.IdleTimeout, auth.IdleTimeout})
	fixture.createAged(t, "almost-idle", "Ben", sessionAge{auth.IdleTimeout, auth.IdleTimeout - time.Millisecond})
	fixture.createAged(t, "old-at-bound", "Anna", sessionAge{auth.AbsoluteLifetime, time.Minute})
	fixture.createAged(t, "almost-old", "Anna", sessionAge{auth.AbsoluteLifetime - time.Millisecond, time.Minute})

	now := fixture.clock.Now()
	idleBefore, createdBefore := now.Add(-auth.IdleTimeout), now.Add(-auth.AbsoluteLifetime)

	deleted, err := fixture.sessions.PurgeExpiredSessions(t.Context(), idleBefore, createdBefore)
	if err != nil || !slices.Equal(sortedHashes(deleted), []string{"hash-idle-at-bound", "hash-old-at-bound"}) {
		t.Errorf("PurgeExpiredSessions = %v, %v, want the two sessions at the bounds", deleted, err)
	}

	if got, want := fixture.remainingSessionIDs(t), []string{"almost-idle", "almost-old", "fresh"}; !slices.Equal(got, want) {
		t.Errorf("sessions left = %v, want %v", got, want)
	}

	later := now.Add(time.Hour)

	count, err := fixture.sessions.PurgeSessions(t.Context(), later.Add(-auth.IdleTimeout), later.Add(-auth.AbsoluteLifetime))
	if err != nil || count != 2 {
		t.Errorf("PurgeSessions an hour later = %d, %v, want 2 (almost-idle, almost-old)", count, err)
	}
}

func TestSessionsCascadeOnAccountDelete(t *testing.T) {
	t.Parallel()

	fixture := newSessionFixture(t)
	fixture.mustCreate(t, fixture.newSession("ben", "Ben", benKidProfileID))
	fixture.mustCreate(t, fixture.newSession("anna", "Anna", ""))

	if err := fixture.accounts.DeleteAccount(t.Context(), "account-Ben", nil); err != nil {
		t.Fatalf("DeleteAccount error = %v", err)
	}

	if got, want := fixture.remainingSessionIDs(t), []string{"anna"}; !slices.Equal(got, want) {
		t.Errorf("sessions left = %v, want %v", got, want)
	}
}

func TestSessionStoreReportsCancelledContext(t *testing.T) {
	t.Parallel()

	fixture := newSessionFixture(t)
	ctx := cancelledContext(t)

	checks := map[string]error{
		"CreateSession": fixture.sessions.CreateSession(ctx, fixture.newSession("s1", "Ben", "")),
		"TouchSession":  fixture.sessions.TouchSession(ctx, "s1", auth.Seen{At: time.Now(), AddressClass: auth.AddressLAN}),
		"SelectProfile": fixture.sessions.SelectProfile(ctx, "s1", benKidProfileID, false),
		"DeleteSession": fixture.sessions.DeleteSession(ctx, "s1"),
		"DeleteOthers":  fixture.sessions.DeleteOtherSessions(ctx, "account-Ben", ""),
	}

	_, checks["SessionByTokenHash"] = fixture.sessions.SessionByTokenHash(ctx, "hash-s1")
	_, checks["AccountSessions"] = fixture.sessions.AccountSessions(ctx, "account-Ben")
	_, checks["AllSessions"] = fixture.sessions.AllSessions(ctx)
	_, checks["DeleteAccountSession"] = fixture.sessions.DeleteAccountSession(ctx, "account-Ben", "s1")
	_, checks["PurgeSessions"] = fixture.sessions.PurgeSessions(ctx, time.Now(), time.Now())

	for operation, err := range checks {
		if err == nil {
			t.Errorf("%s with a cancelled context error = nil, want an error", operation)
		}
	}
}
