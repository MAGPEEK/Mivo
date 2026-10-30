package system

import "slices"

// HealthStatus is the state of the server as a whole or of one health check.
type HealthStatus string

const (
	// HealthStatusOK means the server or component works.
	HealthStatusOK HealthStatus = "ok"
	// HealthStatusUnavailable means the server or component cannot serve requests.
	HealthStatusUnavailable HealthStatus = "unavailable"
)

// HealthCheckDatabase is the name of the check that verifies the database is reachable.
const HealthCheckDatabase = "database"

// HealthCheck is the result of checking one component.
type HealthCheck struct {
	// Name identifies the component, for example HealthCheckDatabase.
	Name string
	// Status is the state of the component.
	Status HealthStatus
	// Detail explains a failure; it is empty for passed checks.
	Detail string
}

// Health is the aggregated state of the server.
type Health struct {
	// Status is ok only when every check is ok.
	Status HealthStatus
	// Checks lists the individual results in the order they were run.
	Checks []HealthCheck
}

// PassedCheck returns an ok result for the named component.
func PassedCheck(name string) HealthCheck {
	return HealthCheck{Name: name, Status: HealthStatusOK}
}

// FailedCheck returns an unavailable result for the named component with the reason.
func FailedCheck(name, detail string) HealthCheck {
	return HealthCheck{Name: name, Status: HealthStatusUnavailable, Detail: detail}
}

// NewHealth aggregates check results. The server is ok only when every check is ok; any
// other status, including an unknown one, makes it unavailable so that a bug in a check
// never reports a broken server as healthy. The checks are copied so later changes to the
// caller's slice do not alter the result.
func NewHealth(checks ...HealthCheck) Health {
	status := HealthStatusOK
	if slices.ContainsFunc(checks, isNotOK) {
		status = HealthStatusUnavailable
	}

	return Health{Status: status, Checks: slices.Clone(checks)}
}

func isNotOK(check HealthCheck) bool {
	return check.Status != HealthStatusOK
}
