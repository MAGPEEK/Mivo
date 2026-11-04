//go:build unix

package folderbrowse_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/magpeek/mivo/internal/adapters/fs/folderbrowse"
	"github.com/magpeek/mivo/internal/domain"
)

func TestUnreadableDirectoryIsCountedNotNamed(t *testing.T) {
	t.Parallel()

	if os.Geteuid() == 0 {
		t.Skip("root ignores directory permissions; run as a non-root user")
	}

	browser, root, id := newBrowser(t, folderbrowse.Options{})
	makeDirs(t, root, "open", "locked")

	locked := filepath.Join(root, "locked")
	if err := os.Chmod(locked, 0); err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = os.Chmod(locked, 0o755) })

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

	if _, err := browser.List(t.Context(), id, "locked"); !isErr(err, domain.ErrForbidden) {
		t.Errorf("List err = %v, want ErrForbidden", err)
	}
}
