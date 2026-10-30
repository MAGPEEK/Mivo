package system_test

import (
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	appsystem "github.com/magpeek/mivo/internal/app/system"
	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/system"
)

const (
	firstGeneratedID  = "0b8c6d4e-3f2a-4c1b-9d8e-7f6a5b4c3d2e"
	secondGeneratedID = "9a1b2c3d-4e5f-4a6b-8c7d-0e1f2a3b4c5d"
	storedID          = "f81d4fae-7dec-11d0-a765-00a0c91e6bf6"
	testProductName   = "Mivo"
)

var errBoom = errors.New("boom")

// testEnvironment bundles the fakes behind one Service.
type testEnvironment struct {
	store     *memoryIdentityStore
	generator *sequenceIDGenerator
	ping      pingFunc
	now       time.Time
	build     appsystem.BuildInfo
}

func newTestEnvironment() *testEnvironment {
	berlin := time.FixedZone("CET", int(time.Hour/time.Second))

	return &testEnvironment{
		store:     &memoryIdentityStore{},
		generator: &sequenceIDGenerator{ids: []string{firstGeneratedID, secondGeneratedID}},
		ping:      pingSucceeds,
		now:       time.Date(2026, 10, 30, 12, 0, 0, 0, berlin),
		build:     appsystem.BuildInfo{Version: "1.2.3", Commit: "abcdef", BuildTime: "2026-10-30T10:00:00Z"},
	}
}

func (environment *testEnvironment) service(t *testing.T) *appsystem.Service {
	t.Helper()

	service, err := appsystem.NewService(appsystem.Dependencies{
		Identities:  environment.store,
		Database:    environment.ping,
		IDs:         environment.generator,
		Clock:       fixedClock(environment.now),
		Build:       environment.build,
		ProductName: testProductName,
	})
	if err != nil {
		t.Fatalf("NewService error = %v", err)
	}

	return service
}

func mustParseServerID(t *testing.T, text string) system.ServerID {
	t.Helper()

	id, err := system.ParseServerID(text)
	if err != nil {
		t.Fatalf("ParseServerID(%q) error = %v", text, err)
	}

	return id
}

func TestNewServiceRejectsMissingDependencies(t *testing.T) {
	t.Parallel()

	complete := func() appsystem.Dependencies {
		return appsystem.Dependencies{
			Identities: &memoryIdentityStore{},
			Database:   pingFunc(pingSucceeds),
			IDs:        &sequenceIDGenerator{},
			Clock:      time.Now,
		}
	}

	tests := []struct {
		name     string
		mutate   func(*appsystem.Dependencies)
		wantText string
	}{
		{name: "identity store", mutate: func(d *appsystem.Dependencies) { d.Identities = nil }, wantText: "identity store"},
		{name: "database checker", mutate: func(d *appsystem.Dependencies) { d.Database = nil }, wantText: "database checker"},
		{name: "id generator", mutate: func(d *appsystem.Dependencies) { d.IDs = nil }, wantText: "id generator"},
		{name: "clock", mutate: func(d *appsystem.Dependencies) { d.Clock = nil }, wantText: "clock"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			dependencies := complete()
			test.mutate(&dependencies)

			service, err := appsystem.NewService(dependencies)
			if !errors.Is(err, domain.ErrInvalid) {
				t.Fatalf("NewService error = %v, want ErrInvalid", err)
			}

			if !strings.Contains(err.Error(), test.wantText) {
				t.Errorf("NewService error = %q, want it to name %q", err, test.wantText)
			}

			if service != nil {
				t.Error("NewService returned a service together with an error")
			}
		})
	}
}

func TestNewServiceReportsEveryMissingDependency(t *testing.T) {
	t.Parallel()

	_, err := appsystem.NewService(appsystem.Dependencies{})
	if err == nil {
		t.Fatal("NewService(empty) error = nil, want error")
	}

	for _, name := range []string{"identity store", "database checker", "id generator", "clock"} {
		if !strings.Contains(err.Error(), name) {
			t.Errorf("NewService(empty) error = %q, want it to name %q", err, name)
		}
	}
}

func TestEnsureIdentityReturnsStoredIdentity(t *testing.T) {
	t.Parallel()

	environment := newTestEnvironment()
	stored := system.Identity{
		ID:        mustParseServerID(t, storedID),
		CreatedAt: time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC),
	}
	environment.store.identity = &stored

	identity, err := environment.service(t).EnsureIdentity(t.Context())
	if err != nil {
		t.Fatalf("EnsureIdentity error = %v", err)
	}

	if identity != stored {
		t.Errorf("EnsureIdentity = %+v, want stored %+v", identity, stored)
	}

	if environment.store.saves() != 0 || environment.generator.calls != 0 {
		t.Errorf("saves = %d, generator calls = %d, want 0 and 0",
			environment.store.saves(), environment.generator.calls)
	}
}

func TestEnsureIdentityCreatesIdentityOnFirstUse(t *testing.T) {
	t.Parallel()

	environment := newTestEnvironment()

	identity, err := environment.service(t).EnsureIdentity(t.Context())
	if err != nil {
		t.Fatalf("EnsureIdentity error = %v", err)
	}

	want := system.Identity{ID: mustParseServerID(t, firstGeneratedID), CreatedAt: environment.now.UTC()}
	if identity != want {
		t.Errorf("EnsureIdentity = %+v, want %+v", identity, want)
	}

	if identity.CreatedAt.Location() != time.UTC {
		t.Errorf("CreatedAt location = %v, want UTC", identity.CreatedAt.Location())
	}

	if *environment.store.identity != want {
		t.Errorf("saved identity = %+v, want %+v", *environment.store.identity, want)
	}
}

