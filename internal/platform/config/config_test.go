package config_test

import (
	"errors"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/magpeek/mivo/internal/platform/brand"
	"github.com/magpeek/mivo/internal/platform/config"
	"github.com/magpeek/mivo/internal/platform/logging"
	"github.com/magpeek/mivo/internal/platform/paths"
)

// Home directory variables of Windows and the other systems; the fixture sets both.
const (
	envUserProfile = "USERPROFILE"
	envHome        = "HOME"
)

// fixture is a config directory and a fake host environment for one test.
type fixture struct {
	configDir string
	vars      map[string]string
	env       paths.Environment
}

// newFixture creates a temp config directory, optionally with config.yaml, and an
// environment whose home directory is a separate temp directory.
func newFixture(t *testing.T, yamlContent string, vars map[string]string) fixture {
	t.Helper()

	configDir := t.TempDir()
	if yamlContent != "" {
		writeConfigFile(t, configDir, yamlContent)
	}

	// A copy keeps table maps unchanged when parallel subtests add variables.
	home := t.TempDir()
	merged := map[string]string{envUserProfile: home, envHome: home}
	maps.Copy(merged, vars)
	vars = merged

	env := paths.Environment{
		GOOS: runtime.GOOS,
		LookupEnv: func(key string) (string, bool) {
			value, ok := vars[key]

			return value, ok
		},
	}

	return fixture{configDir: configDir, vars: vars, env: env}
}

func writeConfigFile(t *testing.T, dir, content string) {
	t.Helper()

	if err := os.WriteFile(filepath.Join(dir, config.FileName), []byte(content), 0o600); err != nil {
		t.Fatalf("write config file: %v", err)
	}
}

func (f fixture) load() (config.Config, error) {
	return config.Load(config.Options{ConfigDirFlag: f.configDir, Environment: f.env})
}

func (f fixture) configFile() string {
	return filepath.Join(f.configDir, config.FileName)
}

func env(suffix string) string {
	return brand.EnvPrefix + "_" + suffix
}

func defaultDirs(t *testing.T, f fixture) paths.DefaultDirectories {
	t.Helper()

	dirs := paths.Defaults(f.env)
	for _, dir := range []paths.Default{dirs.Data, dirs.Cache, dirs.Transcode} {
		if dir.Err != nil {
			t.Fatalf("paths.Defaults: %v", dir.Err)
		}
	}

	return dirs
}

func TestLoadWithoutFileUsesDefaultsAndCreatesNoFile(t *testing.T) {
	t.Parallel()

	f := newFixture(t, "", nil)
	dirs := defaultDirs(t, f)

	got, err := f.load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	want := config.Config{
		ConfigDir: f.configDir,
		Server:    config.Server{Host: "", Port: 8460, LANOnly: true},
		Log:       config.Log{Level: "info", Format: "text"},
		Paths:     config.Paths{Data: dirs.Data.Path, Cache: dirs.Cache.Path, Transcode: dirs.Transcode.Path},
		Libraries: []config.LibraryConfig{},
		Tools:     config.Tools{FFmpeg: "ffmpeg", FFprobe: "ffprobe"},
		Transcode: config.Transcode{MaxSessions: 2},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Load = %+v, want %+v", got, want)
	}

	if _, err := os.Stat(f.configFile()); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("stat %s: %v, want not exist", f.configFile(), err)
	}
}

func TestLoadEmptyFileUsesDefaults(t *testing.T) {
	t.Parallel()

	for _, content := range []string{"\n", "# only a comment\n", "server:\n"} {
		f := newFixture(t, content, nil)

		got, err := f.load()
		if err != nil {
			t.Fatalf("Load(%q): %v", content, err)
		}

		if got.Server.Port != config.DefaultPort || got.Log.Level != logging.LevelInfo {
			t.Errorf("Load(%q) = %+v, want defaults", content, got)
		}
	}
}

type precedenceCase struct {
	key      string
	yaml     string
	envName  string
	envValue string
	wantFile string
	wantEnv  string
	// relative marks path settings whose wantFile and wantEnv resolve against the config dir.
	relative  bool
	defaultOf func(dirs paths.DefaultDirectories) string
	get       func(cfg config.Config) string
}

