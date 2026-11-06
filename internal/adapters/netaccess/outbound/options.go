package outbound

import (
	"context"
	"math/rand/v2"
	"net"
	"time"
)

// Defaults applied by New for zero Options fields.
const (
	DefaultTimeout          = 15 * time.Second
	DefaultTotalTimeout     = time.Minute
	DefaultMaxBodyBytes     = 15 << 20 // matches the image size cap of ADR 0031
	DefaultMaxAttempts      = 3
	DefaultBaseBackoff      = 500 * time.Millisecond
	DefaultMaxBackoff       = 30 * time.Second
	DefaultMaxRetryAfter    = 2 * time.Minute
	DefaultBreakerThreshold = 5
	DefaultBreakerCooldown  = 5 * time.Minute

	// MaxRedirects is the hard redirect limit; Options may only lower it.
	MaxRedirects = 3

	// MaxTrackedHosts bounds the distinct hosts the limiter and breaker keep state for.
	MaxTrackedHosts = 256

	defaultVersion = "dev"

	idleConnsPerHost = 2
	jitterDivisor    = 2
	maxBackoffShift  = 30
)

// Options configures a Client. Zero values select the Default* constants; the rate limiter
// is off unless DefaultRate or HostRates say otherwise.
type Options struct {
	// Version is the product version shown in the User-Agent.
	Version string
	// Timeout bounds one attempt, including reading the body.
	Timeout time.Duration
	// TotalTimeout bounds one call across all attempts and backoff pauses.
	TotalTimeout time.Duration
	// AllowedHosts restricts requests to exact host names or ".domain" suffixes (matched
	// case-insensitively, ASCII only); empty allows any public host. Provider adapters
	// (M6-07/09/10) must set it from the ADR 0031 §1 list. Only the first hop is checked:
	// redirects (for example coverartarchive.org to archive.org) are limited by the SSRF
	// guard, the redirect cap and HTTPS only.
	AllowedHosts []string
	// RedirectHosts are the exact hosts or ".domain" suffixes a redirect may lead to when it
	// leaves the original host (ADR 0031 §1, for example ".archive.org" for Cover Art
	// Archive). Empty means a redirect must stay on the original host.
	RedirectHosts []string
	// MaxBodyBytes caps a response body; larger bodies fail with ErrBodyTooLarge.
	MaxBodyBytes int64
	// MaxRedirects is lowered to at most MaxRedirects; 0 selects MaxRedirects.
	MaxRedirects int
	// DefaultRate limits every host without an entry in HostRates.
	DefaultRate Rate
	// HostRates overrides the rate per host name.
	HostRates map[string]Rate
	// MaxAttempts is the total number of tries per request (1 disables retries).
	MaxAttempts int
	// BaseBackoff and MaxBackoff bound the exponential delay between attempts.
	BaseBackoff time.Duration
	MaxBackoff  time.Duration
	// MaxRetryAfter is the longest server-requested pause that is honoured; a longer
	// Retry-After ends retrying immediately.
	MaxRetryAfter time.Duration
	// BreakerThreshold consecutive failed requests open the per-host breaker for
	// BreakerCooldown.
	BreakerThreshold int
	BreakerCooldown  time.Duration

	// AllowInsecureLoopback permits plain HTTP and loopback addresses. It exists for tests
	// against httptest servers and must never be set in production wiring.
	AllowInsecureLoopback bool

	// Seams for tests; nil selects the real implementation.
	Resolver Resolver
	Now      func() time.Time
	Sleep    func(ctx context.Context, d time.Duration) error
	Jitter   func(d time.Duration) time.Duration
}

func (o Options) withDefaults() Options {
	o.Version = orDefault(o.Version, defaultVersion)
	o.Timeout = orDefault(o.Timeout, DefaultTimeout)
	o.TotalTimeout = orDefault(o.TotalTimeout, DefaultTotalTimeout)
	o.MaxBodyBytes = orDefault(o.MaxBodyBytes, DefaultMaxBodyBytes)
	o.MaxAttempts = orDefault(o.MaxAttempts, DefaultMaxAttempts)
	o.BaseBackoff = orDefault(o.BaseBackoff, DefaultBaseBackoff)
	o.MaxBackoff = orDefault(o.MaxBackoff, DefaultMaxBackoff)
	o.MaxRetryAfter = orDefault(o.MaxRetryAfter, DefaultMaxRetryAfter)
	o.BreakerThreshold = orDefault(o.BreakerThreshold, DefaultBreakerThreshold)
	o.BreakerCooldown = orDefault(o.BreakerCooldown, DefaultBreakerCooldown)

	if o.MaxRedirects <= 0 || o.MaxRedirects > MaxRedirects {
		o.MaxRedirects = MaxRedirects
	}

	if o.Resolver == nil {
		o.Resolver = net.DefaultResolver
	}

	if o.Now == nil {
		o.Now = time.Now
	}

	if o.Sleep == nil {
		o.Sleep = realSleep
	}

	if o.Jitter == nil {
		o.Jitter = halfJitter
	}

	return o
}

func orDefault[T comparable](value, fallback T) T {
	var zero T
	if value == zero {
		return fallback
	}

	return value
}

func realSleep(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// halfJitter returns a random duration in [d/2, d] so concurrent clients do not retry in
// lock step. Retry timing is not security relevant, hence math/rand.
func halfJitter(d time.Duration) time.Duration {
	half := d / jitterDivisor
	if half <= 0 {
		return d
	}

	return half + rand.N(half+1) //nolint:gosec // jitter, not a secret
}
