package playback_test

import (
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/magpeek/mivo/internal/domain/media"
	"github.com/magpeek/mivo/internal/domain/playback"
)

// Values of the generated corpus clips (scripts/gen-testmedia): 320x240, 2 s, stereo.
const (
	clipWidth    = 320
	clipHeight   = 240
	clipDuration = 2 * time.Second
	clipChannels = 2
	// referenceMaxHeight is the transcode cap the owner chose (OD-5 B).
	referenceMaxHeight = 1080
)

// outcome is the expected result of Decide for one input. An empty action means the stream
// kind is absent from the plan; a non-nil err means Decide must fail with it.
type outcome struct {
	mode     playback.Mode
	video    playback.Action
	audio    playback.Action
	reasons  []playback.Reason
	warnings []playback.Warning
	err      error
}

func directPlay(warnings ...playback.Warning) outcome {
	return outcome{
		mode: playback.ModeDirectPlay, video: playback.ActionCopy, audio: playback.ActionCopy,
		warnings: warnings,
	}
}

func remux(reasons ...playback.Reason) outcome {
	return outcome{
		mode: playback.ModeRemux, video: playback.ActionCopy, audio: playback.ActionCopy,
		reasons: reasons,
	}
}

func transcode(video, audio playback.Action, reasons ...playback.Reason) outcome {
	return outcome{mode: playback.ModeTranscode, video: video, audio: audio, reasons: reasons}
}

func failure(err error) outcome {
	return outcome{err: err}
}

func (expected outcome) withWarnings(warnings ...playback.Warning) outcome {
	expected.warnings = warnings

	return expected
}

const (
	copied     = playback.ActionCopy
	transcoded = playback.ActionTranscode
)

func containerReason() playback.Reason {
	return playback.Reason{Code: playback.ReasonContainerNotSupported, Scope: playback.ScopeContainer}
}

func videoCodecReason() playback.Reason {
	return playback.Reason{Code: playback.ReasonVideoCodecNotSupported, Scope: playback.ScopeVideo}
}

func videoProfileReason() playback.Reason {
	return playback.Reason{Code: playback.ReasonVideoProfileNotSupported, Scope: playback.ScopeVideo}
}

func videoBitDepthReason() playback.Reason {
	return playback.Reason{Code: playback.ReasonVideoBitDepthNotSupported, Scope: playback.ScopeVideo}
}

func audioCodecReason() playback.Reason {
	return playback.Reason{Code: playback.ReasonAudioCodecNotSupported, Scope: playback.ScopeAudio}
}

func audioTrackReason() playback.Reason {
	return playback.Reason{Code: playback.ReasonAudioTrackNotDefault, Scope: playback.ScopeAudio}
}

func hdrWarning() playback.Warning {
	return playback.Warning{Code: playback.WarningHDRNotToneMapped, Scope: playback.ScopeVideo}
}

func downscaleWarning() playback.Warning {
	return playback.Warning{Code: playback.WarningVideoDownscaled, Scope: playback.ScopeVideo}
}

func subtitlesWarning() playback.Warning {
	return playback.Warning{Code: playback.WarningSubtitlesNotSupported, Scope: playback.ScopeSubtitle}
}

// clip describes a corpus file with the given container and streams.
func clip(container media.Container, streams ...media.Stream) media.Description {
	return media.Description{Container: container, Duration: clipDuration, Streams: streams}
}

func video(index int, codec, pixelFormat string, bitDepth int) media.Stream {
	return media.Stream{
		Index: index, Kind: media.StreamKindVideo, Codec: codec, PixelFormat: pixelFormat,
		BitDepth: bitDepth, Width: clipWidth, Height: clipHeight, FrameRate: media.NewFrameRate(25, 1),
	}
}

func simpleVideo(index int, codec string) media.Stream {
	return video(index, codec, "yuv420p", 8)
}

func h264High(index int) media.Stream {
	stream := simpleVideo(index, "h264")
	stream.Profile = "High"

	return stream
}

func hevcMain10(index int) media.Stream {
	stream := video(index, "hevc", "yuv420p10le", 10)
	stream.Profile = "Main 10"

	return stream
}

func hevcHDR10(index int) media.Stream {
	stream := hevcMain10(index)
	stream.ColorPrimaries = "bt2020"
	stream.ColorTransfer = "smpte2084"
	stream.ColorSpace = "bt2020nc"
	stream.HDR = media.HDRFormatHDR10

	return stream
}

func audio(index int, codec string) media.Stream {
	return media.Stream{
		Index: index, Kind: media.StreamKindAudio, Codec: codec, Channels: clipChannels,
		SampleRate: 48000,
	}
}

func subtitle(index int, codec string) media.Stream {
	return media.Stream{Index: index, Kind: media.StreamKindSubtitle, Codec: codec}
}

func asDefault(stream media.Stream) media.Stream {
	stream.Default = true

	return stream
}

func audioIndex(index int) *int {
	return &index
}

// assertOutcome compares the parts of a plan the matrix specifies.
func assertOutcome(t *testing.T, plan playback.Plan, err error, want outcome) {
	t.Helper()

	if want.err != nil {
		if !errors.Is(err, want.err) {
			t.Fatalf("Decide() error = %v, want %v", err, want.err)
		}

		return
	}

	if err != nil {
		t.Fatalf("Decide() error = %v, want nil", err)
	}

	if plan.Mode != want.mode {
		t.Errorf("Mode = %q, want %q", plan.Mode, want.mode)
	}

	assertAction(t, "video", plan.Video, want.video)
	assertAction(t, "audio", plan.Audio, want.audio)

	if !slices.Equal(plan.Reasons, want.reasons) {
		t.Errorf("Reasons = %v, want %v", plan.Reasons, want.reasons)
	}

	if !slices.Equal(plan.Warnings, want.warnings) {
		t.Errorf("Warnings = %v, want %v", plan.Warnings, want.warnings)
	}
}

// assertAction checks the decision that lookup (plan.Video or plan.Audio) returns; an empty
// want means the plan must have no stream of that kind.
func assertAction(t *testing.T, kind string, lookup func() (playback.StreamDecision, bool), want playback.Action) {
	t.Helper()

	decision, found := lookup()

	if want == "" {
		if found {
			t.Errorf("%s decision = %+v, want none", kind, decision)
		}

		return
	}

	if !found || decision.Action != want {
		t.Errorf("%s action = %q (present %t), want %q", kind, decision.Action, found, want)
	}
}
