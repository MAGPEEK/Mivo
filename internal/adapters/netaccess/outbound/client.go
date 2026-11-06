package outbound

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/platform/brand"
)

var (
	// ErrBodyTooLarge reports a response body above Options.MaxBodyBytes.
	ErrBodyTooLarge = fmt.Errorf("response body too large: %w", domain.ErrUnsupported)
	// ErrInsecureURL reports a non-HTTPS URL while AllowInsecureLoopback is off.
	ErrInsecureURL = fmt.Errorf("only https urls are allowed: %w", domain.ErrInvalid)
	// ErrTooManyRedirects reports a redirect chain longer than the configured limit.
	ErrTooManyRedirects = fmt.Errorf("too many redirects: %w", domain.ErrUnavailable)
)

// Response is a fully read, size-capped HTTP response. Non-2xx statuses other than the
// retried ones (429, 502, 503, 504) are returned as data so adapters can map 404 and friends.
type Response struct {
	StatusCode int
	Header     http.Header
	Body       []byte
}

// Client is the shared hardened outbound HTTP client. It is safe for concurrent use.
type Client struct {
	opts      Options
	http      *http.Client
	limiter   *limiter
	userAgent string
	allowed   hostMatcher
	// redirectHosts limits where a redirect may lead; empty keeps redirects on the host.
	redirectHosts hostMatcher

	mu       sync.Mutex
	breakers map[string]*breaker
	lastUsed map[string]time.Time
}

// New builds a Client from opts.
func New(opts Options) *Client {
	opts = opts.withDefaults()
	client := &Client{
		opts:          opts,
		userAgent:     fmt.Sprintf("%s/%s (+%s)", brand.DisplayName, opts.Version, brand.RepositoryURL),
		breakers:      map[string]*breaker{},
		lastUsed:      map[string]time.Time{},
		allowed:       newHostMatcher(opts.AllowedHosts),
		redirectHosts: newHostMatcher(opts.RedirectHosts),
		limiter: &limiter{
			buckets:  map[string]*bucket{},
			defaults: opts.DefaultRate,
			perHost:  normalizeRates(opts.HostRates),
			now:      opts.Now,
			sleep:    opts.Sleep,
		},
	}

	dialer := &guardedDialer{
		resolver:      opts.Resolver,
		allowLoopback: opts.AllowInsecureLoopback,
	}
	// No Proxy: a proxy would receive the unchecked destination and bypass the SSRF guard.
	transport := &http.Transport{
		DialContext:           dialer.dialContext,
		TLSHandshakeTimeout:   opts.Timeout,
		ResponseHeaderTimeout: opts.Timeout,
		ForceAttemptHTTP2:     true,
		MaxIdleConnsPerHost:   idleConnsPerHost,
		IdleConnTimeout:       time.Minute,
	}
	client.http = &http.Client{
		Transport:     transport,
		Timeout:       opts.Timeout,
		CheckRedirect: client.checkRedirect,
		// Jar stays nil: no cookies are ever stored.
	}

	return client
}

// UserAgent returns the User-Agent header value sent with every request.
func (c *Client) UserAgent() string { return c.userAgent }

// Get fetches rawURL with an optional header set.
func (c *Client) Get(ctx context.Context, rawURL string, header http.Header) (*Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, fmt.Errorf("build request %s: %w", RedactURL(rawURL), domain.ErrInvalid)
	}

	for name, values := range header {
		req.Header[name] = slices.Clone(values)
	}

	return c.Do(ctx, req)
}

// Do sends req through limiter, breaker and retry. The request must be replayable (no body
// or a GetBody). Only GET and HEAD are retried; other methods get a single attempt. The
// whole call, retries included, is bounded by Options.TotalTimeout. Errors never contain
// secret query values.
func (c *Client) Do(ctx context.Context, req *http.Request) (*Response, error) {
	if err := c.checkScheme(req.URL); err != nil {
		return nil, fmt.Errorf("%s %s: %w", req.Method, redactParsed(req.URL), err)
	}

	host, guard, err := c.admit(req.URL.Hostname())
	if err != nil {
		return nil, fmt.Errorf("%s %s: %w", req.Method, redactParsed(req.URL), err)
	}

	if !guard.allow(c.opts.Now()) {
		return nil, fmt.Errorf("%s %s: circuit open: %w", req.Method, redactParsed(req.URL), domain.ErrUnavailable)
	}

	totalCtx, cancel := context.WithTimeout(ctx, c.opts.TotalTimeout)
	defer cancel()

	resp, err := c.attempts(totalCtx, req, host)
	if err != nil && isCallerError(err) && ctx.Err() == nil {
		// Only the overall deadline ran out: a slow service, not a cancelling caller.
		// Unwrapping drops the callerError marker on purpose, so recordOutcome sees a
		// service failure and the breaker counts it instead of treating it as neutral.
		err = fmt.Errorf("%w: %w", domain.ErrUnavailable, errors.Unwrap(err))
	}

	c.recordOutcome(guard, resp, err)

	return resp, err
}

func (c *Client) recordOutcome(guard *breaker, resp *Response, err error) {
	switch {
	case isCallerError(err) || isPolicyError(err):
		guard.abandon()
	case isServiceFailure(resp, err):
		guard.failure(c.opts.Now())
	default:
		guard.success()
	}
}

