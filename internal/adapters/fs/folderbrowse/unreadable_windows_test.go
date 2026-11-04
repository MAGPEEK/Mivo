//go:build windows

package folderbrowse_test

import (
	"path/filepath"
	"testing"

	"github.com/magpeek/mivo/internal/adapters/fs/folderbrowse"
	"github.com/magpeek/mivo/internal/domain"
)

func TestDeniedDirectoryIsCountedNotNamed(t *testing.T) {
	t.Parallel()

	browser, root, id := newBrowser(t, folderbrowse.Options{})
	makeDirs(t, root, "open", "locked")

	locked := filepath.Join(root, "locked")
	if output, err := runTool(t, "icacls", locked, "/deny", "*S-1-1-0:(OI)(CI)(RD,X)"); err != nil {
		t.Skipf("icacls deny: %v %s", err, output)
	}

	t.Cleanup(func() { _, _ = runTool(t, "icacls", locked, "/remove:d", "*S-1-1-0") })

	listing, err := browser.List(t.Context(), id, "")
	if err != nil {
		t.Fatal(err)
	}

	if len(listing.Folders) != 1 || listing.Folders[0].Name != "open" || listing.UnreadableCount != 1 {
		t.Errorf("listing = %+v", listing)
	}

	if _, err := browser.Check(t.Context(), id, "locked"); !isErr(err, domain.ErrForbidden) {
		t.Errorf("Check err = %v, want ErrForbidden", err)
	}
}
