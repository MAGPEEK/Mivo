package restapi

import (
	"context"
	"fmt"
	"net/http"
	"uuid"

	"github.com/magpeek/mivo/internal/adapters/http/apigen"
	"github.com/magpeek/mivo/internal/app/activity"
	"github.com/magpeek/mivo/internal/app/scan"
)

const operationGetActivity = "getActivity"

// ActivityService is the port of the activity endpoint, implemented by app/activity.Service.
// It refuses everyone but administrators (domain.ErrForbidden) and reports a missing viewer
// as domain.ErrUnauthenticated.
type ActivityService interface {
	Snapshot(ctx context.Context) (activity.Snapshot, error)
}

// GetActivity answers the running scans and playback sessions. It never carries a file
// name, path or stream URL: a session names its item by the catalog title only.
func (api *API) GetActivity(ctx context.Context, _ apigen.GetActivityRequestObject) (apigen.GetActivityResponseObject, error) {
	if api.activity == nil {
		problem := notImplementedProblem()

		return apigen.GetActivitydefaultApplicationProblemPlusJSONResponse{Body: problem, StatusCode: int(problem.Status)}, nil
	}

	snapshot, err := api.activity.Snapshot(ctx)
	if err != nil {
		return nil, operationFailure(operationGetActivity, err, http.StatusUnauthorized, http.StatusForbidden)
	}

	body, err := activityResponse(snapshot)
	if err != nil {
		return nil, operationFailure(operationGetActivity, err)
	}

	return apigen.GetActivity200JSONResponse(body), nil
}

func activityResponse(snapshot activity.Snapshot) (apigen.ActivityStatus, error) {
	scans := make([]apigen.ScanActivity, 0, len(snapshot.Scans))

	for _, running := range snapshot.Scans {
		converted, err := scanActivityResponse(running)
		if err != nil {
			return apigen.ActivityStatus{}, err
		}

		scans = append(scans, converted)
	}

	sessions := make([]apigen.ActivitySession, 0, len(snapshot.Sessions))

	for _, session := range snapshot.Sessions {
		converted, err := activitySessionResponse(session)
		if err != nil {
			return apigen.ActivityStatus{}, err
		}

		sessions = append(sessions, converted)
	}

	return apigen.ActivityStatus{Scans: scans, Sessions: sessions}, nil
}

func scanActivityResponse(running scan.Running) (apigen.ScanActivity, error) {
	id, err := parseStoredID("library", running.LibraryID)
	if err != nil {
		return apigen.ScanActivity{}, err
	}

	return apigen.ScanActivity{
		LibraryID: id, LibraryName: running.LibraryName, StartedAt: apiTimestamp(running.Status.StartedAt),
		Counts: scanCountsResponse(running.Status.Counts),
	}, nil
}

// activitySessionResponse maps one session. A session whose item is gone keeps the nil item
// ID and an empty title, as in the session list.
func activitySessionResponse(session activity.Session) (apigen.ActivitySession, error) {
	info := session.Info

	id, err := uuid.Parse(info.ID)
	if err != nil {
		return apigen.ActivitySession{}, fmt.Errorf("convert session id %q: %w", info.ID, err)
	}

	itemID := uuid.Nil()

	if session.ItemID != "" {
		if itemID, err = parseStoredID("item", session.ItemID); err != nil {
			return apigen.ActivitySession{}, err
		}
	}

	return apigen.ActivitySession{
		ID: id, ItemID: itemID, Title: session.Title, State: apigen.PlaybackSessionState(info.State),
		Mode: apigen.PlaybackMode(info.Mode), Reasons: newPlaybackReasons(info.Reasons),
		VideoEncoder: optionalString(info.Encoder), Hardware: info.Hardware,
		CPUPercent: optionalFloat(info.CPUPercent), ClientName: info.ClientName,
		ClientClass: clientClassResponse(info.Local), StartedAt: apiTimestamp(info.StartedAt),
	}, nil
}

func clientClassResponse(local bool) apigen.ActivityClientClass {
	if !local {
		return apigen.ActivityClientClassRemote
	}

	return apigen.ActivityClientClassLan
}
