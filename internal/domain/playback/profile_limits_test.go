package playback_test

import (
	"errors"
	"testing"

	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/playback"
	"github.com/magpeek/mivo/internal/domain/playback/playbacktest"
)

// Bounds of the audio limits and subtitle formats in api/openapi.yaml (ClientProfile).
const (
	minAudioChannels   = 1
	maxAudioChannels   = 32
	minAudioSampleRate = 8000
	maxAudioSampleRate = 768000
	minAudioBitDepth   = 8
	maxAudioBitDepth   = 64
	maxSubtitleFormats = 8
	omitted            = 0
)

// TestClientProfileValidateAudioLimitsAndSubtitleFormats: the bounds are checked in the domain
// so every adapter answers the same "invalid"; 0 is a field the client left out.
func TestClientProfileValidateAudioLimitsAndSubtitleFormats(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		modify  func(*playback.ClientProfile)
		invalid bool
	}{
		{name: "all omitted", modify: func(profile *playback.ClientProfile) {
			profile.MaxAudioChannels, profile.MaxAudioSampleRate, profile.MaxAudioBitDepth = omitted, omitted, omitted
			profile.SubtitleFormats = nil
		}},
		{name: "lowest limits", modify: func(profile *playback.ClientProfile) {
			profile.MaxAudioChannels = minAudioChannels
			profile.MaxAudioSampleRate = minAudioSampleRate
			profile.MaxAudioBitDepth = minAudioBitDepth
		}},
		{name: "highest limits", modify: func(profile *playback.ClientProfile) {
			profile.MaxAudioChannels = maxAudioChannels
			profile.MaxAudioSampleRate = maxAudioSampleRate
			profile.MaxAudioBitDepth = maxAudioBitDepth
		}},
		{name: "subtitle formats at the limit", modify: func(profile *playback.ClientProfile) {
			profile.SubtitleFormats = repeated("webvtt", maxSubtitleFormats)
		}},
		{name: "negative channels", invalid: true, modify: func(profile *playback.ClientProfile) {
			profile.MaxAudioChannels = -1
		}},
		{name: "too many channels", invalid: true, modify: func(profile *playback.ClientProfile) {
			profile.MaxAudioChannels = maxAudioChannels + 1
		}},
		{name: "sample rate below the range", invalid: true, modify: func(profile *playback.ClientProfile) {
			profile.MaxAudioSampleRate = minAudioSampleRate - 1
		}},
		{name: "negative sample rate", invalid: true, modify: func(profile *playback.ClientProfile) {
			profile.MaxAudioSampleRate = -1
		}},
		{name: "sample rate above the range", invalid: true, modify: func(profile *playback.ClientProfile) {
			profile.MaxAudioSampleRate = maxAudioSampleRate + 1
		}},
		{name: "bit depth below the range", invalid: true, modify: func(profile *playback.ClientProfile) {
			profile.MaxAudioBitDepth = minAudioBitDepth - 1
		}},
		{name: "negative bit depth", invalid: true, modify: func(profile *playback.ClientProfile) {
			profile.MaxAudioBitDepth = -1
		}},
		{name: "bit depth above the range", invalid: true, modify: func(profile *playback.ClientProfile) {
			profile.MaxAudioBitDepth = maxAudioBitDepth + 1
		}},
		{name: "too many subtitle formats", invalid: true, modify: func(profile *playback.ClientProfile) {
			profile.SubtitleFormats = repeated("webvtt", maxSubtitleFormats+1)
		}},
		{name: "subtitle format upper case", invalid: true, modify: func(profile *playback.ClientProfile) {
			profile.SubtitleFormats = []string{"WebVTT"}
		}},
		{name: "subtitle format empty", invalid: true, modify: func(profile *playback.ClientProfile) {
			profile.SubtitleFormats = []string{""}
		}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			profile := playbacktest.ChromiumDesktop()
			test.modify(&profile)

			err := profile.Validate()

			if test.invalid && !errors.Is(err, domain.ErrInvalid) {
				t.Errorf("Validate() = %v, want an error wrapping domain.ErrInvalid", err)
			}

			if !test.invalid && err != nil {
				t.Errorf("Validate() = %v, want nil", err)
			}
		})
	}
}
