package playback

import (
	"fmt"
	"time"

	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/media"
)

// Input is everything Decide considers.
type Input struct {
	Description  media.Description
	Profile      ClientProfile
	Capabilities ServerCapabilities
	Request      Request
	Policy       Policy
}

// Request carries the options of one playback request.
type Request struct {
	// AudioStreamIndex is the ffprobe index of the audio stream the user picked; nil plays
	// the default audio stream.
	AudioStreamIndex *int
	// StartPosition is where playback starts. It does not change the decision, but a
	// position outside the file makes the request invalid.
	StartPosition time.Duration
}

// Policy holds the operator's limits for transcoding.
type Policy struct {
	// MaxTranscodeHeight caps the height of transcoded video; 0 means no cap. The
	// configured default (OD-5: 1080) is applied by the composition root, not here.
	MaxTranscodeHeight int
}

// validate rejects input that breaks a rule; errors wrap domain.ErrInvalid.
func (input Input) validate() error {
	if err := input.Profile.Validate(); err != nil {
		return err
	}

	if err := input.Description.Validate(); err != nil {
		return err
	}

	if err := input.Request.validate(input.Description.Duration); err != nil {
		return err
	}

	// Repeated here as a guard: callers should already have rejected the policy at startup.
	return input.Policy.Validate()
}

// validate checks the start position against the probed duration; an unknown duration (0)
// cannot bound it.
func (request Request) validate(duration time.Duration) error {
	if request.StartPosition < 0 {
		return fmt.Errorf("negative start position %s: %w", request.StartPosition, domain.ErrInvalid)
	}

	if duration > 0 && request.StartPosition >= duration {
		return fmt.Errorf("start position %s at or beyond the duration %s: %w",
			request.StartPosition, duration, domain.ErrInvalid)
	}

	return nil
}

// Validate rejects a negative or odd MaxTranscodeHeight: the cap becomes the output height
// of H.264 4:2:0, which must be even. 0 (no cap) is valid. Callers validate the policy once
// when they are constructed, so a bad configuration fails at startup instead of at the
// first transcode. The error wraps domain.ErrInvalid.
func (policy Policy) Validate() error {
	if policy.MaxTranscodeHeight < noHeightCap || policy.MaxTranscodeHeight%dimensionAlignment != 0 {
		return fmt.Errorf("max transcode height %d must be 0 or a positive even number: %w",
			policy.MaxTranscodeHeight, domain.ErrInvalid)
	}

	return nil
}
