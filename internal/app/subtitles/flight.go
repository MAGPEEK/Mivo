package subtitles

import (
	"context"
	"fmt"
	"sync"
)

// flight is one conversion that every request for the same destination waits for.
type flight struct {
	done chan struct{}
	err  error
}

// flightGroup lets concurrent requests for one destination share a conversion and bounds how
// many conversions run at a time.
type flightGroup struct {
	mutex   sync.Mutex
	flights map[string]*flight
	slots   chan struct{}
}

func newFlightGroup(maxConcurrent int) *flightGroup {
	return &flightGroup{flights: make(map[string]*flight), slots: make(chan struct{}, maxConcurrent)}
}

// do runs work once per key at a time and waits for its result or the end of ctx. The work
// runs detached from the first caller's ctx, so a client that gives up does not fail the other
// requests sharing the conversion; the converter bounds its own run time.
func (group *flightGroup) do(ctx context.Context, key string, work func(context.Context) error) error {
	group.mutex.Lock()

	current, running := group.flights[key]
	if !running {
		current = &flight{done: make(chan struct{})}
		group.flights[key] = current

		go group.run(context.WithoutCancel(ctx), key, current, work)
	}

	group.mutex.Unlock()

	select {
	case <-ctx.Done():
		return fmt.Errorf("wait for conversion: %w", ctx.Err())
	case <-current.done:
		return current.err
	}
}

// run waits for a conversion slot, runs work and publishes the result to every waiter.
func (group *flightGroup) run(ctx context.Context, key string, current *flight, work func(context.Context) error) {
	defer func() {
		group.mutex.Lock()
		delete(group.flights, key)
		group.mutex.Unlock()
		close(current.done)
	}()

	group.slots <- struct{}{}

	defer func() { <-group.slots }()

	current.err = work(ctx)
}
