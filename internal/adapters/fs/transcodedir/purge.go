package transcodedir

import (
	"context"
	"errors"
	"fmt"
	"os"
)

// Purge deletes the leftovers of a previous run: every entry of <root>/sessions whose name is
// a canonical lowercase UUID. Other files and directories, the sessions directory itself and
// everything outside it are never touched; a UUID-named symlink is removed without following
// it. Purge continues after a failed removal, returns the number of entries removed and the
// joined errors, and stops early when ctx ends.
func (w *Workspace) Purge(ctx context.Context) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, fmt.Errorf("purge sessions: %w", err)
	}

	sessions, err := w.openSessions(true)
	if err != nil {
		return 0, fmt.Errorf("purge sessions: %w", err)
	}

	defer closeRoot(sessions)

	names, err := sessionEntryNames(sessions)
	if err != nil {
		return 0, fmt.Errorf("purge sessions: %w", err)
	}

	removeEntry := func(name string) error {
		return removeWithRetry(ctx, w.retry, func() error { return sessions.RemoveAll(name) })
	}

	removed, err := removeEntries(ctx, names, removeEntry)
	if err != nil {
		return removed, fmt.Errorf("purge sessions in %s: %w", w.sessionsDir, err)
	}

	return removed, nil
}

// removeEntries removes every named entry, continues after a failure and stops before the
// next entry once ctx ends. It returns the number removed and the joined errors.
func removeEntries(ctx context.Context, names []string, removeEntry func(name string) error) (int, error) {
	var (
		removed int
		errs    []error
	)

	for _, name := range names {
		if err := ctx.Err(); err != nil {
			return removed, errors.Join(append(errs, err)...)
		}

		err := removeEntry(name)
		if err != nil {
			errs = append(errs, fmt.Errorf("remove %s: %w", name, err))

			continue
		}

		removed++
	}

	return removed, errors.Join(errs...)
}

// sessionEntryNames lists the entries of the sessions directory that are named like a session.
func sessionEntryNames(sessions *os.Root) ([]string, error) {
	dir, err := sessions.Open(".")
	if err != nil {
		return nil, fmt.Errorf("open sessions directory: %w", err)
	}

	defer func() { _ = dir.Close() }()

	entries, err := dir.ReadDir(-1)
	if err != nil {
		return nil, fmt.Errorf("list sessions directory: %w", err)
	}

	var names []string

	for _, entry := range entries {
		if isSessionID(entry.Name()) {
			names = append(names, entry.Name())
		}
	}

	return names, nil
}
