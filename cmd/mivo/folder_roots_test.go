package main

import (
	"errors"
	"path/filepath"
	"testing"
	"uuid"

	"github.com/magpeek/mivo/internal/adapters/fs/folderbrowse"
	"github.com/magpeek/mivo/internal/domain"
)

func TestFolderRootIDIsACanonicalStableUUIDv5(t *testing.T) {
	t.Parallel()

	root := filepath.Join(t.TempDir(), "media")

	id := folderRootID(root)

	parsed, err := uuid.Parse(id)
	if err != nil || parsed.String() != id {
		t.Fatalf("folderRootID = %q, want a canonical lower-case UUID (parse error %v)", id, err)
	}

	const versionNibbleIndex, variantIndex = 14, 19

	if id[versionNibbleIndex] != '5' || (id[variantIndex] != '8' && id[variantIndex] != '9' && id[variantIndex] != 'a' && id[variantIndex] != 'b') {
		t.Errorf("folderRootID = %q, want version 5 and the RFC variant", id)
	}

	if again := folderRootID(root + string(filepath.Separator)); again != id {
		t.Errorf("folderRootID of the same folder with a trailing separator = %q, want %q", again, id)
	}

	if other := folderRootID(root + "2"); other == id {
		t.Errorf("folderRootID of another folder = %q, want a different ID", other)
	}
}

func TestFolderBrowserAdapterRefusesAnUnknownRoot(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	adapter := newFolderBrowserAdapter(folderbrowse.New([]string{root}, folderbrowse.Options{}), []string{root})

	if _, err := adapter.Check(t.Context(), folderRootID(root+"-other"), ""); !errors.Is(err, domain.ErrInvalid) {
		t.Errorf("Check(unknown root) error = %v, want ErrInvalid", err)
	}

	if _, err := adapter.List(t.Context(), folderbrowse.RootID(root), ""); !errors.Is(err, domain.ErrInvalid) {
		t.Errorf("List with the browser's own root ID error = %v, want ErrInvalid: only API IDs are accepted", err)
	}

	roots, err := adapter.Roots(t.Context())
	if err != nil || len(roots) != 1 || roots[0].ID != folderRootID(root) || !roots[0].Available {
		t.Errorf("Roots = %+v, %v; want the root under its API ID", roots, err)
	}
}
