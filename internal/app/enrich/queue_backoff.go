package enrich

import (
	"errors"
	"time"
)

// Retry timing of failed jobs.
const (
	// BaseBackoff is the pause after the first failure; it doubles with every further one.
	BaseBackoff = time.Minute
	// MaxBackoff caps the pause, so an item offline for days is still tried a few times a day.
	MaxBackoff = 6 * time.Hour
	// DefaultRateLimitPause is used when a rate-limited answer carries no Retry-After. The
	// outbound client already waited and retried before giving up, so the pause is longer.
	DefaultRateLimitPause = 5 * time.Minute
	// maxBackoffShift keeps the doubling from overflowing; BaseBackoff << 9 exceeds MaxBackoff.
	maxBackoffShift = 9
)

// Backoff returns the pause before the next try of a job that failed after attempts earlier
// failures: BaseBackoff doubled per attempt, capped at MaxBackoff.
func Backoff(attempts int) time.Duration {
	shift := min(max(attempts, 0), maxBackoffShift)

	return min(BaseBackoff<<shift, MaxBackoff)
}

// RetryAfterError is implemented by a rate-limit error that knows the provider's Retry-After.
type RetryAfterError interface {
	error
	// RetryAfter is the pause the provider asked for.
	RetryAfter() time.Duration
}

// rateLimitPause returns the pause a rate-limited job waits: the provider's Retry-After when
// the error carries one (capped at MaxBackoff), else DefaultRateLimitPause.
func rateLimitPause(err error) time.Duration {
	var retryAfter RetryAfterError
	if errors.As(err, &retryAfter) && retryAfter.RetryAfter() > 0 {
		return min(retryAfter.RetryAfter(), MaxBackoff)
	}

	return DefaultRateLimitPause
}
