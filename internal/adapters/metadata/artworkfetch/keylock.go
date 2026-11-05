package artworkfetch

import (
	"context"
	"fmt"
	"sync"
)

// keyLocks serialises work per key: concurrent fetches of one picture download it once,
// the others wait and then find the stored file. Entries are dropped when unused.
type keyLocks struct {
	mu   sync.Mutex
	held map[string]*keyLock
}

type keyLock struct {
	token   chan struct{}
	waiters int
}

// lock waits for key, or until ctx ends. The returned function releases the key.
func (locks *keyLocks) lock(ctx context.Context, key string) (func(), error) {
	locks.mu.Lock()
	if locks.held == nil {
		locks.held = map[string]*keyLock{}
	}

	entry, ok := locks.held[key]
	if !ok {
		entry = &keyLock{token: make(chan struct{}, 1)}
		locks.held[key] = entry
	}

	entry.waiters++
	locks.mu.Unlock()

	select {
	case entry.token <- struct{}{}:
		return func() { locks.release(key, entry, true) }, nil
	case <-ctx.Done():
		locks.release(key, entry, false)

		return nil, fmt.Errorf("wait for concurrent download: %w", ctx.Err())
	}
}

func (locks *keyLocks) release(key string, entry *keyLock, acquired bool) {
	locks.mu.Lock()
	defer locks.mu.Unlock()

	if acquired {
		<-entry.token
	}

	entry.waiters--
	if entry.waiters == 0 {
		delete(locks.held, key)
	}
}
