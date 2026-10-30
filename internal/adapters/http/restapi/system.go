package restapi

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"slices"
	"time"
	"uuid"

	"github.com/magpeek/mivo/internal/adapters/http/apigen"
	"github.com/magpeek/mivo/internal/adapters/http/requestid"
	appsystem "github.com/magpeek/mivo/internal/app/system"
	"github.com/magpeek/mivo/internal/domain/system"
)

// operationGetSystemInfo names the operation in failure logs.
const operationGetSystemInfo = "getSystemInfo"

// detailCheckFailed replaces a health detail outside the public set.
const detailCheckFailed = "check failed"

// publicHealthDetails is the closed set of details /health may send (api/openapi.yaml,
// HealthCheck.detail). The endpoint is unauthenticated, so any other text, for example a
// raw driver error that slipped into a check, is replaced instead of passed on.
func publicHealthDetails() []string {
	return []string{appsystem.DetailDatabasePingFailed, appsystem.DetailDatabasePingTimedOut}
}

// GetHealth answers 200 when the server is ok and 503 otherwise, with the same body shape.
func (api *API) GetHealth(ctx context.Context, _ apigen.GetHealthRequestObject) (apigen.GetHealthResponseObject, error) {
	response := api.healthResponse(ctx, api.system.Health(ctx))
	if response.Status == apigen.HealthStatusOk {
		return apigen.GetHealth200JSONResponse(response), nil
	}

	return apigen.GetHealth503JSONResponse(response), nil
}

func (api *API) healthResponse(ctx context.Context, health system.Health) apigen.HealthResponse {
	checks := make([]apigen.HealthCheck, 0, len(health.Checks))
	for _, check := range health.Checks {
		checks = append(checks, api.healthCheck(ctx, check))
	}

	return apigen.HealthResponse{Status: publicHealthStatus(health.Status), Checks: checks}
}

func (api *API) healthCheck(ctx context.Context, check system.HealthCheck) apigen.HealthCheck {
	status := publicHealthStatus(check.Status)
	result := apigen.HealthCheck{Name: check.Name, Status: status}
	if status == apigen.HealthStatusOk {
		// The contract omits the detail for passed checks.
		return result
	}

	detail := check.Detail
	if !slices.Contains(publicHealthDetails(), detail) {
		api.logger.WarnContext(ctx, "health check detail is not in the public set",
			slog.String("check", check.Name),
			slog.String("detail", detail),
			slog.String("request_id", requestid.FromContext(ctx)))

		detail = detailCheckFailed
	}

	result.Detail = &detail

	return result
}

// publicHealthStatus fails safe like system.NewHealth: only ok is ok, every other value,
// including one a later domain version adds, is sent as unavailable.
func publicHealthStatus(status system.HealthStatus) apigen.HealthStatus {
	if status == system.HealthStatusOK {
		return apigen.HealthStatusOk
	}

	return apigen.HealthStatusUnavailable
}

// GetSystemInfo describes the server. Every failure is a 500, even when the cause wraps
// ErrNotFound, ErrConflict or ErrInvalid: Info takes no client input, so such an error means
// broken server state (for example an unreadable identity), not a client mistake. The one
// exception is a failure after the request context ended, which gets no response at all.
func (api *API) GetSystemInfo(ctx context.Context, _ apigen.GetSystemInfoRequestObject) (apigen.GetSystemInfoResponseObject, error) {
	info, err := api.system.Info(ctx)
	if err != nil && ctx.Err() != nil {
		// The client went away or the server was Closed: no server fault, so no
		// error log. writeResponseError logs it at debug level and writes nothing.
		return nil, fmt.Errorf("%s: %w", operationGetSystemInfo, err)
	}

	if err != nil {
		return api.systemInfoFailure(ctx, err), nil
	}

	response, err := api.systemInfoResponse(ctx, info)
	if err != nil {
		return api.systemInfoFailure(ctx, err), nil
	}

	return apigen.GetSystemInfo200JSONResponse(response), nil
}

func (api *API) systemInfoFailure(ctx context.Context, err error) apigen.GetSystemInfoResponseObject {
	api.logOperationFailure(ctx, operationGetSystemInfo, err)

	return apigen.GetSystemInfodefaultApplicationProblemPlusJSONResponse{
		Body:       InternalProblem(),
		StatusCode: http.StatusInternalServerError,
	}
}

func (api *API) systemInfoResponse(ctx context.Context, info system.Info) (apigen.SystemInfo, error) {
	serverID, err := uuid.Parse(info.ServerID.String())
	if err != nil {
		return apigen.SystemInfo{}, fmt.Errorf("convert server id %q: %w", info.ServerID, err)
	}

	return apigen.SystemInfo{
		ProductName: info.ProductName,
		Version:     info.Version,
		Commit:      optionalString(info.Commit),
		BuildTime:   api.buildTime(ctx, info.BuildTime),
		ServerID:    serverID,
		APIVersion:  info.APIVersion,
	}, nil
}

// buildTime omits a malformed build time instead of failing the request: the field is
// optional ("omitted when unknown"), and clients use this endpoint for discovery, which a
// bad build stamp must not break. The warning makes the broken stamp visible.
func (api *API) buildTime(ctx context.Context, text string) *time.Time {
	parsed, err := parseBuildTime(text)
	if err != nil {
		api.logger.WarnContext(ctx, "omitting malformed build time",
			slog.Any("error", err),
			slog.String("request_id", requestid.FromContext(ctx)))
	}

	return parsed
}

// parseBuildTime turns the RFC 3339 build time into the contract's date-time. Empty text
// means unknown and yields nil without an error.
func parseBuildTime(text string) (*time.Time, error) {
	if text == "" {
		return nil, nil
	}

	parsed, err := time.Parse(time.RFC3339, text)
	if err != nil {
		return nil, fmt.Errorf("parse build time %q: %w", text, err)
	}

	return &parsed, nil
}

func optionalString(value string) *string {
	if value == "" {
		return nil
	}

	return &value
}
