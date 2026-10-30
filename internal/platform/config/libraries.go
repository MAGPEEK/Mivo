package config

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/magpeek/mivo/internal/platform/paths"
)

// keyLibraries is the list setting of the media libraries (ADR 0022). In config.yaml it is
// a list of {name, path, kind}; the environment variable <PREFIX>_LIBRARIES holds an OS
// path list and replaces the list of the file.
const keyLibraries = "libraries"

// Fields of a library entry in config.yaml.
const (
	libraryFieldPath = "path"
	libraryFieldName = "name"
	libraryFieldKind = "kind"
)

// Library kinds (ADR 0022). The domain has its own kinds; cmd converts between them, so
// platform never imports domain.
const (
	LibraryKindMovies = "movies"
	LibraryKindShows  = "shows"
)

var (
	errMissingLibraryPath = errors.New(`the entry has no "path"`)
	errEmptyLibraryName   = errors.New("library name must not be empty; remove the key to use the folder name")
)

// LibraryConfig is one configured media library.
type LibraryConfig struct {
	// Name is the display name; it defaults to the last element of Path.
	Name string
	// Path is the absolute root directory. It need not exist yet (ADR 0022).
	Path string
	// Kind is LibraryKindMovies (default) or LibraryKindShows.
	Kind string
}

// configuredLibrary is a library together with where it was configured, so that checks
// across entries can name the entry in their error.
type configuredLibrary struct {
	library LibraryConfig
	// origin prefixes errors about the root, e.g. "<file>: line 4: key libraries[0].path".
	origin string
	// label names the entry in errors about another entry, e.g. "libraries[0]".
	label string
}

// libraryField parses one field of a library entry in config.yaml.
type libraryField struct {
	name  string
	apply func(library *LibraryConfig, raw, baseDir string) error
}

// libraryFields lists the fields a library entry accepts; the file walker derives its
// allowed keys from it.
func libraryFields() []libraryField {
	return []libraryField{
		{name: libraryFieldName, apply: applyLibraryName},
		{name: libraryFieldPath, apply: applyLibraryPath},
		{name: libraryFieldKind, apply: applyLibraryKind},
	}
}

func libraryFieldNames() []string {
	fields := libraryFields()
	names := make([]string, 0, len(fields))

	for _, field := range fields {
		names = append(names, field.name)
	}

	return names
}

func applyLibraryName(library *LibraryConfig, raw, _ string) error {
	name := strings.TrimSpace(raw)
	if name == "" {
		return fmt.Errorf("%w: %w", ErrInvalid, errEmptyLibraryName)
	}

	library.Name = name

	return nil
}

func applyLibraryPath(library *LibraryConfig, raw, baseDir string) error {
	root, err := resolvePath(raw, baseDir)
	if err != nil {
		return err
	}

	library.Path = root

	return nil
}

func applyLibraryKind(library *LibraryConfig, raw, _ string) error {
	kind, err := parseChoice(raw, LibraryKindMovies, LibraryKindShows)
	if err != nil {
		return err
	}

	library.Kind = kind

	return nil
}

// fileLibraries turns the library entries of config.yaml into libraries. items is nil when
// the file has no libraries key.
func fileLibraries(items []fileItem, file, configDir string) ([]configuredLibrary, error) {
	if items == nil {
		return nil, nil
	}

	libraries := make([]configuredLibrary, 0, len(items))

	for index, item := range items {
		library, err := fileLibrary(item, listItemLabel(keyLibraries, index), file, configDir)
		if err != nil {
			return nil, err
		}

		libraries = append(libraries, library)
	}

	return libraries, nil
}

func fileLibrary(item fileItem, label, file, configDir string) (configuredLibrary, error) {
	pathValue, ok := item.fields[libraryFieldPath]
	if !ok {
		return configuredLibrary{}, fmt.Errorf("%s: line %d: key %s: %w: %w",
			file, item.line, label, ErrInvalid, errMissingLibraryPath)
	}

	library := LibraryConfig{Kind: LibraryKindMovies}

	for _, field := range libraryFields() {
		value, ok := item.fields[field.name]
		if !ok {
			continue
		}

		if err := field.apply(&library, value.raw, configDir); err != nil {
			return configuredLibrary{}, fmt.Errorf("%s: line %d: key %s: %w",
				file, value.line, joinKey(label, field.name), err)
		}
	}

	if library.Name == "" {
		library.Name = defaultLibraryName(library.Path)
	}

	origin := fmt.Sprintf("%s: line %d: key %s", file, pathValue.line, joinKey(label, libraryFieldPath))

	return configuredLibrary{library: library, origin: origin, label: label}, nil
}

// environmentLibraries reads <PREFIX>_LIBRARIES. ok is false when the variable is unset;
// an empty value sets an empty list, so it can switch off a list from the file or an image
// preset. Each element is a root of kind movies named after its last path element.
func environmentLibraries(env paths.Environment, configDir string) (libraries []configuredLibrary, ok bool, err error) {
	name := envName(keyLibraries)

	raw, ok := env.Lookup(name)
	if !ok {
		return nil, false, nil
	}

	elements := filepath.SplitList(raw)
	libraries = make([]configuredLibrary, 0, len(elements))

	for index, element := range elements {
		label := listItemLabel(keyLibraries, index)
		origin := fmt.Sprintf("environment variable %s: %s", name, joinKey(label, libraryFieldPath))

		library := LibraryConfig{Kind: LibraryKindMovies}
		if err := applyLibraryPath(&library, element, configDir); err != nil {
			return nil, true, fmt.Errorf("%s: %w", origin, err)
		}

		library.Name = defaultLibraryName(library.Path)
		libraries = append(libraries, configuredLibrary{library: library, origin: origin, label: label})
	}

	return libraries, true, nil
}

// defaultLibraryName is the last element of root, or root itself for a volume root such
// as "/" or "D:\", whose last element would be a bare separator.
func defaultLibraryName(root string) string {
	name := filepath.Base(root)
	if name == string(filepath.Separator) || name == "." {
		return root
	}

	return name
}

func libraryConfigs(libraries []configuredLibrary) []LibraryConfig {
	configs := make([]LibraryConfig, 0, len(libraries))

	for _, entry := range libraries {
		configs = append(configs, entry.library)
	}

	return configs
}
