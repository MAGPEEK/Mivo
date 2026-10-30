package transcodedir_test

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
)

func TestPurgeRemovesOnlySessionDirectories(t *testing.T) {
	t.Parallel()

	workspace, root := newWorkspace(t)
	sessions := filepath.Join(root, sessionsDirName)

	removed := []string{
		filepath.Join(sessions, firstSessionID, playlistName),
		filepath.Join(sessions, secondSessionID, "nested", "seg-00000.m4s"),
	}
	for _, path := range removed {
		writeFile(t, path, playlistBody)
	}

	mkdir(t, filepath.Join(sessions, thirdSessionID))

	decoys := []string{
		filepath.Join(root, "user-file.txt"),
		filepath.Join(root, firstSessionID, playlistName),
		filepath.Join(root, "other", secondSessionID, playlistName),
		filepath.Join(sessions, "readme.txt"),
		filepath.Join(sessions, "not-a-uuid", playlistName),
		// A distinct UUID: on case-insensitive file systems the upper- and lowercase
		// spellings of one UUID are the same directory.
		filepath.Join(sessions, strings.ToUpper(unusedSessionID), playlistName),
		filepath.Join(sessions, "{"+secondSessionID+"}", playlistName),
		filepath.Join(sessions, firstSessionID+".bak", playlistName),
	}
	for _, path := range decoys {
		writeFile(t, path, playlistBody)
	}

	count, err := workspace.Purge(t.Context())
	if err != nil {
		t.Fatalf("Purge error = %v", err)
	}

	if count != 3 {
		t.Errorf("Purge count = %d, want 3", count)
	}

	for _, path := range removed {
		assertMissing(t, filepath.Dir(path))
	}

	assertMissing(t, filepath.Join(sessions, thirdSessionID))
	assertExists(t, sessions)

	for _, path := range decoys {
		assertExists(t, path)
	}
}

func TestPurgeRemovesAFileNamedLikeASession(t *testing.T) {
	t.Parallel()

	workspace, root := newWorkspace(t)
	stray := filepath.Join(root, sessionsDirName, firstSessionID)
	writeFile(t, stray, "stray file")

	count, err := workspace.Purge(t.Context())
	if err != nil || count != 1 {
		t.Fatalf("Purge = (%d, %v), want (1, nil)", count, err)
	}

	assertMissing(t, stray)
}

func TestPurgeCreatesTheSessionsDirectoryOnAFreshRoot(t *testing.T) {
	t.Parallel()

	workspace, root := newWorkspace(t)

	count, err := workspace.Purge(t.Context())
	if err != nil || count != 0 {
		t.Fatalf("Purge = (%d, %v), want (0, nil)", count, err)
	}

	assertExists(t, filepath.Join(root, sessionsDirName))
	assertPermissions(t, filepath.Join(root, sessionsDirName))
}

func TestSessionSymlinksAreRemovedWithoutTheirTargets(t *testing.T) {
	t.Parallel()

	for _, operation := range cleanupOperations() {
		t.Run(operation.name, func(t *testing.T) {
			t.Parallel()

			workspace, root := newWorkspace(t)
			outside := filepath.Join(filepath.Dir(root), "outside")
			outsideFile := filepath.Join(outside, "keep.txt")
			writeFile(t, outsideFile, "user data")
			mkdir(t, filepath.Join(root, sessionsDirName))

			link := filepath.Join(root, sessionsDirName, firstSessionID)
			symlinkOrSkip(t, outside, link)

			operation.run(t, workspace)

			assertMissing(t, link)
			assertExists(t, outsideFile)
		})
	}
}

func TestRemoveAndPurgeKeepTheTargetOfANestedSymlink(t *testing.T) {
	t.Parallel()

	for _, operation := range cleanupOperations() {
		t.Run(operation.name, func(t *testing.T) {
			t.Parallel()

			workspace, root := newWorkspace(t)
			outsideFile := filepath.Join(filepath.Dir(root), "outside", "keep.txt")
			writeFile(t, outsideFile, "user data")

			session := filepath.Join(root, sessionsDirName, firstSessionID)
			mkdir(t, session)
			symlinkOrSkip(t, filepath.Join("..", "..", "..", "outside"), filepath.Join(session, "out"))
			assertExists(t, filepath.Join(session, "out", "keep.txt")) // the link resolves

			operation.run(t, workspace)

			assertMissing(t, session)
			assertExists(t, outsideFile)
		})
	}
}

func TestASessionsSymlinkIsRefused(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		// target is relative to the transcode root; decoyDir is where a UUID-named decoy lives.
		target   string
		decoyDir func(root string) string
	}{
		{
			name: "leaving the root", target: filepath.Join("..", "outside"),
			decoyDir: func(root string) string { return filepath.Join(filepath.Dir(root), "outside") },
		},
		{
			// Without the plain-directory check Purge would act on the transcode root itself.
			name: "pointing at the root", target: ".",
			decoyDir: func(root string) string { return root },
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			workspace, root := newWorkspace(t)
			decoyDir := testCase.decoyDir(root)
			decoy := filepath.Join(decoyDir, firstSessionID, playlistName)
			writeFile(t, decoy, playlistBody)

			symlinkOrSkip(t, testCase.target, filepath.Join(root, sessionsDirName))

			assertEveryOperationRefusesTheSessionsEntry(t, workspace)

			assertExists(t, decoy)
			assertMissing(t, filepath.Join(decoyDir, secondSessionID))
		})
	}
}

func TestEveryOperationRefusesASessionsFile(t *testing.T) {
	t.Parallel()

	workspace, root := newWorkspace(t)
	sessions := filepath.Join(root, sessionsDirName)
	writeFile(t, sessions, "not a directory")

	assertEveryOperationRefusesTheSessionsEntry(t, workspace)

	assertExists(t, sessions)
}

func TestPurgeStopsOnACancelledContext(t *testing.T) {
	t.Parallel()

	workspace, root := newWorkspace(t)
	session := filepath.Join(root, sessionsDirName, firstSessionID, playlistName)
	writeFile(t, session, playlistBody)

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	count, err := workspace.Purge(ctx)
	if !errors.Is(err, context.Canceled) || count != 0 {
		t.Errorf("Purge = (%d, %v), want (0, context.Canceled)", count, err)
	}

	assertExists(t, session)
}
