//go:build windows

package transcodedir

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestPurgeContinuesAfterALockedSessionAndReportsIt(t *testing.T) {
	t.Parallel()

	const (
		lockedSession = "0b8c6d4e-3f2a-4c1b-9d8e-7f6a5b4c3d2e"
		freeSession   = "6f1e2d3c-4b5a-4987-8a6b-5c4d3e2f1a0b"
		segmentMode   = 0o600
		shortAttempts = 2
	)

	root := t.TempDir()
	policy := retryPolicy{attempts: shortAttempts, delay: time.Millisecond, retryable: isLockError}

	workspace, err := newWorkspace(root, policy)
	if err != nil {
		t.Fatalf("newWorkspace error = %v", err)
	}

	locked := createSessionFile(t, workspace, lockedSession, segmentMode)
	createSessionFile(t, workspace, freeSession, segmentMode)

	// An open handle without delete sharing blocks deletion like a just-killed FFmpeg.
	handle, err := os.Open(filepath.Clean(locked))
	if err != nil {
		t.Fatalf("open %s: %v", locked, err)
	}

	t.Cleanup(func() { _ = handle.Close() })

	count, err := workspace.Purge(t.Context())
	if !isLockError(err) || count != 1 {
		t.Errorf("Purge = (%d, %v), want (1, a lock error for the locked session)", count, err)
	}

	if _, statErr := os.Stat(filepath.Join(root, sessionsDirName, freeSession)); !errors.Is(statErr, fs.ErrNotExist) {
		t.Errorf("free session must be removed, stat error = %v", statErr)
	}
}

func createSessionFile(t *testing.T, workspace *Workspace, sessionID string, mode fs.FileMode) string {
	t.Helper()

	dir, err := workspace.Create(t.Context(), sessionID)
	if err != nil {
		t.Fatalf("Create(%s) error = %v", sessionID, err)
	}

	path := filepath.Join(dir, "seg-00000.m4s")
	if err := os.WriteFile(path, []byte("segment"), mode); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}

	return path
}