// admit normalises host, applies the allow-list and returns the host's breaker. The number of
// tracked hosts is bounded so a stream of user-supplied URLs cannot grow the maps forever; when
// full, the least recently used idle host makes room, and only all-busy maps refuse new hosts.
func (c *Client) admit(rawHost string) (string, *breaker, error) {
	host, ok := normalizeHost(rawHost)
	if !ok || !c.allowed.allows(host) {
		return "", nil, ErrHostNotAllowed
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	guard, known := c.breakers[host]

	now := c.opts.Now()
	if known {
		c.lastUsed[host] = now

		return host, guard, nil
	}

	if len(c.breakers) >= MaxTrackedHosts && !c.evictIdleLocked(now) {
		return "", nil, fmt.Errorf("%w: too many distinct hosts", ErrHostNotAllowed)
	}

	guard = &breaker{threshold: c.opts.BreakerThreshold, cooldown: c.opts.BreakerCooldown}
	c.breakers[host] = guard
	c.lastUsed[host] = now

	return host, guard, nil
}

// evictIdleLocked drops the least recently used host whose state carries no information (a
// closed breaker without failures and a full token bucket), so forgetting it changes nothing.
// It reports false when every tracked host is still busy. The caller holds c.mu.
func (c *Client) evictIdleLocked(now time.Time) bool {
	victim := ""

	for host, guard := range c.breakers {
		if !guard.idle() || !c.limiter.idle(host, now) {
			continue
		}

		if victim == "" || c.lastUsed[host].Before(c.lastUsed[victim]) {
			victim = host
		}
	}

	if victim == "" {
		return false
	}

	delete(c.breakers, victim)
	delete(c.lastUsed, victim)
	c.limiter.forget(victim)

	return true
}

func (c *Client) checkScheme(target *url.URL) error {
	if target.Scheme == "https" || (c.opts.AllowInsecureLoopback && target.Scheme == "http") {
		return nil
	}

	return ErrInsecureURL
}

func (c *Client) checkRedirect(req *http.Request, via []*http.Request) error {
	if len(via) > c.opts.MaxRedirects {
		return ErrTooManyRedirects
	}

	if err := c.checkScheme(req.URL); err != nil {
		return err
	}

	// net/http copies the ORIGINAL request's headers onto every hop before calling this, so
	// the comparison must be against the first request, not the previous hop: in A -> B -> B
	// the second hop to B would otherwise receive A's credentials again.
	origin, current := hostKey(via[0].URL), hostKey(req.URL)
	if current == origin {
		return nil
	}

	// Without RedirectHosts a redirect may not leave the original host at all: the safe
	// default, so a client wired without the list cannot be steered elsewhere.
	if !c.redirectHosts.enabled || !c.redirectHosts.allows(normalizedHostname(req.URL)) {
		return ErrHostNotAllowed
	}

	stripUnsafeHeaders(req.Header)

	return nil
}

// hostKey identifies an origin by normalised host name and port.
func hostKey(target *url.URL) string {
	return net.JoinHostPort(normalizedHostname(target), target.Port())
}

func normalizedHostname(target *url.URL) string {
	host, ok := normalizeHost(target.Hostname())
	if !ok {
		// Non-ASCII hosts never match an allow-list; keep them distinct from every origin.
		return strings.ToLower(target.Hostname())
	}

	return host
}

// stripUnsafeHeaders keeps only headers known to carry no credentials, so custom API-key
// headers do not follow a redirect to another host (net/http only drops the standard ones).
func stripUnsafeHeaders(header http.Header) {
	for name := range header {
		switch http.CanonicalHeaderKey(name) {
		case "Accept", "Accept-Encoding", "Accept-Language", "User-Agent",
			"If-None-Match", "If-Modified-Since", "Range":
		default:
			delete(header, name)
		}
	}
}

// fetchOnce performs a single attempt and reads the capped body.
func (c *Client) fetchOnce(ctx context.Context, req *http.Request) (*Response, error) {
	attempt, err := cloneForAttempt(ctx, req)
	if err != nil {
		return nil, err
	}

	if attempt.Header.Get("User-Agent") == "" {
		attempt.Header.Set("User-Agent", c.userAgent)
	}

	httpResp, err := c.http.Do(attempt)
	if err != nil {
		return nil, fmt.Errorf("%s %s: %w", req.Method, redactParsed(req.URL), unwrapURLError(err))
	}

	defer func() { _ = httpResp.Body.Close() }()

	body, err := readCapped(httpResp.Body, c.opts.MaxBodyBytes)
	if err != nil {
		return nil, fmt.Errorf("%s %s: %w", req.Method, redactParsed(req.URL), err)
	}

	return &Response{StatusCode: httpResp.StatusCode, Header: httpResp.Header.Clone(), Body: body}, nil
}

func cloneForAttempt(ctx context.Context, req *http.Request) (*http.Request, error) {
	attempt := req.Clone(ctx)

	if req.Body != nil && req.Body != http.NoBody {
		if req.GetBody == nil {
			return nil, fmt.Errorf("request body is not replayable: %w", domain.ErrInvalid)
		}

		body, err := req.GetBody()
		if err != nil {
			return nil, fmt.Errorf("rewind request body: %w", err)
		}

		attempt.Body = body
	}

	return attempt, nil
}

func readCapped(body io.Reader, limit int64) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(body, limit+1))
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}

	if int64(len(data)) > limit {
		return nil, ErrBodyTooLarge
	}

	return data, nil
}

// unwrapURLError drops the *url.Error wrapper, whose message repeats the unredacted URL.
func unwrapURLError(err error) error {
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		return urlErr.Err
	}

	return err
}