func precedenceCases() []precedenceCase {
	return append([]precedenceCase{
		{
			key: "server.host", yaml: "server:\n  host: 127.0.0.1\n",
			envName: env("SERVER_HOST"), envValue: "::1",
			wantFile: "127.0.0.1", wantEnv: "::1",
			defaultOf: func(paths.DefaultDirectories) string { return "" },
			get:       func(cfg config.Config) string { return cfg.Server.Host },
		},
		{
			key: "server.port", yaml: "server:\n  port: 9000\n",
			envName: env("SERVER_PORT"), envValue: "0",
			wantFile: "9000", wantEnv: "0",
			defaultOf: func(paths.DefaultDirectories) string { return "8460" },
			get:       func(cfg config.Config) string { return strconv.Itoa(cfg.Server.Port) },
		},
		{
			key: "server.lan_only", yaml: "server:\n  lan_only: false\n",
			envName: env("SERVER_LAN_ONLY"), envValue: "TRUE",
			wantFile: "false", wantEnv: "true",
			defaultOf: func(paths.DefaultDirectories) string { return "true" },
			get:       func(cfg config.Config) string { return strconv.FormatBool(cfg.Server.LANOnly) },
		},
		{
			key: "log.level", yaml: "log:\n  level: debug\n",
			envName: env("LOG_LEVEL"), envValue: "error",
			wantFile: "debug", wantEnv: "error",
			defaultOf: func(paths.DefaultDirectories) string { return "info" },
			get:       func(cfg config.Config) string { return cfg.Log.Level },
		},
		{
			key: "log.format", yaml: "log:\n  format: json\n",
			envName: env("LOG_FORMAT"), envValue: "text",
			wantFile: "json", wantEnv: "text",
			defaultOf: func(paths.DefaultDirectories) string { return "text" },
			get:       func(cfg config.Config) string { return cfg.Log.Format },
		},
		{
			key: "paths.data", yaml: "paths:\n  data: file-data\n",
			envName: env("PATHS_DATA"), envValue: "env-data",
			wantFile: "file-data", wantEnv: "env-data", relative: true,
			defaultOf: func(dirs paths.DefaultDirectories) string { return dirs.Data.Path },
			get:       func(cfg config.Config) string { return cfg.Paths.Data },
		},
		{
			key: "paths.cache", yaml: "paths:\n  cache: file-cache\n",
			envName: env("PATHS_CACHE"), envValue: "env-cache",
			wantFile: "file-cache", wantEnv: "env-cache", relative: true,
			defaultOf: func(dirs paths.DefaultDirectories) string { return dirs.Cache.Path },
			get:       func(cfg config.Config) string { return cfg.Paths.Cache },
		},
		{
			key: "paths.transcode", yaml: "paths:\n  transcode: file-transcode\n",
			envName: env("PATHS_TRANSCODE"), envValue: "env-transcode",
			wantFile: "file-transcode", wantEnv: "env-transcode", relative: true,
			defaultOf: func(dirs paths.DefaultDirectories) string { return dirs.Transcode.Path },
			get:       func(cfg config.Config) string { return cfg.Paths.Transcode },
		},
	}, toolAndTranscodeCases()...)
}

// toolAndTranscodeCases cover the scalar keys of milestone 2 (ADR 0022, ADR 0024).
func toolAndTranscodeCases() []precedenceCase {
	return []precedenceCase{
		{
			key: "tools.ffmpeg", yaml: "tools:\n  ffmpeg: file-bin/ffmpeg\n",
			envName: env("TOOLS_FFMPEG"), envValue: "env-bin/ffmpeg",
			wantFile: filepath.Join("file-bin", "ffmpeg"), wantEnv: filepath.Join("env-bin", "ffmpeg"), relative: true,
			defaultOf: func(paths.DefaultDirectories) string { return "ffmpeg" },
			get:       func(cfg config.Config) string { return cfg.Tools.FFmpeg },
		},
		{
			key: "tools.ffprobe", yaml: "tools:\n  ffprobe: ffprobe7\n",
			envName: env("TOOLS_FFPROBE"), envValue: "ffprobe8",
			wantFile: "ffprobe7", wantEnv: "ffprobe8",
			defaultOf: func(paths.DefaultDirectories) string { return "ffprobe" },
			get:       func(cfg config.Config) string { return cfg.Tools.FFprobe },
		},
		{
			key: "transcode.max_sessions", yaml: "transcode:\n  max_sessions: 4\n",
			envName: env("TRANSCODE_MAX_SESSIONS"), envValue: "8",
			wantFile: "4", wantEnv: "8",
			defaultOf: func(paths.DefaultDirectories) string { return "2" },
			get:       func(cfg config.Config) string { return strconv.Itoa(cfg.Transcode.MaxSessions) },
		},
	}
}

