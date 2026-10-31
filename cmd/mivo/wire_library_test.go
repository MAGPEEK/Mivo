package main

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/library"
	"github.com/magpeek/mivo/internal/platform/config"
)

// TestLibraryDefinitionsParseEveryConfigKind keeps the config kinds and the domain kinds in
// step: platform must not import domain, so cmd is the only place that maps them.
func TestLibraryDefinitionsParseEveryConfigKind(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	configured := []config.LibraryConfig{
		{Name: "Movies", Path: filepath.Join(root, "movies"), Kind: config.LibraryKindMovies},
		{Name: "Shows", Path: filepath.Join(root, "shows"), Kind: config.LibraryKindShows},
	}

	definitions, err := libraryDefinitions(configured)
	if err != nil {
		t.Fatalf("libraryDefinitions error = %v", err)
	}

	want := []library.Definition{
		{Name: "Movies", Root: configured[0].Path, Kind: library.KindMovies},
		{Name: "Shows", Root: configured[1].Path, Kind: library.KindShows},
	}

	if len(definitions) != len(want) {
		t.Fatalf("definitions = %+v, want %+v", definitions, want)
	}

	for index := range want {
		if definitions[index] != want[index] {
			t.Errorf("definition %d = %+v, want %+v", index, definitions[index], want[index])
		}
	}
}

func TestLibraryDefinitionsNameTheBrokenEntry(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	valid := config.LibraryConfig{Name: "Movies", Path: filepath.Join(root, "movies"), Kind: config.LibraryKindMovies}
	brokenPath := filepath.Join(root, "broken")

	tests := []struct {
		name   string
		broken config.LibraryConfig
	}{
		{name: "name above the domain limit", broken: config.LibraryConfig{
			Name: strings.Repeat("n", library.MaxNameLength+1), Path: brokenPath, Kind: config.LibraryKindMovies,
		}},
		{name: "name that is no UTF-8", broken: config.LibraryConfig{Name: "a\xffb", Path: brokenPath, Kind: config.LibraryKindMovies}},
		{name: "root with a NUL byte", broken: config.LibraryConfig{Name: "Movies 2", Path: brokenPath + "\x00", Kind: config.LibraryKindMovies}},
		{name: "unknown kind", broken: config.LibraryConfig{Name: "Music", Path: brokenPath, Kind: "music"}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			_, err := libraryDefinitions([]config.LibraryConfig{valid, test.broken})
			if !errors.Is(err, domain.ErrInvalid) {
				t.Fatalf("libraryDefinitions error = %v, want ErrInvalid", err)
			}

			if want := "config: libraries[1] (" + test.broken.Path + "): "; !strings.HasPrefix(err.Error(), want) {
				t.Errorf("error = %q, want prefix %q", err, want)
			}
		})
	}
}

func TestLibraryDefinitionsOfAnEmptyConfiguration(t *testing.T) {
	t.Parallel()

	definitions, err := libraryDefinitions([]config.LibraryConfig{})
	if err != nil || definitions == nil || len(definitions) != 0 {
		t.Errorf("libraryDefinitions(empty) = %v, %v; want an empty, non-nil list", definitions, err)
	}
}
