package paths_test

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/magpeek/mivo/internal/platform/brand"
	"github.com/magpeek/mivo/internal/platform/paths"
)

const (
	windowsLocalAppData = `C:\Users\alice\AppData\Local`
	windowsHome         = `C:\Users\alice`
	linuxHome           = "/home/alice"
)

// envOf returns a LookupEnv function backed by a map.
func envOf(vars map[string]string) func(string) (string, bool) {
	return func(key string) (string, bool) {
		value, ok := vars[key]

		return value, ok
	}
}

func windowsEnv(vars map[string]string) paths.Environment {
	return paths.Environment{GOOS: "windows", LookupEnv: envOf(vars)}
}

func linuxEnv(vars map[string]string) paths.Environment {
	return paths.Environment{GOOS: "linux", LookupEnv: envOf(vars)}
}

func at(path string) paths.Default {
	return paths.Default{Path: path}
}

func windowsDirs(base string) paths.DefaultDirectories {
	root := filepath.Join(base, brand.DisplayName)

	return paths.DefaultDirectories{
		Config:    at(root),
		Data:      at(filepath.Join(root, "data")),
		Cache:     at(filepath.Join(root, "cache")),
		Transcode: at(filepath.Join(root, "transcode")),
	}
}

func xdgDirs(configHome, dataHome, cacheHome string) paths.DefaultDirectories {
	cache := filepath.Join(cacheHome, brand.ID)

	return paths.DefaultDirectories{
		Config:    at(filepath.Join(configHome, brand.ID)),
		Data:      at(filepath.Join(dataHome, brand.ID)),
		Cache:     at(cache),
		Transcode: at(filepath.Join(cache, "transcode")),
	}
}

func linuxHomeDirs() paths.DefaultDirectories {
	return xdgDirs(
		filepath.Join(linuxHome, ".config"),
		filepath.Join(linuxHome, ".local", "share"),
		filepath.Join(linuxHome, ".cache"),
	)
}

func TestDefaults(t *testing.T) {
	t.Parallel()

	windowsFallback := windowsDirs(filepath.Join(windowsHome, "AppData", "Local"))

	tests := []struct {
		name string
		env  paths.Environment
		want paths.DefaultDirectories
	}{
		{
			name: "windows uses LOCALAPPDATA and the display name",
			env:  windowsEnv(map[string]string{"LOCALAPPDATA": windowsLocalAppData, "USERPROFILE": windowsHome}),
			want: windowsDirs(windowsLocalAppData),
		},
		{
			name: "windows needs no home directory when LOCALAPPDATA is set",
			env:  windowsEnv(map[string]string{"LOCALAPPDATA": windowsLocalAppData}),
			want: windowsDirs(windowsLocalAppData),
		},
		{
			name: "windows accepts a UNC LOCALAPPDATA",
			env:  windowsEnv(map[string]string{"LOCALAPPDATA": `\\server\share\appdata`}),
			want: windowsDirs(`\\server\share\appdata`),
		},
		{
			name: "windows falls back to USERPROFILE without LOCALAPPDATA",
			env:  windowsEnv(map[string]string{"USERPROFILE": windowsHome}),
			want: windowsFallback,
		},
		{
			name: "windows treats an empty LOCALAPPDATA as unset",
			env:  windowsEnv(map[string]string{"LOCALAPPDATA": "", "USERPROFILE": windowsHome}),
			want: windowsFallback,
		},
		{
			name: "windows ignores a relative LOCALAPPDATA",
			env:  windowsEnv(map[string]string{"LOCALAPPDATA": `AppData\Local`, "USERPROFILE": "C:/Users/alice"}),
			want: windowsDirs(filepath.Join("C:/Users/alice", "AppData", "Local")),
		},
		{
			name: "windows ignores a LOCALAPPDATA without drive",
			env:  windowsEnv(map[string]string{"LOCALAPPDATA": `\AppData`, "USERPROFILE": windowsHome}),
			want: windowsFallback,
		},
		{
			name: "linux uses home fallbacks without XDG variables",
			env:  linuxEnv(map[string]string{"HOME": linuxHome}),
			want: linuxHomeDirs(),
		},
		{
			name: "linux honours absolute XDG variables",
			env: linuxEnv(map[string]string{
				"XDG_CONFIG_HOME": "/xdg/config",
				"XDG_DATA_HOME":   "/xdg/data",
				"XDG_CACHE_HOME":  "/xdg/cache",
				"HOME":            linuxHome,
			}),
			want: xdgDirs("/xdg/config", "/xdg/data", "/xdg/cache"),
		},
		{
			name: "linux ignores relative and empty XDG variables",
			env: linuxEnv(map[string]string{
				"XDG_CONFIG_HOME": "relative/config",
				"XDG_DATA_HOME":   "",
				"HOME":            linuxHome,
			}),
			want: linuxHomeDirs(),
		},
		{
			name: "linux needs no home directory when all XDG variables are set",
			env:  linuxEnv(map[string]string{"XDG_CONFIG_HOME": "/c", "XDG_DATA_HOME": "/d", "XDG_CACHE_HOME": "/k"}),
			want: xdgDirs("/c", "/d", "/k"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := paths.Defaults(tt.env); got != tt.want {
				t.Errorf("Defaults = %+v, want %+v", got, tt.want)
			}
		})
	}
}

