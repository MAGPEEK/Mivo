// Package musicbrainz implements the enrich music lookup against the MusicBrainz web service
// v2 (ADR 0031). Albums are matched as release groups, artists as artists. Provider JSON
// never leaves the package. Every request goes through the shared outbound client, asks for
// JSON and carries only tag-derived names, a year and MusicBrainz IDs; no key is needed.
package musicbrainz

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/magpeek/mivo/internal/adapters/netaccess/outbound"
	"github.com/magpeek/mivo/internal/app/enrich"
	"github.com/magpeek/mivo/internal/domain"
)

var _ enrich.MusicLookup = (*Client)(nil)

// Endpoints and limits of MusicBrainz (ADR 0031 §1 and §7).
const (
	DefaultBaseURL = "https://musicbrainz.org/ws/2"

	// MinRequestInterval is the MusicBrainz rate limit: one request per second per client.
	MinRequestInterval = time.Second

	apiHost        = "musicbrainz.org"
	requestsPerSec = 1
	requestBurst   = 1

	paramFormat = "fmt"
	formatJSON  = "json"
	headerRetry = "Retry-After"
)

// errBadResponse reports a 2xx answer that is not the JSON MusicBrainz documents.
var errBadResponse = fmt.Errorf("musicbrainz returned an unreadable answer: %w", domain.ErrUnavailable)

// AllowedHosts returns the hosts the outbound client must allow for this adapter; MusicBrainz
// does not redirect, so they are the redirect hosts too.
func AllowedHosts() []string {
	return []string{apiHost}
}

// HostRates returns the outbound limiter rate MusicBrainz demands. The adapter paces its own
// requests as well, so the limit holds even when the wiring forgets this.
func HostRates() map[string]outbound.Rate {
	return map[string]outbound.Rate{apiHost: {PerSecond: requestsPerSec, Burst: requestBurst}}
}

// HTTPGetter performs a hardened GET; *outbound.Client satisfies it and sets the User-Agent
// with product name, version and contact URL that MusicBrainz requires.
type HTTPGetter interface {
	Get(ctx context.Context, rawURL string, header http.Header) (*outbound.Response, error)
}

// Options configures a Client. BaseURL defaults to the MusicBrainz web service; Now and
// Sleep are the clock of the request pacer and default to the real one.
type Options struct {
	HTTP    HTTPGetter
	BaseURL string
	Now     func() time.Time
	Sleep   func(ctx context.Context, pause time.Duration) error
}

// Client talks to MusicBrainz and implements enrich.MusicLookup. It is safe for concurrent
// use; concurrent calls share one pacer.
type Client struct {
	http    HTTPGetter
	baseURL string
	pacer   *pacer
}

// New builds a Client from opts.
func New(opts Options) *Client {
	baseURL := opts.BaseURL
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}

	return &Client{
		http:    opts.HTTP,
		baseURL: strings.TrimRight(baseURL, "/"),
		pacer:   newPacer(MinRequestInterval, opts.Now, opts.Sleep),
	}
}

// getJSON waits for the pacer, fetches path with query as JSON and decodes it into out.
func (c *Client) getJSON(ctx context.Context, path string, query url.Values, out any) error {
	if err := c.pacer.wait(ctx); err != nil {
		return fmt.Errorf("musicbrainz GET %s: wait for rate limit: %w", path, err)
	}

	query.Set(paramFormat, formatJSON)

	response, err := c.http.Get(ctx, c.baseURL+path+"?"+query.Encode(), http.Header{"Accept": {"application/json"}})
	if err != nil {
		return fmt.Errorf("musicbrainz GET %s: %w", path, transportError(response, err))
	}

	if err := statusError(response); err != nil {
		return fmt.Errorf("musicbrainz GET %s: %w", path, err)
	}

	if err := json.Unmarshal(response.Body, out); err != nil {
		return fmt.Errorf("musicbrainz GET %s: %w", path, errBadResponse)
	}

	return nil
}

// transportError maps the outbound client's verdict after its retries. MusicBrainz answers
// rate limiting with 503 and Retry-After, which the outbound client reports as unavailable;
// that case becomes ErrRateLimited alone so callers back off instead of giving up.
func transportError(response *outbound.Response, err error) error {
	if response != nil && isRateLimited(response) {
		return fmt.Errorf("status %d (%s): %w", response.StatusCode, err.Error(), domain.ErrRateLimited)
	}

	return err
}

func isRateLimited(response *outbound.Response) bool {
	switch response.StatusCode {
	case http.StatusTooManyRequests:
		return true
	case http.StatusServiceUnavailable:
		return response.Header.Get(headerRetry) != ""
	default:
		return false
	}
}

// statusError maps a non-2xx status that the outbound client returned as data.
func statusError(response *outbound.Response) error {
	status := response.StatusCode

	switch {
	case status >= http.StatusOK && status < http.StatusMultipleChoices:
		return nil
	case isRateLimited(response):
		return fmt.Errorf("status %d: %w", status, domain.ErrRateLimited)
	case status == http.StatusNotFound:
		return fmt.Errorf("status %d: %w", status, domain.ErrNotFound)
	default:
		return fmt.Errorf("status %d: %w", status, domain.ErrUnavailable)
	}
}
