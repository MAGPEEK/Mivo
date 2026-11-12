package restapi

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"uuid"

	"github.com/magpeek/mivo/internal/adapters/http/apigen"
	appplayback "github.com/magpeek/mivo/internal/app/playback"
	"github.com/magpeek/mivo/internal/app/transcoder"
	"github.com/magpeek/mivo/internal/app/trickplay"
	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/playback"
)

// Operation names of the session list, transcoder and trickplay endpoints, for logs.
const (
	operationListPlaybackSessions = "listPlaybackSessions"
	operationGetTranscoderStatus  = "getTranscoderStatus"
	operationTestTranscoder       = "testTranscoder"
	operationGetTrickplayManifest = "getTrickplayManifest"
	operationRequestTrickplay     = "requestTrickplay"
)

// SessionLister lists the sessions the viewer of ctx may see: an administrator all, anyone else
// the sessions of the own sign-in session. Implemented by app/playback.Service.
type SessionLister interface {
	List(ctx context.Context) ([]appplayback.SessionInfo, error)
}

// SessionItem is the catalog item a media file belongs to, for the session list.
type SessionItem struct {
	ID    string
	Title string
}

// SessionItemLookup names the item of a media file without a visibility restriction: the
// session list is an administrator's view and a session may outlive the profile that started
// it. An unknown file wraps domain.ErrNotFound.
type SessionItemLookup interface {
	ItemOfFile(ctx context.Context, fileID string) (SessionItem, error)
}

// TranscoderService is the port of the transcoder endpoints, implemented by
// app/transcoder.Service.
type TranscoderService interface {
	// Status returns the current hardware acceleration snapshot.
	Status() transcoder.Status
	// Test runs the hardware detection and blocks until it ends; transcoder.ErrBusy while
	// another run is active.
	Test(ctx context.Context) (transcoder.Status, error)
}

// TrickplayService is the port of the trickplay endpoints, implemented by
// app/trickplay.Service. Both operations check that the viewer of ctx may see the file.
type TrickplayService interface {
	// Manifest returns the layout of a file's trickplay or an error wrapping
	// domain.ErrNotFound.
	Manifest(ctx context.Context, fileID string) (trickplay.Manifest, error)
	// Request queues the generation; errors wrap domain.ErrNotFound, domain.ErrUnsupported or
	// domain.ErrUnavailable.
	Request(ctx context.Context, fileID string) error
}

// SubtitleTrackLister lists the subtitle files next to a media file as tracks. It is the port
// app/playback uses for its track decision, so the media file resource lists what a session
// can select.
type SubtitleTrackLister interface {
	Tracks(ctx context.Context, fileID string) ([]playback.SubtitleTrack, error)
}

// StreamingServices are the optional services of the session list, the transcoder, the trickplay
// and the subtitle track list (ADR 0030). A missing service makes its endpoints answer 501
// "not_implemented"; a missing Tracks lists only the subtitle streams inside the file.
// Sessions needs SessionItems for the titles.
type StreamingServices struct {
	Sessions     SessionLister
	SessionItems SessionItemLookup
	Transcoder   TranscoderService
	Trickplay    TrickplayService
	Tracks       SubtitleTrackLister
}

// ListPlaybackSessions answers the sessions the viewer may see, oldest first, with the item
// title from the catalog and never a path or file name (ADR 0030 section 7).
func (api *API) ListPlaybackSessions(ctx context.Context, _ apigen.ListPlaybackSessionsRequestObject) (apigen.ListPlaybackSessionsResponseObject, error) {
	if api.streaming.Sessions == nil || api.streaming.SessionItems == nil {
		problem := notImplementedProblem()

		return apigen.ListPlaybackSessionsdefaultApplicationProblemPlusJSONResponse{Body: problem, StatusCode: int(problem.Status)}, nil
	}

	infos, err := api.streaming.Sessions.List(ctx)
	if err != nil {
		return nil, operationFailure(operationListPlaybackSessions, err, http.StatusUnauthorized, http.StatusForbidden)
	}

	summaries := make([]apigen.PlaybackSessionSummary, 0, len(infos))

	for _, info := range infos {
		summary, err := api.sessionSummary(ctx, info)
		if err != nil {
			return nil, operationFailure(operationListPlaybackSessions, err)
		}

		summaries = append(summaries, summary)
	}

	return apigen.ListPlaybackSessions200JSONResponse(apigen.PlaybackSessionList{Sessions: summaries}), nil
}

// sessionSummary maps one session. A session whose item is gone from the catalog stays in the
// list, so an administrator can still see and stop it; it has an empty title and the nil item
// ID.
func (api *API) sessionSummary(ctx context.Context, info appplayback.SessionInfo) (apigen.PlaybackSessionSummary, error) {
	id, err := uuid.Parse(info.ID)
	if err != nil {
		return apigen.PlaybackSessionSummary{}, fmt.Errorf("convert session id %q: %w", info.ID, err)
	}

	item, err := api.sessionItem(ctx, info.FileID)
	if err != nil {
		return apigen.PlaybackSessionSummary{}, err
	}

	itemID := uuid.Nil()

	if item.ID != "" {
		if itemID, err = parseStoredID("item", item.ID); err != nil {
			return apigen.PlaybackSessionSummary{}, err
		}
	}

	summary := apigen.PlaybackSessionSummary{
		ID: id, ItemID: itemID, Title: item.Title, State: apigen.PlaybackSessionState(info.State),
		Mode: apigen.PlaybackMode(info.Mode), ClientName: info.ClientName, Hardware: info.Hardware,
		StartedAt: apiTimestamp(info.StartedAt), VideoEncoder: optionalString(info.Encoder),
		OutputBitRate: optionalInt64(info.OutputBitRate),
	}

	if info.Mode != playback.ModeDirectPlay {
		paused := info.Paused
		summary.Paused = &paused
		summary.TranscodeSpeed = optionalFloat(info.Speed)
	}

	return summary, nil
}

func (api *API) sessionItem(ctx context.Context, fileID string) (SessionItem, error) {
	item, err := api.streaming.SessionItems.ItemOfFile(ctx, fileID)

	switch {
	case err == nil:
		return item, nil
	case errors.Is(err, domain.ErrNotFound):
		api.logger.DebugContext(ctx, "session item is gone from the catalog", slog.String("file", fileID))

		return SessionItem{}, nil
	default:
		return SessionItem{}, fmt.Errorf("item of file %s: %w", fileID, err)
	}
}

// optionalFloat omits 0, which stands for "not reported".
func optionalFloat(value float64) *float64 {
	if value == 0 {
		return nil
	}

	return &value
}
