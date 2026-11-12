package restapi

import (
	"context"
	"fmt"
	"log/slog"
	"slices"

	"github.com/magpeek/mivo/internal/adapters/http/apigen"
	applibrary "github.com/magpeek/mivo/internal/app/library"
	"github.com/magpeek/mivo/internal/domain/library"
	"github.com/magpeek/mivo/internal/domain/media"
	"github.com/magpeek/mivo/internal/domain/playback"
)

// subtitleRoutePrefix and subtitleRouteSuffix frame the WebVTT route of a track
// (getSubtitleTrack, served by the stream adapter). The track list carries the URL so clients
// never build it (ADR 0024).
const (
	subtitleRoutePrefix = "/api/v1/media-files/"
	subtitleRouteInfix  = "/subtitles/"
	subtitleRouteSuffix = ".vtt"
)

// applyTracks sets what a player needs to offer tracks: the subtitle tracks (the streams inside
// the file, then the external ones), the default audio stream and the default subtitle of the
// server's policy without any profile preference (ADR 0030 sections 3 and 4), and the size of
// the video. An external list that failed to load is empty here, so the default is the one a
// session without external subtitles would pick.
func applyTracks(response *apigen.MediaFile, file media.File, external []playback.SubtitleTrack) error {
	description := file.Description
	tracks := slices.Concat(playback.SubtitleTracks(description), external)
	listed := make([]apigen.SubtitleTrack, 0, len(tracks))

	for _, track := range tracks {
		listed = append(listed, newSubtitleTrack(response.ID.String(), track))
	}

	response.SubtitleTracks = &listed

	if video, found := description.PrimaryVideo(); found {
		response.Width = optionalInt32(video.Width)
		response.Height = optionalInt32(video.Height)
		response.Resolution = resolutionResponse(video.Width, video.Height)
	}

	audioIndex, hasAudio := playback.SelectAudio(description, playback.TrackPreference{})
	if !hasAudio {
		return nil
	}

	index, err := streamIndex(audioIndex)
	if err != nil {
		return fmt.Errorf("default audio stream of file %s: %w", file.ID, err)
	}

	response.DefaultAudioStreamIndex = &index

	if id, shown := playback.SelectSubtitleTrack(tracks, audioLanguage(description, audioIndex), playback.TrackPreference{}); shown {
		response.DefaultSubtitleTrackID = &id
	}

	return nil
}

// resolutionResponse classifies a video, and omits an unknown size.
func resolutionResponse(width, height int) *apigen.VideoResolution {
	if width <= 0 && height <= 0 {
		return nil
	}

	resolution := apigen.VideoResolution(library.ResolutionOf(width, height))

	return &resolution
}

func audioLanguage(description media.Description, index int) string {
	for _, stream := range description.Streams {
		if stream.Index == index {
			return stream.Language
		}
	}

	return ""
}

// newSubtitleTrack lists a track with the route of its WebVTT rendition; a picture track has
// none, because only burn-in can show it.
func newSubtitleTrack(fileID string, track playback.SubtitleTrack) apigen.SubtitleTrack {
	listed := apigen.SubtitleTrack{
		ID: track.ID, Language: optionalString(track.Language), Title: optionalString(track.Title),
		Format: track.Format, Forced: track.Forced, Sdh: track.SDH, Default: track.Default,
		External: track.External, TextBased: track.TextBased,
	}

	if track.TextBased {
		url := subtitleRoutePrefix + fileID + subtitleRouteInfix + track.ID + subtitleRouteSuffix
		listed.URL = &url
	}

	return listed
}

// applyExternalTracks adds the subtitle files next to each file to the answer. A failing lookup
// is logged and leaves the streams inside the file, because the item page must open even when
// the subtitle list is out of reach; the session start does fail on it, since its track IDs
// depend on the complete list.
func (api *API) applyExternalTracks(ctx context.Context, response *apigen.ItemDetail, detail applibrary.ItemDetail) {
	lister := api.streaming.Tracks
	if lister == nil {
		return
	}

	for index, itemFile := range detailItemFiles(detail) {
		if index >= len(response.Files) {
			return
		}

		external, err := lister.Tracks(ctx, itemFile.File.ID)
		if err != nil {
			api.logger.WarnContext(ctx, "external subtitle tracks unavailable",
				slog.String("file", itemFile.File.ID), slog.Any("error", err))

			continue
		}

		if len(external) == 0 {
			continue
		}

		if err := applyTracks(&response.Files[index], itemFile.File, external); err != nil {
			api.logger.WarnContext(ctx, "subtitle tracks not listed", slog.Any("error", err))
		}
	}
}
