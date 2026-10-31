package restapi

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/magpeek/mivo/internal/adapters/http/apigen"
	"github.com/magpeek/mivo/internal/adapters/http/requestid"
	appplayback "github.com/magpeek/mivo/internal/app/playback"
	"github.com/magpeek/mivo/internal/domain/playback"
)

// Operation names of the playback session endpoints, as in api/openapi.yaml, for logs.
const (
	operationCreatePlaybackSession = "createPlaybackSession"
	operationGetPlaybackSession    = "getPlaybackSession"
	operationStopPlaybackSession   = "stopPlaybackSession"
)

// playbackSessionsPath is the collection resource of api/openapi.yaml; the Location of a new
// session is its member path. Stream paths are never built here: the app session carries the
// streamUrl (ADR 0024).
const playbackSessionsPath = "/api/v1/playback/sessions/"

// sessionLimitRetryAfterSeconds is the Retry-After of session_limit_reached. A slot frees
// when another viewer stops or a session reaches the idle timeout (appplayback.IdleTimeout),
// rarely within seconds, so a short value would only make players hammer the server.
const sessionLimitRetryAfterSeconds int32 = 30

// reasonCodeSeparator joins the reason codes of a session start in the log line.
const reasonCodeSeparator = ","

// PlaybackService is the port of the playback session endpoints, implemented by
// app/playback.Service. The stream delivery routes are not served here but mounted
// (Options.Mounts, M2-29).
type PlaybackService interface {
	// Start decides how the client plays the file and starts the session.
	Start(ctx context.Context, request appplayback.StartRequest) (appplayback.Session, error)
	// Get returns a session or an error wrapping domain.ErrNotFound.
	Get(ctx context.Context, id string) (appplayback.Session, error)
	// Stop ends a session; an unknown or stopped session is no error.
	Stop(ctx context.Context, id string) error
}

// CreatePlaybackSession validates the request, starts the session and answers 201 with its
// Location. Errors of the service reach writeResponseError, which maps and logs them; only
// session_limit_reached is answered here, because it carries Retry-After.
func (api *API) CreatePlaybackSession(ctx context.Context, request apigen.CreatePlaybackSessionRequestObject) (apigen.CreatePlaybackSessionResponseObject, error) {
	startRequest, err := newStartRequest(request.Body)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", operationCreatePlaybackSession, err)
	}

	session, err := api.playback.Start(ctx, startRequest)
	if errors.Is(err, appplayback.ErrSessionLimitReached) {
		return api.sessionLimitReached(ctx, err), nil
	}

	if err != nil {
		return nil, fmt.Errorf("%s: %w", operationCreatePlaybackSession, err)
	}

	body, err := newPlaybackSession(session)
	if err != nil {
		return api.undescribableSession(ctx, session.ID, err), nil
	}

	api.logSessionStarted(ctx, session)

	location := playbackSessionsPath + body.ID.String()

	return apigen.CreatePlaybackSession201JSONResponse{
		Body:    body,
		Headers: apigen.CreatePlaybackSession201ResponseHeaders{Location: &location},
	}, nil
}

// sessionLimitReached answers 503 with Retry-After. It is no server fault but worth a trace,
// like every 503 writeResponseError logs.
func (api *API) sessionLimitReached(ctx context.Context, err error) apigen.CreatePlaybackSessionResponseObject {
	api.logger.WarnContext(ctx, "playback session limit reached",
		slog.String("operation", operationCreatePlaybackSession),
		slog.String("request_id", requestid.FromContext(ctx)),
		slog.Any("error", err))

	retryAfter := sessionLimitRetryAfterSeconds

	return apigen.CreatePlaybackSession503ApplicationProblemPlusJSONResponse{
		Body:    NewProblem(http.StatusServiceUnavailable, CodeSessionLimitReached),
		Headers: apigen.CreatePlaybackSession503ResponseHeaders{RetryAfter: &retryAfter},
	}
}

// undescribableSession answers 500 for a started session that does not fit the contract (a
// broken ID generator). The client never learns its ID and cannot stop it, so it is stopped
// here instead of keeping its FFmpeg until the idle timeout; the stop must not depend on a
// client that may already have gone.
func (api *API) undescribableSession(ctx context.Context, id string, err error) apigen.CreatePlaybackSessionResponseObject {
	api.logOperationFailure(ctx, operationCreatePlaybackSession, err)

	if stopErr := api.playback.Stop(context.WithoutCancel(ctx), id); stopErr != nil {
		api.logOperationFailure(ctx, operationCreatePlaybackSession, fmt.Errorf("stop undescribable session: %w", stopErr))
	}

	return apigen.CreatePlaybackSessiondefaultApplicationProblemPlusJSONResponse{
		Body:       InternalProblem(),
		StatusCode: http.StatusInternalServerError,
	}
}

// logSessionStarted records one line per start: a start may launch FFmpeg, so operators
// want to see it. The client profile is never logged at info.
func (api *API) logSessionStarted(ctx context.Context, session appplayback.Session) {
	api.logger.InfoContext(ctx, "playback session started",
		slog.String("session", session.ID),
		slog.String("file", session.FileID),
		slog.String("mode", string(session.Plan.Mode)),
		slog.String("reasons", reasonCodes(session.Plan.Reasons)),
		slog.String("request_id", requestid.FromContext(ctx)))
}

func reasonCodes(reasons []playback.Reason) string {
	codes := make([]string, 0, len(reasons))
	for _, reason := range reasons {
		codes = append(codes, string(reason.Code))
	}

	return strings.Join(codes, reasonCodeSeparator)
}

// GetPlaybackSession answers 200 with the session or 404 for an unknown or stopped one.
func (api *API) GetPlaybackSession(ctx context.Context, request apigen.GetPlaybackSessionRequestObject) (apigen.GetPlaybackSessionResponseObject, error) {
	session, err := api.playback.Get(ctx, request.SessionID.String())
	if err != nil {
		return nil, fmt.Errorf("%s: %w", operationGetPlaybackSession, err)
	}

	body, err := newPlaybackSession(session)
	if err != nil {
		api.logOperationFailure(ctx, operationGetPlaybackSession, err)

		return apigen.GetPlaybackSessiondefaultApplicationProblemPlusJSONResponse{
			Body:       InternalProblem(),
			StatusCode: http.StatusInternalServerError,
		}, nil
	}

	return apigen.GetPlaybackSession200JSONResponse(body), nil
}

// StopPlaybackSession answers 204 also for an unknown or already stopped session (DELETE is
// idempotent). The log line says "requested" because it is written for those no-op stops
// too.
func (api *API) StopPlaybackSession(ctx context.Context, request apigen.StopPlaybackSessionRequestObject) (apigen.StopPlaybackSessionResponseObject, error) {
	id := request.SessionID.String()

	if err := api.playback.Stop(ctx, id); err != nil {
		return nil, fmt.Errorf("%s: %w", operationStopPlaybackSession, err)
	}

	api.logger.InfoContext(ctx, "playback session stop requested",
		slog.String("session", id),
		slog.String("request_id", requestid.FromContext(ctx)))

	return apigen.StopPlaybackSession204Response{}, nil
}
