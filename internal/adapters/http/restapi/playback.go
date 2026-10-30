package restapi

import (
	"context"

	"github.com/magpeek/mivo/internal/adapters/http/apigen"
)

// Operation names of the playback session endpoints, as in api/openapi.yaml, for logs.
const (
	operationCreatePlaybackSession = "createPlaybackSession"
	operationGetPlaybackSession    = "getPlaybackSession"
	operationStopPlaybackSession   = "stopPlaybackSession"
)

// PlaybackService is the port of the playback session endpoints. It is a placeholder:
// M2-28 declares the methods the handlers need. Until then every session operation answers
// 501, and afterwards a nil service keeps doing so until M2-32 wires it. The stream
// delivery routes are not served here but mounted (Options.Mounts, M2-29).
type PlaybackService interface{}

// CreatePlaybackSession answers 501 until M2-28 implements it.
func (api *API) CreatePlaybackSession(ctx context.Context, _ apigen.CreatePlaybackSessionRequestObject) (apigen.CreatePlaybackSessionResponseObject, error) {
	return apigen.CreatePlaybackSessiondefaultApplicationProblemPlusJSONResponse(api.notImplemented(ctx, operationCreatePlaybackSession)), nil
}

// GetPlaybackSession answers 501 until M2-28 implements it.
func (api *API) GetPlaybackSession(ctx context.Context, _ apigen.GetPlaybackSessionRequestObject) (apigen.GetPlaybackSessionResponseObject, error) {
	return apigen.GetPlaybackSessiondefaultApplicationProblemPlusJSONResponse(api.notImplemented(ctx, operationGetPlaybackSession)), nil
}

// StopPlaybackSession answers 501 until M2-28 implements it.
func (api *API) StopPlaybackSession(ctx context.Context, _ apigen.StopPlaybackSessionRequestObject) (apigen.StopPlaybackSessionResponseObject, error) {
	return apigen.StopPlaybackSessiondefaultApplicationProblemPlusJSONResponse(api.notImplemented(ctx, operationStopPlaybackSession)), nil
}
