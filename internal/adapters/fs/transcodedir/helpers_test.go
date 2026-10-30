package transcodedir_test

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/magpeek/mivo/internal/adapters/fs/transcodedir"
)

const (
	// Canonical lowercase version 4 UUIDs used as session IDs.
	firstSessionID  = "0b8c6d4e-3f2a-4c1b-9d8e-7f6a5b4c3d2e"
	secondSessionID = "6f1e2d3c-4b5a-4987-8a6b-5c4d3e2f1a0b"
	thirdSessionID  = "a1b2c3d4-e5f6-4a7b-8c9d-0e1f2a3b4c5d"
	unusedSessionID = "f0e1d2c3-b4a5-4697-8879-6a5b4c3d2e1f"

	sessionsDirName = "sessions"
	testFileMode    = 0o600
	testDirMode     = 0o750
	playlistName    = "index.m3u8"
	playlistBody    = "#EXTM3U\n"
)

// newWorkspace returns a workspace on a fresh transcode root inside t.TempDir().
func newWorkspace(t *testing.T) (*transcodedir.Workspace, string) {
	t.Helper()

	root := filepath.Join(t.TempDir(), "transcode")
	if err := os.Mkdir(root, testDirMode); err != nil {
		t.Fatalf("create transcode root: %v", err)
	}

	workspace, err := transcodedir.New(root)
	if err != nil {
		t.Fatalf("New(%q) error = %v", root, err)
	}

	return workspace, root
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), testDirMode); err != nil {
		t.Fatalf("create parent of %s: %v", path, err)
	}

	if err := os.WriteFile(path, []byte(content), testFileMode); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func mkdir(t *testing.T, path string) {
	t.Helper()

	if err := os.MkdirAll(path, testDirMode); err != nil {
		t.Fatalf("create %s: %v", path, err)
	}
}

func assertExists(t *testing.T, path string) {
	t.Helper()

	if _, err := os.Lstat(path); err != nil {
		t.Errorf("%s must still exist: %v", path, err)
	}
}

func assertMissing(t *testing.T, path string) {
	t.Helper()

	_, err := os.Lstat(path)
	if !errors.Is(err, os.ErrNotExist) {
		t.Errorf("Lstat(%s) error = %v, want not exist", path, err)
	}
}

// cleanupOperation deletes the session firstSessionID in one of the two ways the server does.
type cleanupOperation struct {
	name string
	run  func(t *testing.T, workspace *transcodedir.Workspace)
}

func cleanupOperations() []cleanupOperation {
	return []cleanupOperation{
		{name: "Remove", run: func(t *testing.T, workspace *transcodedir.Workspace) {
			t.Helper()

			if err := workspace.Remove(t.Context(), firstSessionID); err != nil {
				t.Fatalf("Remove error = %v", err)
			}
		}},
		{name: "Purge", run: func(t *testing.T, workspace *transcodedir.Workspace) {
			t.Helper()

			if count, err := workspace.Purge(t.Context()); err != nil || count != 1 {
				t.Fatalf("Purge = (%d, %v), want (1, nil)", count, err)
			}
		}},
	}
}

// assertEveryOperationRefusesTheSessionsEntry checks that no method acts through a sessions
// entry that is not a plain directory.
func assertEveryOperationRefusesTheSessionsEntry(t *testing.T, workspace *transcodedir.Workspace) {
	t.Helper()

	if _, err := workspace.Create(t.Context(), secondSessionID); !errors.Is(err, transcodedir.ErrSessionsNotPlainDirectory) {
		t.Errorf("Create error = %v, want ErrSessionsNotPlainDirectory", err)
	}

	if _, err := workspace.Purge(t.Context()); !errors.Is(err, transcodedir.ErrSessionsNotPlainDirectory) {
		t.Errorf("Purge error = %v, want ErrSessionsNotPlainDirectory", err)
	}

	if err := workspace.Remove(t.Context(), firstSessionID); !errors.Is(err, transcodedir.ErrSessionsNotPlainDirectory) {
		t.Errorf("Remove error = %v, want ErrSessionsNotPlainDirectory", err)
	}

	file, err := workspace.OpenFile(firstSessionID, playlistName)
	if !errors.Is(err, transcodedir.ErrSessionsNotPlainDirectory) {
		t.Errorf("OpenFile error = %v, want ErrSessionsNotPlainDirectory", err)
	}

	if file != nil {
		_ = file.Close()
	}
}

// symlinkOrSkip creates a symlink or skips the test where the OS does not allow it (Windows
// without developer mode or administrator rights).
func symlinkOrSkip(t *testing.T, target, link string) {
	t.Helper()

	if err := os.Symlink(target, link); err != nil {
		if runtime.GOOS == "windows" {
			t.Skipf("symlinks not permitted on this Windows account: %v", err)
		}

		t.Fatalf("symlink %s -> %s: %v", link, target, err)
	}
}
