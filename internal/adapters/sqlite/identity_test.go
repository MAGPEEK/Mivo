package sqlite_test

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/magpeek/mivo/internal/adapters/sqlite"
	"github.com/magpeek/mivo/internal/adapters/uuidgen"
	appsystem "github.com/magpeek/mivo/internal/app/system"
	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/system"
)

const (
	firstServerID  = "0b8c6d4e-3f2a-4c1b-9d8e-7f6a5b4c3d2e"
	secondServerID = "5f1e2d3c-4b5a-4968-8776-655443322110"
)

func mustServerID(t *testing.T, text string) system.ServerID {
	t.Helper()

	id, err := system.ParseServerID(text)
	if err != nil {
		t.Fatalf("ParseServerID(%q) error = %v", text, err)
	}

	return id
}

func TestLoadIdentityFromAnEmptyTableReturnsNotFound(t *testing.T) {
	t.Parallel()

	db, _ := openMigratedTestDB(t)

	_, err := sqlite.NewIdentityRepository(db).LoadIdentity(t.Context())
	if !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("LoadIdentity error = %v, want domain.ErrNotFound", err)
	}
}

// CreatedAt is stored as Unix milliseconds: sub-millisecond digits and the time zone are not
// kept, so a load returns the instant truncated to milliseconds, in UTC.
func TestSaveThenLoadRoundTripsIDAndCreatedAtInMillisecondsUTC(t *testing.T) {
	t.Parallel()

	db, _ := openMigratedTestDB(t)
	repository := sqlite.NewIdentityRepository(db)

	berlin := time.FixedZone("CET", 3600)
	saved := system.Identity{
		ID:        mustServerID(t, firstServerID),
		CreatedAt: time.Date(2026, time.October, 30, 12, 34, 56, 789_654_321, berlin),
	}

	if err := repository.SaveIdentity(t.Context(), saved); err != nil {
		t.Fatalf("SaveIdentity error = %v", err)
	}

	loaded, err := repository.LoadIdentity(t.Context())
	if err != nil {
		t.Fatalf("LoadIdentity error = %v", err)
	}

	wantCreatedAt := time.Date(2026, time.October, 30, 11, 34, 56, 789_000_000, time.UTC)
	if loaded.ID != saved.ID || !loaded.CreatedAt.Equal(wantCreatedAt) || loaded.CreatedAt.Location() != time.UTC {
		t.Errorf("LoadIdentity = %v %v, want %v %v", loaded.ID, loaded.CreatedAt, saved.ID, wantCreatedAt)
	}
}

func TestSavingASecondIdentityReturnsConflictAndKeepsTheFirst(t *testing.T) {
	t.Parallel()

	db, _ := openMigratedTestDB(t)
	repository := sqlite.NewIdentityRepository(db)
	createdAt := time.UnixMilli(1_700_000_000_000).UTC()

	first := system.Identity{ID: mustServerID(t, firstServerID), CreatedAt: createdAt}
	if err := repository.SaveIdentity(t.Context(), first); err != nil {
		t.Fatalf("first SaveIdentity error = %v", err)
	}

	second := system.Identity{ID: mustServerID(t, secondServerID), CreatedAt: createdAt.Add(time.Hour)}
	if err := repository.SaveIdentity(t.Context(), second); !errors.Is(err, domain.ErrConflict) {
		t.Errorf("second SaveIdentity error = %v, want domain.ErrConflict", err)
	}

	loaded, err := repository.LoadIdentity(t.Context())
	if err != nil {
		t.Fatalf("LoadIdentity error = %v", err)
	}

	if loaded != first {
		t.Errorf("LoadIdentity = %+v, want the first identity %+v", loaded, first)
	}
}

// A zero ID or a zero CreatedAt (the year 1, stored as a large negative number) can only
// come from a caller bug; neither is stored, so the table stays empty.
func TestSaveIdentityRejectsZeroValues(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		identity system.Identity
	}{
		{name: "zero id", identity: system.Identity{CreatedAt: time.Now()}},
		{name: "zero created at", identity: system.Identity{ID: mustServerID(t, firstServerID)}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			db, _ := openMigratedTestDB(t)
			repository := sqlite.NewIdentityRepository(db)

			if err := repository.SaveIdentity(t.Context(), test.identity); !errors.Is(err, domain.ErrInvalid) {
				t.Errorf("SaveIdentity error = %v, want domain.ErrInvalid", err)
			}

			if _, err := repository.LoadIdentity(t.Context()); !errors.Is(err, domain.ErrNotFound) {
				t.Errorf("LoadIdentity after the rejected save error = %v, want domain.ErrNotFound", err)
			}
		})
	}
}

// A stored ID that is not canonical (e.g. edited by hand) must not reach clients.
func TestLoadIdentityRejectsAMalformedStoredID(t *testing.T) {
	t.Parallel()

	db, path := openMigratedTestDB(t)

	_, err := openInspectionDB(t, path).ExecContext(t.Context(),
		"INSERT INTO server_identity (id, server_id, created_at) VALUES (1, 'NOT-A-UUID', 0)")
	if err != nil {
		t.Fatalf("store malformed identity: %v", err)
	}

	_, err = sqlite.NewIdentityRepository(db).LoadIdentity(t.Context())
	if !errors.Is(err, domain.ErrInvalid) {
		t.Errorf("LoadIdentity error = %v, want domain.ErrInvalid", err)
	}
}

