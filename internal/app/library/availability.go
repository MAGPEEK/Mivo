package library

import (
	"context"
	"sync"
	"time"
)

// rootChecks runs RootChecker calls with a timeout. A stat on a hung network mount can block
// in the kernel and ignore cancellation, so a check runs in its own goroutine and the caller
// stops waiting at the timeout. While a check of a root is still running, later callers join
// it instead of starting another: a client polling the library list must not pile up one
// blocked goroutine (and OS thread) per request.
//
// Trade-off: a check that never returns keeps its root unavailable, because every later
// caller joins it and gives up at the timeout; at most one goroutine per root stays blocked.
// No fresh check is started next to a stuck one: on a hung mount it would block as well, so
// it would only add blocked threads. A mount that recovers normally lets the stuck call
// return too, and the next call checks again.
type rootChecks struct {
	checker RootChecker
	timeout time.Duration

	mutex    sync.Mutex
	inFlight map[string]*rootCheck
}

// rootCheck is one running RootAvailable call; available is valid once done is closed.
type rootCheck struct {
	done      chan struct{}
	available bool
}

func newRootChecks(checker RootChecker, timeout time.Duration) *rootChecks {
	return &rootChecks{checker: checker, timeout: timeout, inFlight: map[string]*rootCheck{}}
}

// availableAll checks every root at the same time, so several hung mounts cost one timeout
// rather than one each. The result has one entry per root, in order.
func (checks *rootChecks) availableAll(ctx context.Context, roots []string) []bool {
	available := make([]bool, len(roots))

	var group sync.WaitGroup
	for index, root := range roots {
		group.Go(func() { available[index] = checks.available(ctx, root) })
	}

	group.Wait()

	return available
}

// available reports false when the check fails, outlasts the timeout or ctx ends first.
func (checks *rootChecks) available(ctx context.Context, root string) bool {
	// The check is shared by every caller that joins it, so it must not end with the caller
	// that happened to start it; run gives it a context of its own (see run).
	check := checks.start(root) //nolint:contextcheck // shared check, detached from the starting caller by design

	waitCtx, cancel := context.WithTimeout(ctx, checks.timeout)
	defer cancel()

	select {
	case <-check.done:
		return check.available
	case <-waitCtx.Done():
		return false
	}
}

// start returns the running check of root or starts a new one.
func (checks *rootChecks) start(root string) *rootCheck {
	checks.mutex.Lock()
	defer checks.mutex.Unlock()

	if running, found := checks.inFlight[root]; found {
		return running
	}

	check := &rootCheck{done: make(chan struct{})}
	checks.inFlight[root] = check

	go checks.run(root, check)

	return check
}

// run performs the check on a context of its own, detached from the caller that started it,
// because callers that join later wait for the same result and the caller's values do not
// matter to a stat. The timeout still reaches a cooperating checker as the deadline.
func (checks *rootChecks) run(root string, check *rootCheck) {
	checkCtx, cancel := context.WithTimeout(context.Background(), checks.timeout)
	defer cancel()

	check.available = checks.checker.RootAvailable(checkCtx, root)

	checks.mutex.Lock()
	delete(checks.inFlight, root)
	checks.mutex.Unlock()

	close(check.done)
}
