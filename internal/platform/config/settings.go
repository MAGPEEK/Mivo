package config

import (
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/magpeek/mivo/internal/platform/brand"
	"github.com/magpeek/mivo/internal/platform/logging"
	"github.com/magpeek/mivo/internal/platform/paths"
)

// Key paths of all scalar settings. They are the YAML key paths and the source of the
// environment variable names. Multi-word keys use snake_case so the environment name stays
// unambiguous (ADR 0022).
const (
	keyServerHost           = "server.host"
	keyServerPort           = "server.port"
	keyLogLevel             = "log.level"
	keyLogFormat            = "log.format"
	keyPathsData            = "paths.data"
	keyPathsCache           = "paths.cache"
	keyPathsTranscode       = "paths.transcode"
	keyToolsFFmpeg          = "tools.ffmpeg"
	keyToolsFFprobe         = "tools.ffprobe"
	keyTranscodeMaxSessions = "transcode.max_sessions"
)

// keySeparator separates the sections of a key path.
const keySeparator = "."

const (
	minPort = 0
	maxPort = 65535

	// The transcode limit counts re-encoding sessions only (ADR 0024, OD-4). Its maximum is
	// the fixed safety cap of 8 FFmpeg sessions of ADR 0024: a higher limit could never be
	// reached, so it would be silently ineffective.
	minTranscodeSessions = 1
	maxTranscodeSessions = 8
)

// errEmptyTool reports an empty program setting.
var errEmptyTool = errors.New("program must not be empty; use a name looked up on PATH or a path")

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

// settings returns every configurable scalar key in a stable order. The list setting
// "libraries" has its own handler (libraries.go).
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
		{key: keyToolsFFmpeg, apply: applyTool(ffmpegField)},
		{key: keyToolsFFprobe, apply: applyTool(ffprobeField)},
		{key: keyTranscodeMaxSessions, apply: applyMaxSessions},
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
func ffmpegField(cfg *Config) *string    { return &cfg.Tools.FFmpeg }
func ffprobeField(cfg *Config) *string   { return &cfg.Tools.FFprobe }

func dataDefault(defaults paths.DefaultDirectories) paths.Default      { return defaults.Data }
func cacheDefault(defaults paths.DefaultDirectories) paths.Default     { return defaults.Cache }
func transcodeDefault(defaults paths.DefaultDirectories) paths.Default { return defaults.Transcode }

// applyHost accepts any value; the listener reports unusable hosts when it binds.
func applyHost(cfg *Config, raw, _ string) error {
	cfg.Server.Host = raw

	return nil
}

func applyPort(cfg *Config, raw, _ string) error {
	port, err := parseIntInRange("port", raw, minPort, maxPort)
	if err != nil {
		return err
	}

	cfg.Server.Port = port

	return nil
}

func applyMaxSessions(cfg *Config, raw, _ string) error {
	sessions, err := parseIntInRange("re-encoding session limit", raw, minTranscodeSessions, maxTranscodeSessions)
	if err != nil {
		return fmt.Errorf("%w; the server runs at most %d FFmpeg sessions at once", err, maxTranscodeSessions)
	}

	cfg.Transcode.MaxSessions = sessions

	return nil
}

// parseIntInRange parses a decimal integer between minimum and maximum inclusive; what
// names the value in the error.
func parseIntInRange(what, raw string, minimum, maximum int) (int, error) {
	value, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("%w: %s %q is not a number", ErrInvalid, what, raw)
	}

	if value < minimum || value > maximum {
		return 0, fmt.Errorf("%w: %s %d is outside %d-%d", ErrInvalid, what, value, minimum, maximum)
	}

	return value, nil
}

// applyChoice accepts one of the allowed values, case-insensitively, and stores it in
// lower case.
func applyChoice(field func(cfg *Config) *string, allowed ...string) applyFunc {
	return func(cfg *Config, raw, _ string) error {
		value, err := parseChoice(raw, allowed...)
		if err != nil {
			return err
		}

		*field(cfg) = value

		return nil
	}
}

// parseChoice returns raw in lower case if it is one of the allowed values.
func parseChoice(raw string, allowed ...string) (string, error) {
	value := strings.ToLower(raw)
	if !slices.Contains(allowed, value) {
		return "", fmt.Errorf("%w: %q is not one of %s", ErrInvalid, raw, strings.Join(allowed, ", "))
	}

	return value, nil
}

// applyPath stores the path made absolute by paths.Resolve, relative to baseDir.
func applyPath(field func(cfg *Config) *string) applyFunc {
	return func(cfg *Config, raw, baseDir string) error {
		resolved, err := resolvePath(raw, baseDir)
		if err != nil {
			return err
		}

		*field(cfg) = resolved

		return nil
	}
}

func resolvePath(raw, baseDir string) (string, error) {
	resolved, err := paths.Resolve(raw, baseDir)
	if err != nil {
		return "", fmt.Errorf("%w: %w", ErrInvalid, err)
	}

	return resolved, nil
}

// applyTool stores an external program. A bare name ("ffmpeg") is kept for the PATH lookup
// at startup; a value with a path separator or a drive follows the path rules, so a
// relative path resolves against baseDir rather than the working directory.
func applyTool(field func(cfg *Config) *string) applyFunc {
	return func(cfg *Config, raw, baseDir string) error {
		if strings.TrimSpace(raw) == "" {
			return fmt.Errorf("%w: %w", ErrInvalid, errEmptyTool)
		}

		if isCommandName(raw) {
			*field(cfg) = raw

			return nil
		}

		return applyPath(field)(cfg, raw, baseDir)
	}
}

// isCommandName reports a bare program name. "~" is excluded so that "~ffmpeg" gets the
// home-shorthand error of the path rules instead of a confusing lookup failure.
func isCommandName(value string) bool {
	return !strings.ContainsRune(value, '/') && !strings.ContainsRune(value, filepath.Separator) &&
		filepath.VolumeName(value) == "" && !strings.HasPrefix(value, "~")
}
