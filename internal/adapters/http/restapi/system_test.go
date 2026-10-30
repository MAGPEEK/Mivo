package restapi_test

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/magpeek/mivo/internal/adapters/http/apigen"
	"github.com/magpeek/mivo/internal/adapters/http/restapi"
	appsystem "github.com/magpeek/mivo/internal/app/system"
	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/system"
)

const (
	healthPath = "/api/v1/health"
	infoPath   = "/api/v1/system/info"
)

// The use case cmd/mivo wires in must keep satisfying the port the handlers declare.
var _ restapi.SystemService = (*appsystem.Service)(nil)

func TestGetHealth(t *testing.T) {
	t.Parallel()

	pingFailed := appsystem.DetailDatabasePingFailed
	timedOut := appsystem.DetailDatabasePingTimedOut

	tests := []struct {
		name       string
		health     system.Health
		wantStatus int
		want       apigen.HealthResponse
	}{
		{
			name:       "healthy",
			health:     system.NewHealth(system.PassedCheck(system.HealthCheckDatabase)),
			wantStatus: http.StatusOK,
			want: apigen.HealthResponse{Status: apigen.HealthStatusOk, Checks: []apigen.HealthCheck{
				{Name: "database", Status: apigen.HealthStatusOk},
			}},
		},
		{
			name:       "no checks",
			health:     system.NewHealth(),
			wantStatus: http.StatusOK,
			want:       apigen.HealthResponse{Status: apigen.HealthStatusOk, Checks: []apigen.HealthCheck{}},
		},
		{
			name:       "database ping failed",
			health:     system.NewHealth(system.FailedCheck(system.HealthCheckDatabase, pingFailed)),
			wantStatus: http.StatusServiceUnavailable,
			want: apigen.HealthResponse{Status: apigen.HealthStatusUnavailable, Checks: []apigen.HealthCheck{
				{Name: "database", Status: apigen.HealthStatusUnavailable, Detail: &pingFailed},
			}},
		},
		{
			name: "database ping timed out next to a passed check",
			health: system.NewHealth(
				system.PassedCheck("library"),
				system.FailedCheck(system.HealthCheckDatabase, timedOut),
			),
			wantStatus: http.StatusServiceUnavailable,
			want: apigen.HealthResponse{Status: apigen.HealthStatusUnavailable, Checks: []apigen.HealthCheck{
				{Name: "library", Status: apigen.HealthStatusOk},
				{Name: "database", Status: apigen.HealthStatusUnavailable, Detail: &timedOut},
			}},
		},
		{
			name:       "unknown status fails safe",
			health:     system.Health{Status: "degraded", Checks: []system.HealthCheck{{Name: "x", Status: "degraded", Detail: pingFailed}}},
			wantStatus: http.StatusServiceUnavailable,
			want: apigen.HealthResponse{Status: apigen.HealthStatusUnavailable, Checks: []apigen.HealthCheck{
				{Name: "x", Status: apigen.HealthStatusUnavailable, Detail: &pingFailed},
			}},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			handler, _ := newTestHandler(t, fakeSystemService{health: test.health})
			got := serve(t, handler, http.MethodGet, healthPath)

			assertHealthResponse(t, got, test.wantStatus, test.want)
		})
	}
}

func assertHealthResponse(t *testing.T, got response, wantStatus int, want apigen.HealthResponse) {
	t.Helper()

	if got.status != wantStatus {
		t.Errorf("status = %d, want %d", got.status, wantStatus)
	}

	if contentType := got.header.Get("Content-Type"); contentType != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", contentType)
	}

	// The contract requires checks as an array, never null.
	if !strings.Contains(string(got.body), `"checks":[`) {
		t.Errorf("body %s has no checks array", got.body)
	}

	body := decode[apigen.HealthResponse](t, got.body)
	if body.Status != want.Status || !sameChecks(body.Checks, want.Checks) {
		t.Errorf("body = %s, want %+v", got.body, want)
	}
}

func sameChecks(got, want []apigen.HealthCheck) bool {
	if len(got) != len(want) {
		return false
	}

	for index := range got {
		if got[index].Name != want[index].Name || got[index].Status != want[index].Status {
			return false
		}

		if (got[index].Detail == nil) != (want[index].Detail == nil) {
			return false
		}

		if got[index].Detail != nil && *got[index].Detail != *want[index].Detail {
			return false
		}
	}

	return true
}

// TestGetHealthNeverSendsUnknownDetails guards the unauthenticated endpoint: a raw error
// text that slipped into a check is replaced and only logged.
func TestGetHealthNeverSendsUnknownDetails(t *testing.T) {
	t.Parallel()

	const leakedText = `open C:\ProgramData\mivo\mivo.db: access denied`

	health := system.NewHealth(system.FailedCheck(system.HealthCheckDatabase, leakedText))
	handler, logs := newTestHandler(t, fakeSystemService{health: health})

	got := serve(t, handler, http.MethodGet, healthPath)

	if strings.Contains(string(got.body), "access denied") {
		t.Errorf("body %s exposes the raw detail", got.body)
	}

	body := decode[apigen.HealthResponse](t, got.body)
	if detail := body.Checks[0].Detail; detail == nil || *detail != "check failed" {
		t.Errorf("detail = %v, want the generic replacement", detail)
	}

	if !strings.Contains(logs.String(), "access denied") {
		t.Errorf("log %q does not record the replaced detail", logs.String())
	}
}

