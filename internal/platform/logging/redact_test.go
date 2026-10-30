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

// opaqueCredentials resolves to a group whose own keys look harmless, so only the
// sensitive key it is logged under can trigger redaction.
type opaqueCredentials struct{ value string }

func (credentials opaqueCredentials) LogValue() slog.Value {
	return slog.GroupValue(slog.String("value", credentials.value), slog.String("kind", "bearer"))
}

func TestNewRedactsSensitiveKeys(t *testing.T) {
	t.Parallel()

	keys := []string{
		// The words from sensitiveKeySuffixes in their plain form.
		"password", "passphrase", "secret", "token", "apikey", "authorization",
		"cookie", "privatekey", "sessionid",
		// Matching is case-insensitive.
		"PASSWORD", "ApiKey", "Authorization",
		// The separators '_', '-' and '.' are ignored.
		"api_key", "x-api-key", "API.KEY", "Set-Cookie", "Proxy-Authorization",
		"access_token", "refresh_token", "client_secret", "jwt.secret", "private_key", "session_id",
		// Prefixed names end with a sensitive word.
		"dbPassword", "githubToken", "accessToken", "refreshToken", "clientSecret", "jwtSecret",
		"setCookie", "proxyAuthorization", "privateKey", "X-Auth-Token",
	}

	for _, key := range keys {
		t.Run(key, func(t *testing.T) {
			t.Parallel()

			var output bytes.Buffer
			logWith(key, secretValue)(newJSONLogger(t, &output))

			assertRedacted(t, output.Bytes(), []string{key})
		})
	}
}

func TestNewRedactsSensitiveStructures(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		log  func(logger *slog.Logger)
		// path is the key path of the redacted field in the decoded JSON record.
		path []string
	}{
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
			name: "logger WithGroup with a sensitive name",
			log: func(logger *slog.Logger) {
				logger.WithGroup("authorization").Info("call", "scheme", secretValue)
			},
			path: []string{"authorization", "scheme"},
		},
		{
			name: "logger With",
			log: func(logger *slog.Logger) {
				logger.With("apikey", secretValue).Info("call")
			},
			path: []string{"apikey"},
		},
		{
			name: "LogValuer with a sensitive field",
			log: func(logger *slog.Logger) {
				logger.Info("session", "session", sessionCredentials{token: secretValue})
			},
			path: []string{"session", "token"},
		},
		{
			name: "LogValuer group under a sensitive key",
			log: func(logger *slog.Logger) {
				logger.Info("call", "client_secret", opaqueCredentials{value: secretValue})
			},
			path: []string{"client_secret", "value"},
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
			test.log(newJSONLogger(t, &output))

			assertRedacted(t, output.Bytes(), test.path)
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
		{name: "sensitive word followed by a qualifier", key: "tokenCount"},
		{name: "qualifier after a separator", key: "token_count"},
		{name: "password hint", key: "passwordHint"},
		{name: "sensitive word inside a longer word", key: "secretary"},
		{name: "public key", key: "publicKey"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			var output bytes.Buffer
			logWith(test.key, "visible-value")(newJSONLogger(t, &output))

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

func newJSONLogger(t *testing.T, output *bytes.Buffer) *slog.Logger {
	t.Helper()

	return newTestLogger(t, logging.Options{
		Level: logging.LevelInfo, Format: logging.FormatJSON, Writer: output,
	})
}

// assertRedacted checks that the secret is gone and that the field at path carries the marker.
func assertRedacted(t *testing.T, record []byte, path []string) {
	t.Helper()

	assertNoSecret(t, string(record))
	if got := lookupPath(t, record, path); got != logging.RedactedValue {
		t.Errorf("field %v = %v, want %q", path, got, logging.RedactedValue)
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
