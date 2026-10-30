package config_test

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/magpeek/mivo/internal/platform/config"
	"github.com/magpeek/mivo/internal/platform/paths"
)

// yamlQuoted writes a path as a single-quoted YAML scalar, in which a backslash is literal.
func yamlQuoted(path string) string {
	return "'" + strings.ReplaceAll(path, "'", "''") + "'"
}

// pathList joins paths with the OS list separator, like PATH.
func pathList(elements ...string) string {
	return strings.Join(elements, string(os.PathListSeparator))
}

// librariesYAML writes one library entry per path, without name and kind.
func librariesYAML(roots ...string) string {
	var builder strings.Builder

	builder.WriteString("libraries:\n")

	for _, root := range roots {
		builder.WriteString("  - path: " + yamlQuoted(root) + "\n")
	}

	return builder.String()
}

func TestLoadLibrariesFromFile(t *testing.T) {
	t.Parallel()

	shows := filepath.Join(t.TempDir(), "TV Shows")
	// Relative roots resolve against the config directory; a root inside it is rejected, so
	// the relative ones here point next to it.
	yaml := "libraries:\n" +
		"  - name: Filme\n" +
		"    path: ../movies\n" +
		"  - path: " + yamlQuoted(shows) + "\n" +
		"    kind: Shows\n" +
		"  - {name: '  Kids  ', path: ../movies-kids, kind: movies}\n"
	f := newFixture(t, yaml, nil)

	got, err := f.load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	beside := filepath.Dir(f.configDir)
	want := []config.LibraryConfig{
		{Name: "Filme", Path: filepath.Join(beside, "movies"), Kind: config.LibraryKindMovies},
		{Name: "TV Shows", Path: shows, Kind: config.LibraryKindShows},
		{Name: "Kids", Path: filepath.Join(beside, "movies-kids"), Kind: config.LibraryKindMovies},
	}
	if !reflect.DeepEqual(got.Libraries, want) {
		t.Errorf("Libraries = %+v, want %+v", got.Libraries, want)
	}
}

func TestLoadLibrariesPrecedence(t *testing.T) {
	t.Parallel()

	base := t.TempDir()
	fileRoot := filepath.Join(base, "file-movies")
	envRoot := filepath.Join(base, "env-movies")

	tests := []struct {
		name string
		yaml string
		vars map[string]string
		want func(configDir string) []config.LibraryConfig
	}{
		{
			name: "default is no library",
			want: func(string) []config.LibraryConfig { return []config.LibraryConfig{} },
		},
		{
			name: "file over default",
			yaml: librariesYAML(fileRoot),
			want: func(string) []config.LibraryConfig { return []config.LibraryConfig{movies("file-movies", fileRoot)} },
		},
		{
			name: "env over default, relative entries resolve against the config dir",
			vars: map[string]string{env("LIBRARIES"): pathList(envRoot, filepath.Join("..", "rel", "Anime"))},
			want: func(configDir string) []config.LibraryConfig {
				return []config.LibraryConfig{
					movies("env-movies", envRoot),
					movies("Anime", filepath.Join(filepath.Dir(configDir), "rel", "Anime")),
				}
			},
		},
		{
			name: "env replaces the file list",
			yaml: librariesYAML(fileRoot),
			vars: map[string]string{env("LIBRARIES"): envRoot},
			want: func(string) []config.LibraryConfig { return []config.LibraryConfig{movies("env-movies", envRoot)} },
		},
		{
			name: "empty env removes the file list",
			yaml: librariesYAML(fileRoot),
			vars: map[string]string{env("LIBRARIES"): ""},
			want: func(string) []config.LibraryConfig { return []config.LibraryConfig{} },
		},
		{
			name: "empty file list",
			yaml: "libraries: []\n",
			want: func(string) []config.LibraryConfig { return []config.LibraryConfig{} },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newFixture(t, tt.yaml, tt.vars)

			got, err := f.load()
			if err != nil {
				t.Fatalf("Load: %v", err)
			}

			if want := tt.want(f.configDir); !reflect.DeepEqual(got.Libraries, want) {
				t.Errorf("Libraries = %+v, want %+v", got.Libraries, want)
			}
		})
	}
}

func movies(name, root string) config.LibraryConfig {
	return config.LibraryConfig{Name: name, Path: root, Kind: config.LibraryKindMovies}
}

// ADR 0022: a root that does not exist yet (NAS share mounted late) is not a startup error.
func TestLoadAcceptsMissingLibraryRoot(t *testing.T) {
	t.Parallel()

	missing := filepath.Join(t.TempDir(), "not", "mounted")
	f := newFixture(t, librariesYAML(missing), nil)

	got, err := f.load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if len(got.Libraries) != 1 || got.Libraries[0].Path != missing {
		t.Errorf("Libraries = %+v, want the missing root %s", got.Libraries, missing)
	}
}

type libraryErrorCase struct {
	name      string
	yaml      string
	vars      map[string]string
	wantErr   error
	wantParts []string
}

