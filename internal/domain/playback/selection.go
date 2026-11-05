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
}

// selectStreams picks the video stream (never cover art, default disposition preferred) and
// the requested or default audio stream (default disposition, else the first). An unknown
// requested index is a bad request.
func selectStreams(description media.Description, request Request) (selection, error) {
	chosen := selection{hasSubtitles: hasSubtitles(description)}
	chosen.video, chosen.hasVideo = description.PrimaryVideo()
	chosen.audio, chosen.hasAudio = description.DefaultAudio()
	first, _ := firstAudio(description)
	chosen.audioIsFirst = chosen.audio.Index == first.Index

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

	chosen.audioIsFirst = requested.Index == first.Index
	chosen.audio = requested

	return chosen, nil
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