func TestEnsureIdentityIsIdempotent(t *testing.T) {
	t.Parallel()

	environment := newTestEnvironment()
	service := environment.service(t)

	first, firstErr := service.EnsureIdentity(t.Context())
	second, secondErr := service.EnsureIdentity(t.Context())

	if err := errors.Join(firstErr, secondErr); err != nil {
		t.Fatalf("EnsureIdentity error = %v", err)
	}

	if first != second {
		t.Errorf("second EnsureIdentity = %+v, want first %+v", second, first)
	}

	if saves := environment.store.saves(); saves != 1 {
		t.Errorf("saves = %d, want 1", saves)
	}
}

func TestEnsureIdentityCreatesOneIdentityUnderConcurrentCalls(t *testing.T) {
	t.Parallel()

	const callers = 8

	environment := newTestEnvironment()
	service := environment.service(t)
	results := make([]system.Identity, callers)
	errs := make([]error, callers)

	var group sync.WaitGroup
	for caller := range callers {
		group.Go(func() {
			results[caller], errs[caller] = service.EnsureIdentity(t.Context())
		})
	}

	group.Wait()

	if err := errors.Join(errs...); err != nil {
		t.Fatalf("EnsureIdentity error = %v", err)
	}

	for caller, result := range results {
		if result != results[0] {
			t.Errorf("caller %d got %+v, want %+v", caller, result, results[0])
		}
	}

	if saves := environment.store.saves(); saves != 1 {
		t.Errorf("saves = %d, want 1", saves)
	}
}

func TestEnsureIdentityFailures(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		configure func(*testEnvironment)
		wantErr   error
		wantText  string
	}{
		{
			name:      "load fails",
			configure: func(environment *testEnvironment) { environment.store.loadErr = errBoom },
			wantErr:   errBoom,
			wantText:  "ensure identity: load",
		},
		{
			name:      "generator fails",
			configure: func(environment *testEnvironment) { environment.generator.err = errBoom },
			wantErr:   errBoom,
			wantText:  "ensure identity: generate id",
		},
		{
			name: "generator returns uppercase id",
			configure: func(environment *testEnvironment) {
				environment.generator.ids = []string{strings.ToUpper(firstGeneratedID)}
			},
			wantErr:  domain.ErrInvalid,
			wantText: "ensure identity: validate generated id",
		},
		{
			name:      "save fails",
			configure: func(environment *testEnvironment) { environment.store.saveErr = errBoom },
			wantErr:   errBoom,
			wantText:  "ensure identity: save",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			environment := newTestEnvironment()
			test.configure(environment)

			identity, err := environment.service(t).EnsureIdentity(t.Context())
			assertWrappedError(t, err, test.wantErr, test.wantText)

			if identity != (system.Identity{}) {
				t.Errorf("EnsureIdentity = %+v on error, want zero identity", identity)
			}

			if environment.store.identity != nil {
				t.Errorf("stored identity = %+v after failure, want none", *environment.store.identity)
			}
		})
	}
}

func assertWrappedError(t *testing.T, err, wantErr error, wantText string) {
	t.Helper()

	if !errors.Is(err, wantErr) {
		t.Fatalf("error = %v, want it to wrap %v", err, wantErr)
	}

	if !strings.Contains(err.Error(), wantText) {
		t.Errorf("error = %q, want context %q", err, wantText)
	}
}

func TestInfoDescribesServer(t *testing.T) {
	t.Parallel()

	environment := newTestEnvironment()
	service := environment.service(t)

	identity, err := service.EnsureIdentity(t.Context())
	if err != nil {
		t.Fatalf("EnsureIdentity error = %v", err)
	}

	info, err := service.Info(t.Context())
	if err != nil {
		t.Fatalf("Info error = %v", err)
	}

	want := system.Info{
		ProductName: testProductName,
		Version:     environment.build.Version,
		Commit:      environment.build.Commit,
		BuildTime:   environment.build.BuildTime,
		ServerID:    identity.ID,
		APIVersion:  "1",
	}
	if info != want {
		t.Errorf("Info = %+v, want %+v", info, want)
	}

	if saves := environment.store.saves(); saves != 1 {
		t.Errorf("saves = %d, want 1", saves)
	}
}

func TestInfoReturnsPersistedServerIDWithoutPriorEnsure(t *testing.T) {
	t.Parallel()

	environment := newTestEnvironment()
	stored := system.Identity{ID: mustParseServerID(t, storedID), CreatedAt: environment.now.UTC()}
	environment.store.identity = &stored

	info, err := environment.service(t).Info(t.Context())
	if err != nil {
		t.Fatalf("Info error = %v", err)
	}

	if info.ServerID != stored.ID {
		t.Errorf("Info.ServerID = %q, want %q", info.ServerID, stored.ID)
	}
}

func TestInfoWrapsIdentityErrors(t *testing.T) {
	t.Parallel()

	environment := newTestEnvironment()
	environment.store.loadErr = errBoom

	info, err := environment.service(t).Info(t.Context())
	assertWrappedError(t, err, errBoom, "system info")

	if info != (system.Info{}) {
		t.Errorf("Info = %+v on error, want zero value", info)
	}
}
