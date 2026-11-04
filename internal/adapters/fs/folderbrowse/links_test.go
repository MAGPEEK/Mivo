package folderbrowse_test

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/magpeek/mivo/internal/adapters/fs/folderbrowse"
	"github.com/magpeek/mivo/internal/domain"
)

// runTool runs a Windows tool to prepare a fixture; the arguments are test-controlled.
func runTool(t *testing.T, name string, args ...string) ([]byte, error) {
	t.Helper()

	return exec.CommandContext(t.Context(), name, args...).CombinedOutput() //nolint:gosec // G204: fixed tools, temp paths
}

func isErr(err, target error) bool { return errors.Is(err, target) }

func symlinkOrSkip(t *testing.T, target, link string) {
	t.Helper()

	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlinks not available: %v", err)
	}
}

func TestLinksInsideAreListedAndOutsideAreNot(t *testing.T) {
	t.Parallel()

	browser, root, id := newBrowser(t, folderbrowse.Options{})
	outside := t.TempDir()
	makeDirs(t, root, "real/deep")
	makeDirs(t, outside, "secret")
	symlinkOrSkip(t, "real", filepath.Join(root, "inside"))
	symlinkOrSkip(t, outside, filepath.Join(root, "escape"))
	symlinkOrSkip(t, filepath.Join("..", filepath.Base(outside)), filepath.Join(root, "dotdot"))
	symlinkOrSkip(t, filepath.Join(root, "real"), filepath.Join(root, "absolute"))

	listing, err := browser.List(t.Context(), id, "")
	if err != nil {
		t.Fatal(err)
	}

	if got := strings.Join(names(listing), ","); got != "inside,real" {
		t.Errorf("folders = %s, want inside,real", got)
	}

	if _, err := browser.Check(t.Context(), id, "escape"); err == nil {
		t.Error("Check followed a link out of the root")
	}

	if _, err := browser.Check(t.Context(), id, "escape/secret"); err == nil {
		t.Error("Check followed a link out of the root")
	}

	if _, err := browser.List(t.Context(), id, "escape"); err == nil {
		t.Error("List followed a link out of the root")
	}

	if _, err := browser.Check(t.Context(), id, "inside/deep"); err != nil {
		t.Errorf("Check through inner link: %v", err)
	}
}

func TestJunctionIsNotListed(t *testing.T) {
	t.Parallel()

	if runtime.GOOS != "windows" {
		t.Skip("junctions exist on Windows only")
	}

	browser, root, id := newBrowser(t, folderbrowse.Options{})
	target := t.TempDir()
	makeDirs(t, root, "plain")

	output, err := runTool(t, "cmd", "/c", "mklink", "/J", filepath.Join(root, "junction"), target)
	if err != nil {
		t.Skipf("mklink /J: %v %s", err, output)
	}

	listing, err := browser.List(t.Context(), id, "")
	if err != nil || strings.Join(names(listing), ",") != "plain" {
		t.Errorf("listing = %v, %v", names(listing), err)
	}

	makeDirs(t, target, "sub")

	for _, path := range []string{"junction", "junction/sub"} {
		if _, err := browser.Check(t.Context(), id, path); !isErr(err, domain.ErrInvalid) {
			t.Errorf("Check(%q) err = %v, want ErrInvalid", path, err)
		}
	}
}

func TestHiddenAttributeIsNotListed(t *testing.T) {
	t.Parallel()

	if runtime.GOOS != "windows" {
		t.Skip("attributes exist on Windows only")
	}

	browser, root, id := newBrowser(t, folderbrowse.Options{})
	makeDirs(t, root, "visible", "secret")

	if output, err := runTool(t, "attrib", "+H", filepath.Join(root, "secret")); err != nil {
		t.Skipf("attrib: %v %s", err, output)
	}

	listing, err := browser.List(t.Context(), id, "")
	if err != nil || strings.Join(names(listing), ",") != "visible" {
		t.Errorf("listing = %v, %v", names(listing), err)
	}

	makeDirs(t, root, "secret/inner")

	for _, path := range []string{"secret", "secret/inner"} {
		if _, err := browser.Check(t.Context(), id, path); !isErr(err, domain.ErrInvalid) {
			t.Errorf("Check(%q) err = %v, want ErrInvalid", path, err)
		}
	}
}

func TestWindowsOnlyPathRules(t *testing.T) {
	t.Parallel()

	if runtime.GOOS != "windows" {
		t.Skip("Windows path rules")
	}

	browser, _, id := newBrowser(t, folderbrowse.Options{})

	for _, path := range []string{"a:b", "dir.", "dir ", "CON", "COM1", "CON.txt", "a\x00b", "ok/NUL"} {
		if _, err := browser.Check(t.Context(), id, path); !isErr(err, domain.ErrInvalid) {
			t.Errorf("Check(%q) err = %v, want ErrInvalid", path, err)
		}
	}
}
