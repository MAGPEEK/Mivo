package transcodedir_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/magpeek/mivo/internal/adapters/fs/transcodedir"
	"github.com/magpeek/mivo/internal/domain"
)

type sessionIDCase struct {
	name string
	id   string
}

// invalidSessionIDs returns IDs that every method taking a session ID must reject.
func invalidSessionIDs() []sessionIDCase {
	return []sessionIDCase{
		{name: "empty", id: ""},
		{name: "uppercase", id: strings.ToUpper(firstSessionID)},
		{name: "braces", id: "{" + firstSessionID + "}"},
		{name: "urn", id: "urn:uuid:" + firstSessionID},
		{name: "without hyphens", id: strings.ReplaceAll(firstSessionID, "-", "")},
		{name: "trailing space", id: firstSessionID + " "},
		{name: "parent", id: ".."},
		{name: "current", id: "."},
		{name: "traversal", id: "../" + firstSessionID},
		{name: "nested", id: firstSessionID + "/x"},
		{name: "sessions", id: sessionsDirName},
		{name: "not hex", id: "0b8c6d4e-3f2a-4c1b-9d8e-7f6a5b4c3d2g"},
	}
}

func TestNewRejectsRootsThatAreNotAbsolute(t *testing.T) {
	t.Parallel()

	for _, root := range []string{"", "transcode", "./transcode", filepath.Join("..", "transcode")} {
		t.Run(root, func(t *testing.T) {
			t.Parallel()

			if _, err := transcodedir.New(root); !errors.Is(err, domain.ErrInvalid) {
				t.Errorf("New(%q) error = %v, want ErrInvalid", root, err)
			}
		})
	}
}

func TestNewDoesNotTouchTheFileSystem(t *testing.T) {
	t.Parallel()

	root := filepath.Join(t.TempDir(), "transcode")

	if _, err := transcodedir.New(root); err != nil {
		t.Fatalf("New error = %v", err)
	}

	assertMissing(t, root)
}

func TestCreateMakesTheSessionDirectoryBelowSessions(t *testing.T) {
	t.Parallel()

	workspace, root := newWorkspace(t)

	dir, err := workspace.Create(t.Context(), firstSessionID)
	if err != nil {
		t.Fatalf("Create error = %v", err)
	}

	want := filepath.Join(root, sessionsDirName, firstSessionID)
	if dir != want {
		t.Errorf("Create dir = %q, want %q", dir, want)
	}

	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		t.Fatalf("session directory missing or not a directory: %v", err)
	}

	assertPermissions(t, filepath.Join(root, sessionsDirName))
	assertPermissions(t, dir)
}

func assertPermissions(t *testing.T, dir string) {
	t.Helper()

	if runtime.GOOS == "windows" {
		return // Windows ignores Unix permission bits.
	}

	info, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("stat %s: %v", dir, err)
	}

	if got := info.Mode().Perm(); got != testDirMode {
		t.Errorf("mode of %s = %o, want %o", dir, got, testDirMode)
	}
}

func TestCreateMakesAMissingTranscodeRoot(t *testing.T) {
	t.Parallel()

	root := filepath.Join(t.TempDir(), "missing", "transcode")

	workspace, err := transcodedir.New(root)
	if err != nil {
		t.Fatalf("New error = %v", err)
	}

	if _, err := workspace.Create(t.Context(), firstSessionID); err != nil {
		t.Fatalf("Create error = %v", err)
	}

	assertExists(t, filepath.Join(root, sessionsDirName, firstSessionID))
}

func TestCreateRejectsNonCanonicalSessionIDs(t *testing.T) {
	t.Parallel()

	for _, testCase := range invalidSessionIDs() {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			workspace, root := newWorkspace(t)

			if _, err := workspace.Create(t.Context(), testCase.id); !errors.Is(err, domain.ErrInvalid) {
				t.Errorf("Create(%q) error = %v, want ErrInvalid", testCase.id, err)
			}

			assertMissing(t, filepath.Join(root, sessionsDirName))
		})
	}
}

func TestCreateReportsAnExistingSessionAsConflict(t *testing.T) {
	t.Parallel()

	workspace, root := newWorkspace(t)
	writeFile(t, filepath.Join(root, sessionsDirName, firstSessionID, playlistName), playlistBody)

	if _, err := workspace.Create(t.Context(), firstSessionID); !errors.Is(err, domain.ErrConflict) {
		t.Errorf("Create error = %v, want ErrConflict", err)
	}

	assertExists(t, filepath.Join(root, sessionsDirName, firstSessionID, playlistName))
}