type precedenceLayer struct {
	name    string
	useFile bool
	useEnv  bool
}

func TestLoadPrecedence(t *testing.T) {
	t.Parallel()

	layers := []precedenceLayer{
		{name: "default"},
		{name: "file over default", useFile: true},
		{name: "env over default", useEnv: true},
		{name: "env over file", useFile: true, useEnv: true},
	}

	for _, layer := range layers {
		for _, tc := range precedenceCases() {
			t.Run(tc.key+"/"+layer.name, func(t *testing.T) {
				t.Parallel()
				assertPrecedence(t, tc, layer)
			})
		}
	}
}

func assertPrecedence(t *testing.T, tc precedenceCase, layer precedenceLayer) {
	t.Helper()

	f := newFixture(t, "", nil)
	want := tc.defaultOf(defaultDirs(t, f))

	if layer.useFile {
		writeConfigFile(t, f.configDir, tc.yaml)
		want = tc.wantFile
	}

	if layer.useEnv {
		f.vars[tc.envName] = tc.envValue
		want = tc.wantEnv
	}

	if tc.relative && (layer.useFile || layer.useEnv) {
		want = filepath.Join(f.configDir, want)
	}

	got, err := f.load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if value := tc.get(got); value != want {
		t.Errorf("%s = %q, want %q", tc.key, value, want)
	}
}

func TestLoadReportsUnreadableFile(t *testing.T) {
	t.Parallel()

	f := newFixture(t, "", nil)
	if err := os.Mkdir(f.configFile(), 0o750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	_, err := f.load()
	if err == nil || !strings.Contains(err.Error(), f.configFile()) {
		t.Errorf("Load error = %v, want it to name %s", err, f.configFile())
	}
}

func TestLoadNormalisesChoices(t *testing.T) {
	t.Parallel()

	f := newFixture(t, "log:\n  level: DEBUG\n  format: JSON\n", nil)

	got, err := f.load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if got.Log != (config.Log{Level: logging.LevelDebug, Format: logging.FormatJSON}) {
		t.Errorf("Log = %+v, want debug/json", got.Log)
	}
}

func TestLoadMakesConfigDirAbsolute(t *testing.T) {
	t.Parallel()

	f := newFixture(t, "", nil)

	got, err := config.Load(config.Options{ConfigDirFlag: "relative-config", Environment: f.env})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if !filepath.IsAbs(got.ConfigDir) || filepath.Base(got.ConfigDir) != "relative-config" {
		t.Errorf("ConfigDir = %q, want an absolute path ending in relative-config", got.ConfigDir)
	}
}

func TestLoadUsesConfigDirFromEnvironment(t *testing.T) {
	t.Parallel()

	f := newFixture(t, "server:\n  port: 1234\n", nil)
	f.vars[paths.ConfigDirEnv()] = f.configDir

	got, err := config.Load(config.Options{Environment: f.env})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if got.ConfigDir != f.configDir || got.Server.Port != 1234 {
		t.Errorf("Load = %+v, want config dir %s and port 1234", got, f.configDir)
	}
}

func TestLoadConfigDirError(t *testing.T) {
	t.Parallel()

	f := newFixture(t, "", map[string]string{paths.ConfigDirEnv(): ""})

	_, err := config.Load(config.Options{Environment: f.env})
	if !errors.Is(err, paths.ErrEmptyPath) || !strings.Contains(err.Error(), paths.ConfigDirEnv()) {
		t.Errorf("Load error = %v, want ErrEmptyPath naming %s", err, paths.ConfigDirEnv())
	}
}

func TestLoadRejectsDriveRelativeConfigDirOnWindows(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		flag     string
		vars     map[string]string
		wantPart string
	}{
		{name: "flag", flag: "C:cfg", wantPart: "flag value"},
		{name: "environment", vars: map[string]string{paths.ConfigDirEnv(): "D:cfg"}, wantPart: paths.ConfigDirEnv()},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newFixture(t, "", tt.vars)
			f.env.GOOS = "windows"

			_, err := config.Load(config.Options{ConfigDirFlag: tt.flag, Environment: f.env})
			if !errors.Is(err, paths.ErrDriveRelative) || !strings.Contains(err.Error(), tt.wantPart) {
				t.Errorf("Load error = %v, want ErrDriveRelative naming %s", err, tt.wantPart)
			}
		})
	}
}
