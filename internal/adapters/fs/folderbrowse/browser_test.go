package folderbrowse_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/magpeek/mivo/internal/adapters/fs/folderbrowse"
	"github.com/magpeek/mivo/internal/domain"
)

func makeDirs(t *testing.T, base string, names ...string) {
	t.Helper()

	for _, name := range names {
		if err := os.MkdirAll(filepath.Join(base, filepath.FromSlash(name)), 0o750); err != nil {
			t.Fatalf("mkdir %s: %v", name, err)
		}
	}
}

// newBrowser returns a Browser over a fresh root and the root directory and id.
func newBrowser(t *testing.T, options folderbrowse.Options) (*folderbrowse.Browser, string, string) {
	t.Helper()

	root := t.TempDir()

	return folderbrowse.New([]string{root}, options), root, folderbrowse.RootID(root)
}

func names(listing folderbrowse.Listing) []string {
	result := make([]string, 0, len(listing.Folders))
	for _, folder := range listing.Folders {
		result = append(result, folder.Path)
	}

	return result
}

func TestListSortsAndHidesSystemFolders(t *testing.T) {
	t.Parallel()

	browser, root, id := newBrowser(t, folderbrowse.Options{})
	makeDirs(t, root, "beta", "Alpha", "Zed", "alpha2", "@eaDir", "#recycle", ".hidden", "$RECYCLE.BIN", "sub/inner")

	if err := os.WriteFile(filepath.Join(root, "file.txt"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	listing, err := browser.List(t.Context(), id, "")
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	got := strings.Join(names(listing), ",")
	// Case-insensitive order: "Zed" sorts after "sub".
	if want := "Alpha,alpha2,beta,sub,Zed"; got != want {
		t.Errorf("folders = %s, want %s", got, want)
	}

	nested, err := browser.List(t.Context(), id, "sub")
	if err != nil || len(nested.Folders) != 1 || nested.Folders[0].Path != "sub/inner" {
		t.Errorf("nested = %+v, %v", nested, err)
	}

	for _, folder := range listing.Folders {
		if filepath.IsAbs(folder.Path) || strings.Contains(folder.Path, root) {
			t.Errorf("absolute path leaked: %q", folder.Path)
		}
	}
}

func TestListRefusesBadPaths(t *testing.T) {
	t.Parallel()

	browser, _, id := newBrowser(t, folderbrowse.Options{})
	bad := []string{
		"..", "a/../../x", "../x", "/etc", "C:/Windows", "C:x", "//server/share", `a\..\b`, "a\x00b", "a//b", "./a", "a/.",
	}

	for _, path := range bad {
		t.Run(fmt.Sprintf("%q", path), func(t *testing.T) {
			t.Parallel()

			if _, err := browser.List(t.Context(), id, path); !isErr(err, domain.ErrInvalid) {
				t.Errorf("List err = %v, want ErrInvalid", err)
			}

			if _, err := browser.Check(t.Context(), id, path); !isErr(err, domain.ErrInvalid) {
				t.Errorf("Check err = %v, want ErrInvalid", err)
			}
		})
	}
}

func TestEncodedDotsAreAnOrdinaryMissingName(t *testing.T) {
	t.Parallel()

	browser, _, id := newBrowser(t, folderbrowse.Options{})

	if _, err := browser.Check(t.Context(), id, "%2e%2e"); !isErr(err, domain.ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

func TestUnknownRootIsInvalid(t *testing.T) {
	t.Parallel()

	browser, _, _ := newBrowser(t, folderbrowse.Options{})

	if _, err := browser.List(t.Context(), "nope", ""); !isErr(err, domain.ErrInvalid) {
		t.Errorf("List err = %v", err)
	}

	if _, err := browser.Check(t.Context(), "nope", "a"); !isErr(err, domain.ErrInvalid) {
		t.Errorf("Check err = %v", err)
	}
}

func TestReservedDirectoriesAreNotListed(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	makeDirs(t, root, "data/cache", "media/keep", "other")
	browser := folderbrowse.New([]string{root}, folderbrowse.Options{Reserved: []string{filepath.Join(root, "data")}})
	id := folderbrowse.RootID(root)

	listing, err := browser.List(t.Context(), id, "")
	if err != nil || strings.Join(names(listing), ",") != "media,other" {
		t.Errorf("listing = %v, %v", names(listing), err)
	}

	for _, path := range []string{"data", "data/cache"} {
		if _, err := browser.List(t.Context(), id, path); !isErr(err, domain.ErrInvalid) {
			t.Errorf("List(%s) err = %v", path, err)
		}

		if _, err := browser.Check(t.Context(), id, path); !isErr(err, domain.ErrInvalid) {
			t.Errorf("Check(%s) err = %v", path, err)
		}
	}
}

func TestReservedInsideAnotherRootEntryHidesOnlyThatEntry(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	makeDirs(t, root, "a/b", "c")
	browser := folderbrowse.New([]string{root}, folderbrowse.Options{Reserved: []string{filepath.Join(root, "a", "b")}})

	listing, err := browser.List(t.Context(), folderbrowse.RootID(root), "a")
	if err != nil || len(listing.Folders) != 0 {
		t.Errorf("listing = %+v, %v", listing, err)
	}
}

func TestListCapTruncates(t *testing.T) {
	t.Parallel()

	browser, root, id := newBrowser(t, folderbrowse.Options{ListingCap: 3})
	makeDirs(t, root, "d1", "d2", "d3", "d4", "d5", "d6", "d7")

	listing, err := browser.List(t.Context(), id, "")
	if err != nil {
		t.Fatal(err)
	}

	if len(listing.Folders) != 3 || !listing.Truncated {
		t.Errorf("listing = %+v", listing)
	}

	exact, _ := folderbrowse.New([]string{root}, folderbrowse.Options{ListingCap: 7}).List(t.Context(), id, "")
	if len(exact.Folders) != 7 || exact.Truncated {
		t.Errorf("exact cap: %d truncated=%v", len(exact.Folders), exact.Truncated)
	}
}

func TestListLargeDirectory(t *testing.T) {
	t.Parallel()

	const count = 1500

	browser, root, id := newBrowser(t, folderbrowse.Options{})

	for index := range count {
		makeDirs(t, root, fmt.Sprintf("dir%04d", index))
	}

	listing, err := browser.List(t.Context(), id, "")
	if err != nil || len(listing.Folders) != folderbrowse.DefaultListingCap || !listing.Truncated {
		t.Errorf("got %d folders truncated=%v err=%v", len(listing.Folders), listing.Truncated, err)
	}

	if listing.Folders[0].Name != "dir0000" {
		t.Errorf("first = %s", listing.Folders[0].Name)
	}
}

func TestMissingRootIsUnavailable(t *testing.T) {
	t.Parallel()

	missing := filepath.Join(t.TempDir(), "gone")
	browser := folderbrowse.New([]string{missing}, folderbrowse.Options{})
	id := folderbrowse.RootID(missing)

	if _, err := browser.List(t.Context(), id, ""); !isErr(err, domain.ErrUnavailable) {
		t.Errorf("List err = %v", err)
	}

	if _, err := browser.Check(t.Context(), id, "a"); !isErr(err, domain.ErrUnavailable) {
		t.Errorf("Check err = %v", err)
	}

	roots := browser.Roots(t.Context())
	if len(roots) != 1 || roots[0].Available || roots[0].ID != id || roots[0].Path != missing {
		t.Errorf("roots = %+v", roots)
	}
}

func TestRootsReportsAvailableRoot(t *testing.T) {
	t.Parallel()

	browser, root, id := newBrowser(t, folderbrowse.Options{})

	roots := browser.Roots(t.Context())
	if len(roots) != 1 || !roots[0].Available || roots[0].ID != id || roots[0].Path != root {
		t.Errorf("roots = %+v", roots)
	}

	if len(id) != 16 {
		t.Errorf("id %q is not 16 hex digits", id)
	}

	if id != folderbrowse.RootID(root+string(filepath.Separator)) {
		t.Error("id is not stable for the cleaned path")
	}
}

func TestCancelledContext(t *testing.T) {
	t.Parallel()

	browser, _, id := newBrowser(t, folderbrowse.Options{})
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	if _, err := browser.List(ctx, id, ""); !isErr(err, context.Canceled) {
		t.Errorf("List err = %v", err)
	}

	if _, err := browser.Check(ctx, id, ""); !isErr(err, context.Canceled) {
		t.Errorf("Check err = %v", err)
	}

	if browser.Roots(ctx)[0].Available {
		t.Error("cancelled Roots must report unavailable")
	}
}

func TestCheckReturnsCleanedAbsolutePath(t *testing.T) {
	t.Parallel()

	browser, root, id := newBrowser(t, folderbrowse.Options{})
	makeDirs(t, root, "movies/2024")

	got, err := browser.Check(t.Context(), id, "movies/2024")
	if err != nil || got != filepath.Join(root, "movies", "2024") {
		t.Errorf("Check = %q, %v", got, err)
	}

	whole, err := browser.Check(t.Context(), id, "")
	if err != nil || whole != root {
		t.Errorf("Check root = %q, %v", whole, err)
	}

	if _, err := browser.Check(t.Context(), id, "missing"); !isErr(err, domain.ErrNotFound) {
		t.Errorf("missing err = %v", err)
	}

	if err := os.WriteFile(filepath.Join(root, "f"), nil, 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := browser.Check(t.Context(), id, "f"); !isErr(err, domain.ErrInvalid) {
		t.Errorf("file err = %v", err)
	}
}

func TestServiceUserNeverFails(t *testing.T) {
	t.Parallel()

	browser, _, _ := newBrowser(t, folderbrowse.Options{})
	if browser.ServiceUser() == "" {
		t.Log("service user unknown in this environment")
	}
}
