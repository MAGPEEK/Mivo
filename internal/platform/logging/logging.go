// Package logging builds the server's structured logger on top of log/slog.
//
// The package only constructs loggers; it never installs a process-wide default and keeps
// no package-level logger. Callers inject the returned *slog.Logger through constructors
// (CLAUDE.md: no globals).
//
// # Redaction
//
// Every logger it returns replaces the value of sensitive attributes with RedactedValue.
// A key is normalised first (everything except ASCII letters and digits removed, the rest
// lower-cased) and is sensitive when it contains a sensitive word, unless the text after
// that word is exactly one safe qualifier such as "count" or "hint". The word lists live in
// sensitiveWords and safeQualifiers, the rule in isSensitiveKey (redact.go). So "x-api-key",
// "aws_secret_access_key", "passwordHash" and "tokens" are redacted, while "tokenCount" and
// "passwordHint" stay visible. Every leaf below a group with a sensitive key is redacted as
// well, whether the group comes from slog.Group, Logger.WithGroup or a slog.LogValuer.
//
// The rule fails closed and over-redacts on purpose: an unknown qualifier hides the value, so
// harmless keys such as "nextPageToken" or "secretary" are redacted too. Rename such a key
// rather than weakening the rule.
//
// Limits: redaction works on attribute keys only. A value logged as a whole under a
// harmless key, for example a struct, a map or an http.Header passed through slog.Any, is
// encoded without inspection, and secrets inside it are written in clear text. Types that
// hold secrets must therefore implement slog.LogValuer and leave the secret out or put it
// under a sensitive key, and HTTP headers must be logged through an allowlist of header
// names (to be done in adapters/http), never as a whole.
package logging

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"
)

// Accepted values of Options.Level. They match the log.level config key (ADR 0018).
const (
	LevelDebug = "debug"
	LevelInfo  = "info"
	LevelWarn  = "warn"
	LevelError = "error"
)

// Accepted values of Options.Format. They match the log.format config key (ADR 0018).
const (
	FormatText = "text"
	FormatJSON = "json"
)

var (
	// ErrUnknownLevel is returned by New when Options.Level is not one of the Level* values.
	ErrUnknownLevel = errors.New("unknown log level")

	// ErrUnknownFormat is returned by New when Options.Format is not one of the Format* values.
	ErrUnknownFormat = errors.New("unknown log format")

	// ErrNoWriter is returned by New when Options.Writer is nil.
	ErrNoWriter = errors.New("no log writer")
)

// Options configures New. Level and Format are plain strings rather than the config struct
// so that this package does not depend on platform/config. Both are matched
// case-insensitively, so "INFO" from an environment variable is accepted.
type Options struct {
	// Level is the minimum level that is written: debug, info, warn or error.
	Level string
	// Format is the output encoding: text (key=value) or json (one object per line).
	Format string
	// Writer receives the encoded records, typically os.Stderr.
	Writer io.Writer
}

// New returns a logger that writes records at or above options.Level to options.Writer in
// options.Format, with sensitive attributes replaced by RedactedValue.
func New(options Options) (*slog.Logger, error) {
	if options.Writer == nil {
		return nil, ErrNoWriter
	}

	level, err := parseLevel(options.Level)
	if err != nil {
		return nil, err
	}

	handlerOptions := &slog.HandlerOptions{Level: level, ReplaceAttr: redactSensitive}

	switch strings.ToLower(options.Format) {
	case FormatText:
		return slog.New(slog.NewTextHandler(options.Writer, handlerOptions)), nil
	case FormatJSON:
		return slog.New(slog.NewJSONHandler(options.Writer, handlerOptions)), nil
	default:
		return nil, fmt.Errorf("%w %q (want %s or %s)", ErrUnknownFormat, options.Format, FormatText, FormatJSON)
	}
}

// parseLevel accepts exactly the four documented names. slog.Level.UnmarshalText is not
// used because it also accepts offsets such as "info+2", which the config does not allow.
func parseLevel(name string) (slog.Level, error) {
	switch strings.ToLower(name) {
	case LevelDebug:
		return slog.LevelDebug, nil
	case LevelInfo:
		return slog.LevelInfo, nil
	case LevelWarn:
		return slog.LevelWarn, nil
	case LevelError:
		return slog.LevelError, nil
	default:
		return 0, fmt.Errorf("%w %q (want %s, %s, %s or %s)",
			ErrUnknownLevel, name, LevelDebug, LevelInfo, LevelWarn, LevelError)
	}
}
