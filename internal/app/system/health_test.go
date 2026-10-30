package system_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	appsystem "github.com/magpeek/mivo/internal/app/system"
	"github.com/magpeek/mivo/internal/domain/system"
)

// shortParentDeadline lets the deadline test finish quickly instead of waiting the full
// DatabasePingTimeout; the timeout itself is checked through the context deadline.
const shortParentDeadline = 20 * time.Millisecond

// deadlineSlack absorbs clock granularity and scheduling delay when the ping deadline is
// compared with DatabasePingTimeout.
const deadlineSlack = 250 * time.Millisecond

func TestHealthIsOKWhenPingSucceeds(t *testing.T) {
	t.Parallel()

	environment := newTestEnvironment()

	health := environment.service(t).Health(t.Context())

	want := system.NewHealth(system.PassedCheck(system.HealthCheckDatabase))
	if health.Status != system.HealthStatusOK || len(health.Checks) != 1 || health.Checks[0] != want.Checks[0] {
		t.Errorf("Health = %+v, want %+v", health, want)
	}
}

func TestHealthIsUnavailableWhenPingFails(t *testing.T) {
	t.Parallel()

	environment := newTestEnvironment()
	environment.ping = func(context.Context) error { return errBoom }

	health := environment.service(t).Health(t.Context())

	assertDatabaseUnavailable(t, health, appsystem.DetailDatabasePingFailed)
}

func TestHealthDetailNeverContainsAdapterErrorText(t *testing.T) {
	t.Parallel()

	// Driver errors can reveal file paths; none of this may reach the health detail.
	const adapterErrorText = `open C:\data\library.db: access denied`

	tests := []struct {
		name       string
		pingErr    error
		wantDetail string
	}{
		{name: "plain failure", pingErr: errors.New(adapterErrorText), wantDetail: appsystem.DetailDatabasePingFailed},
		{
			name:       "wrapped deadline",
			pingErr:    fmt.Errorf("%s: %w", adapterErrorText, context.DeadlineExceeded),
			wantDetail: appsystem.DetailDatabasePingTimedOut,
		},
		{
			name:       "wrapped cancellation",
			pingErr:    fmt.Errorf("%s: %w", adapterErrorText, context.Canceled),
			wantDetail: appsystem.DetailDatabasePingFailed,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			environment := newTestEnvironment()
			environment.ping = func(context.Context) error { return test.pingErr }

			health := environment.service(t).Health(t.Context())

			assertDatabaseUnavailable(t, health, test.wantDetail)

			if detail := health.Checks[0].Detail; strings.Contains(detail, "access denied") {
				t.Errorf("Detail = %q leaks the adapter error", detail)
			}
		})
	}
}

func TestHealthBoundsPingWithTimeout(t *testing.T) {
	t.Parallel()

	environment := newTestEnvironment()

	var deadline time.Time

	var hasDeadline bool

	environment.ping = func(ctx context.Context) error {
		deadline, hasDeadline = ctx.Deadline()

		return context.DeadlineExceeded
	}

	before := time.Now()
	health := environment.service(t).Health(t.Context())

	if !hasDeadline {
		t.Fatal("ping context has no deadline, want DatabasePingTimeout")
	}

	// Clock granularity differs between platforms (Windows ticks coarsely), so allow slack.
	remaining := deadline.Sub(before)
	if remaining < appsystem.DatabasePingTimeout-deadlineSlack || remaining > appsystem.DatabasePingTimeout+deadlineSlack {
		t.Errorf("ping deadline is %v after the call, want %v +/- %v", remaining, appsystem.DatabasePingTimeout, deadlineSlack)
	}

	assertDatabaseUnavailable(t, health, appsystem.DetailDatabasePingTimedOut)
}

func TestHealthHonoursContextDeadlineOfBlockingPing(t *testing.T) {
	t.Parallel()

	environment := newTestEnvironment()
	environment.ping = func(ctx context.Context) error {
		<-ctx.Done()

		return ctx.Err()
	}

	ctx, cancel := context.WithTimeout(t.Context(), shortParentDeadline)
	defer cancel()

	health := environment.service(t).Health(ctx)

	assertDatabaseUnavailable(t, health, appsystem.DetailDatabasePingTimedOut)
}

func TestHealthTreatsLateAnswerAsUnavailable(t *testing.T) {
	t.Parallel()

	environment := newTestEnvironment()
	// This checker ignores ctx and claims success although the deadline has passed.
	environment.ping = func(ctx context.Context) error {
		<-ctx.Done()

		return nil
	}

	ctx, cancel := context.WithTimeout(t.Context(), shortParentDeadline)
	defer cancel()

	health := environment.service(t).Health(ctx)

	assertDatabaseUnavailable(t, health, appsystem.DetailDatabasePingTimedOut)
}

func TestHealthReportsCancelledCaller(t *testing.T) {
	t.Parallel()

	environment := newTestEnvironment()

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	health := environment.service(t).Health(ctx)

	assertDatabaseUnavailable(t, health, appsystem.DetailDatabasePingFailed)
}

func assertDatabaseUnavailable(t *testing.T, health system.Health, wantDetail string) {
	t.Helper()

	if health.Status != system.HealthStatusUnavailable {
		t.Errorf("Status = %q, want %q", health.Status, system.HealthStatusUnavailable)
	}

	if len(health.Checks) != 1 {
		t.Fatalf("Checks = %+v, want exactly the database check", health.Checks)
	}

	check := health.Checks[0]
	if check.Name != system.HealthCheckDatabase || check.Status != system.HealthStatusUnavailable {
		t.Errorf("check = %+v, want unavailable %q check", check, system.HealthCheckDatabase)
	}

	if check.Detail != wantDetail {
		t.Errorf("Detail = %q, want exactly %q", check.Detail, wantDetail)
	}
}
