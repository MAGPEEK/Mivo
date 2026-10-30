package system_test

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/system"
)

// loadSignalBuffer is large enough for every load a single test performs, so signalling a
// load never blocks the store.
const loadSignalBuffer = 16

// memoryIdentityStore is an in-memory IdentityStore. Like the real adapter it never
// overwrites a stored identity but reports domain.ErrConflict. The optional fields script
// failures and let tests observe and pause the store; they are set before the store is used.
type memoryIdentityStore struct {
	mutex     sync.Mutex
	identity  *system.Identity
	saveCount int

	// loadErr, when set, is returned by every load.
	loadErr error
	// scriptedLoadErrs is consumed one entry per load before loadErr applies; a nil entry
	// means the load behaves normally.
	scriptedLoadErrs []error
	// saveErr, when set, is returned by every save without storing anything.
	saveErr error
	// conflictWith simulates another writer that wins the race: the save stores this
	// identity instead and reports a conflict.
	conflictWith *system.Identity
	// loaded, when set, receives a value after every load.
	loaded chan struct{}
	// saveStarted and releaseSave, when set, pause every save: the store signals
	// saveStarted and waits for releaseSave before it does anything else.
	saveStarted chan struct{}
	releaseSave chan struct{}
}

func (store *memoryIdentityStore) LoadIdentity(context.Context) (system.Identity, error) {
	defer store.signalLoad()

	store.mutex.Lock()
	defer store.mutex.Unlock()

	if err := store.nextLoadErr(); err != nil {
		return system.Identity{}, err
	}

	if store.identity == nil {
		return system.Identity{}, fmt.Errorf("identity: %w", domain.ErrNotFound)
	}

	return *store.identity, nil
}

func (store *memoryIdentityStore) nextLoadErr() error {
	if len(store.scriptedLoadErrs) > 0 {
		next := store.scriptedLoadErrs[0]
		store.scriptedLoadErrs = store.scriptedLoadErrs[1:]

		return next
	}

	return store.loadErr
}

func (store *memoryIdentityStore) signalLoad() {
	if store.loaded == nil {
		return
	}

	select {
	case store.loaded <- struct{}{}:
	default:
	}
}

func (store *memoryIdentityStore) SaveIdentity(_ context.Context, identity system.Identity) error {
	store.pauseSave()

	store.mutex.Lock()
	defer store.mutex.Unlock()

	if store.saveErr != nil {
		return store.saveErr
	}

	if store.conflictWith != nil {
		store.identity = store.conflictWith

		return fmt.Errorf("identity written by another process: %w", domain.ErrConflict)
	}

	if store.identity != nil {
		return fmt.Errorf("identity already stored: %w", domain.ErrConflict)
	}

	store.saveCount++
	store.identity = &identity

	return nil
}

func (store *memoryIdentityStore) pauseSave() {
	if store.releaseSave == nil {
		return
	}

	store.saveStarted <- struct{}{}
	<-store.releaseSave
}

// put stores identity directly, as another process sharing the data directory would.
func (store *memoryIdentityStore) put(identity system.Identity) {
	store.mutex.Lock()
	defer store.mutex.Unlock()

	store.identity = &identity
}

func (store *memoryIdentityStore) saves() int {
	store.mutex.Lock()
	defer store.mutex.Unlock()

	return store.saveCount
}

// sequenceIDGenerator returns the configured IDs in order and then fails.
type sequenceIDGenerator struct {
	mutex sync.Mutex
	ids   []string
	err   error
	calls int
}

func (generator *sequenceIDGenerator) NewID() (string, error) {
	generator.mutex.Lock()
	defer generator.mutex.Unlock()

	generator.calls++

	if generator.err != nil {
		return "", generator.err
	}

	if len(generator.ids) == 0 {
		return "", fmt.Errorf("sequence id generator exhausted after %d calls", generator.calls-1)
	}

	next := generator.ids[0]
	generator.ids = generator.ids[1:]

	return next, nil
}

func (generator *sequenceIDGenerator) callCount() int {
	generator.mutex.Lock()
	defer generator.mutex.Unlock()

	return generator.calls
}

// pingFunc adapts a function to the DatabaseChecker port.
type pingFunc func(ctx context.Context) error

func (ping pingFunc) Ping(ctx context.Context) error {
	return ping(ctx)
}

func pingSucceeds(context.Context) error { return nil }

// fixedClock returns a clock that always reports moment.
func fixedClock(moment time.Time) func() time.Time {
	return func() time.Time { return moment }
}
