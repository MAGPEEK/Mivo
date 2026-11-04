package folderbrowse

import (
	"context"
	"sync"
	"time"
)

// availabilityCheck is one running stat of a root, shared by all concurrent callers so a hung
// mount never collects a second blocked stat (ADR 0022 Notes M2-18).
type availabilityCheck struct {
	done   chan struct{}
	result bool
}

// Roots returns the configured roots with their availability.
func (b *Browser) Roots(ctx context.Context) []Root {
	roots := make([]Root, len(b.roots))

	var group sync.WaitGroup

	for index, root := range b.roots {
		roots[index] = Root{ID: root.id, Path: root.path}

		group.Go(func() {
			roots[index].Available = b.available(ctx, root.path)
		})
	}

	group.Wait()

	return roots
}

func (b *Browser) available(ctx context.Context, path string) bool {
	if ctx.Err() != nil {
		return false
	}

	check := b.startCheck(path)

	timer := time.NewTimer(b.timeout)
	defer timer.Stop()

	select {
	case <-check.done:
		return check.result
	case <-timer.C:
		return false
	case <-ctx.Done():
		return false
	}
}

func (b *Browser) startCheck(path string) *availabilityCheck {
	b.mu.Lock()
	defer b.mu.Unlock()

	if running, ok := b.inflight[path]; ok {
		return running
	}

	check := &availabilityCheck{done: make(chan struct{})}
	b.inflight[path] = check

	go func() {
		check.result = b.statRoot(path)

		b.mu.Lock()
		delete(b.inflight, path)
		b.mu.Unlock()
		close(check.done)
	}()

	return check
}

func statDirectory(path string) bool {
	root, err := openRoot(path)
	if err != nil {
		return false
	}

	defer func() { _ = root.Close() }()

	info, err := root.Stat(".")

	return err == nil && info.IsDir()
}
