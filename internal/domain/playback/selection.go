package playback

import (
	"fmt"

	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/media"
)

// selection holds the streams a plan works with (ADR 0023, stream selection).
type selection struct {
	video    media.Stream
	hasVideo bool
	audio    media.Stream
	hasAudio bool
	// audioIsFirst is false when the selected audio stream is not the one a browser plays
	// from the original file: browsers cannot switch tracks of a progressive file, and Chrome,
	// Edge (Matroska) and Firefox (Matroska, MP4) play the first audio track whatever the
	// default flag says.
	audioIsFirst bool
	hasSubtitles bool
	// subtitle is the track to show, requested or picked by the track preference; the
	// delivery decision (sidecar or burn-in) is M4-04b.
	subtitle    SubtitleTrack
	hasSubtitle bool
}

// selectStreams picks the video stream (never cover art, default disposition preferred),
// the requested audio stream or the one the profile's track preference selects, and the
// requested or preferred subtitle. An unknown requested audio index or subtitle id is a
// bad request.
func selectStreams(input Input) (selection, error) {
	description := input.Description
	chosen := selection{hasSubtitles: hasSubtitles(description)}
	chosen.video, chosen.hasVideo = description.PrimaryVideo()

	audio, hasAudio, err := selectAudio(description, input.Request, input.Profile.Tracks)
	if err != nil {
		return selection{}, err
	}

	first, _ := firstAudio(description)
	chosen.audio, chosen.hasAudio = audio, hasAudio
	chosen.audioIsFirst = audio.Index == first.Index

	tracks := append(SubtitleTracks(description), input.ExternalSubtitles...)

	chosen.subtitle, chosen.hasSubtitle, err = selectSubtitle(tracks, audio.Language, input)
	if err != nil {
		return selection{}, err
	}

	return chosen, nil
}

// selectAudio returns the requested audio stream, or the preferred one without a request.
func selectAudio(description media.Description, request Request, preference TrackPreference) (media.Stream, bool, error) {
	if request.AudioStreamIndex == nil {
		stream, found := selectAudioStream(description, preference)

		return stream, found, nil
	}

	requested, err := description.AudioByIndex(*request.AudioStreamIndex)
	if err != nil {
		// AudioByIndex reports domain.ErrNotFound; for a request that names the stream this
		// is invalid input (HTTP 400), not a missing resource (404), so the cause is only
		// formatted with %v and never wrapped.
		return media.Stream{}, false, fmt.Errorf("select audio stream: %v: %w", err, domain.ErrInvalid) //nolint:errorlint // wrapping ErrNotFound would turn a bad request into a 404
	}

	return requested, true, nil
}

// selectSubtitle returns the requested subtitle track (an empty id is "off"), or the one the
// profile's track preference selects for the audio language without a request.
func selectSubtitle(tracks []SubtitleTrack, audioLanguage string, input Input) (SubtitleTrack, bool, error) {
	requested := input.Request.SubtitleTrackID
	if requested == nil {
		id, found := SelectSubtitleTrack(tracks, audioLanguage, input.Profile.Tracks)
		if !found {
			return SubtitleTrack{}, false, nil
		}

		requested = &id
	}

	if *requested == "" {
		return SubtitleTrack{}, false, nil
	}

	track, err := findSubtitleTrack(tracks, *requested)
	if err != nil {
		return SubtitleTrack{}, false, fmt.Errorf("select subtitle track: %w", err)
	}

	return track, true, nil
}

// firstAudio returns the first audio stream in file order, the one browsers play.
func firstAudio(description media.Description) (media.Stream, bool) {
	for _, stream := range description.Streams {
		if stream.Kind == media.StreamKindAudio {
			return stream, true
		}
	}

	return media.Stream{}, false
}

func hasSubtitles(description media.Description) bool {
	for _, stream := range description.Streams {
		if stream.Kind == media.StreamKindSubtitle {
			return true
		}
	}

	return false
}
