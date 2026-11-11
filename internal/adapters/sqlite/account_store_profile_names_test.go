package sqlite_test

import (
	"database/sql"
	"errors"
	"os"
	"slices"
	"sync"
	"testing"

	"github.com/pressly/goose/v3"

	"github.com/magpeek/mivo/internal/adapters/sqlite"
	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/account"
)

// profileNameKeySchemaVersion is the migration that adds profiles.name_key and its index.
const profileNameKeySchemaVersion = 15

// insertLegacyProfile is a managed profile of account-Anna as a server before migration 00015
// wrote it: without a name key.
const insertLegacyProfile = `INSERT INTO profiles (id, user_id, name, is_primary, avatar_color, created_at, updated_at)
	VALUES (?, 'account-Anna', ?, 0, 'teal', ?, ?)`

// profileNameKeys returns "id=key" for every profile, NULL keys as "id=NULL", ordered by id.
func profileNameKeys(t *testing.T, inspection *sql.DB) []string {
	t.Helper()

	return queryStrings(t, inspection, "SELECT id || '=' || COALESCE(name_key, 'NULL') FROM profiles ORDER BY id")
}

func wantProfileNameKeys(t *testing.T, inspection *sql.DB, want ...string) {
	t.Helper()

	if got := profileNameKeys(t, inspection); !slices.Equal(got, want) {
		t.Errorf("profile name keys = %v, want %v", got, want)
	}
}

func mustBackfillProfileNameKeys(t *testing.T, store *sqlite.AccountStore, wantUnresolved int) {
	t.Helper()

	unresolved, err := store.BackfillProfileNameKeys(t.Context())
	if err != nil {
		t.Fatalf("BackfillProfileNameKeys error = %v", err)
	}

	if unresolved != wantUnresolved {
		t.Errorf("BackfillProfileNameKeys = %d profiles without a key, want %d", unresolved, wantUnresolved)
	}
}

// renamedProfile is write with another name and the id of the profile first created as name.
func renamedProfile(owner, createdAs, name string) sqlite.ProfileWrite {
	write := managedWrite(owner, createdAs)
	write.Record.Profile.Name = name

	return write
}

func TestProfileNameIsUniquePerAccountIgnoringCase(t *testing.T) {
	t.Parallel()

	tests := map[string]struct{ first, second string }{
		"same name":           {first: "Mia", second: "Mia"},
		"ASCII case":          {first: "Mia", second: "mia"},
		"non-ASCII case":      {first: "Ärzte", second: "ärzte"},
		"the primary profile": {first: "Kids", second: "ANNA"},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			fixture := newAccountFixture(t)
			fixture.mustCreateAccount(t, "Anna", account.RoleAdmin)
			mustCreateProfile(t, fixture.store, managedWrite("Anna", test.first))

			second := managedWrite("Anna", test.second)
			second.Record.Profile.ID = "profile-second"

			err := fixture.store.CreateProfile(t.Context(), second, nil)
			wantError(t, "CreateProfile(duplicate name)", err, account.ErrNameTaken)
			wantError(t, "CreateProfile(duplicate name)", err, domain.ErrConflict)

			if got := countRows(t, fixture.inspection, "SELECT COUNT(*) FROM profiles"); got != 2 {
				t.Errorf("profiles = %d, want the primary and the first managed one", got)
			}
		})
	}
}

func TestProfileNameMayRepeatInAnotherAccount(t *testing.T) {
	t.Parallel()

	fixture := newAccountFixture(t)
	fixture.mustCreateAccount(t, "Anna", account.RoleAdmin)
	fixture.mustCreateAccount(t, "Ben", account.RoleUser)

	mustCreateProfile(t, fixture.store, managedWrite("Anna", "Mia"))
	mustCreateProfile(t, fixture.store, managedWrite("Ben", "mia"))
	// A managed profile may also carry the name of another account.
	mustCreateProfile(t, fixture.store, managedWrite("Ben", "Anna"))
}

