// Package paths computes the OS-specific default directories of the server (ADR 0018),
// resolves and compares configured paths and creates directories on demand.
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
	"path/filepath"
	"runtime"
	"strings"

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
	envUserProfile   = "USERPROFILE"
	envHome          = "HOME"
	envXDGConfigHome = "XDG_CONFIG_HOME"
	envXDGDataHome   = "XDG_DATA_HOME"
	envXDGCacheHome  = "XDG_CACHE_HOME"

	// Fallbacks below the home directory when the base directory variable is unusable:
	// %USERPROFILE%\AppData\Local on Windows, ~/.config, ~/.local/share and ~/.cache
	// in the XDG specification.
	windowsAppDataDirName = "AppData"
	windowsLocalDirName   = "Local"
	xdgConfigDirName      = ".config"
	xdgLocalDirName       = ".local"
	xdgShareDirName       = "share"
	xdgCacheDirName       = ".cache"

	// homeShorthand is expanded by shells only, never by the server.
	homeShorthand = "~"

	windowsSeparator = `\`
	unixSeparator    = "/"
)

var (
	// ErrNoHomeDir reports that a default directory needs the home directory, which is unknown.
	ErrNoHomeDir = errors.New("home directory is unknown")

	// ErrEmptyPath reports a directory setting that is present but empty.
	ErrEmptyPath = errors.New("path must not be empty")

	// ErrHomeShorthand reports a path starting with "~", which only shells expand.
	ErrHomeShorthand = errors.New(`path starts with "~", which is not expanded to the home directory; use an absolute path`)

	// ErrDriveRelative reports a Windows path such as "C:data" that depends on the current
	// directory of a drive.
	ErrDriveRelative = errors.New(`drive-relative paths such as "C:data" are not supported; use "C:\data"`)
)

// Environment is the view of the host that the default directories depend on.
type Environment struct {
	// GOOS is the target operating system, e.g. "windows" or "linux".
	GOOS string
	// LookupEnv returns an environment variable; nil means no variable is set. The home
	// directory is read through it as well (USERPROFILE on Windows, HOME elsewhere).
	LookupEnv func(key string) (string, bool)
}

// Default is one default directory or the reason why there is none.
type Default struct {
	// Path is the absolute default directory; empty when Err is set.
	Path string
	// Err explains the missing default; it wraps ErrNoHomeDir.
	Err error
}

// DefaultDirectories are the OS defaults of every server directory. Each one is computed
// independently, so a missing variable only affects the directories that need it.
type DefaultDirectories struct {
	Config    Default
	Data      Default
	Cache     Default
	Transcode Default
}

// HostEnvironment describes the OS the process runs on with the given variable lookup,
// normally os.LookupEnv. The lookup is a parameter so that an environment injected by the
// caller (tests, M1-10) is never mixed with the variables of the host.
func HostEnvironment(lookupEnv func(key string) (string, bool)) Environment {
	return Environment{GOOS: runtime.GOOS, LookupEnv: lookupEnv}
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
// flagValue is empty when the flag was not given. Flag and variable values may be relative;
// the caller resolves them against the working directory. A Windows drive-relative value
// ("C:cfg") is rejected like in Resolve, because it depends on the hidden current
// directory of that drive.
func ConfigDir(flagValue string, env Environment) (string, error) {
	if flagValue != "" {
		if err := checkConfigDirValue(flagValue, env.GOOS); err != nil {
			return "", fmt.Errorf("flag value: %w", err)
		}

		return filepath.Clean(flagValue), nil
	}

	if value, ok := env.Lookup(ConfigDirEnv()); ok {
		if err := checkConfigDirValue(value, env.GOOS); err != nil {
			return "", fmt.Errorf("environment variable %s: %w", ConfigDirEnv(), err)
		}

		return filepath.Clean(value), nil
	}

	defaultDir := Defaults(env).Config
	if defaultDir.Err != nil {
		return "", fmt.Errorf("default config directory: %w", defaultDir.Err)
	}

	return defaultDir.Path, nil
}

// Defaults returns the OS default directories from the ADR 0018 table.
func Defaults(env Environment) DefaultDirectories {
	if env.GOOS == goosWindows {
		return windowsDefaults(env)
	}

	return xdgDefaults(env)
}

// windowsDefaults keeps everything below %LOCALAPPDATA%\<DisplayName>.
func windowsDefaults(env Environment) DefaultDirectories {
	base := baseDir(env, envLocalAppData, windowsAppDataDirName, windowsLocalDirName)
	root := below(base, brand.DisplayName)

	return DefaultDirectories{
		Config:    root,
		Data:      below(root, dataDirName),
		Cache:     below(root, cacheDirName),
		Transcode: below(root, transcodeDirName),
	}
}

// xdgDefaults follows the XDG Base Directory specification used on Linux.
func xdgDefaults(env Environment) DefaultDirectories {
	cache := below(baseDir(env, envXDGCacheHome, xdgCacheDirName), brand.ID)

	return DefaultDirectories{
		Config:    below(baseDir(env, envXDGConfigHome, xdgConfigDirName), brand.ID),
		Data:      below(baseDir(env, envXDGDataHome, xdgLocalDirName, xdgShareDirName), brand.ID),
		Cache:     cache,
		Transcode: below(cache, transcodeDirName),
	}
}

// baseDir returns the variable or its fallback below the home directory. Relative values
// are ignored: the XDG specification says they are invalid, and a relative LOCALAPPDATA or
// home directory would make the result depend on the working directory.
func baseDir(env Environment, variable string, homeRelative ...string) Default {
	if value, ok := env.Lookup(variable); ok && isAbsolute(env.GOOS, value) {
		return Default{Path: value}
	}

	homeVariable := homeVariableOf(env.GOOS)

	home, _ := env.Lookup(homeVariable)
	if !isAbsolute(env.GOOS, home) {
		return Default{Err: fmt.Errorf("%s and %s are unset or not absolute: %w", variable, homeVariable, ErrNoHomeDir)}
	}

	return Default{Path: filepath.Join(append([]string{home}, homeRelative...)...)}
}

// below joins elements to parent and keeps the error of a parent without default.
func below(parent Default, elements ...string) Default {
	if parent.Err != nil {
		return parent
	}

	return Default{Path: filepath.Join(append([]string{parent.Path}, elements...)...)}
}

func homeVariableOf(goos string) string {
	if goos == goosWindows {
		return envUserProfile
	}

	return envHome
}

// isAbsolute reports whether value is absolute on goos, independent of the host OS, so
// the Windows defaults are testable on Linux and vice versa.
func isAbsolute(goos, value string) bool {
	if goos != goosWindows {
		return strings.HasPrefix(value, "/")
	}

	return isWindowsDriveAbsolute(value) || isWindowsUNC(value)
}

// isWindowsDriveAbsolute matches "C:\" and "C:/" prefixes.
func isWindowsDriveAbsolute(value string) bool {
	const driveLetter, colon, separator = 0, 1, 2

	return len(value) > separator && isASCIILetter(value[driveLetter]) &&
		value[colon] == ':' && isWindowsSeparator(value[separator])
}

// isWindowsDriveRelative matches "C:" and "C:data": a drive letter whose colon is not
// followed by a separator.
func isWindowsDriveRelative(value string) bool {
	const driveLetter, colon, minLength = 0, 1, 2

	return len(value) >= minLength && isASCIILetter(value[driveLetter]) &&
		value[colon] == ':' && !isWindowsDriveAbsolute(value)
}

// isWindowsUNC matches "\\server\share" paths, including "\\?\" long paths.
func isWindowsUNC(value string) bool {
	return strings.HasPrefix(value, `\\`) || strings.HasPrefix(value, "//")
}

func isWindowsSeparator(character byte) bool {
	return character == '\\' || character == '/'
}

func isASCIILetter(character byte) bool {
	return ('a' <= character && character <= 'z') || ('A' <= character && character <= 'Z')
}

// Resolve makes a configured path absolute. baseDir must be absolute. Relative paths
// resolve against baseDir. On Windows a rooted path without drive ("\data") resolves
// against the volume of baseDir, like Windows resolves it against the current drive, and a
// drive-relative path ("C:data") is rejected because it depends on hidden process state.
func Resolve(value, baseDir string) (string, error) {
	if err := checkUsable(value); err != nil {
		return "", err
	}

	switch {
	case filepath.IsAbs(value):
		return filepath.Clean(value), nil
	case filepath.VolumeName(value) != "":
		return "", fmt.Errorf("%q: %w", value, ErrDriveRelative)
	case os.IsPathSeparator(value[0]):
		return filepath.Join(filepath.VolumeName(baseDir), value), nil
	default:
		return filepath.Join(baseDir, value), nil
	}
}

// checkUsable rejects values that are never a usable path.
func checkUsable(value string) error {
	if value == "" {
		return ErrEmptyPath
	}

	if strings.HasPrefix(value, homeShorthand) {
		return fmt.Errorf("%q: %w", value, ErrHomeShorthand)
	}

	return nil
}

// checkConfigDirValue rejects config directory values that are never usable on goos. The
// check follows goos rather than the host so that it is testable everywhere.
func checkConfigDirValue(value, goos string) error {
	if err := checkUsable(value); err != nil {
		return err
	}

	if goos == goosWindows && isWindowsDriveRelative(value) {
		return fmt.Errorf("%q: %w", value, ErrDriveRelative)
	}

	return nil
}

// Within reports whether path is root itself or lies below it. Both must be cleaned
// absolute paths of goos. On Windows the comparison ignores case and treats "/" like "\",
// as the file system does; elsewhere it is exact. Symlinks are not evaluated, so neither
// path has to exist.
func Within(goos, root, path string) bool {
	root, path = comparablePath(goos, root), comparablePath(goos, path)
	if root == path {
		return true
	}

	separator := separatorOf(goos)

	prefix := root
	if !strings.HasSuffix(prefix, separator) {
		prefix += separator
	}

	return strings.HasPrefix(path, prefix)
}

// comparablePath folds the spellings of one Windows path to a single form. Upper case
// mirrors the up-case table NTFS compares names with.
func comparablePath(goos, path string) string {
	if goos != goosWindows {
		return path
	}

	return strings.ToUpper(strings.ReplaceAll(path, "/", windowsSeparator))
}

func separatorOf(goos string) string {
	if goos == goosWindows {
		return windowsSeparator
	}

	return unixSeparator
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
