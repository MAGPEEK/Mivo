package config

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/magpeek/mivo/internal/platform/brand"
	"github.com/magpeek/mivo/internal/platform/logging"
	"github.com/magpeek/mivo/internal/platform/paths"
)

// Key paths of all settings. They are the YAML key paths and the source of the
// environment variable names.
const (
	keyServerHost     = "server.host"
	keyServerPort     = "server.port"
	keyLogLevel       = "log.level"
	keyLogFormat      = "log.format"
	keyPathsData      = "paths.data"
	keyPathsCache     = "paths.cache"
	keyPathsTranscode = "paths.transcode"
)

// keySeparator separates the sections of a key path.
const keySeparator = "."

const (
	minPort = 0
	maxPort = 65535
)

// applyFunc parses and validates a raw value and stores it in cfg. baseDir is the
// directory relative paths resolve against.
type applyFunc func(cfg *Config, raw, baseDir string) error

// setting maps one key path to its parser. The table is explicit on purpose: it is easier
// to read and to keep within the complexity limits than reflection over struct tags.
type setting struct {
	key   string
	apply applyFunc
	// pathField and pathDefault are set for directory settings so Load can fill in and
	// check the OS default.
	pathField   func(cfg *Config) *string
	pathDefault func(defaults paths.DefaultDirectories) paths.Default
}

// settings returns every configurable key in a stable order.
func settings() []setting {
	return []setting{
		{key: keyServerHost, apply: applyHost},
		{key: keyServerPort, apply: applyPort},
		{
			key: keyLogLevel,
			apply: applyChoice(logLevelField,
				logging.LevelDebug, logging.LevelInfo, logging.LevelWarn, logging.LevelError),
		},
		{key: keyLogFormat, apply: applyChoice(logFormatField, logging.FormatText, logging.FormatJSON)},
		pathSetting(keyPathsData, dataField, dataDefault),
		pathSetting(keyPathsCache, cacheField, cacheDefault),
		pathSetting(keyPathsTranscode, transcodeField, transcodeDefault),
	}
}

func pathSetting(
	key string, field func(cfg *Config) *string, fallback func(paths.DefaultDirectories) paths.Default,
) setting {
	return setting{key: key, apply: applyPath(field), pathField: field, pathDefault: fallback}
}

// envName turns a key path into its environment variable, e.g. server.port → <PREFIX>_SERVER_PORT.
func envName(key string) string {
	return brand.EnvPrefix + "_" + strings.ToUpper(strings.ReplaceAll(key, keySeparator, "_"))
}

func logLevelField(cfg *Config) *string  { return &cfg.Log.Level }
func logFormatField(cfg *Config) *string { return &cfg.Log.Format }
func dataField(cfg *Config) *string      { return &cfg.Paths.Data }
func cacheField(cfg *Config) *string     { return &cfg.Paths.Cache }
func transcodeField(cfg *Config) *string { return &cfg.Paths.Transcode }

func dataDefault(defaults paths.DefaultDirectories) paths.Default      { return defaults.Data }
func cacheDefault(defaults paths.DefaultDirectories) paths.Default     { return defaults.Cache }
func transcodeDefault(defaults paths.DefaultDirectories) paths.Default { return defaults.Transcode }

// applyHost accepts any value; the listener reports unusable hosts when it binds.
func applyHost(cfg *Config, raw, _ string) error {
	cfg.Server.Host = raw

	return nil
}

func applyPort(cfg *Config, raw, _ string) error {
	port, err := strconv.Atoi(raw)
	if err != nil {
		return fmt.Errorf("%w: port %q is not a number", ErrInvalid, raw)
	}

	if port < minPort || port > maxPort {
		return fmt.Errorf("%w: port %d is outside %d-%d", ErrInvalid, port, minPort, maxPort)
	}

	cfg.Server.Port = port

	return nil
}

// applyChoice accepts one of the allowed values, case-insensitively, and stores it in
// lower case.
func applyChoice(field func(cfg *Config) *string, allowed ...string) applyFunc {
	return func(cfg *Config, raw, _ string) error {
		value := strings.ToLower(raw)
		if !slices.Contains(allowed, value) {
			return fmt.Errorf("%w: %q is not one of %s", ErrInvalid, raw, strings.Join(allowed, ", "))
		}

		*field(cfg) = value

		return nil
	}
}

// applyPath stores the path made absolute by paths.Resolve, relative to baseDir.
func applyPath(field func(cfg *Config) *string) applyFunc {
	return func(cfg *Config, raw, baseDir string) error {
		resolved, err := paths.Resolve(raw, baseDir)
		if err != nil {
			return fmt.Errorf("%w: %w", ErrInvalid, err)
		}

		*field(cfg) = resolved

		return nil
	}
}
