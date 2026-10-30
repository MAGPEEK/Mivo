//go:build windows

package transcodedir_test

import (
	"os/exec"
	"path/filepath"
	"testing"
)

// NTFS junctions need no privilege, unlike symlinks, so these tests always run on Windows and
// prove there that cleanup and file access never act through a link. Every path lives below
// t.TempDir().

// mklinkJunction creates the junction link pointing at the absolute directory target.
func mklinkJunction(t *testing.T, link, target string) {
	t.Helper()

	// mklink is a cmd built-in, so cmd is required. link and target are t.TempDir() paths passed
	// as separate arguments; no command string is assembled from them.
	command := exec.CommandContext(t.Context(), "cmd", "/c", "mklink", "/J", link, target) //nolint:gosec // G204: test-only temp paths, see above
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("mklink /J %s %s: %v: %s", link, target, err, output)
	}
}

// outsideDir returns a directory next to the transcode root holding the file keep.txt.
func outsideDir(t *testing.T, root string) (string, string) {
	t.Helper()

	outside := filepath.Join(filepath.Dir(root), "outside")
	keep := filepath.Join(outside, "keep.txt")
	writeFile(t, keep, "user data")

	return outside, keep
}

func TestJunctionNamedLikeASessionIsRemovedWithoutItsTarget(t *testing.T) {
	t.Parallel()

	for _, operation := range cleanupOperations() {
		t.Run(operation.name, func(t *testing.T) {
			t.Parallel()

			workspace, root := newWorkspace(t)
			outside, keep := outsideDir(t, root)
			mkdir(t, filepath.Join(root, sessionsDirName))

			link := filepath.Join(root, sessionsDirName, firstSessionID)
			mklinkJunction(t, link, outside)
			assertExists(t, filepath.Join(link, "keep.txt")) // the junction resolves

			operation.run(t, workspace)

			assertMissing(t, link)
			assertExists(t, keep)
		})
	}
}

func TestASessionsJunctionIsRefused(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name   string
		target func(root string) string
	}{
		{name: "leaving the root", target: func(root string) string { return filepath.Join(filepath.Dir(root), "outside") }},
		// Without the plain-directory check Purge would act on the transcode root itself.
		{name: "pointing at the root", target: func(root string) string { return root }},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			workspace, root := newWorkspace(t)
			target := testCase.target(root)
			decoy := filepath.Join(target, firstSessionID, playlistName)
			writeFile(t, decoy, playlistBody)

			mklinkJunction(t, filepath.Join(root, sessionsDirName), target)

			assertEveryOperationRefusesTheSessionsEntry(t, workspace)

			assertExists(t, decoy)
			assertMissing(t, filepath.Join(target, secondSessionID))
		})
	}
}

func TestOpenFileDoesNotFollowAJunctionInsideASession(t *testing.T) {
	t.Parallel()

	workspace, root := newWorkspace(t)
	outside, _ := outsideDir(t, root)

	session, err := workspace.Create(t.Context(), firstSessionID)
	if err != nil {
		t.Fatalf("Create error = %v", err)
	}

	mklinkJunction(t, filepath.Join(session, "out"), outside)

	file, err := workspace.OpenFile(firstSessionID, filepath.Join("out", "keep.txt"))
	if err == nil {
		_ = file.Close()
		t.Error("OpenFile through a junction error = nil, want an error")
	}
}

func TestRemoveAndPurgeKeepTheTargetOfAJunctionInsideASession(t *testing.T) {
	t.Parallel()

	for _, operation := range cleanupOperations() {
		t.Run(operation.name, func(t *testing.T) {
			t.Parallel()

			workspace, root := newWorkspace(t)
			outside, keep := outsideDir(t, root)

			session := filepath.Join(root, sessionsDirName, firstSessionID)
			writeFile(t, filepath.Join(session, playlistName), playlistBody)
			mklinkJunction(t, filepath.Join(session, "out"), outside)
			assertExists(t, filepath.Join(session, "out", "keep.txt")) // the junction resolves

			operation.run(t, workspace)

			assertMissing(t, session)
			assertExists(t, keep)
		})
	}
}
