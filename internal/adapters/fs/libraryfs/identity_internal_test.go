package libraryfs

import (
	"os"
	"path/filepath"
	"testing"
)

// openDirectory opens a directory of the test for identification.
func openDirectory(t *testing.T, path string) *os.File {
	t.Helper()

	directory, err := os.OpenInRoot(filepath.Dir(path), filepath.Base(path))
	if err != nil {
		t.Fatalf("open %s: %v", path, err)
	}

	t.Cleanup(func() { closeFile(directory) })

	return directory
}

func TestIdentifyDirectoryTellsDirectoriesApart(t *testing.T) {
	t.Parallel()

	base := t.TempDir()
	for _, name := range []string{"a", "b"} {
		if err := os.Mkdir(filepath.Join(base, name), internalTestDirMode); err != nil {
			t.Fatalf("create %s: %v", name, err)
		}
	}

	first, err := identifyDirectory(openDirectory(t, filepath.Join(base, "a")))
	if err != nil {
		t.Fatalf("identify a: %v", err)
	}

	again, err := identifyDirectory(openDirectory(t, filepath.Join(base, "a")))
	if err != nil {
		t.Fatalf("identify a again: %v", err)
	}

	other, err := identifyDirectory(openDirectory(t, filepath.Join(base, "b")))
	if err != nil {
		t.Fatalf("identify b: %v", err)
	}

	if !first.known() || first != again || first == other {
		t.Errorf("identities a=%+v, a again=%+v, b=%+v: want known, stable and distinct", first, again, other)
	}
}

func TestIdentifyDirectoryRejectsAFile(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "file.mkv")
	if err := os.WriteFile(path, []byte("video"), internalTestFileMode); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}

	if _, err := identifyDirectory(openDirectory(t, path)); err == nil {
		t.Error("identifyDirectory(file) error = nil, want an error")
	}
}
