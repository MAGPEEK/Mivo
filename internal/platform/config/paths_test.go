package config_test

import (
	"errors"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/magpeek/mivo/internal/platform/config"
	"github.com/magpeek/mivo/internal/platform/paths"
)

func TestLoadPathsAreAlwaysAbsolute(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		yaml string
		vars map[string]string
	}{
		{name: "OS defaults"},
		{name: "relative file values", yaml: "paths:\n  data: d\n  cache: ./c\n  transcode: ../t\n"},
		{name: "relative env values", vars: map[string]string{
			env("PATHS_DATA"): "d", env("PATHS_CACHE"): filepath.Join("x", "..", "c"), env("PATHS_TRANSCODE"): "..",
		}},
		{name: "relative LOCALAPPDATA and XDG values are ignored", vars: map[string]string{
			"LOCALAPPDATA": "relative", "XDG_DATA_HOME": "relative", "XDG_CACHE_HOME": "relative",
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newFixture(t, tt.yaml, tt.vars)

			got, err := f.load()
			if err != nil {
				t.Fatalf("Load: %v", err)
			}

			for _, dir := range []string{got.ConfigDir, got.Paths.Data, got.Paths.Cache, got.Paths.Transcode} {
				if !filepath.IsAbs(dir) {
					t.Errorf("Load = %+v: %q is not absolute", got, dir)
				}
			}
		})
	}
}

func TestLoadKeepsAbsolutePaths(t *testing.T) {
	t.Parallel()

	absolute := filepath.Join(t.TempDir(), "media", "..", "data")
	f := newFixture(t, "", map[string]string{env("PATHS_DATA"): absolute})

	got, err := f.load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if want := filepath.Clean(absolute); got.Paths.Data != want {
		t.Errorf("Paths.Data = %q, want %q", got.Paths.Data, want)
	}
}

func TestLoadRejectsHomeShorthand(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		yaml     string
		vars     map[string]string
		wantPart string
	}{
		{name: "file", yaml: "paths:\n  data: ~/mivo\n", wantPart: "line 2: key paths.data"},
		{name: "environment", vars: map[string]string{env("PATHS_CACHE"): "~/cache"}, wantPart: env("PATHS_CACHE")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newFixture(t, tt.yaml, tt.vars)

			_, err := f.load()
			if !errors.Is(err, config.ErrInvalid) || !errors.Is(err, paths.ErrHomeShorthand) {
				t.Fatalf("Load error = %v, want ErrInvalid and ErrHomeShorthand", err)
			}

			if !strings.Contains(err.Error(), tt.wantPart) {
				t.Errorf("Load error = %q, want it to contain %q", err, tt.wantPart)
			}
		})
	}
}

// A path without drive such as "\data" is rooted on the volume of the config directory
// and a drive-relative path is rejected (ADR 0018 notes). Elsewhere both are plain names.
func TestLoadWindowsPathsWithoutDrive(t *testing.T) {
	t.Parallel()

	if runtime.GOOS != "windows" {
		t.Skip("drive and volume semantics exist only on Windows")
	}

	f := newFixture(t, "paths:\n  data: \\srv\\data\n", nil)

	got, err := f.load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if want := filepath.VolumeName(f.configDir) + `\srv\data`; got.Paths.Data != want {
		t.Errorf("Paths.Data = %q, want %q", got.Paths.Data, want)
	}

	f.vars[env("PATHS_CACHE")] = "D:cache"

	if _, err := f.load(); !errors.Is(err, paths.ErrDriveRelative) {
		t.Errorf("Load error = %v, want ErrDriveRelative", err)
	}
}

// otherOS returns an environment for the OS the tests do not run on, whose defaults are
// therefore not absolute on this host.
func otherOS(f fixture) paths.Environment {
	other := f.env
	if runtime.GOOS == "windows" {
		other.GOOS = "linux"
		f.vars[envHome] = "/home/alice"
	} else {
		other.GOOS = "windows"
		f.vars[envUserProfile] = `C:\Users\alice`
	}

	return other
}

func TestLoadRejectsDefaultsThatAreNotAbsoluteOnThisHost(t *testing.T) {
	t.Parallel()

	f := newFixture(t, "", nil)
	f.env = otherOS(f)

	_, err := f.load()
	if !errors.Is(err, config.ErrInvalid) || !strings.Contains(err.Error(), "paths.data") {
		t.Errorf("Load error = %v, want ErrInvalid naming paths.data", err)
	}
}

// baseVariables are the variables a missing default of the host OS names besides home.
func baseVariables() (data, cache string) {
	if runtime.GOOS == "windows" {
		return "LOCALAPPDATA", "LOCALAPPDATA"
	}

	return "XDG_DATA_HOME", "XDG_CACHE_HOME"
}

func TestLoadWithoutHomeDirectory(t *testing.T) {
	t.Parallel()

	dataVariable, cacheVariable := baseVariables()
	tests := []struct {
		name     string
		vars     map[string]string
		wantPart []string
	}{
		{name: "paths unset", vars: map[string]string{}, wantPart: []string{"paths.data", env("PATHS_DATA"), dataVariable}},
		{
			name:     "only data set",
			vars:     map[string]string{env("PATHS_DATA"): "data"},
			wantPart: []string{"paths.cache", env("PATHS_CACHE"), cacheVariable},
		},
		{
			name:     "transcode unset",
			vars:     map[string]string{env("PATHS_DATA"): "data", env("PATHS_CACHE"): "cache"},
			wantPart: []string{"paths.transcode", env("PATHS_TRANSCODE"), cacheVariable},
		},
		{name: "all paths set", vars: map[string]string{
			env("PATHS_DATA"): "data", env("PATHS_CACHE"): "cache", env("PATHS_TRANSCODE"): "transcode",
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newFixture(t, "", tt.vars)
			delete(f.vars, envHome)
			delete(f.vars, envUserProfile)

			_, err := f.load()
			assertNoHomeDirError(t, err, tt.wantPart)
		})
	}
}

// assertNoHomeDirError expects success for no wantParts, else ErrNoHomeDir naming each part.
func assertNoHomeDirError(t *testing.T, err error, wantParts []string) {
	t.Helper()

	if wantParts == nil {
		if err != nil {
			t.Fatalf("Load: %v", err)
		}

		return
	}

	if !errors.Is(err, paths.ErrNoHomeDir) {
		t.Fatalf("Load error = %v, want ErrNoHomeDir", err)
	}

	for _, part := range wantParts {
		if !strings.Contains(err.Error(), part) {
			t.Errorf("Load error = %q, want it to name %s", err, part)
		}
	}
}
