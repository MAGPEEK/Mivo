package outbound

import (
	"net/url"
	"strings"
)

const redactedValue = "REDACTED"

// isSecretParam reports whether a query parameter name carries a credential.
func isSecretParam(name string) bool {
	switch strings.ToLower(name) {
	case "api_key", "apikey", "key", "token", "access_token",
		"auth", "secret", "password", "client_secret", "signature":
		return true
	}

	return false
}

// RedactURL returns rawURL with credentials and secret query values masked, safe for errors
// and logs. Unparseable input is replaced entirely rather than risk leaking a key.
func RedactURL(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return "<invalid url>"
	}

	return redactParsed(parsed)
}

func redactParsed(parsed *url.URL) string {
	clean := *parsed
	clean.User = nil
	clean.Fragment = ""

	query := clean.Query()
	for name := range query {
		if isSecretParam(name) {
			query.Set(name, redactedValue)
		}
	}

	clean.RawQuery = query.Encode()

	return clean.String()
}
