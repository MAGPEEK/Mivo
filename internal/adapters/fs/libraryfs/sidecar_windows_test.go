//go:build windows

package libraryfs_test

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/magpeek/mivo/internal/domain"
)

// Symlinks need developer mode on Windows; a junction needs no rights and leaves the root too.
func TestReadSidecarRefusesAJunctionLeavingTheRoot(t *testing.T) {
	t.Parallel()

	fileSystem, _ := newFileSystem(t)
	root, outside := newLibrary(t)

	writeFile(t, filepath.Join(outside, "poster.jpg"), "secret")
	mklinkJunction(t, filepath.Join(root, "Extras"), outside)

	content, err := fileSystem.ReadSidecar(t.Context(), root, "Extras/poster.jpg", sidecarLimit)
	if !errors.Is(err, domain.ErrForbidden) || content != nil {
		t.Errorf("ReadSidecar(through junction) = %q, %v, want ErrForbidden", content, err)
	}

	assertPaths(t, walkPaths(t, fileSystem, root))
}
