package logging

import (
	"log/slog"
	"strings"
	"unicode"
)

// RedactedValue replaces the value of every sensitive attribute in log output.
const RedactedValue = "[REDACTED]"

// sensitiveKeySuffixes returns the normalised words that mark a key as credential-bearing.
// It is the single place to extend when a new kind of secret appears. Matching is by
// suffix (see isSensitiveKey), so derived names need no entry of their own: "token" also
// covers accesstoken, refreshtoken and xauthtoken; "secret" covers clientsecret and
// jwtsecret; "authorization" covers proxyauthorization; "cookie" covers setcookie;
// "apikey" covers xapikey.
func sensitiveKeySuffixes() []string {
	return []string{
		"password",
		"passphrase",
		"secret",
		"token",
		"apikey",
		"authorization",
		"cookie",
		"privatekey",
		"sessionid",
	}
}

// redactSensitive is the slog.HandlerOptions.ReplaceAttr hook. slog calls it for every
// leaf attribute, including those inside groups, after LogValuer values are resolved, but
// never for a group itself. A leaf is therefore also redacted when any enclosing group
// (slog.Group, Logger.WithGroup or a LogValuer resolving to a group) has a sensitive key,
// so a group named "authorization" cannot leak its contents.
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

// isSensitiveKey reports whether the normalised key ends with one of the sensitive words.
// Suffix matching catches prefixed names such as "dbPassword" or "x-api-key", while a
// sensitive word followed by a qualifier ("tokenCount", "passwordHint") stays visible
// because such keys describe a credential rather than hold one.
func isSensitiveKey(key string) bool {
	normalised := normaliseKey(key)
	for _, suffix := range sensitiveKeySuffixes() {
		if strings.HasSuffix(normalised, suffix) {
			return true
		}
	}

	return false
}

// normaliseKey lower-cases key and drops the separators '_', '-' and '.', so that
// "API_KEY", "x-api-key", "Set-Cookie" and "apiKey" compare equal to their plain forms.
func normaliseKey(key string) string {
	return strings.Map(func(character rune) rune {
		switch character {
		case '_', '-', '.':
			return -1
		default:
			return unicode.ToLower(character)
		}
	}, key)
}
