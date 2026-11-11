package restapi

import (
	"context"

	"github.com/magpeek/mivo/internal/adapters/http/apigen"
)

// The session list, the transcoder status and test, and the trickplay manifest of ADR 0030
// sections 5, 7 and 8 answer 501 until their handlers exist (task M4-14b), so the admin and
// player screens can be built against the contract while the server keeps compiling. The
// management guard of ADR 0028 arrives with the handlers; a stub reveals nothing.

// ListPlaybackSessions is a stub until M4-14b.
func (api *API) ListPlaybackSessions(_ context.Context, _ apigen.ListPlaybackSessionsRequestObject) (apigen.ListPlaybackSessionsResponseObject, error) {
	problem := notImplementedProblem()

	return apigen.ListPlaybackSessionsdefaultApplicationProblemPlusJSONResponse{Body: problem, StatusCode: int(problem.Status)}, nil
}

// GetTranscoderStatus is a stub until M4-14b.
func (api *API) GetTranscoderStatus(_ context.Context, _ apigen.GetTranscoderStatusRequestObject) (apigen.GetTranscoderStatusResponseObject, error) {
	problem := notImplementedProblem()

	return apigen.GetTranscoderStatusdefaultApplicationProblemPlusJSONResponse{Body: problem, StatusCode: int(problem.Status)}, nil
}

// TestTranscoder is a stub until M4-14b.
func (api *API) TestTranscoder(_ context.Context, _ apigen.TestTranscoderRequestObject) (apigen.TestTranscoderResponseObject, error) {
	problem := notImplementedProblem()

	return apigen.TestTranscoderdefaultApplicationProblemPlusJSONResponse{Body: problem, StatusCode: int(problem.Status)}, nil
}

// GetTrickplayManifest is a stub until M4-14b.
func (api *API) GetTrickplayManifest(_ context.Context, _ apigen.GetTrickplayManifestRequestObject) (apigen.GetTrickplayManifestResponseObject, error) {
	problem := notImplementedProblem()

	return apigen.GetTrickplayManifestdefaultApplicationProblemPlusJSONResponse{Body: problem, StatusCode: int(problem.Status)}, nil
}

// RequestTrickplay is a stub until M4-14b.
func (api *API) RequestTrickplay(_ context.Context, _ apigen.RequestTrickplayRequestObject) (apigen.RequestTrickplayResponseObject, error) {
	problem := notImplementedProblem()

	return apigen.RequestTrickplaydefaultApplicationProblemPlusJSONResponse{Body: problem, StatusCode: int(problem.Status)}, nil
}
