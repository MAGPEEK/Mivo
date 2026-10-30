// Package config loads the server configuration (ADR 0018).
//
// Precedence, later wins: built-in defaults < config.yaml in the config directory <
// environment variables. Environment variables are named after the YAML key path with
// the brand prefix, e.g. server.port becomes <PREFIX>_SERVER_PORT. Relative paths, from
// the file or the environment, resolve against the config directory so the result does
// not depend on the working directory of the process.
package config

import (
	"errors"
	"fmt"
	"path/filepath"

	"github.com/magpeek/mivo/internal/platform/paths"
)

// FileName is the name of the optional configuration file inside the config directory.
const FileName = "config.yaml"

// DefaultPort is the default HTTP port (ADR 0018).
const DefaultPort = 8460

// Log levels and formats accepted by log.level and log.format.
const (
	LogLevelDebug = "debug"
	LogLevelInfo  = "info"
	LogLevelWarn  = "warn"
	LogLevelError = "error"

	LogFormatText = "text"
	LogFormatJSON = "json"
)

// ErrInvalid marks a configuration value that failed validation.
var ErrInvalid = errors.New("invalid configuration value")

// Config is the complete server configuration. It is passed by value; app and domain never
// import this package.
type Config struct {
	// ConfigDir is the absolute config directory the file was looked up in. It is not a
	// YAML key.
	ConfigDir string
	Server    Server
	Log       Log
	Paths     Paths
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
	// Level is one of the LogLevel constants.
	Level string
	// Format is one of the LogFormat constants.
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
	// Environment is the view of the host: OS, environment variables and home directory.
	Environment paths.Environment
}

// Load builds the configuration from defaults, the optional config file and the
// environment. A missing config file is not an error and no file is created.
func Load(options Options) (Config, error) {
	configDir, err := resolveConfigDir(options)
	if err != nil {
		return Config{}, err
	}

	// A failed default is only fatal if neither the file nor the environment set the path.
	defaults, defaultsErr := paths.Defaults(options.Environment)
	cfg := defaultConfig(configDir, defaults)

	if err := applyFile(&cfg, configDir); err != nil {
		return Config{}, err
	}

	if err := applyEnvironment(&cfg, options.Environment, configDir); err != nil {
		return Config{}, err
	}

	if err := requirePaths(cfg, defaultsErr); err != nil {
		return Config{}, err
	}

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

func defaultConfig(configDir string, defaults paths.Directories) Config {
	return Config{
		ConfigDir: configDir,
		Server:    Server{Host: "", Port: DefaultPort},
		Log:       Log{Level: LogLevelInfo, Format: LogFormatText},
		Paths: Paths{
			Data:      defaults.Data,
			Cache:     defaults.Cache,
			Transcode: defaults.Transcode,
		},
	}
}

// requirePaths reports a path that has neither an OS default nor a configured value.
func requirePaths(cfg Config, defaultsErr error) error {
	if defaultsErr == nil {
		return nil
	}

	for _, entry := range settings() {
		if entry.pathField == nil || *entry.pathField(&cfg) != "" {
			continue
		}

		return fmt.Errorf("%s: no value in %s or %s and no OS default: %w",
			entry.key, FileName, envName(entry.key), defaultsErr)
	}

	return nil
}