func TestProfileNameRenameOntoASiblingIsNameTaken(t *testing.T) {
	t.Parallel()

	fixture := newAccountFixture(t)
	ctx := t.Context()
	fixture.mustCreateAccount(t, "Anna", account.RoleAdmin)
	mustCreateProfile(t, fixture.store, managedWrite("Anna", "Mia"))
	mustCreateProfile(t, fixture.store, managedWrite("Anna", "Tom"))

	err := fixture.store.UpdateProfile(ctx, renamedProfile("Anna", "Tom", "MIA"))
	wantError(t, "UpdateProfile(sibling's name)", err, account.ErrNameTaken)
	wantError(t, "UpdateProfile(sibling's name)", err, domain.ErrConflict)

	wantError(t, "UpdateProfile(primary profile's name)",
		fixture.store.UpdateProfile(ctx, renamedProfile("Anna", "Tom", "anna")), account.ErrNameTaken)

	if got := mustReadProfile(t, fixture.store, "profile-Anna-Tom").Profile.Name; got != "Tom" {
		t.Errorf("name after refused renames = %q, want Tom", got)
	}

	if err := fixture.store.UpdateProfile(ctx, renamedProfile("Anna", "Tom", "TOM")); err != nil {
		t.Errorf("UpdateProfile(own name in another case) error = %v", err)
	}

	if err := fixture.store.UpdateProfile(ctx, renamedProfile("Anna", "Tom", "Tim")); err != nil {
		t.Errorf("UpdateProfile(free name) error = %v", err)
	}

	// The old name is free again once its holder was renamed.
	mustCreateProfile(t, fixture.store, managedWrite("Anna", "tom"))
}

// Renaming an account renames its primary profile, which must not land on a managed profile.
func TestProfileNameAccountRenameOntoAManagedProfileIsNameTaken(t *testing.T) {
	t.Parallel()

	fixture := newAccountFixture(t)
	ctx := t.Context()
	fixture.mustCreateAccount(t, "Anna", account.RoleAdmin)
	mustCreateProfile(t, fixture.store, managedWrite("Anna", "Mia"))

	_, err := fixture.store.UpdateAccount(ctx, "account-Anna", renameTo("MIA"))
	wantError(t, "UpdateAccount(managed profile's name)", err, account.ErrNameTaken)

	if got := mustReadProfile(t, fixture.store, "profile-Anna").Profile.Name; got != "Anna" {
		t.Errorf("primary profile name after the refused rename = %q, want Anna", got)
	}

	if got := queryStrings(t, fixture.inspection, "SELECT name FROM users"); !slices.Equal(got, []string{"Anna"}) {
		t.Errorf("account names after the refused rename = %v, want [Anna]", got)
	}
}

func TestProfileNameKeyIsWrittenWithEveryName(t *testing.T) {
	t.Parallel()

	fixture := newAccountFixture(t)
	ctx := t.Context()
	fixture.mustCreateAccount(t, "Anna", account.RoleAdmin)
	mustCreateProfile(t, fixture.store, managedWrite("Anna", "Ärzte"))
	wantProfileNameKeys(t, fixture.inspection, "profile-Anna=anna", "profile-Anna-Ärzte=ärzte")

	if _, err := fixture.store.UpdateAccount(ctx, "account-Anna", renameTo("Änne")); err != nil {
		t.Fatalf("UpdateAccount(rename) error = %v", err)
	}

	if err := fixture.store.UpdateProfile(ctx, renamedProfile("Anna", "Ärzte", "MIA")); err != nil {
		t.Fatalf("UpdateProfile(rename) error = %v", err)
	}

	wantProfileNameKeys(t, fixture.inspection, "profile-Anna=änne", "profile-Anna-Ärzte=mia")
}

// Only the name index is "name taken": the one-primary index and a reused id stay plain
// conflicts.
func TestProfileNameOtherConstraintsAreNotNameTaken(t *testing.T) {
	t.Parallel()

	fixture := newAccountFixture(t)
	ctx := t.Context()
	fixture.mustCreateAccount(t, "Anna", account.RoleAdmin)
	mustCreateProfile(t, fixture.store, managedWrite("Anna", "Mia"))

	reusedID := managedWrite("Anna", "Tom")
	reusedID.Record.Profile.ID = "profile-Anna-Mia"

	err := fixture.store.CreateProfile(ctx, reusedID, nil)
	wantError(t, "CreateProfile(reused id)", err, domain.ErrConflict)

	if errors.Is(err, account.ErrNameTaken) {
		t.Errorf("CreateProfile(reused id) error = %v, must not be ErrNameTaken", err)
	}

	// The inspection connection does not enforce foreign keys, so it can plant a primary
	// profile for an account that does not exist yet; creating that account then violates
	// profiles_one_primary, not the name index.
	execute(t, fixture.inspection, `INSERT INTO profiles (id, user_id, name, name_key, is_primary, avatar_color, created_at, updated_at)
		VALUES ('profile-planted', 'account-Ben', 'Planted', 'planted', 1, 'teal', 0, 0)`)

	err = fixture.store.CreateAccount(ctx, newAccount("Ben", account.RoleUser))
	wantError(t, "CreateAccount(second primary profile)", err, domain.ErrConflict)

	if errors.Is(err, account.ErrNameTaken) {
		t.Errorf("CreateAccount(second primary profile) error = %v, must not be ErrNameTaken", err)
	}
}

