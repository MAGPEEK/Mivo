package system_test

import (
	"context"
	"errors"
	"testing"
	"time"

	appsystem "github.com/magpeek/mivo/internal/app/system"
	"github.com/magpeek/mivo/internal/domain/system"
)

// awaitTimeout turns a regression that would block forever into a test failure.
const awaitTimeout = 5 * time.Second

type ensureResult struct {
	identity system.Identity
	err      error
}

// ensureAsync runs EnsureIdentity in its own goroutine and delivers the result.
func ensureAsync(ctx context.Context, service *appsystem.Service) <-chan ensureResult {
	results := make(chan ensureResult, 1)

	go func() {
		identity, err := service.EnsureIdentity(ctx)
		results <- ensureResult{identity: identity, err: err}
	}()

	return results
}

// newPausedEnvironment returns an environment whose store reports every load and pauses
// every save until the test closes store.releaseSave.
func newPausedEnvironment() *testEnvironment {
	environment := newTestEnvironment()
	environment.store.loaded = make(chan struct{}, loadSignalBuffer)
	environment.store.saveStarted = make(chan struct{}, loadSignalBuffer)
	environment.store.releaseSave = make(chan struct{})

	return environment
}

func await[T any](t *testing.T, channel <-chan T, what string) T {
	t.Helper()

	select {
	case value := <-channel:
		return value
	case <-time.After(awaitTimeout):
		t.Fatalf("timed out after %v waiting for %s", awaitTimeout, what)

		var zero T

		return zero
	}
}

func awaitLoads(t *testing.T, store *memoryIdentityStore, count int) {
	t.Helper()

	for range count {
		await(t, store.loaded, "identity load")
	}
}

// startPausedCreator starts a first EnsureIdentity and returns once it holds the create
// slot and is paused inside SaveIdentity, after its two loads.
func startPausedCreator(t *testing.T, environment *testEnvironment, service *appsystem.Service) <-chan ensureResult {
	t.Helper()

	creator := ensureAsync(t.Context(), service)
	await(t, environment.store.saveStarted, "creator save")
	awaitLoads(t, environment.store, 2)

	return creator
}

func TestEnsureIdentityReturnsStoredIdentityAfterSaveConflict(t *testing.T) {
	t.Parallel()

	environment := newTestEnvironment()
	winner := system.Identity{ID: mustParseServerID(t, storedID), CreatedAt: time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)}
	environment.store.conflictWith = &winner

	identity, err := environment.service(t).EnsureIdentity(t.Context())
	if err != nil {
		t.Fatalf("EnsureIdentity error = %v", err)
	}

	if identity != winner {
		t.Errorf("EnsureIdentity = %+v, want the conflicting stored %+v", identity, winner)
	}
}

func TestEnsureIdentityWaiterUsesIdentityCreatedMeanwhile(t *testing.T) {
	t.Parallel()

	environment := newPausedEnvironment()
	service := environment.service(t)
	creator := startPausedCreator(t, environment, service)

	waiter := ensureAsync(t.Context(), service)
	// The waiter's first load sees no identity yet, so it must queue for the create slot.
	awaitLoads(t, environment.store, 1)
	close(environment.store.releaseSave)

	created := await(t, creator, "creator result")
	waited := await(t, waiter, "waiter result")

	if err := errors.Join(created.err, waited.err); err != nil {
		t.Fatalf("EnsureIdentity error = %v", err)
	}

	if waited.identity != created.identity {
		t.Errorf("waiter got %+v, want the creator's %+v", waited.identity, created.identity)
	}

	if saves, calls := environment.store.saves(), environment.generator.callCount(); saves != 1 || calls != 1 {
		t.Errorf("saves = %d, generator calls = %d, want 1 and 1", saves, calls)
	}
}

func TestEnsureIdentityWaiterReturnsContextErrorWhenCancelled(t *testing.T) {
	t.Parallel()

	environment := newPausedEnvironment()
	service := environment.service(t)
	creator := startPausedCreator(t, environment, service)

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	waiter := ensureAsync(ctx, service)
	awaitLoads(t, environment.store, 1)

	select {
	case result := <-waiter:
		t.Fatalf("waiter returned %+v while the creator held the slot, want it to wait", result)
	default:
	}

	cancel()

	waited := await(t, waiter, "cancelled waiter result")
	if !errors.Is(waited.err, context.Canceled) {
		t.Errorf("waiter error = %v, want context.Canceled", waited.err)
	}

	if waited.identity != (system.Identity{}) {
		t.Errorf("waiter identity = %+v, want zero", waited.identity)
	}

	close(environment.store.releaseSave)

	if created := await(t, creator, "creator result"); created.err != nil {
		t.Errorf("creator error = %v after waiter cancelled", created.err)
	}

	if saves := environment.store.saves(); saves != 1 {
		t.Errorf("saves = %d, want 1", saves)
	}
}

func TestEnsureIdentityLoadsWithoutWaitingForCreator(t *testing.T) {
	t.Parallel()

	environment := newPausedEnvironment()
	service := environment.service(t)
	creator := startPausedCreator(t, environment, service)

	// Another process stores an identity while this creator is still paused in its save.
	stored := system.Identity{ID: mustParseServerID(t, storedID), CreatedAt: environment.now.UTC()}
	environment.store.put(stored)

	ctx, cancel := context.WithTimeout(t.Context(), awaitTimeout)
	defer cancel()

	identity, err := service.EnsureIdentity(ctx)
	if err != nil {
		t.Fatalf("EnsureIdentity error = %v while the create slot was held, want the stored identity", err)
	}

	if identity != stored {
		t.Errorf("EnsureIdentity = %+v, want %+v", identity, stored)
	}

	close(environment.store.releaseSave)

	// The creator's save now conflicts, so it adopts the identity that was stored first.
	created := await(t, creator, "creator result")
	if created.err != nil || created.identity != stored {
		t.Errorf("creator = %+v, %v; want %+v, nil", created.identity, created.err, stored)
	}
}
