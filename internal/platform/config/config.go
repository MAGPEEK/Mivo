// Package config loads the server configuration (ADR 0018).
//
// Precedence, later wins: built-in defaults < config.yaml in the config directory <
// environment variables. Environment variables are named after the YAML key path with
// the brand prefix, e.g. server.port becomes <PREFIX>_SERVER_PORT. Relative paths, from
// the file or the environment, resolve against the config directory so the result does
// not depend on the working directory of the process. Every directory in Config.Paths is
// absolute. The library list (ADR 0022) comes from the file or, as a whole, from
// <PREFIX>_LIBRARIES; it is never merged entry by entry.
package config

import (
	"errors"
	"fmt"
	"path/filepath"

	"github.com/magpeek/mivo/internal/platform/logging"
	"github.com/magpeek/mivo/internal/platform/paths"
)

// FileName is the name of the optional configuration file inside the config directory.
const FileName = "config.yaml"

// DefaultPort is the default HTTP port (ADR 0018).
const DefaultPort = 8460

// Defaults of the milestone-2 settings (ADR 0022, ADR 0024). A bare program name is looked
// up on PATH at startup.
const (
	DefaultFFmpeg  = "ffmpeg"
	DefaultFFprobe = "ffprobe"
	// DefaultTranscodeMaxSessions follows OD-4: two re-encoding sessions saturate a typical
	// NAS CPU.
	DefaultTranscodeMaxSessions = 2
)

var (
	// ErrInvalid marks a configuration value that failed validation.
	ErrInvalid = errors.New("invalid configuration value")

	// ErrUnknownKey marks a key in config.yaml that is not a setting.
	ErrUnknownKey = errors.New("unknown key")

	// errNoDefault explains a missing directory whose OS default reported no reason.
	errNoDefault = errors.New("no OS default")
)

// Config is the complete server configuration. It is passed by value; app and domain never
// import this package.
type Config struct {
	// ConfigDir is the absolute config directory the file was looked up in. It is not a
	// YAML key.
	ConfigDir string
	Server    Server
	Log       Log
	Paths     Paths
	// Libraries are the media libraries in configuration order; empty, never nil, when
	// none is configured. Roots are absolute, unique and do not overlap each other or a
	// directory in Paths or ConfigDir.
	Libraries []LibraryConfig
	Tools     Tools
	Transcode Transcode
}

// Tools names the external programs the server runs.
type Tools struct {
	// FFmpeg is a bare program name looked up on PATH at startup or an absolute path.
	FFmpeg string
	// FFprobe is a bare program name looked up on PATH at startup or an absolute path.
	FFprobe string
}

// Transcode holds the limits of FFmpeg sessions (ADR 0024).
type Transcode struct {
	// MaxSessions limits concurrent playback sessions that re-encode a stream; remux
	// sessions only count towards the fixed safety cap of the playback service.
	MaxSessions int
}

// Server holds the HTTP listener settings.
type Server struct {
	// Host is the interface to listen on; empty means all interfaces.
	Host string
	// Port is the TCP port; 0 lets the OS choose one (used by tests).
	Port int
}

// Log holds the logging settings.
type Log struct {
	// Level is one of the logging.Level* constants.
	Level string
	// Format is one of the logging.Format* constants.
	Format string
}

// Paths holds the absolute directories the server writes to.
type Paths struct {
	Data      string
	Cache     string
	Transcode string
}

// Options are the inputs of Load.
type Options struct {
	// ConfigDirFlag is the value of the --config-dir flag; empty when not given.
	ConfigDirFlag string
	// Environment is the view of the host: OS and environment variables.
	Environment paths.Environment
}

// Load builds the configuration from defaults, the optional config file and the
// environment. A missing config file is not an error and no file is created.
func Load(options Options) (Config, error) {
	configDir, err := resolveConfigDir(options)
	if err != nil {
		return Config{}, err
	}

	// A missing default is only fatal if neither the file nor the environment set the path.
	defaults := paths.Defaults(options.Environment)
	cfg := defaultConfig(configDir, defaults)

	libraries, err := applyFile(&cfg, configDir)
	if err != nil {
		return Config{}, err
	}

	if err := applyEnvironment(&cfg, options.Environment, configDir); err != nil {
		return Config{}, err
	}

	envLibraries, ok, err := environmentLibraries(options.Environment, configDir)
	if err != nil {
		return Config{}, err
	}

	if ok {
		libraries = envLibraries
	}

	if err := requirePaths(cfg, defaults); err != nil {
		return Config{}, err
	}

	if err := checkLibraryRoots(libraries, cfg, options.Environment.GOOS); err != nil {
		return Config{}, err
	}

	cfg.Libraries = libraryConfigs(libraries)

	return cfg, nil
}

func resolveConfigDir(options Options) (string, error) {
	configDir, err := paths.ConfigDir(options.ConfigDirFlag, options.Environment)
	if err != nil {
		return "", fmt.Errorf("config directory: %w", err)
	}

	absolute, err := filepath.Abs(configDir)
	if err != nil {
		return "", fmt.Errorf("config directory %s: %w", configDir, err)
	}

	return absolute, nil
}

func defaultConfig(configDir string, defaults paths.DefaultDirectories) Config {
	return Config{
		ConfigDir: configDir,
		Server:    Server{Host: "", Port: DefaultPort},
		Log:       Log{Level: logging.LevelInfo, Format: logging.FormatText},
		Paths: Paths{
			Data:      defaults.Data.Path,
			Cache:     defaults.Cache.Path,
			Transcode: defaults.Transcode.Path,
		},
		Libraries: []LibraryConfig{},
		Tools:     Tools{FFmpeg: DefaultFFmpeg, FFprobe: DefaultFFprobe},
		Transcode: Transcode{MaxSessions: DefaultTranscodeMaxSessions},
	}
}

// requirePaths reports a directory that has neither an OS default nor a configured value,
// and an OS default that is not absolute on this host (an Environment for another OS).
func requirePaths(cfg Config, defaults paths.DefaultDirectories) error {
	for _, entry := range settings() {
		if entry.pathField == nil {
			continue
		}

		value := *entry.pathField(&cfg)
		if value == "" {
			reason := entry.pathDefault(defaults).Err
			if reason == nil {
				reason = errNoDefault
			}

			return fmt.Errorf("%s: no value in %s or %s and no OS default: %w",
				entry.key, FileName, envName(entry.key), reason)
		}

		if !filepath.IsAbs(value) {
			return fmt.Errorf("%s: %w: %q is not an absolute path on this host", entry.key, ErrInvalid, value)
		}
	}

	return nil
}
