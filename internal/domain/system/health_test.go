package system_test

import (
	"slices"
	"testing"

	"github.com/magpeek/mivo/internal/domain/system"
)

func TestNewHealthAggregatesChecks(t *testing.T) {
	t.Parallel()

	databaseOK := system.PassedCheck(system.HealthCheckDatabase)
	databaseDown := system.FailedCheck(system.HealthCheckDatabase, "connection refused")
	libraryOK := system.PassedCheck("library")
	unknown := system.HealthCheck{Name: "transcoder", Status: "degraded"}

	tests := []struct {
		name       string
		checks     []system.HealthCheck
		wantStatus system.HealthStatus
	}{
		{name: "no checks", checks: nil, wantStatus: system.HealthStatusOK},
		{name: "single passed", checks: []system.HealthCheck{databaseOK}, wantStatus: system.HealthStatusOK},
		{name: "all passed", checks: []system.HealthCheck{databaseOK, libraryOK}, wantStatus: system.HealthStatusOK},
		{name: "single failed", checks: []system.HealthCheck{databaseDown}, wantStatus: system.HealthStatusUnavailable},
		{
			name:       "one of several failed",
			checks:     []system.HealthCheck{libraryOK, databaseDown},
			wantStatus: system.HealthStatusUnavailable,
		},
		{name: "unknown status fails safe", checks: []system.HealthCheck{unknown}, wantStatus: system.HealthStatusUnavailable},
		{name: "empty status fails safe", checks: []system.HealthCheck{{Name: "x"}}, wantStatus: system.HealthStatusUnavailable},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			health := system.NewHealth(test.checks...)
			if health.Status != test.wantStatus {
				t.Errorf("Status = %q, want %q", health.Status, test.wantStatus)
			}

			if !slices.Equal(health.Checks, test.checks) {
				t.Errorf("Checks = %+v, want %+v", health.Checks, test.checks)
			}
		})
	}
}

func TestNewHealthCopiesChecks(t *testing.T) {
	t.Parallel()

	checks := []system.HealthCheck{system.PassedCheck(system.HealthCheckDatabase)}
	health := system.NewHealth(checks...)

	checks[0] = system.FailedCheck("changed", "after aggregation")

	if health.Checks[0].Name != system.HealthCheckDatabase {
		t.Errorf("Checks[0].Name = %q after caller mutation, want %q", health.Checks[0].Name, system.HealthCheckDatabase)
	}
}

func TestCheckConstructors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		check system.HealthCheck
		want  system.HealthCheck
	}{
		{
			name:  "passed",
			check: system.PassedCheck(system.HealthCheckDatabase),
			want:  system.HealthCheck{Name: "database", Status: system.HealthStatusOK},
		},
		{
			name:  "failed",
			check: system.FailedCheck(system.HealthCheckDatabase, "disk I/O error"),
			want:  system.HealthCheck{Name: "database", Status: system.HealthStatusUnavailable, Detail: "disk I/O error"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			if test.check != test.want {
				t.Errorf("check = %+v, want %+v", test.check, test.want)
			}
		})
	}
}

func TestHealthStatusWireValues(t *testing.T) {
	t.Parallel()

	// The values are part of the HTTP contract; changing them breaks clients.
	if system.HealthStatusOK != "ok" || system.HealthStatusUnavailable != "unavailable" {
		t.Errorf("status values = %q/%q, want ok/unavailable", system.HealthStatusOK, system.HealthStatusUnavailable)
	}
}
