// Package coverart lists album covers from the Cover Art Archive (ADR 0031 §1, §6). It asks
// for the images of a MusicBrainz release group (falling back to the release) and turns the
// approved front covers into artwork references for the artwork fetcher (M6-08). Provider JSON
// never leaves the package; requests carry nothing but the MusicBrainz ID. Local covers keep
// priority because enrichment merges provider artwork below them (ADR 0027 §8).
package coverart

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/magpeek/mivo/internal/adapters/netaccess/outbound"
	"github.com/magpeek/mivo/internal/app/enrich"
	"github.com/magpeek/mivo/internal/domain"
)

// DefaultBaseURL is the Cover Art Archive API endpoint (ADR 0031 §1).
const DefaultBaseURL = "https://coverartarchive.org"

const (
	apiHost = "coverartarchive.org"
	// redirectDomain covers the archive.org mirrors the API and its images redirect to.
	redirectDomain = ".archive.org"

	pathReleaseGroup = "/release-group/"
	pathRelease      = "/release/"
)

// errBadResponse reports a 2xx answer that is not the JSON the Cover Art Archive documents.
var errBadResponse = fmt.Errorf("cover art archive returned an unreadable answer: %w", domain.ErrUnavailable)

// AllowedHosts returns the hosts the outbound client must allow for this adapter.
func AllowedHosts() []string { return []string{apiHost} }

// RedirectHosts returns where the outbound client may follow a redirect to.
func RedirectHosts() []string { return []string{apiHost, redirectDomain} }

// HTTPGetter performs a hardened GET; *outbound.Client satisfies it and adds the brand
// User-Agent (ADR 0031 §5), the rate limit and the retries.
type HTTPGetter interface {
	Get(ctx context.Context, rawURL string, header http.Header) (*outbound.Response, error)
}

// Options configures a Client. BaseURL defaults to DefaultBaseURL; tests point it at a fake.
type Options struct {
	HTTP    HTTPGetter
	BaseURL string
}

// Client talks to the Cover Art Archive. It is safe for concurrent use.
type Client struct {
	http    HTTPGetter
	baseURL string
}

// New builds a Client from opts.
func New(opts Options) *Client {
	baseURL := opts.BaseURL
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}

	return &Client{http: opts.HTTP, baseURL: strings.TrimRight(baseURL, "/")}
}

// CoverRequest names the album whose covers are listed. At least one ID is required; the
// release group is asked first because its cover represents the album as a whole.
type CoverRequest struct {
	ReleaseGroupID string
	ReleaseID      string
}

// Covers returns the album's front covers, best first, as poster references with HTTPS
// coverartarchive.org URLs. An album without covers (404 for every ID) yields no references
// and no error. Errors wrap domain.ErrInvalid (malformed ID), domain.ErrRateLimited (429 or
// 503, which the archive sends when throttling) or domain.ErrUnavailable.
func (c *Client) Covers(ctx context.Context, request CoverRequest) ([]enrich.ArtworkRef, error) {
	paths, err := lookupPaths(request)
	if err != nil {
		return nil, err
	}

	for _, path := range paths {
		listing, found, err := c.fetchListing(ctx, path)
		if err != nil {
			return nil, err
		}

		if found {
			return frontCovers(listing), nil
		}
	}

	return nil, nil
}

// lookupPaths validates the IDs and returns the API paths to try in order.
func lookupPaths(request CoverRequest) ([]string, error) {
	var paths []string

	for _, candidate := range []struct{ prefix, id string }{
		{pathReleaseGroup, request.ReleaseGroupID},
		{pathRelease, request.ReleaseID},
	} {
		if candidate.id == "" {
			continue
		}

		id, ok := normalizedMBID(candidate.id)
		if !ok {
			return nil, fmt.Errorf("cover art archive id %q: %w", candidate.id, domain.ErrInvalid)
		}

		paths = append(paths, candidate.prefix+id)
	}

	if len(paths) == 0 {
		return nil, fmt.Errorf("cover art archive request without id: %w", domain.ErrInvalid)
	}

	return paths, nil
}

// fetchListing GETs path and decodes it; found is false for a 404.
func (c *Client) fetchListing(ctx context.Context, path string) (listing imageListing, found bool, err error) {
	header := http.Header{"Accept": []string{"application/json"}}

	response, getErr := c.http.Get(ctx, c.baseURL+path, header)
	if response != nil && isThrottled(response.StatusCode) {
		return imageListing{}, false, fmt.Errorf("cover art archive GET %s: status %d: %w",
			path, response.StatusCode, domain.ErrRateLimited)
	}

	if getErr != nil {
		return imageListing{}, false, fmt.Errorf("cover art archive GET %s: %w", path, getErr)
	}

	switch {
	case response.StatusCode == http.StatusNotFound:
		return imageListing{}, false, nil
	case response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices:
		return imageListing{}, false, fmt.Errorf("cover art archive GET %s: status %d: %w",
			path, response.StatusCode, domain.ErrUnavailable)
	}

	if err := json.Unmarshal(response.Body, &listing); err != nil {
		return imageListing{}, false, fmt.Errorf("cover art archive GET %s: %w", path, errors.Join(errBadResponse, err))
	}

	return listing, true, nil
}

// isThrottled reports the statuses the archive answers when a client asks too often; it
// uses 503 for that as well as 429.
func isThrottled(status int) bool {
	return status == http.StatusTooManyRequests || status == http.StatusServiceUnavailable
}