func TestCreateHonoursACancelledContext(t *testing.T) {
	t.Parallel()

	workspace, root := newWorkspace(t)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	if _, err := workspace.Create(ctx, firstSessionID); !errors.Is(err, context.Canceled) {
		t.Errorf("Create error = %v, want context.Canceled", err)
	}

	assertMissing(t, filepath.Join(root, sessionsDirName))
}

func TestRemoveDeletesTheSessionRecursively(t *testing.T) {
	t.Parallel()

	workspace, root := newWorkspace(t)

	dir, err := workspace.Create(t.Context(), firstSessionID)
	if err != nil {
		t.Fatalf("Create error = %v", err)
	}

	writeFile(t, filepath.Join(dir, playlistName), playlistBody)
	writeFile(t, filepath.Join(dir, "nested", "seg-00000.m4s"), "segment")
	writeFile(t, filepath.Join(root, sessionsDirName, secondSessionID, playlistName), playlistBody)

	if err := workspace.Remove(t.Context(), firstSessionID); err != nil {
		t.Fatalf("Remove error = %v", err)
	}

	assertMissing(t, dir)
	assertExists(t, filepath.Join(root, sessionsDirName))
	assertExists(t, filepath.Join(root, sessionsDirName, secondSessionID, playlistName))
}

func TestRemoveOfAMissingSessionIsNotAnError(t *testing.T) {
	t.Parallel()

	workspace, root := newWorkspace(t)

	if err := workspace.Remove(t.Context(), firstSessionID); err != nil {
		t.Errorf("Remove without sessions directory error = %v, want nil", err)
	}

	mkdir(t, filepath.Join(root, sessionsDirName))

	if err := workspace.Remove(t.Context(), firstSessionID); err != nil {
		t.Errorf("Remove of a missing session error = %v, want nil", err)
	}
}

func TestRemoveWithoutTranscodeRootIsNotAnError(t *testing.T) {
	t.Parallel()

	workspace, err := transcodedir.New(filepath.Join(t.TempDir(), "missing"))
	if err != nil {
		t.Fatalf("New error = %v", err)
	}

	if err := workspace.Remove(t.Context(), firstSessionID); err != nil {
		t.Errorf("Remove error = %v, want nil", err)
	}
}

func TestRemoveRejectsNonCanonicalSessionIDs(t *testing.T) {
	t.Parallel()

	for _, testCase := range invalidSessionIDs() {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			workspace, root := newWorkspace(t)
			decoy := filepath.Join(root, sessionsDirName, strings.ToUpper(firstSessionID), playlistName)
			writeFile(t, decoy, playlistBody)

			if err := workspace.Remove(t.Context(), testCase.id); !errors.Is(err, domain.ErrInvalid) {
				t.Errorf("Remove(%q) error = %v, want ErrInvalid", testCase.id, err)
			}

			assertExists(t, decoy)
		})
	}
}

func TestRemoveSucceedsOnceALockedFileIsReleased(t *testing.T) {
	t.Parallel()

	const releaseAfter = 300 * time.Millisecond

	workspace, root := newWorkspace(t)
	segment := filepath.Join(root, sessionsDirName, firstSessionID, "seg-00000.m4s")
	writeFile(t, segment, "segment")

	// An open handle without FILE_SHARE_DELETE blocks deletion on Windows, like the files of
	// an FFmpeg process that was just killed. Other platforms delete immediately.
	locked, err := os.Open(filepath.Clean(segment))
	if err != nil {
		t.Fatalf("open segment: %v", err)
	}

	release := time.AfterFunc(releaseAfter, func() { _ = locked.Close() })
	t.Cleanup(func() {
		release.Stop()
		_ = locked.Close()
	})

	if err := workspace.Remove(t.Context(), firstSessionID); err != nil {
		t.Fatalf("Remove error = %v", err)
	}

	assertMissing(t, filepath.Dir(segment))
}

func TestRemoveGivesUpWhenTheContextIsCancelledWhileLocked(t *testing.T) {
	t.Parallel()

	if runtime.GOOS != "windows" {
		t.Skip("open files block deletion only on Windows")
	}

	workspace, root := newWorkspace(t)
	segment := filepath.Join(root, sessionsDirName, firstSessionID, "seg-00000.m4s")
	writeFile(t, segment, "segment")

	locked, err := os.Open(filepath.Clean(segment))
	if err != nil {
		t.Fatalf("open segment: %v", err)
	}

	t.Cleanup(func() { _ = locked.Close() })

	ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
	defer cancel()

	if err := workspace.Remove(ctx, firstSessionID); !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("Remove error = %v, want context.DeadlineExceeded", err)
	}

	assertExists(t, segment)
}