func libraryFileErrorCases() []libraryErrorCase {
	return []libraryErrorCase{
		{name: "missing path", yaml: "libraries:\n  - name: x\n", wantParts: []string{"line 2: key libraries[0]", "path"}},
		{name: "empty entry", yaml: "libraries:\n  - {}\n", wantParts: []string{"line 2: key libraries[0]", "path"}},
		{
			name: "empty path", yaml: "libraries:\n  - path: ''\n",
			wantErr: paths.ErrEmptyPath, wantParts: []string{"line 2: key libraries[0].path"},
		},
		{
			name: "home shorthand", yaml: "libraries:\n  - path: a\n  - path: ~/media\n",
			wantErr: paths.ErrHomeShorthand, wantParts: []string{"line 3: key libraries[1].path"},
		},
		{name: "null path", yaml: "libraries:\n  - path:\n", wantParts: []string{"line 2: key libraries[0].path", "null"}},
		{name: "list as path", yaml: "libraries:\n  - path: [a, b]\n", wantParts: []string{"line 2: key libraries[0].path"}},
		{name: "unknown kind", yaml: "libraries:\n  - path: a\n    kind: music\n", wantParts: []string{"line 3: key libraries[0].kind", "music"}},
		{name: "empty name", yaml: "libraries:\n  - path: a\n    name: ' '\n", wantParts: []string{"line 3: key libraries[0].name"}},
		{name: "null list", yaml: "libraries:\n", wantParts: []string{"line 1: key libraries", "null"}},
		{name: "scalar instead of a list", yaml: "libraries: /media\n", wantParts: []string{"line 1: key libraries", "list"}},
		{name: "mapping instead of a list", yaml: "libraries:\n  path: /media\n", wantParts: []string{"line 2: key libraries", "list"}},
		{name: "entry is a scalar", yaml: "libraries:\n  - /media\n", wantParts: []string{"line 2: key libraries[0]", "mapping"}},
		{
			name: "duplicate field", yaml: "libraries:\n  - path: a\n    path: b\n",
			wantParts: []string{"line 3: key libraries[0].path", "duplicate key"},
		},
		{
			name: "duplicate list", yaml: "libraries: []\nlibraries: []\n",
			wantParts: []string{"line 2: key libraries", "duplicate key"},
		},
		{
			name: "complex key in an entry", yaml: "libraries:\n  - ? [path]\n    : a\n",
			wantErr: config.ErrUnknownKey, wantParts: []string{"line 2: unknown key in libraries[0]"},
		},
	}
}

func libraryEnvErrorCases() []libraryErrorCase {
	name := env("LIBRARIES")

	return []libraryErrorCase{
		{
			name: "env empty entry", vars: map[string]string{name: pathList("a", "")},
			wantErr: paths.ErrEmptyPath, wantParts: []string{"environment variable " + name + ": libraries[1].path"},
		},
		{
			name: "env home shorthand", vars: map[string]string{name: "~/media"},
			wantErr: paths.ErrHomeShorthand, wantParts: []string{"environment variable " + name + ": libraries[0].path"},
		},
	}
}

func TestLoadLibraryErrors(t *testing.T) {
	t.Parallel()

	for _, tt := range append(libraryFileErrorCases(), libraryEnvErrorCases()...) {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newFixture(t, tt.yaml, tt.vars)

			_, err := f.load()
			assertLoadError(t, err, tt)

			if tt.yaml != "" && !strings.HasPrefix(err.Error(), f.configFile()+": ") {
				t.Errorf("Load error = %q, want it to start with %s", err, f.configFile())
			}
		})
	}
}

// assertLoadError expects tt.wantErr (default ErrInvalid) and every part of the message.
func assertLoadError(t *testing.T, err error, tt libraryErrorCase) {
	t.Helper()

	wantErr := tt.wantErr
	if wantErr == nil {
		wantErr = config.ErrInvalid
	}

	if !errors.Is(err, wantErr) {
		t.Fatalf("Load error = %v, want %v", err, wantErr)
	}

	for _, part := range tt.wantParts {
		if !strings.Contains(err.Error(), part) {
			t.Errorf("Load error = %q, want it to contain %q", err, part)
		}
	}
}

// serverDirs points data, cache and transcode below base/server so that library roots can
// be placed next to, inside and around them.
func serverDirs(base string) map[string]string {
	server := filepath.Join(base, "server")

	return map[string]string{
		env("PATHS_DATA"):      filepath.Join(server, "data"),
		env("PATHS_CACHE"):     filepath.Join(server, "cache"),
		env("PATHS_TRANSCODE"): filepath.Join(server, "transcode"),
	}
}

type overlapCase struct {
	name      string
	roots     func(base, configDir string) []string
	wantParts []string
}

