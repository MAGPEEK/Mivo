package libraryfs_test

import (
	"context"
	"path/filepath"
	"testing"
)

func TestRootAvailable(t *testing.T) {
	t.Parallel()

	root, outside := newLibrary(t)
	file := filepath.Join(outside, "secret.mkv")

	tests := []struct {
		name string
		root string
		want bool
	}{
		{name: "existing directory", root: root, want: true},
		{name: "missing directory", root: filepath.Join(root, "missing")},
		{name: "regular file", root: file},
		{name: "relative root", root: "library"},
		{name: "empty root", root: ""},
	}

	fileSystem, _ := newFileSystem(t)

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			if got := fileSystem.RootAvailable(t.Context(), test.root); got != test.want {
				t.Errorf("RootAvailable(%q) = %t, want %t", test.root, got, test.want)
			}
		})
	}
}

func TestRootAvailableAfterCancellation(t *testing.T) {
	t.Parallel()

	root, _ := newLibrary(t)
	fileSystem, _ := newFileSystem(t)

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	if fileSystem.RootAvailable(ctx, root) {
		t.Error("RootAvailable with a cancelled context = true, want false")
	}
}
