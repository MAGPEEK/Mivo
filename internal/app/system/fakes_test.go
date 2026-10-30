package system_test

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/system"
)

// memoryIdentityStore is an in-memory IdentityStore. loadErr and saveErr, when set, are
// returned instead of touching the stored identity.
type memoryIdentityStore struct {
	mutex     sync.Mutex
	identity  *system.Identity
	loadErr   error
	saveErr   error
	saveCount int
}

func (store *memoryIdentityStore) LoadIdentity(context.Context) (system.Identity, error) {
	store.mutex.Lock()
	defer store.mutex.Unlock()

	if store.loadErr != nil {
		return system.Identity{}, store.loadErr
	}

	if store.identity == nil {
		return system.Identity{}, fmt.Errorf("identity: %w", domain.ErrNotFound)
	}

	return *store.identity, nil
}

func (store *memoryIdentityStore) SaveIdentity(_ context.Context, identity system.Identity) error {
	store.mutex.Lock()
	defer store.mutex.Unlock()

	if store.saveErr != nil {
		return store.saveErr
	}

	store.saveCount++
	store.identity = &identity

	return nil
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
