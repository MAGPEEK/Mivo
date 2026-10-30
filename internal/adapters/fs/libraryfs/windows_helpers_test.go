//go:build windows

package libraryfs_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// NTFS junctions need no privilege, unlike symlinks, so the junction tests always run on
// Windows. Every path lives below t.TempDir().

// verbatimPrefix makes Win32 pass a path to NTFS unchanged, so tests can create names that the
// normal path rules strip or turn into devices ("aux", "name.", "name ").
const verbatimPrefix = `\\?\`

// mklinkJunction creates the junction link pointing at the absolute directory target.
func mklinkJunction(t *testing.T, link, target string) {
	t.Helper()

	mkdir(t, filepath.Dir(link))

	// mklink is a cmd built-in, so cmd is required. link and target are t.TempDir() paths passed
	// as separate arguments; no command string is assembled from them.
	command := exec.CommandContext(t.Context(), "cmd", "/c", "mklink", "/J", link, target) //nolint:gosec // G204: test-only temp paths, see above
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("mklink /J %s %s: %v: %s", link, target, err, output)
	}
}

// everyoneSID is the well-known SID of "Everyone", which includes the test user.
const everyoneSID = "*S-1-1-0"

// ACE rights for icacls: deny listing the directory itself, or it and every folder below it.
const (
	listDirectory          = "(RD)"
	listDirectoryInherited = "(OI)(CI)(RD)"
)

// denyListing adds an ACE that denies listing to everyone and removes it before t.TempDir's
// cleanup. The owner keeps the right to change the ACL, so the removal works.
func denyListing(t *testing.T, directory, rights string) {
	t.Helper()

	runIcacls(t, directory, "/deny", everyoneSID+":"+rights)
	t.Cleanup(func() { runIcacls(t, directory, "/remove:d", everyoneSID) })
}

func runIcacls(t *testing.T, directory string, arguments ...string) {
	t.Helper()

	// directory is a t.TempDir() path and the other arguments are constants, passed as separate
	// arguments; no command string is assembled.
	command := exec.Command("icacls", append([]string{directory}, arguments...)...) //nolint:gosec,noctx // G204: test-only temp path; also runs in t.Cleanup, after t.Context() is done
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("icacls %s %v: %v: %s", directory, arguments, err, output)
	}
}

// writeVerbatimFile creates root\directory\file through the verbatim prefix and removes the
// directory the same way before t.TempDir's cleanup, which cannot delete such names.
func writeVerbatimFile(t *testing.T, root, directory, file string) {
	t.Helper()

	verbatimDirectory := verbatimPrefix + filepath.Join(root, directory)
	if err := os.Mkdir(verbatimDirectory, testDirMode); err != nil {
		t.Fatalf("create %q: %v", verbatimDirectory, err)
	}

	t.Cleanup(func() {
		if err := os.RemoveAll(verbatimDirectory); err != nil {
			t.Errorf("remove %q: %v", verbatimDirectory, err)
		}
	})

	verbatimFile := verbatimDirectory + `\` + file
	if err := os.WriteFile(verbatimFile, []byte(testContent), testFileMode); err != nil {
		t.Fatalf("write %q: %v", verbatimFile, err)
	}
}
