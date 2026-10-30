//go:build !windows

package transcodedir_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestPurgeContinuesAfterAPermanentFailureAndReportsIt(t *testing.T) {
	t.Parallel()

	const (
		readOnlyDirMode = 0o500
		// A permanent error must not be retried, so Purge returns well below one retry budget.
		maxPurgeDuration = time.Second
	)

	if os.Geteuid() == 0 {
		t.Skip("root ignores directory permissions; run as an unprivileged user")
	}

	workspace, root := newWorkspace(t)
	sessions := filepath.Join(root, sessionsDirName)
	stuck := filepath.Join(sessions, firstSessionID)
	stuckFile := filepath.Join(stuck, playlistName)
	writeFile(t, stuckFile, playlistBody)
	writeFile(t, filepath.Join(sessions, secondSessionID, playlistName), playlistBody)

	// Without write permission on the session directory its file cannot be unlinked.
	if err := os.Chmod(stuck, readOnlyDirMode); err != nil {
		t.Fatalf("chmod %s: %v", stuck, err)
	}

	t.Cleanup(func() { _ = os.Chmod(stuck, testDirMode) })

	started := time.Now()
	count, err := workspace.Purge(t.Context())

	if err == nil || count != 1 {
		t.Errorf("Purge = (%d, %v), want (1, an error for the read-only session)", count, err)
	}

	if elapsed := time.Since(started); elapsed > maxPurgeDuration {
		t.Errorf("Purge took %v, want no retries for a permanent error (< %v)", elapsed, maxPurgeDuration)
	}

	assertExists(t, stuckFile)
	assertMissing(t, filepath.Join(sessions, secondSessionID))
}
