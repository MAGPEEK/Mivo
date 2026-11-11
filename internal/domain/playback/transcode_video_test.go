package playback_test

import (
	"errors"
	"slices"
	"testing"

	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/media"
	"github.com/magpeek/mivo/internal/domain/playback"
	"github.com/magpeek/mivo/internal/domain/playback/playbacktest"
)

func hardwareFallbackReason() playback.Reason {
	return playback.Reason{Code: playback.ReasonHardwareAccelerationUnavailable, Scope: playback.ScopeVideo}
}

// hevcOnlyInsideHLS plays HEVC Main 10 as a copy inside HLS, but not the H.264 transcode
// target, so a video it would copy cannot be re-encoded for it.
func hevcOnlyInsideHLS() playback.ClientProfile {
	profile := playbacktest.ChromiumDesktop()
	profile.HLS.VideoCodecs = []string{"hevc"}
	profile.CodecLimits = nil
	profile.SubtitleFormats = []string{playback.SubtitleFormatWebVTT}

	return profile
}

// TestDecideBurnInOfACopyableVideoWithoutAPlayableTarget: the video alone would be copied, the
// burn-in needs a re-encode, and the client plays no H.264 inside HLS. That is a client limit.
func TestDecideBurnInOfACopyableVideoWithoutAPlayableTarget(t *testing.T) {
	t.Parallel()

	input := playback.Input{
		Description:  clip(media.ContainerMKV, hevcMain10(0), audio(1, "aac"), subtitle(2, pgsCodec)),
		Profile:      hevcOnlyInsideHLS(),
		Capabilities: playbacktest.FullCapabilities(),
		Request:      trackRequest("s:2"),
		Policy:       playback.Policy{MaxTranscodeHeight: referenceMaxHeight},
	}

	withoutSubtitle := input
	withoutSubtitle.Request = playback.Request{}

	if video, _ := mustDecide(t, withoutSubtitle).Video(); video.Action != playback.ActionCopy {
		t.Fatalf("video without the subtitle = %s, want a copy", video.Action)
	}

	t.Run("refuses", func(t *testing.T) {
		t.Parallel()

		if _, err := playback.Decide(input); !errors.Is(err, playback.ErrNoCompatibleFormat) {
			t.Fatalf("Decide() error = %v, want %v", err, playback.ErrNoCompatibleFormat)
		}
	})

	t.Run("degrades when allowed", func(t *testing.T) {
		t.Parallel()

		degrading := input
		degrading.Request.AllowDegradeSubtitles = true

		plan := mustDecide(t, degrading)

		if video, _ := plan.Video(); video.Action != playback.ActionCopy {
			t.Errorf("video = %s, want a copy", video.Action)
		}

		if plan.Subtitle != subtitlesOff() || !slices.Contains(plan.Warnings, burnInUnavailableWarning()) {
			t.Errorf("Subtitle = %+v, Warnings = %v, want off with the burn-in warning", plan.Subtitle, plan.Warnings)
		}
	})
}

// TestDecideBurnInCarriesTheBitrateCap: a cap above the source does not re-encode the video,
// but a video that is re-encoded for a burn-in is capped and sized like every transcode.
func TestDecideBurnInCarriesTheBitrateCap(t *testing.T) {
	t.Parallel()

	const (
		sourceBitrate = 2 * megabit
		bitrateCap    = 4 * megabit
	)

	input := referenceInput(clip(media.ContainerMP4,
		sized(fullHDWidth, fullHDHeight, sourceBitrate), audio(1, "aac"), subtitle(2, pgsCodec)))
	input.Request = trackRequest("s:2")
	input.Request.MaxBitrate = bitrateCap

	plan := mustDecide(t, input)
	video, _ := plan.Video()

	if video.Action != playback.ActionTranscode || video.MaxBitrate != bitrateCap {
		t.Errorf("video = %s with cap %d, want a transcode capped at %d", video.Action, video.MaxBitrate, int64(bitrateCap))
	}

	if video.TargetHeight != hdHeight || !video.Downscaled {
		t.Errorf("height = %d (downscaled %t), want the %d of the ladder step", video.TargetHeight, video.Downscaled, hdHeight)
	}

	if !slices.Equal(plan.Reasons, []playback.Reason{burnInReason()}) {
		t.Errorf("Reasons = %v, want only the burn-in", plan.Reasons)
	}
}

