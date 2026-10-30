// Package paths computes the OS-specific default directories of the server (ADR 0018) and
// creates them on demand.
//
// The computation is pure: it sees the host only through an Environment value, so the
// Windows and Linux defaults are testable on any host. Paths are joined with the host
// separator because the server always computes them for the OS it runs on.
package paths

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"runtime"

	"github.com/magpeek/mivo/internal/platform/brand"
)

const (
	// goosWindows is the GOOS value of Windows; every other OS uses the XDG layout.
	goosWindows = "windows"

	// dirMode keeps server directories private to the service account and its group.
	dirMode fs.FileMode = 0o750

	configDirEnvSuffix = "_CONFIG_DIR"

	dataDirName      = "data"
	cacheDirName     = "cache"
	transcodeDirName = "transcode"

	envLocalAppData  = "LOCALAPPDATA"
	envXDGConfigHome = "XDG_CONFIG_HOME"
	envXDGDataHome   = "XDG_DATA_HOME"
	envXDGCacheHome  = "XDG_CACHE_HOME"
)

var (
	// ErrNoHomeDir reports that a default directory needs the home directory, which is unknown.
	ErrNoHomeDir = errors.New("home directory is unknown")

	// ErrEmptyPath reports a directory setting that is present but empty.
	ErrEmptyPath = errors.New("path must not be empty")
)

// Environment is the view of the host that the default directories depend on.
type Environment struct {
	// GOOS is the target operating system, e.g. "windows" or "linux".
	GOOS string
	// LookupEnv returns an environment variable; nil means no variable is set.
	LookupEnv func(key string) (string, bool)
	// HomeDir is the user's home directory; empty when unknown.
	HomeDir string
}

// Directories are the directories the server uses.
type Directories struct {
	Config    string
	Data      string
	Cache     string
	Transcode string
}

// HostEnvironment describes the host the process runs on.
func HostEnvironment() Environment {
	home, err := os.UserHomeDir()
	if err != nil {
		// Defaults reports ErrNoHomeDir only if a directory actually needs the home directory.
		home = ""
	}

	return Environment{GOOS: runtime.GOOS, LookupEnv: os.LookupEnv, HomeDir: home}
}

// Lookup returns the environment variable key and whether it is set.
func (env Environment) Lookup(key string) (string, bool) {
	if env.LookupEnv == nil {
		return "", false
	}

	return env.LookupEnv(key)
}

// ConfigDirEnv is the name of the environment variable that overrides the config directory.
func ConfigDirEnv() string {
	return brand.EnvPrefix + configDirEnvSuffix
}

// ConfigDir resolves the config directory: flag value > ConfigDirEnv() > OS default.
// flagValue is empty when the flag was not given.
func ConfigDir(flagValue string, env Environment) (string, error) {
	if flagValue != "" {
		return filepath.Clean(flagValue), nil
	}

	if value, ok := env.Lookup(ConfigDirEnv()); ok {
		if value == "" {
			return "", fmt.Errorf("environment variable %s: %w", ConfigDirEnv(), ErrEmptyPath)
		}

		return filepath.Clean(value), nil
	}

	defaults, err := Defaults(env)
	if err != nil {
		return "", fmt.Errorf("default config directory: %w", err)
	}

	return defaults.Config, nil
}

// Defaults returns the OS default directories from the ADR 0018 table.
func Defaults(env Environment) (Directories, error) {
	if env.GOOS == goosWindows {
		return windowsDefaults(env)
	}

	return xdgDefaults(env)
}

// windowsDefaults keeps everything below %LOCALAPPDATA%\<DisplayName>.
func windowsDefaults(env Environment) (Directories, error) {
	base, ok := env.Lookup(envLocalAppData)
	if !ok || base == "" {
		if env.HomeDir == "" {
			return Directories{}, fmt.Errorf("%s is unset and %w", envLocalAppData, ErrNoHomeDir)
		}

		base = filepath.Join(env.HomeDir, "AppData", "Local")
	}

	root := filepath.Join(base, brand.DisplayName)

	return Directories{
		Config:    root,
		Data:      filepath.Join(root, dataDirName),
		Cache:     filepath.Join(root, cacheDirName),
		Transcode: filepath.Join(root, transcodeDirName),
	}, nil
}

// xdgDefaults follows the XDG Base Directory specification used on Linux.
func xdgDefaults(env Environment) (Directories, error) {
	configHome, err := xdgBase(env, envXDGConfigHome, ".config")
	if err != nil {
		return Directories{}, err
	}

	dataHome, err := xdgBase(env, envXDGDataHome, ".local", "share")
	if err != nil {
		return Directories{}, err
	}

	cacheHome, err := xdgBase(env, envXDGCacheHome, ".cache")
	if err != nil {
		return Directories{}, err
	}

	cache := filepath.Join(cacheHome, brand.ID)

	return Directories{
		Config:    filepath.Join(configHome, brand.ID),
		Data:      filepath.Join(dataHome, brand.ID),
		Cache:     cache,
		Transcode: filepath.Join(cache, transcodeDirName),
	}, nil
}

// xdgBase returns the XDG variable or its fallback below the home directory. The XDG
// specification says relative values are invalid and must be ignored.
func xdgBase(env Environment, variable string, homeRelative ...string) (string, error) {
	if value, ok := env.Lookup(variable); ok && path.IsAbs(value) {
		return value, nil
	}

	if env.HomeDir == "" {
		return "", fmt.Errorf("%s is unset and %w", variable, ErrNoHomeDir)
	}

	return filepath.Join(append([]string{env.HomeDir}, homeRelative...)...), nil
}

// Ensure creates every directory (and missing parents) with mode 0o750.
func Ensure(dirs ...string) error {
	for _, dir := range dirs {
		if dir == "" {
			return fmt.Errorf("create directory: %w", ErrEmptyPath)
		}

		if err := os.MkdirAll(dir, dirMode); err != nil {
			return fmt.Errorf("create directory %s: %w", dir, err)
		}
	}

	return nil
}
