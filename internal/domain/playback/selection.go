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
	// audioIsDefault is false when the user picked an audio stream other than the one a
	// browser plays by default; browsers cannot switch tracks of a progressive file.
	audioIsDefault bool
	hasSubtitles   bool
}

// selectStreams picks the video stream (never cover art, default disposition preferred) and
// the requested or default audio stream. An unknown requested index is a bad request.
func selectStreams(description media.Description, request Request) (selection, error) {
	chosen := selection{audioIsDefault: true, hasSubtitles: hasSubtitles(description)}
	chosen.video, chosen.hasVideo = description.PrimaryVideo()
	chosen.audio, chosen.hasAudio = description.DefaultAudio()

	if request.AudioStreamIndex == nil {
		return chosen, nil
	}

	requested, err := description.AudioByIndex(*request.AudioStreamIndex)
	if err != nil {
		// AudioByIndex reports domain.ErrNotFound; for a request that names the stream this
		// is invalid input (HTTP 400), not a missing resource (404), so the cause is only
		// formatted with %v and never wrapped.
		return selection{}, fmt.Errorf("select audio stream: %v: %w", err, domain.ErrInvalid) //nolint:errorlint // wrapping ErrNotFound would turn a bad request into a 404
	}

	chosen.audioIsDefault = requested.Index == chosen.audio.Index
	chosen.audio = requested

	return chosen, nil
}

func hasSubtitles(description media.Description) bool {
	for _, stream := range description.Streams {
		if stream.Kind == media.StreamKindSubtitle {
			return true
		}
	}

	return false
}