// TestDecideNamesTheVideoEncoderOfTheHardwareSetting: the encoder of a transcoded video comes
// from Policy.HardwareAcceleration, for a codec transcode and for a burn-in alike.
func TestDecideNamesTheVideoEncoderOfTheHardwareSetting(t *testing.T) {
	t.Parallel()

	withNVENC := softwareCapabilities(usableEncoder(playback.HardwareAPINVENC, encoderNVENC))
	hardwareOnly := withNVENC
	hardwareOnly.H264Encoder = ""

	codecTranscode := clip(media.ContainerMKV, simpleVideo(0, "mpeg2video"), audio(1, "aac"))
	burnInSource := mp4With(subtitle(2, pgsCodec))

	tests := []struct {
		name         string
		description  media.Description
		capabilities playback.ServerCapabilities
		setting      playback.HardwareAcceleration
		request      playback.Request
		encoder      string
		hardware     bool
		fallback     bool
	}{
		{name: "auto takes the hardware encoder", description: codecTranscode, capabilities: withNVENC, encoder: encoderNVENC, hardware: true},
		{
			name: "off stays in software", description: codecTranscode, capabilities: withNVENC,
			setting: playback.HardwareAccelerationOff, encoder: playback.EncoderLibx264,
		},
		{
			name: "a named API without an encoder falls back with a reason", description: codecTranscode,
			capabilities: withNVENC, setting: playback.HardwareAccelerationVAAPI,
			encoder: playback.EncoderLibx264, fallback: true,
		},
		{
			name: "a server without a GPU encodes in software", description: codecTranscode,
			capabilities: softwareCapabilities(), encoder: playback.EncoderLibx264,
		},
		{
			name: "a burn-in uses the hardware encoder", description: burnInSource, capabilities: withNVENC,
			request: trackRequest("s:2"), encoder: encoderNVENC, hardware: true,
		},
		{
			name: "a burn-in needs no software encoder", description: burnInSource, capabilities: hardwareOnly,
			request: trackRequest("s:2"), encoder: encoderNVENC, hardware: true,
		},
		{
			name: "a burn-in falls back with a reason", description: burnInSource, capabilities: withNVENC,
			setting: playback.HardwareAccelerationQSV, request: trackRequest("s:2"),
			encoder: playback.EncoderLibx264, fallback: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			input := referenceInput(test.description)
			input.Capabilities = test.capabilities
			input.Request = test.request
			input.Policy.HardwareAcceleration = test.setting

			plan := mustDecide(t, input)
			video, _ := plan.Video()

			if video.Action != playback.ActionTranscode || video.Encoder != test.encoder || video.Hardware != test.hardware {
				t.Errorf("video = %s with %q (hardware %t), want a transcode with %q (hardware %t)",
					video.Action, video.Encoder, video.Hardware, test.encoder, test.hardware)
			}

			if slices.Contains(plan.Reasons, hardwareFallbackReason()) != test.fallback {
				t.Errorf("Reasons = %v, want the hardware fallback: %t", plan.Reasons, test.fallback)
			}
		})
	}
}

// TestDecideCopiesAreNeverHardware: a copied video has no encoder, whatever the server has.
func TestDecideCopiesAreNeverHardware(t *testing.T) {
	t.Parallel()

	input := referenceInput(clip(media.ContainerMKV, h264High(0), audio(1, "aac")))
	input.Capabilities = softwareCapabilities(usableEncoder(playback.HardwareAPINVENC, encoderNVENC))

	video, _ := mustDecide(t, input).Video()

	if video.Action != playback.ActionCopy || video.Encoder != "" || video.Hardware {
		t.Errorf("video = %+v, want a copy without an encoder", video)
	}
}

func TestPolicyValidateHardwareAcceleration(t *testing.T) {
	t.Parallel()

	known := []playback.HardwareAcceleration{
		"", playback.HardwareAccelerationAuto, playback.HardwareAccelerationOff, playback.HardwareAccelerationVAAPI,
		playback.HardwareAccelerationQSV, playback.HardwareAccelerationNVENC, playback.HardwareAccelerationAMF,
	}

	for _, setting := range known {
		if err := (playback.Policy{HardwareAcceleration: setting}).Validate(); err != nil {
			t.Errorf("Validate(%q) = %v, want no error", setting, err)
		}
	}

	if err := (playback.Policy{HardwareAcceleration: "cuda"}).Validate(); !errors.Is(err, domain.ErrInvalid) {
		t.Errorf("Validate(cuda) = %v, want %v", err, domain.ErrInvalid)
	}
}
