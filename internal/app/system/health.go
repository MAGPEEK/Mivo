package system

import (
	"context"
	"errors"
	"time"

	"github.com/magpeek/mivo/internal/domain/system"
)

// DatabasePingTimeout bounds the database check of Health, so a hung database makes the
// health endpoint report "unavailable" quickly instead of hanging the caller.
const DatabasePingTimeout = 2 * time.Second

// Fixed health details. They never carry the adapter's error text, because Health is served
// to unauthenticated clients and raw driver errors can reveal paths or internals.
const (
	// DetailDatabasePingFailed reports that the database answered the ping with an error or
	// that the caller gave up before the ping finished.
	DetailDatabasePingFailed = "database ping failed"
	// DetailDatabasePingTimedOut reports that the ping did not finish before its deadline,
	// which is DatabasePingTimeout or the caller's earlier deadline.
	DetailDatabasePingTimedOut = "database ping timed out"
)

// Health pings the database within DatabasePingTimeout and reports the aggregated state.
// An unreachable database is a result, not an error: the server is then "unavailable".
//
// The check detail is always one of the fixed Detail* messages. The raw ping error is
// deliberately not passed on: the DatabaseChecker adapter owns it and logs it (see the port
// doc), so clients never see driver internals and Health keeps a single error-free result.
func (service *Service) Health(ctx context.Context) system.Health {
	return system.NewHealth(service.checkDatabase(ctx))
}

func (service *Service) checkDatabase(ctx context.Context) system.HealthCheck {
	pingContext, cancel := context.WithTimeout(ctx, DatabasePingTimeout)
	defer cancel()

	err := service.database.Ping(pingContext)
	if err == nil {
		// A checker that ignores ctx may answer late; past the deadline it still counts as
		// unavailable so the timeout means the same for every implementation.
		err = pingContext.Err()
	}

	if err != nil {
		return system.FailedCheck(system.HealthCheckDatabase, pingFailureDetail(err))
	}

	return system.PassedCheck(system.HealthCheckDatabase)
}

// pingFailureDetail names no duration: the deadline that fired may be the caller's, which
// can be shorter than DatabasePingTimeout.
func pingFailureDetail(err error) string {
	if errors.Is(err, context.DeadlineExceeded) {
		return DetailDatabasePingTimedOut
	}

	return DetailDatabasePingFailed
}
