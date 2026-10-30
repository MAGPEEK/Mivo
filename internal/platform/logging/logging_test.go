package logging_test

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"github.com/magpeek/mivo/internal/platform/logging"
)

// logAtEveryLevel writes one record per level so a test can see which ones survive.
func logAtEveryLevel(logger *slog.Logger) {
	logger.Debug("debug record")
	logger.Info("info record")
	logger.Warn("warn record")
	logger.Error("error record")
}

func TestNewDropsRecordsBelowTheConfiguredLevel(t *testing.T) {
	t.Parallel()

	allMessages := []string{"debug record", "info record", "warn record", "error record"}
	tests := []struct {
		level       string
		wantVisible []string
	}{
		{level: logging.LevelDebug, wantVisible: allMessages},
		{level: logging.LevelInfo, wantVisible: allMessages[1:]},
		{level: logging.LevelWarn, wantVisible: allMessages[2:]},
		{level: logging.LevelError, wantVisible: allMessages[3:]},
		{level: "WARN", wantVisible: allMessages[2:]},
	}

	for _, test := range tests {
		t.Run(test.level, func(t *testing.T) {
			t.Parallel()

			var output bytes.Buffer
			logger := newTestLogger(t, logging.Options{
				Level: test.level, Format: logging.FormatText, Writer: &output,
			})
			logAtEveryLevel(logger)

			for _, message := range allMessages {
				want := containsString(test.wantVisible, message)
				if got := strings.Contains(output.String(), message); got != want {
					t.Errorf("level %s: record %q visible = %t, want %t\noutput:\n%s",
						test.level, message, got, want, output.String())
				}
			}
		})
	}
}

func TestNewWritesTheConfiguredFormat(t *testing.T) {
	t.Parallel()

	tests := []struct {
		format     string
		wantPrefix string
	}{
		{format: logging.FormatText, wantPrefix: "time="},
		{format: logging.FormatJSON, wantPrefix: `{"time":`},
		{format: "JSON", wantPrefix: `{"time":`},
	}

	for _, test := range tests {
		t.Run(test.format, func(t *testing.T) {
			t.Parallel()

			var output bytes.Buffer
			logger := newTestLogger(t, logging.Options{
				Level: logging.LevelInfo, Format: test.format, Writer: &output,
			})
			logger.Info("hello", "count", 3)

			if !strings.HasPrefix(output.String(), test.wantPrefix) {
				t.Errorf("format %s: output %q does not start with %q",
					test.format, output.String(), test.wantPrefix)
			}
		})
	}
}

func TestNewJSONWritesOneValidObjectPerLine(t *testing.T) {
	t.Parallel()

	var output bytes.Buffer
	logger := newTestLogger(t, logging.Options{
		Level: logging.LevelDebug, Format: logging.FormatJSON, Writer: &output,
	})
	logAtEveryLevel(logger)

	wantLevels := []string{"DEBUG", "INFO", "WARN", "ERROR"}
	records := decodeJSONLines(t, &output)
	if len(records) != len(wantLevels) {
		t.Fatalf("got %d JSON records, want %d", len(records), len(wantLevels))
	}

	for index, record := range records {
		for _, key := range []string{slog.TimeKey, slog.LevelKey, slog.MessageKey} {
			if _, ok := record[key]; !ok {
				t.Errorf("record %d has no %q field: %v", index, key, record)
			}
		}

		if record[slog.LevelKey] != wantLevels[index] {
			t.Errorf("record %d level = %v, want %s", index, record[slog.LevelKey], wantLevels[index])
		}
	}
}

func TestNewRejectsInvalidOptions(t *testing.T) {
	t.Parallel()

	// Each case builds its own writer so that parallel subtests never share a buffer. Every
	// case quotes a value, so the message check can never pass on an empty expectation.
	tests := []struct {
		name      string
		level     string
		format    string
		wantErr   error
		wantQuote string
	}{
		{name: "unknown level", level: "loud", format: logging.FormatText, wantErr: logging.ErrUnknownLevel, wantQuote: `"loud"`},
		{name: "empty level", level: "", format: logging.FormatText, wantErr: logging.ErrUnknownLevel, wantQuote: `""`},
		{name: "slog offset level", level: "info+2", format: logging.FormatText, wantErr: logging.ErrUnknownLevel, wantQuote: `"info+2"`},
		{name: "unknown format", level: logging.LevelInfo, format: "xml", wantErr: logging.ErrUnknownFormat, wantQuote: `"xml"`},
		{name: "empty format", level: logging.LevelInfo, format: "", wantErr: logging.ErrUnknownFormat, wantQuote: `""`},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			logger, err := logging.New(logging.Options{
				Level: test.level, Format: test.format, Writer: &bytes.Buffer{},
			})
			if !errors.Is(err, test.wantErr) {
				t.Fatalf("New() error = %v, want %v", err, test.wantErr)
			}

			if !strings.Contains(err.Error(), test.wantQuote) {
				t.Errorf("New() error = %q, want it to quote the invalid value as %s", err, test.wantQuote)
			}

			if logger != nil {
				t.Errorf("New() logger = %v, want nil on error", logger)
			}
		})
	}
}

// TestNewRejectsMissingWriter is separate from the table above because there is no invalid
// value to quote. Level and format are valid, so only the missing writer can cause the error.
func TestNewRejectsMissingWriter(t *testing.T) {
	t.Parallel()

	logger, err := logging.New(logging.Options{Level: logging.LevelInfo, Format: logging.FormatJSON})
	if !errors.Is(err, logging.ErrNoWriter) {
		t.Fatalf("New() error = %v, want %v", err, logging.ErrNoWriter)
	}

	if logger != nil {
		t.Errorf("New() logger = %v, want nil on error", logger)
	}
}

func newTestLogger(t *testing.T, options logging.Options) *slog.Logger {
	t.Helper()

	logger, err := logging.New(options)
	if err != nil {
		t.Fatalf("New(%+v) error = %v", options, err)
	}

	return logger
}

func decodeJSONLines(t *testing.T, output *bytes.Buffer) []map[string]any {
	t.Helper()

	var records []map[string]any
	scanner := bufio.NewScanner(output)
	for scanner.Scan() {
		var record map[string]any
		if err := json.Unmarshal(scanner.Bytes(), &record); err != nil {
			t.Fatalf("line %q is not valid JSON: %v", scanner.Text(), err)
		}

		records = append(records, record)
	}

	if err := scanner.Err(); err != nil {
		t.Fatalf("scan output: %v", err)
	}

	return records
}

func containsString(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}

	return false
}
