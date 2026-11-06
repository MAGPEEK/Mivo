package restapi

import (
	"errors"
	"net/http"

	"github.com/magpeek/mivo/internal/adapters/http/apigen"
	appaccess "github.com/magpeek/mivo/internal/app/access"
	"github.com/magpeek/mivo/internal/app/auth"
	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/platform/brand"
)

// SessionCookieName is the browser session cookie of ADR 0032 §3, derived from the brand so
// the product name lives in one place.
const SessionCookieName = brand.ID + "_session"

// Problem codes of accounts and sessions (api/openapi.yaml, Problem.code, ADR 0032).
const (
	// CodeUnauthenticated reports a request without a valid session (401).
	CodeUnauthenticated = "unauthenticated"
	// CodeSetupRequired reports a request while no account exists yet (401).
	CodeSetupRequired = "setup_required"
	// CodeInvalidCredentials reports a wrong name or password at sign-in (401).
	CodeInvalidCredentials = "invalid_credentials" //nolint:gosec // a problem code, no credential
	// CodeProfileRequired reports a session without a selected profile on an operation that
	// needs one (403).
	CodeProfileRequired = "profile_required"
	// CodeTooManyAttempts reports a refused attempt after too many failures (429 with
	// Retry-After and retryAfterSeconds).
	CodeTooManyAttempts = "too_many_attempts"
)

// DefaultRetryAfterSeconds is the Retry-After of a rate-limited request whose error does not
// say how long to wait.
const DefaultRetryAfterSeconds int32 = 60

// retryAfterSecondsError is an error that knows when a retry may succeed, such as
// *auth.ThrottledError.
type retryAfterSecondsError interface {
	RetryAfterSeconds() int
}

// authProblem maps the account and session errors. The specific sentinels come before the
// generic ones they wrap: ErrProfileRequired wraps ErrForbidden, ErrSetupRequired and
// ErrInvalidCredentials wrap ErrUnauthenticated. No WWW-Authenticate header is ever sent,
// so browsers show no basic-auth prompt.
func authProblem(err error) (apigen.Problem, bool) {
	switch {
	case errors.Is(err, appaccess.ErrProfileRequired):
		return NewProblem(http.StatusForbidden, CodeProfileRequired), true
	case errors.Is(err, auth.ErrSetupRequired):
		return NewProblem(http.StatusUnauthorized, CodeSetupRequired), true
	case errors.Is(err, auth.ErrInvalidCredentials):
		return NewProblem(http.StatusUnauthorized, CodeInvalidCredentials), true
	case errors.Is(err, domain.ErrUnauthenticated):
		return NewProblem(http.StatusUnauthorized, CodeUnauthenticated), true
	case errors.Is(err, domain.ErrRateLimited):
		return tooManyAttemptsProblem(err), true
	default:
		return apigen.Problem{}, false
	}
}

// tooManyAttemptsProblem is 429 with the wait the error names, else DefaultRetryAfterSeconds.
func tooManyAttemptsProblem(err error) apigen.Problem {
	problem := NewProblem(http.StatusTooManyRequests, CodeTooManyAttempts)
	seconds := DefaultRetryAfterSeconds

	var retryAfter retryAfterSecondsError
	if errors.As(err, &retryAfter) && retryAfter.RetryAfterSeconds() > 0 {
		seconds = int32(min(retryAfter.RetryAfterSeconds(), maxRetryAfterSeconds)) //nolint:gosec // bounded above
	}

	problem.RetryAfterSeconds = &seconds

	return problem
}

// maxRetryAfterSeconds bounds a reported wait so it fits the int32 of the problem; a day is
// far beyond every throttle of ADR 0032.
const maxRetryAfterSeconds = 24 * 60 * 60