// directoriesByName lets the tests address a DefaultDirectories field by name.
func directoriesByName(dirs paths.DefaultDirectories) map[string]paths.Default {
	return map[string]paths.Default{
		"config": dirs.Config, "data": dirs.Data, "cache": dirs.Cache, "transcode": dirs.Transcode,
	}
}

func TestDefaultsWithoutUsableHome(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		env  paths.Environment
		// failing maps every directory without default to the variables its error names;
		// the other directories must have a default.
		failing map[string][]string
	}{
		{
			name: "windows without LOCALAPPDATA and USERPROFILE",
			env:  windowsEnv(nil),
			failing: map[string][]string{
				"config": {"LOCALAPPDATA", "USERPROFILE"}, "data": {"LOCALAPPDATA", "USERPROFILE"},
				"cache": {"LOCALAPPDATA", "USERPROFILE"}, "transcode": {"LOCALAPPDATA", "USERPROFILE"},
			},
		},
		{
			name: "windows with a relative USERPROFILE",
			env:  windowsEnv(map[string]string{"USERPROFILE": `Users\alice`}),
			failing: map[string][]string{
				"config": {"USERPROFILE"}, "data": {"USERPROFILE"}, "cache": {"USERPROFILE"}, "transcode": {"USERPROFILE"},
			},
		},
		{
			name:    "linux without XDG_CONFIG_HOME only loses the config directory",
			env:     linuxEnv(map[string]string{"XDG_DATA_HOME": "/d", "XDG_CACHE_HOME": "/k"}),
			failing: map[string][]string{"config": {"XDG_CONFIG_HOME", "HOME"}},
		},
		{
			name: "linux with a relative home directory",
			env:  linuxEnv(map[string]string{"XDG_CONFIG_HOME": "/c", "HOME": "home/alice"}),
			failing: map[string][]string{
				"data": {"XDG_DATA_HOME", "HOME"}, "cache": {"XDG_CACHE_HOME", "HOME"}, "transcode": {"XDG_CACHE_HOME"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			for name, dir := range directoriesByName(paths.Defaults(tt.env)) {
				assertDefault(t, name, dir, tt.failing[name])
			}
		})
	}
}

func assertDefault(t *testing.T, name string, dir paths.Default, wantVariables []string) {
	t.Helper()

	if wantVariables == nil {
		if dir.Err != nil || dir.Path == "" {
			t.Errorf("%s = %+v, want a default", name, dir)
		}

		return
	}

	if !errors.Is(dir.Err, paths.ErrNoHomeDir) || dir.Path != "" {
		t.Fatalf("%s = %+v, want ErrNoHomeDir and no path", name, dir)
	}

	for _, variable := range wantVariables {
		if !strings.Contains(dir.Err.Error(), variable) {
			t.Errorf("%s error = %q, want it to name %s", name, dir.Err, variable)
		}
	}
}

