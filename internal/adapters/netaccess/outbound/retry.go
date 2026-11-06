package outbound

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/magpeek/mivo/internal/domain"
)

// attempts runs the rate-limited retry loop and maps an exhausted retryable outcome to
// ErrRateLimited or ErrUnavailable.
func (c *Client) attempts(ctx context.Context, req *http.Request, host string) (*Response, error) {
	for attempt := 1; ; attempt++ {
		if err := c.limiter.wait(ctx, host); err != nil {
			return nil, callerError{fmt.Errorf("wait for rate limit of %s: %w", host, err)}
		}

		resp, err := c.fetchOnce(ctx, req)
		if err != nil && ctx.Err() != nil {
			return nil, callerError{fmt.Errorf("%s %s: %w", req.Method, redactParsed(req.URL), ctx.Err())}
		}

		if !isServiceFailure(resp, err) {
			return resp, err
		}

		delay, retry := c.retryDelay(resp, attempt)
		if !retry || attempt >= c.opts.MaxAttempts || !isIdempotent(req.Method) {
			return resp, c.exhausted(req, resp, err)
		}

		if sleepErr := c.opts.Sleep(ctx, delay); sleepErr != nil {
			return nil, callerError{fmt.Errorf("wait before retry of %s: %w", host, sleepErr)}
		}
	}
}

// exhausted turns the last failed attempt into a domain error. A 429 that named a pause
// becomes a *RateLimitError, so the enrichment queue waits as long as the provider asked.
func (c *Client) exhausted(req *http.Request, resp *Response, err error) error {
	if err != nil {
		return fmt.Errorf("%w: %w", domain.ErrUnavailable, err)
	}

	message := fmt.Sprintf("%s %s: status %d", req.Method, redactParsed(req.URL), resp.StatusCode)

	if resp.StatusCode != http.StatusTooManyRequests {
		return fmt.Errorf("%s: %w", message, domain.ErrUnavailable)
	}

	if after, ok := parseRetryAfter(resp.Header.Get("Retry-After"), c.opts.Now()); ok && after > 0 {
		return &RateLimitError{message: message, after: after}
	}

	return fmt.Errorf("%s: %w", message, domain.ErrRateLimited)
}

// RateLimitError is a rate-limited answer (429) with the pause the server asked for in
// Retry-After. It wraps domain.ErrRateLimited.
type RateLimitError struct {
	message string
	after   time.Duration
}

// Error implements error.
func (err *RateLimitError) Error() string {
	return fmt.Sprintf("%s (retry after %s): %v", err.message, err.after, domain.ErrRateLimited)
}

// Unwrap lets errors.Is match domain.ErrRateLimited.
func (err *RateLimitError) Unwrap() error { return domain.ErrRateLimited }

// RetryAfter is the pause the server asked for.
func (err *RateLimitError) RetryAfter() time.Duration { return err.after }

// retryDelay returns the pause before the next attempt: exponential backoff with jitter,
// raised to the server's Retry-After. retry is false when Retry-After exceeds the cap.
func (c *Client) retryDelay(resp *Response, attempt int) (delay time.Duration, retry bool) {
	backoff := c.opts.BaseBackoff << min(attempt-1, maxBackoffShift)
	if backoff <= 0 || backoff > c.opts.MaxBackoff {
		backoff = c.opts.MaxBackoff
	}

	delay = c.opts.Jitter(backoff)

	if resp == nil {
		return delay, true
	}

	if after, ok := parseRetryAfter(resp.Header.Get("Retry-After"), c.opts.Now()); ok {
		if after > c.opts.MaxRetryAfter {
			return 0, false
		}

		delay = max(delay, after)
	}

	return delay, true
}

// isIdempotent reports methods that are safe to repeat after an ambiguous failure.
func isIdempotent(method string) bool {
	return method == http.MethodGet || method == http.MethodHead
}

// isServiceFailure reports outcomes worth retrying and counting against the breaker.
// Blocked addresses, oversized bodies, bad schemes and caller cancellation are final.
func isServiceFailure(resp *Response, err error) bool {
	if err != nil {
		return !isCallerError(err) && !isPolicyError(err)
	}

	switch resp.StatusCode {
	case http.StatusTooManyRequests, http.StatusBadGateway,
		http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		return true
	}

	return false
}

// callerError marks a failure caused by the caller's context, which is neither retried nor
// counted against the circuit breaker.
type callerError struct{ error }

func (e callerError) Unwrap() error { return e.error }

func isCallerError(err error) bool {
	var caller callerError

	return errors.As(err, &caller)
}

func isPolicyError(err error) bool {
	return errors.Is(err, ErrBlockedAddress) || errors.Is(err, ErrBodyTooLarge) ||
		errors.Is(err, ErrHostNotAllowed) ||
		errors.Is(err, ErrInsecureURL) || errors.Is(err, ErrTooManyRedirects) ||
		errors.Is(err, domain.ErrInvalid)
}

// parseRetryAfter understands both delta-seconds and HTTP-date forms (RFC 9110 §10.2.3).
func parseRetryAfter(value string, now time.Time) (time.Duration, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, false
	}

	if seconds, err := strconv.ParseInt(value, 10, 64); err == nil {
		if seconds < 0 {
			return 0, false
		}

		// Cap before multiplying: a huge value must not overflow into a negative duration.
		if seconds > int64(math.MaxInt64/time.Second) {
			return math.MaxInt64, true
		}

		return time.Duration(seconds) * time.Second, true
	}

	date, err := http.ParseTime(value)
	if err != nil {
		return 0, false
	}

	return max(date.Sub(now), 0), true
}
