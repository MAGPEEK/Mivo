package restapi_test

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/magpeek/mivo/internal/adapters/http/restapi"
	appaccess "github.com/magpeek/mivo/internal/app/access"
	"github.com/magpeek/mivo/internal/app/auth"
	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/platform/brand"
)

func TestProblemForAuthErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		err         error
		wantStatus  int32
		wantCode    string
		wantSeconds int32
	}{
		{name: "unauthenticated", err: fmt.Errorf("resolve: %w", domain.ErrUnauthenticated), wantStatus: 401, wantCode: "unauthenticated"},
		{name: "setup required", err: fmt.Errorf("sign in: %w", auth.ErrSetupRequired), wantStatus: 401, wantCode: "setup_required"},
		{name: "invalid credentials", err: auth.ErrInvalidCredentials, wantStatus: 401, wantCode: "invalid_credentials"},
		{name: "profile required", err: fmt.Errorf("list items: %w", appaccess.ErrProfileRequired), wantStatus: 403, wantCode: "profile_required"},
		{name: "plain forbidden stays forbidden", err: domain.ErrForbidden, wantStatus: 403, wantCode: "forbidden"},
		{
			name: "throttled sign-in", err: fmt.Errorf("sign in: %w", &auth.ThrottledError{Wait: 1500 * time.Millisecond}),
			wantStatus: 429, wantCode: "too_many_attempts", wantSeconds: 2,
		},
		{
			name: "rate limited without a wait", err: domain.ErrRateLimited,
			wantStatus: 429, wantCode: "too_many_attempts", wantSeconds: restapi.DefaultRetryAfterSeconds,
		},
		{
			name: "huge wait is bounded", err: &auth.ThrottledError{Wait: 1000 * 24 * time.Hour},
			wantStatus: 429, wantCode: "too_many_attempts", wantSeconds: 24 * 60 * 60,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			problem := restapi.ProblemForError(test.err)
			if problem.Status != test.wantStatus || problem.Code != test.wantCode {
				t.Fatalf("ProblemForError(%v) = %d %q, want %d %q", test.err, problem.Status, problem.Code, test.wantStatus, test.wantCode)
			}

			gotSeconds := int32(0)
			if problem.RetryAfterSeconds != nil {
				gotSeconds = *problem.RetryAfterSeconds
			}

			if gotSeconds != test.wantSeconds {
				t.Errorf("retryAfterSeconds = %d, want %d", gotSeconds, test.wantSeconds)
			}
		})
	}
}

func TestWriteProblemSendsRetryAfterAndNoAuthenticateChallenge(t *testing.T) {
	t.Parallel()

	recorder := httptest.NewRecorder()
	restapi.WriteProblem(recorder, restapi.ProblemForError(&auth.ThrottledError{Wait: 90 * time.Second}))

	if recorder.Code != http.StatusTooManyRequests || recorder.Header().Get("Retry-After") != "90" {
		t.Errorf("WriteProblem = %d Retry-After %q, want 429 90", recorder.Code, recorder.Header().Get("Retry-After"))
	}

	unauthenticated := httptest.NewRecorder()
	restapi.WriteProblem(unauthenticated, restapi.ProblemForError(domain.ErrUnauthenticated))

	if unauthenticated.Code != http.StatusUnauthorized || unauthenticated.Header().Get("WWW-Authenticate") != "" {
		t.Errorf("WriteProblem = %d WWW-Authenticate %q, want 401 without a challenge",
			unauthenticated.Code, unauthenticated.Header().Get("WWW-Authenticate"))
	}

	if unauthenticated.Header().Get("Retry-After") != "" {
		t.Error("401 carries Retry-After, want none")
	}
}

func TestSessionCookieNameIsBrandDerived(t *testing.T) {
	t.Parallel()

	if want := brand.ID + "_session"; restapi.SessionCookieName != want {
		t.Errorf("SessionCookieName = %q, want %q", restapi.SessionCookieName, want)
	}

	if !errors.Is(appaccess.ErrProfileRequired, domain.ErrForbidden) {
		t.Error("ErrProfileRequired does not wrap domain.ErrForbidden")
	}
}
