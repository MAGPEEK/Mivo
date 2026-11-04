package images

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/magpeek/mivo/internal/domain"
)

// flight is one render that every request for the same key waits for.
type flight struct {
	done chan struct{}
	// waiters counts the requests sharing the flight; tests use it to know all have joined.
	waiters int
	data    []byte
	err     error
}

// flightGroup lets concurrent requests for one key share a render and bounds how many
// renders run at a time.
type flightGroup struct {
	mutex   sync.Mutex
	flights map[Key]*flight
	slots   chan struct{}
	timeout time.Duration
}

func newFlightGroup(maxConcurrent int, timeout time.Duration) *flightGroup {
	return &flightGroup{
		flights: make(map[Key]*flight),
		slots:   make(chan struct{}, maxConcurrent),
		timeout: timeout,
	}
}

// do runs work once per key at a time and waits for its result or the end of ctx. The work
// runs detached from the caller's ctx (bounded by the group timeout), so a client that gives
// up does not fail the other requests sharing the render.
func (group *flightGroup) do(ctx context.Context, key Key, work func(context.Context) ([]byte, error)) ([]byte, error) {
	group.mutex.Lock()

	current, running := group.flights[key]
	if !running {
		current = &flight{done: make(chan struct{})}
		group.flights[key] = current

		go group.run(context.WithoutCancel(ctx), key, current, work)
	}

	current.waiters++
	group.mutex.Unlock()

	select {
	case <-ctx.Done():
		return nil, fmt.Errorf("wait for render: %w", ctx.Err())
	case <-current.done:
		return current.data, current.err
	}
}

// run waits for a render slot, runs work and publishes the result to every waiter.
func (group *flightGroup) run(ctx context.Context, key Key, current *flight, work func(context.Context) ([]byte, error)) {
	defer func() {
		group.mutex.Lock()
		delete(group.flights, key)
		group.mutex.Unlock()
		close(current.done)
	}()

	ctx, cancel := context.WithTimeout(ctx, group.timeout)
	defer cancel()

	select {
	case <-ctx.Done():
		current.err = fmt.Errorf("wait for a render slot: %w", errors.Join(ctx.Err(), domain.ErrUnavailable))

		return
	case group.slots <- struct{}{}:
	}

	defer func() { <-group.slots }()

	current.data, current.err = work(ctx)
}

// waiting returns the number of requests that joined the running flight for key.
func (group *flightGroup) waiting(key Key) int {
	group.mutex.Lock()
	defer group.mutex.Unlock()

	if current, ok := group.flights[key]; ok {
		return current.waiters
	}

	return 0
}
