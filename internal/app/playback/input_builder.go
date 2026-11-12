package playback

import (
	"context"
	"fmt"
	"slices"

	"github.com/magpeek/mivo/internal/domain/playback"
)

// inputBuilder assembles the decision input of one request: the file's embedded and external
// subtitle tracks, the client's choices and the server's capabilities and policy.
type inputBuilder struct {
	capabilities playback.ServerCapabilities
	policy       playback.Policy
	subtitles    SubtitleTracks
}

// build returns the input of request for file. The external tracks come from the optional
// SubtitleTracks port; a failing lookup fails the start, because the default subtitle policy
// and the track IDs of the request depend on the complete list.
func (builder inputBuilder) build(ctx context.Context, playable PlayableFile, request StartRequest) (playback.Input, error) {
	var external []playback.SubtitleTrack

	if builder.subtitles != nil {
		tracks, err := builder.subtitles.Tracks(ctx, playable.File.ID)
		if err != nil {
			return playback.Input{}, fmt.Errorf("load subtitle tracks: %w", err)
		}

		external = tracks
	}

	return playback.Input{
		Description:       playable.File.Description,
		Layout:            playable.File.Layout,
		ExternalSubtitles: external,
		Profile:           request.Profile,
		Capabilities:      builder.capabilities,
		Request: playback.Request{
			AudioStreamIndex:      request.AudioStreamIndex,
			SubtitleTrackID:       request.SubtitleTrackID,
			AllowDegradeSubtitles: request.AllowDegradeSubtitles,
			StartPosition:         request.StartPosition,
			MaxBitrate:            request.MaxBitrate,
		},
		Policy: builder.policy,
	}, nil
}

// withoutSubtitle decides input again with subtitles switched off and adds the warning that
// the requested burn-in did not happen. Used when the re-encoding limit blocks a burn-in.
func withoutSubtitle(input playback.Input) (playback.Plan, error) {
	off := ""
	input.Request.SubtitleTrackID = &off

	plan, err := playback.Decide(input)
	if err != nil {
		return playback.Plan{}, fmt.Errorf("decide without subtitle: %w", err)
	}

	plan.Warnings = append(plan.Warnings, playback.Warning{
		Code: playback.WarningSubtitleBurnInUnavailable, Scope: playback.ScopeSubtitle,
	})

	return plan, nil
}

// burnInTrack returns the track the plan draws into the video; nil when none.
func burnInTrack(input playback.Input, plan playback.Plan) *playback.SubtitleTrack {
	if plan.Subtitle.Mode != playback.SubtitleBurnIn {
		return nil
	}

	tracks := slices.Concat(playback.SubtitleTracks(input.Description), input.ExternalSubtitles)

	for _, track := range tracks {
		if track.ID == plan.Subtitle.TrackID {
			return &track
		}
	}

	return nil
}

// outputBitRate is what the client receives: the cap of a capped video transcode, else the
// source's own rate (0 when unknown).
func outputBitRate(plan playback.Plan, sourceBitRate int64) int64 {
	if video, found := plan.Video(); found && video.Action == playback.ActionTranscode && video.MaxBitrate > 0 {
		return video.MaxBitrate
	}

	return sourceBitRate
}
