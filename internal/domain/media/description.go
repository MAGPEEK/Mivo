package media

import (
	"fmt"
	"time"

	"github.com/magpeek/mivo/internal/domain"
)

// Description is the technical description of one media file as ffprobe saw it.
type Description struct {
	Container Container
	// FormatName is ffprobe's raw format_name (for example "matroska,webm"), kept for
	// diagnostics and for the corpus checks.
	FormatName string
	// Duration is 0 when unknown.
	Duration time.Duration
	// BitRate is the overall bit rate in bits per second; 0 when unknown.
	BitRate int64
	// Streams are in ffprobe order.
	Streams []Stream
}

// Validate rejects descriptions that no probe can produce: an unknown container name,
// negative duration or bit rate, invalid streams and duplicate stream indexes. Errors wrap
// domain.ErrInvalid.
func (description Description) Validate() error {
	if _, err := ParseContainer(string(description.Container)); err != nil {
		return fmt.Errorf("validate description: %w", err)
	}

	if description.Duration < 0 {
		return fmt.Errorf("validate description: negative duration %s: %w",
			description.Duration, domain.ErrInvalid)
	}

	if description.BitRate < 0 {
		return fmt.Errorf("validate description: negative bit rate %d: %w",
			description.BitRate, domain.ErrInvalid)
	}

	return description.validateStreams()
}

func (description Description) validateStreams() error {
	seen := make(map[int]bool, len(description.Streams))

	for _, stream := range description.Streams {
		if err := stream.validate(); err != nil {
			return fmt.Errorf("validate description: %w", err)
		}

		if seen[stream.Index] {
			return fmt.Errorf("validate description: duplicate stream index %d: %w",
				stream.Index, domain.ErrInvalid)
		}

		seen[stream.Index] = true
	}

	return nil
}

// PrimaryVideo returns the video stream to play: the first one with the default
// disposition, else the first one. Attached pictures (cover art) never count as video.
// The second result is false when the file has no playable video stream.
func (description Description) PrimaryVideo() (Stream, bool) {
	return description.preferDefault(isPlayableVideo)
}

// DefaultAudio returns the audio stream a player picks without a user choice: the first one
// with the default disposition, else the first one. The second result is false when the
// file has no audio.
func (description Description) DefaultAudio() (Stream, bool) {
	return description.preferDefault(isAudio)
}

// AudioByIndex returns the audio stream with the given ffprobe index. A missing index or a
// stream of another kind wraps domain.ErrNotFound.
func (description Description) AudioByIndex(index int) (Stream, error) {
	for _, stream := range description.Streams {
		if stream.Index == index && isAudio(stream) {
			return stream, nil
		}
	}

	return Stream{}, fmt.Errorf("audio stream %d: %w", index, domain.ErrNotFound)
}

// preferDefault returns the first matching stream with the default disposition, else the
// first matching stream.
func (description Description) preferDefault(matches func(Stream) bool) (Stream, bool) {
	var (
		first      Stream
		foundFirst bool
	)

	for _, stream := range description.Streams {
		if !matches(stream) {
			continue
		}

		if stream.Default {
			return stream, true
		}

		if !foundFirst {
			first, foundFirst = stream, true
		}
	}

	return first, foundFirst
}

func isPlayableVideo(stream Stream) bool {
	return stream.Kind == StreamKindVideo && !stream.AttachedPicture
}

func isAudio(stream Stream) bool {
	return stream.Kind == StreamKindAudio
}
