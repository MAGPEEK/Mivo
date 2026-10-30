package logging

import (
	"log/slog"
	"strings"
)

// RedactedValue replaces the value of every sensitive attribute in log output.
const RedactedValue = "[REDACTED]"

// Sensitive attribute keys. A key matches case-insensitively, so the constants are lower
// case: "apiKey", "APIKEY" and "Authorization" are all redacted. Only whole keys match;
// "passwordHint" or "tokenCount" stay visible because they carry no credential.
// Extend this set (and isSensitiveKey) when a new credential-bearing key is introduced.
const (
	sensitiveKeyPassword      = "password"
	sensitiveKeyToken         = "token"
	sensitiveKeySecret        = "secret"
	sensitiveKeyAPIKey        = "apikey"
	sensitiveKeyAuthorization = "authorization"
)

// redactSensitive is the slog.HandlerOptions.ReplaceAttr hook. slog calls it for every
// leaf attribute, including those inside groups, after LogValuer values are resolved, but
// never for a group itself. A leaf is therefore also redacted when any enclosing group
// has a sensitive key, so slog.Group("secret", ...) cannot leak its contents.
func redactSensitive(groups []string, attr slog.Attr) slog.Attr {
	if isSensitiveKey(attr.Key) || anySensitiveKey(groups) {
		return slog.String(attr.Key, RedactedValue)
	}

	return attr
}

func anySensitiveKey(keys []string) bool {
	for _, key := range keys {
		if isSensitiveKey(key) {
			return true
		}
	}

	return false
}

func isSensitiveKey(key string) bool {
	switch strings.ToLower(key) {
	case sensitiveKeyPassword,
		sensitiveKeyToken,
		sensitiveKeySecret,
		sensitiveKeyAPIKey,
		sensitiveKeyAuthorization:
		return true
	default:
		return false
	}
}
