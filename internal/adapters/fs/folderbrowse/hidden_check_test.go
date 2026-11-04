package folderbrowse_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/magpeek/mivo/internal/adapters/fs/folderbrowse"
	"github.com/magpeek/mivo/internal/domain"
)

func TestHiddenAndSystemFoldersAreRefused(t *testing.T) {
	t.Parallel()

	browser, root, id := newBrowser(t, folderbrowse.Options{})
	paths := []string{".git", "@eaDir", "#recycle", "#snapshot", "$RECYCLE.BIN", "System Volume Information", "ok/.hidden", "@appdata/x"}

	for _, path := range paths {
		makeDirs(t, root, path)

		if _, err := browser.Check(t.Context(), id, path); !isErr(err, domain.ErrInvalid) {
			t.Errorf("Check(%q) err = %v, want ErrInvalid", path, err)
		}

		if _, err := browser.List(t.Context(), id, path); !isErr(err, domain.ErrInvalid) {
			t.Errorf("List(%q) err = %v, want ErrInvalid", path, err)
		}
	}
}

func TestLinkIntoReservedDirectoryIsHidden(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	makeDirs(t, root, "data/cache", "plain")
	symlinkOrSkip(t, "data", filepath.Join(root, "viadata"))

	browser := folderbrowse.New([]string{root}, folderbrowse.Options{Reserved: []string{filepath.Join(root, "data")}})
	id := folderbrowse.RootID(root)

	listing, err := browser.List(t.Context(), id, "")
	if err != nil || strings.Join(names(listing), ",") != "plain" {
		t.Errorf("listing = %v, %v", names(listing), err)
	}

	if _, err := browser.Check(t.Context(), id, "viadata/cache"); !isErr(err, domain.ErrInvalid) {
		t.Errorf("Check through link err = %v", err)
	}
}

func TestErrorsDoNotLeakTheRootPath(t *testing.T) {
	t.Parallel()

	missing := filepath.Join(t.TempDir(), "gone-root")
	browser := folderbrowse.New([]string{missing}, folderbrowse.Options{})
	id := folderbrowse.RootID(missing)

	live, root, liveID := newBrowser(t, folderbrowse.Options{})
	if err := os.WriteFile(filepath.Join(root, "file"), nil, 0o600); err != nil {
		t.Fatal(err)
	}

	_, errs := browser.List(t.Context(), id, "")
	_, errCheck := live.Check(t.Context(), liveID, "file")

	for _, err := range []error{errs, errCheck} {
		if err == nil || strings.Contains(err.Error(), root) || strings.Contains(err.Error(), missing) {
			t.Errorf("error leaks a path or is nil: %v", err)
		}
	}
}