func TestProfileNameConcurrentInsertsHaveExactlyOneWinner(t *testing.T) {
	t.Parallel()

	fixture := newAccountFixture(t)
	fixture.mustCreateAccount(t, "Anna", account.RoleAdmin)

	names := []string{"Mia", "MIA"}
	results := make([]error, len(names))

	var group sync.WaitGroup

	for index, name := range names {
		group.Go(func() {
			results[index] = fixture.store.CreateProfile(t.Context(), managedWrite("Anna", name), nil)
		})
	}

	group.Wait()

	wins, taken := 0, 0

	for _, err := range results {
		switch {
		case err == nil:
			wins++
		case errors.Is(err, account.ErrNameTaken):
			taken++
		default:
			t.Errorf("CreateProfile error = %v, want nil or ErrNameTaken", err)
		}
	}

	if wins != 1 || taken != 1 {
		t.Errorf("wins = %d, name taken = %d, want 1 and 1", wins, taken)
	}

	if got := countRows(t, fixture.inspection, "SELECT COUNT(*) FROM profiles WHERE is_primary = 0"); got != 1 {
		t.Errorf("managed profiles = %d, want 1", got)
	}
}

func TestProfileNameBackfillFillsMissingKeys(t *testing.T) {
	t.Parallel()

	fixture := newAccountFixture(t)
	fixture.mustCreateAccount(t, "Anna", account.RoleAdmin)
	fixture.mustCreateAccount(t, "Ben", account.RoleUser)
	mustCreateProfile(t, fixture.store, managedWrite("Anna", "Ärzte"))
	mustCreateProfile(t, fixture.store, managedWrite("Ben", "ärzte"))
	mustBackfillProfileNameKeys(t, fixture.store, 0)

	execute(t, fixture.inspection, "UPDATE profiles SET name_key = NULL")
	wantProfileNameKeys(t, fixture.inspection,
		"profile-Anna=NULL", "profile-Anna-Ärzte=NULL", "profile-Ben=NULL", "profile-Ben-ärzte=NULL")

	want := []string{"profile-Anna=anna", "profile-Anna-Ärzte=ärzte", "profile-Ben=ben", "profile-Ben-ärzte=ärzte"}

	mustBackfillProfileNameKeys(t, fixture.store, 0)
	wantProfileNameKeys(t, fixture.inspection, want...)

	mustBackfillProfileNameKeys(t, fixture.store, 0)
	wantProfileNameKeys(t, fixture.inspection, want...)

	// The filled keys protect the names like keys written with the profile.
	duplicate := managedWrite("Anna", "ÄRZTE")
	wantError(t, "CreateProfile(backfilled name)", fixture.store.CreateProfile(t.Context(), duplicate, nil), account.ErrNameTaken)
}