func TestIdentityRepositoryReportsDatabaseErrors(t *testing.T) {
	t.Parallel()

	identity := system.Identity{ID: mustServerID(t, firstServerID), CreatedAt: time.UnixMilli(0).UTC()}

	tests := []struct {
		name    string
		ctx     func(t *testing.T) context.Context
		closeDB bool
		wantErr error
	}{
		{name: "cancelled context", ctx: cancelledContext, wantErr: context.Canceled},
		{name: "closed database", ctx: func(t *testing.T) context.Context { return t.Context() }, closeDB: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			db, _ := openMigratedTestDB(t)
			repository := sqlite.NewIdentityRepository(db)

			if test.closeDB {
				if err := db.Close(); err != nil {
					t.Fatalf("Close error = %v", err)
				}
			}

			_, loadErr := repository.LoadIdentity(test.ctx(t))
			saveErr := repository.SaveIdentity(test.ctx(t), identity)

			for operation, err := range map[string]error{"LoadIdentity": loadErr, "SaveIdentity": saveErr} {
				assertDatabaseError(t, operation, err, test.wantErr)
			}
		})
	}
}

// assertDatabaseError checks that err is a failure of the database itself, not one of the
// domain outcomes callers branch on.
func assertDatabaseError(t *testing.T, operation string, err, wantErr error) {
	t.Helper()

	if err == nil || errors.Is(err, domain.ErrNotFound) || errors.Is(err, domain.ErrConflict) {
		t.Errorf("%s error = %v, want a database error", operation, err)
	}

	if wantErr != nil && !errors.Is(err, wantErr) {
		t.Errorf("%s error = %v, want %v", operation, err, wantErr)
	}
}

func TestConcurrentSavesStoreExactlyOneIdentity(t *testing.T) {
	t.Parallel()

	db, _ := openMigratedTestDB(t)
	repository := sqlite.NewIdentityRepository(db)
	ids := []string{firstServerID, secondServerID, "9a8b7c6d-5e4f-4a3b-8c2d-1e0f9a8b7c6d"}

	errs := make([]error, len(ids))

	var group sync.WaitGroup
	for index, text := range ids {
		identity := system.Identity{ID: mustServerID(t, text), CreatedAt: time.UnixMilli(0).UTC()}

		group.Go(func() { errs[index] = repository.SaveIdentity(t.Context(), identity) })
	}

	group.Wait()

	saved := 0

	for index, err := range errs {
		switch {
		case err == nil:
			saved++
		case !errors.Is(err, domain.ErrConflict):
			t.Errorf("SaveIdentity(%s) error = %v, want nil or domain.ErrConflict", ids[index], err)
		}
	}

	if saved != 1 {
		t.Errorf("successful saves = %d, want exactly 1", saved)
	}
}

// Two server processes on the same data directory race to create the identity. The
// ErrConflict contract lets the loser adopt the winner's identity, so both report one ID,
// and a restart (new DB on the same file) keeps it.
func TestEnsureIdentityAcrossTwoOpenDatabasesAgreesOnOneID(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), testDatabaseFile)
	migrateOnce(t, path)

	const processes = 2

	results := make([]system.Identity, processes)
	errs := make([]error, processes)

	var group sync.WaitGroup
	for index := range processes {
		group.Go(func() { results[index], errs[index] = ensureIdentityInNewProcess(t, path) })
	}

	group.Wait()

	for index, err := range errs {
		if err != nil {
			t.Fatalf("process %d EnsureIdentity error = %v", index, err)
		}
	}

	afterRestart, err := ensureIdentityInNewProcess(t, path)
	if err != nil {
		t.Fatalf("EnsureIdentity after restart error = %v", err)
	}

	for index, identity := range append(results, afterRestart) {
		if !sameStoredIdentity(identity, afterRestart) {
			t.Errorf("identity %d = %+v, want the stored identity %+v", index, identity, afterRestart)
		}
	}
}

// sameStoredIdentity compares at storage precision: the process that created the identity
// returns its in-memory CreatedAt, which may carry sub-millisecond digits the table does not
// keep.
func sameStoredIdentity(got, stored system.Identity) bool {
	return got.ID == stored.ID && got.CreatedAt.Truncate(time.Millisecond).Equal(stored.CreatedAt)
}

func migrateOnce(t *testing.T, path string) {
	t.Helper()

	db, err := sqlite.Open(t.Context(), sqlite.Options{Path: path})
	if err != nil {
		t.Fatalf("Open error = %v", err)
	}

	if err := errors.Join(db.Migrate(t.Context()), db.Close()); err != nil {
		t.Fatalf("Migrate and Close error = %v", err)
	}
}

// ensureIdentityInNewProcess simulates a server start on an already migrated database: open,
// ensure the identity, close. It runs in goroutines, so it returns errors instead of failing.
func ensureIdentityInNewProcess(t *testing.T, path string) (system.Identity, error) {
	db, err := sqlite.Open(t.Context(), sqlite.Options{Path: path})
	if err != nil {
		return system.Identity{}, err
	}

	defer func() { _ = db.Close() }()

	service, err := appsystem.NewService(appsystem.Dependencies{
		Identities:  sqlite.NewIdentityRepository(db),
		Database:    db,
		IDs:         uuidgen.New(),
		Clock:       time.Now,
		ProductName: "test",
	})
	if err != nil {
		return system.Identity{}, err
	}

	return service.EnsureIdentity(t.Context())
}