func TestConfigDirEnvUsesBrandPrefix(t *testing.T) {
	t.Parallel()

	if want := brand.EnvPrefix + "_CONFIG_DIR"; paths.ConfigDirEnv() != want {
		t.Errorf("ConfigDirEnv = %q, want %q", paths.ConfigDirEnv(), want)
	}
}

func TestConfigDir(t *testing.T) {
	t.Parallel()

	flagDir := filepath.Join("flag", "dir")
	envDir := filepath.Join("env", "dir")
	linuxDefault := filepath.Join(linuxHome, ".config", brand.ID)

	tests := []struct {
		name      string
		flagValue string
		vars      map[string]string
		want      string
	}{
		{
			name:      "flag wins over env and default",
			flagValue: flagDir,
			vars:      map[string]string{paths.ConfigDirEnv(): envDir},
			want:      flagDir,
		},
		{
			name: "env wins over default",
			vars: map[string]string{paths.ConfigDirEnv(): envDir},
			want: envDir,
		},
		{
			name: "OS default without flag and env",
			want: linuxDefault,
		},
		{
			name:      "flag value is cleaned",
			flagValue: flagDir + string(filepath.Separator) + "." + string(filepath.Separator),
			want:      flagDir,
		},
		{
			name: "env value is cleaned",
			vars: map[string]string{paths.ConfigDirEnv(): envDir + string(filepath.Separator)},
			want: envDir,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			vars := map[string]string{"HOME": linuxHome}
			for key, value := range tt.vars {
				vars[key] = value
			}

			got, err := paths.ConfigDir(tt.flagValue, linuxEnv(vars))
			if err != nil {
				t.Fatalf("ConfigDir: %v", err)
			}

			if got != tt.want {
				t.Errorf("ConfigDir = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestConfigDirErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		flagValue string
		env       paths.Environment
		wantErr   error
		wantMsg   string
	}{
		{
			name:    "empty env variable",
			env:     linuxEnv(map[string]string{paths.ConfigDirEnv(): "", "HOME": linuxHome}),
			wantErr: paths.ErrEmptyPath,
			wantMsg: paths.ConfigDirEnv(),
		},
		{
			name:    "env variable with home shorthand",
			env:     linuxEnv(map[string]string{paths.ConfigDirEnv(): "~/mivo", "HOME": linuxHome}),
			wantErr: paths.ErrHomeShorthand,
			wantMsg: paths.ConfigDirEnv(),
		},
		{
			name:      "flag with home shorthand",
			flagValue: "~",
			env:       linuxEnv(map[string]string{"HOME": linuxHome}),
			wantErr:   paths.ErrHomeShorthand,
			wantMsg:   "flag value",
		},
		{
			name:    "no default without home directory",
			env:     linuxEnv(nil),
			wantErr: paths.ErrNoHomeDir,
			wantMsg: "default config directory",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := paths.ConfigDir(tt.flagValue, tt.env)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("ConfigDir error = %v, want %v", err, tt.wantErr)
			}

			if !strings.Contains(err.Error(), tt.wantMsg) {
				t.Errorf("ConfigDir error = %q, want it to contain %q", err, tt.wantMsg)
			}
		})
	}
}

func TestHostEnvironmentUsesTheGivenLookup(t *testing.T) {
	t.Parallel()

	env := paths.HostEnvironment(envOf(map[string]string{"ONLY_HERE": "injected"}))
	if env.GOOS != runtime.GOOS {
		t.Errorf("GOOS = %q, want %q", env.GOOS, runtime.GOOS)
	}

	if value, ok := env.Lookup("ONLY_HERE"); !ok || value != "injected" {
		t.Errorf("Lookup(ONLY_HERE) = (%q, %v), want the injected value", value, ok)
	}

	// The home directory comes from the injected lookup too, never from the host.
	for _, dir := range directoriesByName(paths.Defaults(env)) {
		if !errors.Is(dir.Err, paths.ErrNoHomeDir) {
			t.Errorf("default = %+v, want ErrNoHomeDir without injected home variables", dir)
		}
	}
}

func TestEnvironmentLookupWithoutFunction(t *testing.T) {
	t.Parallel()

	if value, ok := (paths.Environment{}).Lookup("ANY"); ok || value != "" {
		t.Errorf("Lookup = (%q, %v), want (\"\", false)", value, ok)
	}
}

type resolveCase struct {
	name    string
	value   string
	want    string
	wantErr error
}

func resolveCases(baseDir string) []resolveCase {
	absolute := filepath.Join(baseDir, "elsewhere")
	cases := []resolveCase{
		{name: "relative joins the base", value: "data", want: filepath.Join(baseDir, "data")},
		{name: "relative is cleaned", value: filepath.Join("a", "..", "b"), want: filepath.Join(baseDir, "b")},
		{name: "absolute is kept", value: absolute, want: absolute},
		{name: "absolute is cleaned", value: absolute + string(filepath.Separator) + ".", want: absolute},
		{name: "empty", value: "", wantErr: paths.ErrEmptyPath},
		{name: "home shorthand alone", value: "~", wantErr: paths.ErrHomeShorthand},
		{name: "home shorthand with path", value: "~/media", wantErr: paths.ErrHomeShorthand},
		{name: "home shorthand of a user", value: "~alice/media", wantErr: paths.ErrHomeShorthand},
	}

	if runtime.GOOS == "windows" {
		volume := filepath.VolumeName(baseDir)
		cases = append(cases,
			resolveCase{name: "rooted uses the base volume", value: `\data`, want: volume + `\data`},
			resolveCase{name: "rooted with slash", value: "/data/x", want: volume + `\data\x`},
			resolveCase{name: "drive-relative", value: "C:data", wantErr: paths.ErrDriveRelative},
		)
	} else {
		cases = append(cases,
			resolveCase{name: "backslash is a file name character", value: `\data`, want: filepath.Join(baseDir, `\data`)},
		)
	}

	return cases
}

func TestResolve(t *testing.T) {
	t.Parallel()

	baseDir := t.TempDir()

	for _, tt := range resolveCases(baseDir) {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := paths.Resolve(tt.value, baseDir)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("Resolve(%q) error = %v, want %v", tt.value, err, tt.wantErr)
			}

			if got != tt.want {
				t.Errorf("Resolve(%q) = %q, want %q", tt.value, got, tt.want)
			}

			if tt.wantErr == nil && !filepath.IsAbs(got) {
				t.Errorf("Resolve(%q) = %q, want an absolute path", tt.value, got)
			}
		})
	}
}

