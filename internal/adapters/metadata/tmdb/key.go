package tmdb

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

const (
	apiKeyParam   = "api_key"
	redactedValue = "REDACTED"

	// readAccessTokenPrefix starts every TMDB v4 read access token (a JWT); such a key is
	// sent as a bearer header, a v3 key as the api_key parameter.
	readAccessTokenPrefix = "eyJ"

	firstPrintableASCII = '!'
	lastPrintableASCII  = '~'
)

// authorize adds the key to query or returns a header carrying it. A missing or malformed key
// fails with ErrInvalidKey and is never echoed: keys with spaces or control characters could
// otherwise smuggle header lines or query parameters.
func (c *Client) authorize(ctx context.Context, query url.Values) (http.Header, error) {
	if c.keys == nil {
		return nil, ErrInvalidKey
	}

	key, err := c.keys.APIKey(ctx)
	if err != nil {
		return nil, fmt.Errorf("read key: %w", err)
	}

	if !isPrintableASCII(key) {
		return nil, ErrInvalidKey
	}

	header := http.Header{"Accept": []string{"application/json"}}
	if IsReadAccessToken(key) {
		header.Set("Authorization", "Bearer "+key)

		return header, nil
	}

	query.Set(apiKeyParam, key)

	return header, nil
}

// IsReadAccessToken reports whether key is a TMDB v4 read access token, which travels as a
// bearer header; any other key is a v3 key sent as the api_key parameter.
func IsReadAccessToken(key string) bool {
	return strings.HasPrefix(key, readAccessTokenPrefix) && strings.Contains(key, ".")
}

// isPrintableASCII reports whether key is non-empty and holds only visible ASCII characters.
func isPrintableASCII(key string) bool {
	if key == "" {
		return false
	}

	for index := range len(key) {
		if key[index] < firstPrintableASCII || key[index] > lastPrintableASCII {
			return false
		}
	}

	return true
}
