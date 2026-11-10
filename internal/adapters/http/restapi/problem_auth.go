package restapi

import (
	"errors"
	"net/http"

	"github.com/magpeek/mivo/internal/adapters/http/apigen"
	appaccess "github.com/magpeek/mivo/internal/app/access"
	"github.com/magpeek/mivo/internal/app/auth"
	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/account"
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
	// CodePINInvalid reports a missing or wrong profile PIN (403).
	CodePINInvalid = "pin_invalid"
	// CodeProfileLocked reports a profile locked after too many wrong PINs (429 with
	// Retry-After and retryAfterSeconds).
	CodeProfileLocked = "profile_locked"
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
// generic ones they wrap: ErrProfileRequired and ErrPINInvalid wrap ErrForbidden,
// ErrSetupRequired and ErrInvalidCredentials wrap ErrUnauthenticated, a *ProfileLockedError
// unwraps to ErrRateLimited like the sign-in throttle. An *account.InvalidFieldError is
// "invalid" with the field it names. No WWW-Authenticate header is ever sent, so browsers
// show no basic-auth prompt.
func authProblem(err error) (apigen.Problem, bool) {
	var (
		locked   *auth.ProfileLockedError
		fieldErr *account.InvalidFieldError
	)

	switch {
	case errors.Is(err, appaccess.ErrProfileRequired):
		return NewProblem(http.StatusForbidden, CodeProfileRequired), true
	case errors.Is(err, auth.ErrPINInvalid):
		return NewProblem(http.StatusForbidden, CodePINInvalid), true
	case errors.As(err, &locked):
		return retryLaterProblem(CodeProfileLocked, err), true
	case errors.As(err, &fieldErr):
		return invalidFieldProblem(fieldErr), true
	case errors.Is(err, auth.ErrSetupRequired):
		return NewProblem(http.StatusUnauthorized, CodeSetupRequired), true
	case errors.Is(err, auth.ErrInvalidCredentials):
		return NewProblem(http.StatusUnauthorized, CodeInvalidCredentials), true
	case errors.Is(err, domain.ErrUnauthenticated):
		return NewProblem(http.StatusUnauthorized, CodeUnauthenticated), true
	case errors.Is(err, domain.ErrRateLimited):
		return retryLaterProblem(CodeTooManyAttempts, err), true
	default:
		return apigen.Problem{}, false
	}
}

// invalidFieldProblem is 400 "invalid" naming the request property that broke a rule. The
// field names of domain/account match the API properties.
func invalidFieldProblem(err *account.InvalidFieldError) apigen.Problem {
	problem := NewProblem(http.StatusBadRequest, CodeInvalid)
	field := err.Field
	problem.Field = &field

	return problem
}

// retryLaterProblem is 429 with the wait the error names, else DefaultRetryAfterSeconds.
func retryLaterProblem(code string, err error) apigen.Problem {
	problem := NewProblem(http.StatusTooManyRequests, code)
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