func TestEnsureCreatesDirectories(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	dirs := []string{
		filepath.Join(root, "data"),
		filepath.Join(root, "cache", "nested"),
		filepath.Join(root, "cache", "nested"),
	}

	if err := paths.Ensure(dirs...); err != nil {
		t.Fatalf("Ensure: %v", err)
	}

	for _, dir := range dirs {
		info, err := os.Stat(dir)
		if err != nil {
			t.Fatalf("stat %s: %v", dir, err)
		}

		if !info.IsDir() {
			t.Errorf("%s is not a directory", dir)
		}

		// Windows has no Unix permission bits; umask may only remove bits on Unix.
		if runtime.GOOS != "windows" && info.Mode().Perm()&^0o750 != 0 {
			t.Errorf("%s mode = %v, want at most 0750", dir, info.Mode().Perm())
		}
	}
}

func TestEnsureErrors(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	blocker := filepath.Join(root, "file")

	if err := os.WriteFile(blocker, []byte("x"), 0o600); err != nil {
		t.Fatalf("write blocker: %v", err)
	}

	blocked := filepath.Join(blocker, "sub")

	err := paths.Ensure(filepath.Join(root, "ok"), blocked)
	if err == nil || !strings.Contains(err.Error(), blocked) {
		t.Errorf("Ensure error = %v, want it to name %s", err, blocked)
	}

	if err := paths.Ensure(""); !errors.Is(err, paths.ErrEmptyPath) {
		t.Errorf("Ensure(\"\") error = %v, want ErrEmptyPath", err)
	}
}
