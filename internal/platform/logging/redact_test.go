package logging_test

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"

	"github.com/magpeek/mivo/internal/platform/logging"
)

const secretValue = "hunter2-super-secret"

// sessionCredentials implements slog.LogValuer to prove that resolved values are redacted too.
type sessionCredentials struct{ token string }

func (credentials sessionCredentials) LogValue() slog.Value {
	return slog.GroupValue(slog.String("token", credentials.token), slog.String("user", "alice"))
}

func TestNewRedactsSensitiveAttributes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		log  func(logger *slog.Logger)
		// path is the key path of the redacted field in the decoded JSON record.
		path []string
	}{
		{name: "password", log: logWith("password", secretValue), path: []string{"password"}},
		{name: "token", log: logWith("token", secretValue), path: []string{"token"}},
		{name: "secret", log: logWith("secret", secretValue), path: []string{"secret"}},
		{name: "apiKey", log: logWith("apiKey", secretValue), path: []string{"apiKey"}},
		{name: "authorization", log: logWith("authorization", secretValue), path: []string{"authorization"}},
		{name: "upper case", log: logWith("PASSWORD", secretValue), path: []string{"PASSWORD"}},
		{name: "mixed case", log: logWith("ApiKey", secretValue), path: []string{"ApiKey"}},
		{name: "header case", log: logWith("Authorization", secretValue), path: []string{"Authorization"}},
		// The number is only checked through the path: timestamps may contain any digits.
		{name: "non-string value", log: logWith("token", 424242), path: []string{"token"}},
		{
			name: "nested in group",
			log: func(logger *slog.Logger) {
				logger.Info("login", slog.Group("user", slog.String("name", "alice"), slog.String("Password", secretValue)))
			},
			path: []string{"user", "Password"},
		},
		{
			name: "nested in two groups",
			log: func(logger *slog.Logger) {
				logger.Info("call", slog.Group("http", slog.Group("headers", slog.String("Authorization", secretValue))))
			},
			path: []string{"http", "headers", "Authorization"},
		},
		{
			name: "logger WithGroup",
			log: func(logger *slog.Logger) {
				logger.WithGroup("request").Info("call", "token", secretValue)
			},
			path: []string{"request", "token"},
		},
		{
			name: "logger With",
			log: func(logger *slog.Logger) {
				logger.With("apikey", secretValue).Info("call")
			},
			path: []string{"apikey"},
		},
		{
			name: "LogValuer",
			log: func(logger *slog.Logger) {
				logger.Info("session", "session", sessionCredentials{token: secretValue})
			},
			path: []string{"session", "token"},
		},
		{
			name: "every field below a sensitive group",
			log: func(logger *slog.Logger) {
				logger.Info("config", slog.Group("secret", slog.String("value", secretValue)))
			},
			path: []string{"secret", "value"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			var output bytes.Buffer
			test.log(newTestLogger(t, logging.Options{
				Level: logging.LevelInfo, Format: logging.FormatJSON, Writer: &output,
			}))

			assertNoSecret(t, output.String())
			if got := lookupPath(t, output.Bytes(), test.path); got != logging.RedactedValue {
				t.Errorf("field %v = %v, want %q", test.path, got, logging.RedactedValue)
			}
		})
	}
}

func TestNewKeepsNonSensitiveAttributes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		key  string
	}{
		{name: "plain key", key: "user"},
		{name: "key containing a sensitive word", key: "tokenCount"},
		{name: "key with a sensitive suffix", key: "passwordHint"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			var output bytes.Buffer
			logWith(test.key, "visible-value")(newTestLogger(t, logging.Options{
				Level: logging.LevelInfo, Format: logging.FormatJSON, Writer: &output,
			}))

			if got := lookupPath(t, output.Bytes(), []string{test.key}); got != "visible-value" {
				t.Errorf("field %q = %v, want it unchanged", test.key, got)
			}
		})
	}
}

func TestNewRedactsInTextFormat(t *testing.T) {
	t.Parallel()

	var output bytes.Buffer
	logger := newTestLogger(t, logging.Options{
		Level: logging.LevelInfo, Format: logging.FormatText, Writer: &output,
	})
	logger.Info("login", "user", "alice", slog.Group("auth", slog.String("Token", secretValue)))

	assertNoSecret(t, output.String())
	if want := "auth.Token=" + logging.RedactedValue; !strings.Contains(output.String(), want) {
		t.Errorf("output %q does not contain %q", output.String(), want)
	}

	if !strings.Contains(output.String(), "user=alice") {
		t.Errorf("output %q lost the non-sensitive attribute", output.String())
	}
}

func logWith(key string, value any) func(logger *slog.Logger) {
	return func(logger *slog.Logger) {
		logger.Info("event", key, value)
	}
}

func assertNoSecret(t *testing.T, output string) {
	t.Helper()

	if strings.Contains(output, secretValue) {
		t.Errorf("secret leaked into log output: %s", output)
	}
}

// lookupPath decodes a single JSON record and follows the key path through nested objects.
func lookupPath(t *testing.T, line []byte, path []string) any {
	t.Helper()

	var current any
	if err := json.Unmarshal(line, &current); err != nil {
		t.Fatalf("output %q is not valid JSON: %v", line, err)
	}

	for _, key := range path {
		object, ok := current.(map[string]any)
		if !ok {
			t.Fatalf("path %v: %v is not an object", path, current)
		}

		current = object[key]
	}

	return current
}
