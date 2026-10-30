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

func windowsDirs(base string) paths.Directories {
	root := filepath.Join(base, brand.DisplayName)

	return paths.Directories{
		Config:    root,
		Data:      filepath.Join(root, "data"),
		Cache:     filepath.Join(root, "cache"),
		Transcode: filepath.Join(root, "transcode"),
	}
}

func xdgDirs(configHome, dataHome, cacheHome string) paths.Directories {
	cache := filepath.Join(cacheHome, brand.ID)

	return paths.Directories{
		Config:    filepath.Join(configHome, brand.ID),
		Data:      filepath.Join(dataHome, brand.ID),
		Cache:     cache,
		Transcode: filepath.Join(cache, "transcode"),
	}
}

func TestDefaults(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		env  paths.Environment
		want paths.Directories
	}{
		{
			name: "windows uses LOCALAPPDATA and the display name",
			env: paths.Environment{
				GOOS:      "windows",
				LookupEnv: envOf(map[string]string{"LOCALAPPDATA": windowsLocalAppData}),
				HomeDir:   windowsHome,
			},
			want: windowsDirs(windowsLocalAppData),
		},
		{
			name: "windows falls back to the home directory without LOCALAPPDATA",
			env:  paths.Environment{GOOS: "windows", HomeDir: windowsHome},
			want: windowsDirs(filepath.Join(windowsHome, "AppData", "Local")),
		},
		{
			name: "windows treats an empty LOCALAPPDATA as unset",
			env: paths.Environment{
				GOOS:      "windows",
				LookupEnv: envOf(map[string]string{"LOCALAPPDATA": ""}),
				HomeDir:   windowsHome,
			},
			want: windowsDirs(filepath.Join(windowsHome, "AppData", "Local")),
		},
		{
			name: "linux uses home fallbacks without XDG variables",
			env:  paths.Environment{GOOS: "linux", LookupEnv: envOf(nil), HomeDir: linuxHome},
			want: xdgDirs(
				filepath.Join(linuxHome, ".config"),
				filepath.Join(linuxHome, ".local", "share"),
				filepath.Join(linuxHome, ".cache"),
			),
		},
		{
			name: "linux honours absolute XDG variables",
			env: paths.Environment{
				GOOS: "linux",
				LookupEnv: envOf(map[string]string{
					"XDG_CONFIG_HOME": "/xdg/config",
					"XDG_DATA_HOME":   "/xdg/data",
					"XDG_CACHE_HOME":  "/xdg/cache",
				}),
				HomeDir: linuxHome,
			},
			want: xdgDirs("/xdg/config", "/xdg/data", "/xdg/cache"),
		},
		{
			name: "linux ignores relative and empty XDG variables",
			env: paths.Environment{
				GOOS: "linux",
				LookupEnv: envOf(map[string]string{
					"XDG_CONFIG_HOME": "relative/config",
					"XDG_DATA_HOME":   "",
				}),
				HomeDir: linuxHome,
			},
			want: xdgDirs(
				filepath.Join(linuxHome, ".config"),
				filepath.Join(linuxHome, ".local", "share"),
				filepath.Join(linuxHome, ".cache"),
			),
		},
		{
			name: "linux needs no home directory when all XDG variables are set",
			env: paths.Environment{
				GOOS: "linux",
				LookupEnv: envOf(map[string]string{
					"XDG_CONFIG_HOME": "/c",
					"XDG_DATA_HOME":   "/d",
					"XDG_CACHE_HOME":  "/k",
				}),
			},
			want: xdgDirs("/c", "/d", "/k"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := paths.Defaults(tt.env)
			if err != nil {
				t.Fatalf("Defaults: %v", err)
			}

			if got != tt.want {
				t.Errorf("Defaults = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestDefaultsWithoutHomeDir(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		env         paths.Environment
		wantVarName string
	}{
		{
			name:        "windows without LOCALAPPDATA",
			env:         paths.Environment{GOOS: "windows"},
			wantVarName: "LOCALAPPDATA",
		},
		{
			name:        "linux without XDG_CONFIG_HOME",
			env:         paths.Environment{GOOS: "linux"},
			wantVarName: "XDG_CONFIG_HOME",
		},
		{
			name: "linux without XDG_DATA_HOME",
			env: paths.Environment{
				GOOS:      "linux",
				LookupEnv: envOf(map[string]string{"XDG_CONFIG_HOME": "/c"}),
			},
			wantVarName: "XDG_DATA_HOME",
		},
		{
			name: "linux without XDG_CACHE_HOME",
			env: paths.Environment{
				GOOS:      "linux",
				LookupEnv: envOf(map[string]string{"XDG_CONFIG_HOME": "/c", "XDG_DATA_HOME": "/d"}),
			},
			wantVarName: "XDG_CACHE_HOME",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := paths.Defaults(tt.env)
			if !errors.Is(err, paths.ErrNoHomeDir) {
				t.Fatalf("Defaults error = %v, want ErrNoHomeDir", err)
			}

			if !strings.Contains(err.Error(), tt.wantVarName) {
				t.Errorf("Defaults error = %q, want it to name %s", err, tt.wantVarName)
			}
		})
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

			env := paths.Environment{GOOS: "linux", LookupEnv: envOf(tt.vars), HomeDir: linuxHome}

			got, err := paths.ConfigDir(tt.flagValue, env)
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
		name    string
		env     paths.Environment
		wantErr error
		wantMsg string
	}{
		{
			name: "empty env variable",
			env: paths.Environment{
				GOOS:      "linux",
				LookupEnv: envOf(map[string]string{paths.ConfigDirEnv(): ""}),
				HomeDir:   linuxHome,
			},
			wantErr: paths.ErrEmptyPath,
			wantMsg: paths.ConfigDirEnv(),
		},
		{
			name:    "no default without home directory",
			env:     paths.Environment{GOOS: "linux"},
			wantErr: paths.ErrNoHomeDir,
			wantMsg: "default config directory",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := paths.ConfigDir("", tt.env)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("ConfigDir error = %v, want %v", err, tt.wantErr)
			}

			if !strings.Contains(err.Error(), tt.wantMsg) {
				t.Errorf("ConfigDir error = %q, want it to contain %q", err, tt.wantMsg)
			}
		})
	}
}

func TestHostEnvironment(t *testing.T) {
	t.Parallel()

	env := paths.HostEnvironment()
	if env.GOOS != runtime.GOOS {
		t.Errorf("GOOS = %q, want %q", env.GOOS, runtime.GOOS)
	}

	if env.LookupEnv == nil {
		t.Error("LookupEnv is nil, want os.LookupEnv")
	}
}

func TestEnvironmentLookupWithoutFunction(t *testing.T) {
	t.Parallel()

	if value, ok := (paths.Environment{}).Lookup("ANY"); ok || value != "" {
		t.Errorf("Lookup = (%q, %v), want (\"\", false)", value, ok)
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
