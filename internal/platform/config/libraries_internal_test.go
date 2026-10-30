package config

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/magpeek/mivo/internal/platform/paths"
)

func TestDefaultLibraryName(t *testing.T) {
	t.Parallel()

	volumeRoot := filepath.VolumeName(t.TempDir()) + string(filepath.Separator)

	tests := []struct {
		name string
		root string
		want string
	}{
		{name: "last path element", root: filepath.Join(t.TempDir(), "TV Shows"), want: "TV Shows"},
		{name: "volume root keeps the whole path", root: volumeRoot, want: volumeRoot},
	}

	for _, tt := range tests {
		if got := defaultLibraryName(tt.root); got != tt.want {
			t.Errorf("%s: defaultLibraryName(%q) = %q, want %q", tt.name, tt.root, got, tt.want)
		}
	}
}

// A path setting can only be empty when its OS default is missing; should a default ever
// come without a reason, the error must still read well and wrap nothing nil.
func TestRequirePathsWithoutDefaultReason(t *testing.T) {
	t.Parallel()

	err := requirePaths(Config{}, paths.DefaultDirectories{})
	if !errors.Is(err, errNoDefault) || !strings.Contains(err.Error(), keyPathsData) {
		t.Errorf("requirePaths error = %v, want errNoDefault naming %s", err, keyPathsData)
	}
}

func TestEveryLibraryFieldIsAcceptedInTheFile(t *testing.T) {
	t.Parallel()

	for _, field := range libraryFields() {
		content, err := parseFile([]byte("libraries:\n  - " + field.name + ": x\n"))
		if err != nil {
			t.Fatalf("parseFile(%s): %v", field.name, err)
		}

		items := content.lists[keyLibraries]
		if len(items) != 1 || items[0].fields[field.name].raw != "x" || items[0].fields[field.name].line != 2 {
			t.Errorf("parseFile(%s) = %+v, want one entry with raw x on line 2", field.name, items)
		}
	}
}
