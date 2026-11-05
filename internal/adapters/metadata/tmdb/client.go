// Package tmdb implements the enrich movie and show lookups against the TMDB v3 API (ADR 0031).
// Provider JSON never leaves the package: answers are mapped to match candidates, metadata
// records and artwork references. Every request goes through the shared outbound client and
// carries only a title, a year, a language, season numbers, provider IDs and the key.
package tmdb

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/magpeek/mivo/internal/adapters/netaccess/outbound"
	"github.com/magpeek/mivo/internal/domain"
)

// Default endpoints of TMDB (ADR 0031 §1).
const (
	DefaultBaseURL      = "https://api.themoviedb.org/3"
	DefaultImageBaseURL = "https://image.tmdb.org/t/p"

	apiHost   = "api.themoviedb.org"
	imageHost = "image.tmdb.org"
)

// ErrInvalidKey reports that TMDB refused the key (HTTP 401) or that none is configured. It
// wraps domain.ErrUnavailable so callers treat TMDB as unusable; errors.Is(err, ErrInvalidKey)
// is the "invalid key" flag the settings screen shows.
var ErrInvalidKey = fmt.Errorf("tmdb api key missing or invalid: %w", domain.ErrUnavailable)

// errBadResponse reports a 2xx answer that is not the JSON TMDB documents.
var errBadResponse = fmt.Errorf("tmdb returned an unreadable answer: %w", domain.ErrUnavailable)

// AllowedHosts returns the hosts the outbound client must allow for this adapter.
func AllowedHosts() []string {
	return []string{apiHost, imageHost}
}

// KeyProvider supplies the key to use: the owner's own key or the bundled one. It is asked
// on every request because the own key can change at runtime in Settings. An empty key
// means TMDB is not usable.
type KeyProvider interface {
	// APIKey returns the current key; it must never be logged.
	APIKey(ctx context.Context) (string, error)
}

// HTTPGetter performs a hardened GET; *outbound.Client satisfies it.
type HTTPGetter interface {
	Get(ctx context.Context, rawURL string, header http.Header) (*outbound.Response, error)
}

// Options configures a Client. BaseURL and ImageBaseURL default to the TMDB endpoints; tests
// and E2E point them at a fake.
type Options struct {
	HTTP         HTTPGetter
	Keys         KeyProvider
	BaseURL      string
	ImageBaseURL string
}

// Client talks to TMDB. It is safe for concurrent use; Movies and Shows expose the ports.
type Client struct {
	http         HTTPGetter
	keys         KeyProvider
	baseURL      string
	imageBaseURL string
}

// New builds a Client from opts.
func New(opts Options) *Client {
	return &Client{
		http:         opts.HTTP,
		keys:         opts.Keys,
		baseURL:      strings.TrimRight(orDefault(opts.BaseURL, DefaultBaseURL), "/"),
		imageBaseURL: strings.TrimRight(orDefault(opts.ImageBaseURL, DefaultImageBaseURL), "/"),
	}
}

func orDefault(value, fallback string) string {
	if value == "" {
		return fallback
	}

	return value
}

// getJSON fetches path with query and decodes the answer into out. The key is added here and
// nowhere else; errors name the path only, never the URL with the key.
func (c *Client) getJSON(ctx context.Context, path string, query url.Values, out any) error {
	header, err := c.authorize(ctx, query)
	if err != nil {
		return fmt.Errorf("tmdb GET %s: %w", path, err)
	}

	response, err := c.http.Get(ctx, c.baseURL+path+"?"+query.Encode(), header)
	if err != nil {
		return fmt.Errorf("tmdb GET %s: %w", path, redactKey(err, query))
	}

	if err := statusError(response.StatusCode); err != nil {
		return fmt.Errorf("tmdb GET %s: %w", path, err)
	}

	if err := json.Unmarshal(response.Body, out); err != nil {
		return fmt.Errorf("tmdb GET %s: %w", path, errBadResponse)
	}

	return nil
}

// statusError maps a non-2xx status to a domain error. 429 and 5xx arrive as errors from the
// outbound client after its retries; they are mapped here too in case retries are off.
func statusError(status int) error {
	switch {
	case status >= http.StatusOK && status < http.StatusMultipleChoices:
		return nil
	case status == http.StatusUnauthorized:
		return ErrInvalidKey
	case status == http.StatusNotFound:
		return fmt.Errorf("status %d: %w", status, domain.ErrNotFound)
	case status == http.StatusTooManyRequests:
		return fmt.Errorf("status %d: %w", status, domain.ErrRateLimited)
	default:
		return fmt.Errorf("status %d: %w", status, domain.ErrUnavailable)
	}
}

// redactKey is a second line of defence: the outbound client already masks api_key in URLs,
// but an error that still carried the raw key must not leave the adapter.
func redactKey(err error, query url.Values) error {
	key := query.Get(apiKeyParam)
	if key == "" || !strings.Contains(err.Error(), key) {
		return err
	}

	var sentinels []error

	for _, sentinel := range keptSentinels() {
		if errors.Is(err, sentinel) {
			sentinels = append(sentinels, sentinel)
		}
	}

	return &redactedError{message: strings.ReplaceAll(err.Error(), key, redactedValue), sentinels: sentinels}
}

// keptSentinels are the errors callers branch on; a redacted error keeps only these.
func keptSentinels() []error {
	return []error{
		domain.ErrUnavailable, domain.ErrRateLimited, domain.ErrNotFound,
		context.Canceled, context.DeadlineExceeded,
	}
}

// redactedError is a masked message with the matching sentinels. It deliberately drops the
// original error: anything in its chain may still print the key.
type redactedError struct {
	message   string
	sentinels []error
}

func (e *redactedError) Error() string { return e.message }

func (e *redactedError) Unwrap() []error { return e.sentinels }