func TestGetSystemInfo(t *testing.T) {
	t.Parallel()

	buildTime := time.Date(2026, 10, 29, 12, 30, 0, 0, time.UTC)

	tests := []struct {
		name          string
		commit        string
		buildTime     string
		wantCommit    *string
		wantBuildTime *time.Time
	}{
		{name: "all fields", commit: "abc123", buildTime: "2026-10-29T12:30:00Z", wantCommit: ptr("abc123"), wantBuildTime: &buildTime},
		{name: "build time with offset", buildTime: "2026-10-29T14:30:00+02:00", wantBuildTime: &buildTime},
		{name: "unknown commit and build time", commit: "", buildTime: ""},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			service := fakeSystemService{info: testInfo(t, test.commit, test.buildTime)}
			handler, _ := newTestHandler(t, service)

			got := serve(t, handler, http.MethodGet, infoPath)
			if got.status != http.StatusOK || got.header.Get("Content-Type") != "application/json" {
				t.Fatalf("status = %d, Content-Type = %q, want 200 application/json", got.status, got.header.Get("Content-Type"))
			}

			assertSystemInfo(t, decode[apigen.SystemInfo](t, got.body), test.wantCommit, test.wantBuildTime)
		})
	}
}

func testInfo(t *testing.T, commit, buildTime string) system.Info {
	t.Helper()

	return system.Info{
		ProductName: "Mivo",
		Version:     "1.2.3",
		Commit:      commit,
		BuildTime:   buildTime,
		ServerID:    mustServerID(t, testServerID),
		APIVersion:  system.APIVersion,
	}
}

func assertSystemInfo(t *testing.T, got apigen.SystemInfo, wantCommit *string, wantBuildTime *time.Time) {
	t.Helper()

	if got.ProductName != "Mivo" || got.Version != "1.2.3" || got.APIVersion != system.APIVersion {
		t.Errorf("info = %+v, want product Mivo, version 1.2.3, api version %s", got, system.APIVersion)
	}

	if got.ServerID.String() != testServerID {
		t.Errorf("serverId = %s, want %s", got.ServerID, testServerID)
	}

	if (got.Commit == nil) != (wantCommit == nil) || (got.Commit != nil && *got.Commit != *wantCommit) {
		t.Errorf("commit = %v, want %v", got.Commit, wantCommit)
	}

	if (got.BuildTime == nil) != (wantBuildTime == nil) || (got.BuildTime != nil && !got.BuildTime.Equal(*wantBuildTime)) {
		t.Errorf("buildTime = %v, want %v", got.BuildTime, wantBuildTime)
	}
}

func TestGetSystemInfoOmitsMalformedBuildTime(t *testing.T) {
	t.Parallel()

	handler, logs := newTestHandler(t, fakeSystemService{info: testInfo(t, "abc", "yesterday")})

	got := serve(t, handler, http.MethodGet, infoPath)
	if got.status != http.StatusOK {
		t.Fatalf("status = %d, want 200", got.status)
	}

	if strings.Contains(string(got.body), "buildTime") {
		t.Errorf("body %s contains the malformed buildTime", got.body)
	}

	if !strings.Contains(logs.String(), "level=WARN") || !strings.Contains(logs.String(), "yesterday") {
		t.Errorf("log %q has no warning about the malformed build time", logs.String())
	}
}

// TestGetSystemInfoFailuresAreInternal pins the orchestrator rule: Info takes no client
// input, so every failure is a 500 without the error text, whatever sentinel it wraps.
func TestGetSystemInfoFailuresAreInternal(t *testing.T) {
	t.Parallel()

	const databasePath = `C:\ProgramData\mivo\mivo.db`

	tests := []struct {
		name string
		err  error
	}{
		{name: "plain error", err: errors.New("disk I/O error at " + databasePath)},
		{name: "wraps not found", err: fmt.Errorf("load %s: %w", databasePath, domain.ErrNotFound)},
		{name: "wraps conflict", err: fmt.Errorf("save %s: %w", databasePath, domain.ErrConflict)},
		{name: "wraps invalid", err: fmt.Errorf("stored id in %s: %w", databasePath, domain.ErrInvalid)},
		{name: "wraps forbidden", err: fmt.Errorf("open %s: %w", databasePath, domain.ErrForbidden)},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			handler, logs := newTestHandler(t, fakeSystemService{infoErr: test.err})

			got := serve(t, handler, http.MethodGet, infoPath)
			assertProblem(t, got, http.StatusInternalServerError, restapi.CodeInternal)

			if strings.Contains(string(got.body), "mivo.db") {
				t.Errorf("body %s exposes the internal error", got.body)
			}

			if !strings.Contains(logs.String(), "mivo.db") || !strings.Contains(logs.String(), "level=ERROR") {
				t.Errorf("log %q does not record the cause", logs.String())
			}
		})
	}
}

func TestGetSystemInfoRejectsZeroServerID(t *testing.T) {
	t.Parallel()

	info := testInfo(t, "", "")
	info.ServerID = system.ServerID{}

	handler, logs := newTestHandler(t, fakeSystemService{info: info})

	got := serve(t, handler, http.MethodGet, infoPath)
	assertProblem(t, got, http.StatusInternalServerError, restapi.CodeInternal)

	if !strings.Contains(logs.String(), "convert server id") {
		t.Errorf("log %q does not record the conversion failure", logs.String())
	}
}

func ptr[T any](value T) *T {
	return &value
}