// Two legacy profiles of one account with the same name: the older one gets the key, the
// newer one keeps NULL and its name and is counted, on every run, until it is renamed.
func TestProfileNameBackfillLeavesTheNewerOfTwoCollidingProfiles(t *testing.T) {
	t.Parallel()

	fixture := newAccountFixture(t)
	ctx := t.Context()
	fixture.mustCreateAccount(t, "Anna", account.RoleAdmin)
	execute(t, fixture.inspection, insertLegacyProfile, "profile-a-newer", "MIA", 20, 20)
	execute(t, fixture.inspection, insertLegacyProfile, "profile-b-older", "Mia", 10, 10)
	execute(t, fixture.inspection, insertLegacyProfile, "profile-c-twin", "mia", 20, 20)
	execute(t, fixture.inspection, insertLegacyProfile, "profile-d-other", "Tom", 30, 30)

	want := []string{"profile-Anna=anna", "profile-a-newer=NULL", "profile-b-older=mia", "profile-c-twin=NULL", "profile-d-other=tom"}

	mustBackfillProfileNameKeys(t, fixture.store, 2)
	wantProfileNameKeys(t, fixture.inspection, want...)

	mustBackfillProfileNameKeys(t, fixture.store, 2)
	wantProfileNameKeys(t, fixture.inspection, want...)

	if got := mustReadProfile(t, fixture.store, "profile-a-newer").Profile.Name; got != "MIA" {
		t.Errorf("name of the profile left without a key = %q, want MIA", got)
	}

	newer := managedWrite("Anna", "MIA")
	newer.Record.Profile.ID = "profile-a-newer"
	wantError(t, "UpdateProfile(keeping the colliding name)", fixture.store.UpdateProfile(ctx, newer), account.ErrNameTaken)

	newer.Record.Profile.Name = "Mia 2"
	if err := fixture.store.UpdateProfile(ctx, newer); err != nil {
		t.Fatalf("UpdateProfile(rename the colliding profile) error = %v", err)
	}

	mustBackfillProfileNameKeys(t, fixture.store, 1)
	wantProfileNameKeys(t, fixture.inspection,
		"profile-Anna=anna", "profile-a-newer=mia 2", "profile-b-older=mia", "profile-c-twin=NULL", "profile-d-other=tom")
}

func TestProfileNameBackfillFailsWithACancelledContext(t *testing.T) {
	t.Parallel()

	fixture := newAccountFixture(t)

	if _, err := fixture.store.BackfillProfileNameKeys(cancelledContext(t)); err == nil {
		t.Error("BackfillProfileNameKeys with a cancelled context succeeded, want an error")
	}
}

// Rolling back 00015 on a database with profiles removes the column and the index and keeps
// the profiles; re-applying it leaves their keys NULL for the backfill.
func TestProfileNameKeyMigrationRollsBackAndReappliesWithProfiles(t *testing.T) {
	t.Parallel()

	db, path := openMigratedTestDB(t)
	inspection := openInspectionDB(t, path)
	store := sqlite.NewAccountStore(db, newTestClock().Now)

	if err := store.CreateAccount(t.Context(), newAccount("Anna", account.RoleAdmin)); err != nil {
		t.Fatalf("CreateAccount error = %v", err)
	}

	mustCreateProfile(t, store, managedWrite("Anna", "Mia"))

	provider, err := goose.NewProvider(goose.DialectSQLite3, inspection, os.DirFS("migrations"),
		goose.WithDisableGlobalRegistry(true))
	if err != nil {
		t.Fatalf("NewProvider error = %v", err)
	}

	if _, err := provider.DownTo(t.Context(), profileNameKeySchemaVersion-1); err != nil {
		t.Fatalf("DownTo(%d) error = %v", profileNameKeySchemaVersion-1, err)
	}

	const nameKeyObjects = `SELECT (SELECT COUNT(*) FROM pragma_table_info('profiles') WHERE name = 'name_key')
		+ (SELECT COUNT(*) FROM sqlite_schema WHERE type = 'index' AND name = 'profiles_unique_name')`

	if got := countRows(t, inspection, nameKeyObjects); got != 0 {
		t.Errorf("name key column and index after rolling back 00015 = %d, want 0", got)
	}

	if got := queryStrings(t, inspection, "SELECT name FROM profiles ORDER BY name"); !slices.Equal(got, []string{"Anna", "Mia"}) {
		t.Errorf("profiles after rolling back 00015 = %v, want [Anna Mia]", got)
	}

	if err := db.Migrate(t.Context()); err != nil {
		t.Fatalf("Migrate after rollback error = %v", err)
	}

	if got := countRows(t, inspection, nameKeyObjects); got != 2 {
		t.Errorf("name key column and index after re-applying 00015 = %d, want 2", got)
	}

	wantProfileNameKeys(t, inspection, "profile-Anna=NULL", "profile-Anna-Mia=NULL")
	mustBackfillProfileNameKeys(t, store, 0)
	wantProfileNameKeys(t, inspection, "profile-Anna=anna", "profile-Anna-Mia=mia")
}