func overlapCases() []overlapCase {
	return []overlapCase{
		{
			name: "duplicate root",
			roots: func(base, _ string) []string {
				return []string{filepath.Join(base, "m"), filepath.Join(base, "x", "..", "m")}
			},
			wantParts: []string{"key libraries[1].path", "is the same directory as the root", "of libraries[0]"},
		},
		{
			name: "root inside an earlier root",
			roots: func(base, _ string) []string {
				return []string{filepath.Join(base, "m"), filepath.Join(base, "m", "kids")}
			},
			wantParts: []string{"key libraries[1].path", "lies inside the root", "of libraries[0]"},
		},
		{
			name: "root containing an earlier root",
			roots: func(base, _ string) []string {
				return []string{filepath.Join(base, "m", "kids"), filepath.Join(base, "m")}
			},
			wantParts: []string{"key libraries[1].path", "contains the root", "of libraries[0]"},
		},
		{
			name:      "root is the data directory",
			roots:     func(base, _ string) []string { return []string{filepath.Join(base, "server", "data")} },
			wantParts: []string{"key libraries[0].path", "is the same directory as the data directory (paths.data)"},
		},
		{
			name:      "root contains the server directories",
			roots:     func(base, _ string) []string { return []string{filepath.Join(base, "server")} },
			wantParts: []string{"contains the data directory (paths.data)"},
		},
		{
			name:      "root inside the cache directory",
			roots:     func(base, _ string) []string { return []string{filepath.Join(base, "server", "cache", "m")} },
			wantParts: []string{"lies inside the cache directory (paths.cache)"},
		},
		{
			name:      "root inside the transcode directory",
			roots:     func(base, _ string) []string { return []string{filepath.Join(base, "server", "transcode", "m")} },
			wantParts: []string{"lies inside the transcode directory (paths.transcode)"},
		},
		{
			name:      "root is the config directory",
			roots:     func(_, configDir string) []string { return []string{configDir} },
			wantParts: []string{"is the same directory as the config directory"},
		},
		{
			name:      "root contains the config directory",
			roots:     func(_, configDir string) []string { return []string{filepath.Dir(configDir)} },
			wantParts: []string{"contains the config directory"},
		},
		{
			name:      "root inside the config directory",
			roots:     func(_, configDir string) []string { return []string{filepath.Join(configDir, "media")} },
			wantParts: []string{"lies inside the config directory"},
		},
	}
}

func TestLoadRejectsOverlappingLibraryRoots(t *testing.T) {
	t.Parallel()

	for _, tt := range overlapCases() {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			base := t.TempDir()
			f := newFixture(t, "", serverDirs(base))
			writeConfigFile(t, f.configDir, librariesYAML(tt.roots(base, f.configDir)...))

			_, err := f.load()
			assertLoadError(t, err, libraryErrorCase{wantParts: append([]string{f.configFile()}, tt.wantParts...)})
		})
	}
}

func TestLoadRejectsOverlappingRootsFromTheEnvironment(t *testing.T) {
	t.Parallel()

	base := t.TempDir()
	vars := serverDirs(base)
	vars[env("LIBRARIES")] = pathList(filepath.Join(base, "m"), filepath.Join(base, "m", "kids"))
	f := newFixture(t, "", vars)

	_, err := f.load()
	assertLoadError(t, err, libraryErrorCase{wantParts: []string{
		"environment variable " + env("LIBRARIES") + ": libraries[1].path", "lies inside the root", "of libraries[0]",
	}})
}

func TestLoadAcceptsNeighbouringLibraryRoots(t *testing.T) {
	t.Parallel()

	base := t.TempDir()
	f := newFixture(t, librariesYAML(
		filepath.Join(base, "m"), filepath.Join(base, "m2"), filepath.Join(base, "server-media"),
	), serverDirs(base))

	got, err := f.load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if len(got.Libraries) != 3 {
		t.Errorf("Libraries = %+v, want three", got.Libraries)
	}
}

// Library roots compare case-insensitively on Windows. The injected GOOS decides, so the
// rule is tested on every host; directories are host paths that differ only in case.
func TestLoadComparesLibraryRootsByOS(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		goos     string
		roots    func(base string) []string
		wantPart string
	}{
		{name: "windows duplicate", goos: "windows", roots: func(base string) []string {
			return []string{filepath.Join(base, "Media"), filepath.Join(base, "MEDIA")}
		}, wantPart: "is the same directory as the root"},
		{name: "windows data directory", goos: "windows", roots: func(base string) []string {
			return []string{filepath.Join(base, "SERVER", "Data")}
		}, wantPart: "data directory (paths.data)"},
		{name: "windows nested", goos: "windows", roots: func(base string) []string {
			return []string{filepath.Join(base, "media"), filepath.Join(base, "MEDIA", "kids")}
		}, wantPart: "lies inside the root"},
		{name: "linux duplicate differs in case", goos: "linux", roots: func(base string) []string {
			return []string{filepath.Join(base, "Media"), filepath.Join(base, "MEDIA")}
		}},
		{name: "linux data directory differs in case", goos: "linux", roots: func(base string) []string {
			return []string{filepath.Join(base, "SERVER", "Data")}
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			base := t.TempDir()
			f := newFixture(t, "", serverDirs(base))
			f.env.GOOS = tt.goos
			writeConfigFile(t, f.configDir, librariesYAML(tt.roots(base)...))

			_, err := f.load()
			if tt.wantPart == "" {
				if err != nil {
					t.Fatalf("Load: %v", err)
				}

				return
			}

			assertLoadError(t, err, libraryErrorCase{wantParts: []string{tt.wantPart}})
		})
	}
}
