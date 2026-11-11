package playback_test

import (
	"reflect"
	"testing"

	"github.com/magpeek/mivo/internal/domain/media"
	"github.com/magpeek/mivo/internal/domain/playback"
)

const (
	encoderNVENC     = "h264_nvenc"
	encoderQSV       = "h264_qsv"
	encoderVAAPI     = "h264_vaapi"
	encoderAMF       = "h264_amf"
	encoderHEVCNVENC = "hevc_nvenc"
	renderDevice     = "/dev/dri/renderD128"
	secondDevice     = "/dev/dri/renderD129"
)

// usableEncoder is a hardware H.264 encoder that passed its test encode.
func usableEncoder(api playback.HardwareAPI, name string) playback.HardwareEncoder {
	return playback.HardwareEncoder{API: api, Name: name, Codec: playback.TranscodeVideoCodec, Usable: true}
}

func softwareCapabilities(encoders ...playback.HardwareEncoder) playback.ServerCapabilities {
	return playback.ServerCapabilities{
		HLSMuxing: true, H264Encoder: playback.EncoderLibx264, AACEncoder: playback.EncoderAAC,
		HardwareEncoders: encoders,
	}
}

func softwareEncoder() playback.Encoder {
	return playback.Encoder{Name: playback.EncoderLibx264}
}

func softwareFallback() playback.Encoder {
	return playback.Encoder{
		Name: playback.EncoderLibx264, FallbackReason: playback.ReasonHardwareAccelerationUnavailable,
	}
}

func hardwareEncoder(api playback.HardwareAPI, name string) playback.Encoder {
	return playback.Encoder{Name: name, Hardware: true, API: api}
}

// allUsable lists the APIs in the reverse of the auto order, so a test passes only when the
// order comes from the engine and not from the list.
func allUsable() []playback.HardwareEncoder {
	return []playback.HardwareEncoder{
		usableEncoder(playback.HardwareAPIAMF, encoderAMF),
		usableEncoder(playback.HardwareAPIVAAPI, encoderVAAPI),
		usableEncoder(playback.HardwareAPIQSV, encoderQSV),
		usableEncoder(playback.HardwareAPINVENC, encoderNVENC),
	}
}

func TestChooseVideoEncoderOnTheDS925PlusIsLibx264WithoutAReason(t *testing.T) {
	t.Parallel()

	// The owner's Synology DS925+ has no GPU: detection reports no hardware entries at all,
	// which is correct and must stay silent (ADR 0030 §5).
	capabilities := softwareCapabilities()

	for _, setting := range []playback.HardwareAcceleration{"", playback.HardwareAccelerationAuto} {
		got := playback.ChooseVideoEncoder(capabilities, setting)
		if got != softwareEncoder() {
			t.Errorf("setting %q: got %+v, want libx264 in software without a reason", setting, got)
		}

		if reasons := got.Reasons(); len(reasons) != 0 {
			t.Errorf("setting %q: reasons = %v, want none", setting, reasons)
		}
	}
}

func TestChooseVideoEncoder(t *testing.T) {
	t.Parallel()

	degraded := usableEncoder(playback.HardwareAPINVENC, encoderNVENC)
	degraded.Degraded = true

	failed := usableEncoder(playback.HardwareAPINVENC, encoderNVENC)
	failed.Usable = false

	hevc := playback.HardwareEncoder{
		API: playback.HardwareAPINVENC, Name: encoderHEVCNVENC, Codec: "hevc", Usable: true,
	}

	vaapiOnDevice := usableEncoder(playback.HardwareAPIVAAPI, encoderVAAPI)
	vaapiOnDevice.Device = renderDevice

	vaapiOnSecondDevice := vaapiOnDevice
	vaapiOnSecondDevice.Device = secondDevice

	failedVAAPI := vaapiOnDevice
	failedVAAPI.Usable = false

	vaapiResult := hardwareEncoder(playback.HardwareAPIVAAPI, encoderVAAPI)
	vaapiResult.Device = renderDevice

	vaapiSecondResult := vaapiResult
	vaapiSecondResult.Device = secondDevice

	tests := []struct {
		name     string
		encoders []playback.HardwareEncoder
		setting  playback.HardwareAcceleration
		want     playback.Encoder
	}{
		{name: "off ignores usable hardware", encoders: allUsable(), setting: playback.HardwareAccelerationOff, want: softwareEncoder()},
		{name: "off without hardware", setting: playback.HardwareAccelerationOff, want: softwareEncoder()},
		{name: "auto prefers NVENC", encoders: allUsable(), setting: playback.HardwareAccelerationAuto, want: hardwareEncoder(playback.HardwareAPINVENC, encoderNVENC)},
		{name: "unset setting is auto", encoders: allUsable(), want: hardwareEncoder(playback.HardwareAPINVENC, encoderNVENC)},
		{name: "auto takes QSV before VAAPI and AMF", encoders: allUsable()[:3], setting: playback.HardwareAccelerationAuto, want: hardwareEncoder(playback.HardwareAPIQSV, encoderQSV)},
		{name: "auto takes VAAPI before AMF", encoders: allUsable()[:2], setting: playback.HardwareAccelerationAuto, want: hardwareEncoder(playback.HardwareAPIVAAPI, encoderVAAPI)},
		{name: "auto takes AMF last", encoders: allUsable()[:1], setting: playback.HardwareAccelerationAuto, want: hardwareEncoder(playback.HardwareAPIAMF, encoderAMF)},
		{
			name:     "auto skips an encoder that failed its test",
			encoders: []playback.HardwareEncoder{failed, usableEncoder(playback.HardwareAPIQSV, encoderQSV)},
			setting:  playback.HardwareAccelerationAuto, want: hardwareEncoder(playback.HardwareAPIQSV, encoderQSV),
		},
		{
			name:     "auto skips a degraded encoder",
			encoders: []playback.HardwareEncoder{degraded, usableEncoder(playback.HardwareAPIAMF, encoderAMF)},
			setting:  playback.HardwareAccelerationAuto, want: hardwareEncoder(playback.HardwareAPIAMF, encoderAMF),
		},
		{name: "auto with only failed encoders is silent software", encoders: []playback.HardwareEncoder{failed, failedVAAPI}, setting: playback.HardwareAccelerationAuto, want: softwareEncoder()},
		{name: "auto with only a degraded encoder is silent software", encoders: []playback.HardwareEncoder{degraded}, setting: playback.HardwareAccelerationAuto, want: softwareEncoder()},
		{name: "auto never chooses HEVC", encoders: []playback.HardwareEncoder{hevc}, setting: playback.HardwareAccelerationAuto, want: softwareEncoder()},
		{
			name:     "auto takes H.264 of a later API over HEVC of an earlier one",
			encoders: []playback.HardwareEncoder{hevc, vaapiOnDevice},
			setting:  playback.HardwareAccelerationAuto, want: vaapiResult,
		},
		{name: "named API is chosen although auto would prefer another", encoders: allUsable(), setting: playback.HardwareAccelerationAMF, want: hardwareEncoder(playback.HardwareAPIAMF, encoderAMF)},
		{name: "named NVENC", encoders: allUsable(), setting: playback.HardwareAccelerationNVENC, want: hardwareEncoder(playback.HardwareAPINVENC, encoderNVENC)},
		{name: "named QSV", encoders: allUsable(), setting: playback.HardwareAccelerationQSV, want: hardwareEncoder(playback.HardwareAPIQSV, encoderQSV)},
		{name: "named VAAPI carries its device", encoders: []playback.HardwareEncoder{vaapiOnDevice}, setting: playback.HardwareAccelerationVAAPI, want: vaapiResult},
		{
			name:     "named API takes the first usable device",
			encoders: []playback.HardwareEncoder{failedVAAPI, vaapiOnSecondDevice},
			setting:  playback.HardwareAccelerationVAAPI, want: vaapiSecondResult,
		},
		{name: "named API without entries falls back with a reason", setting: playback.HardwareAccelerationVAAPI, want: softwareFallback()},
		{
			name:     "named API does not switch to another usable API",
			encoders: []playback.HardwareEncoder{usableEncoder(playback.HardwareAPINVENC, encoderNVENC)},
			setting:  playback.HardwareAccelerationVAAPI, want: softwareFallback(),
		},
		{name: "named API that failed its test falls back with a reason", encoders: []playback.HardwareEncoder{failed}, setting: playback.HardwareAccelerationNVENC, want: softwareFallback()},
		{name: "named API that is degraded falls back with a reason", encoders: []playback.HardwareEncoder{degraded}, setting: playback.HardwareAccelerationNVENC, want: softwareFallback()},
		{name: "named API with only HEVC falls back with a reason", encoders: []playback.HardwareEncoder{hevc}, setting: playback.HardwareAccelerationNVENC, want: softwareFallback()},
		{name: "unknown setting falls back with a reason", encoders: allUsable(), setting: "cuda", want: softwareFallback()},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			got := playback.ChooseVideoEncoder(softwareCapabilities(test.encoders...), test.setting)
			if got != test.want {
				t.Errorf("ChooseVideoEncoder() = %+v, want %+v", got, test.want)
			}
		})
	}
}

func TestChooseVideoEncoderWithoutSoftwareEncoder(t *testing.T) {
	t.Parallel()

	// A minimal FFmpeg build without libx264: the software result has no name, which the
	// engine reports as "transcode unavailable" instead of starting a session that fails.
	withoutLibx264 := playback.ServerCapabilities{HLSMuxing: true, AACEncoder: playback.EncoderAAC}

	if got := playback.ChooseVideoEncoder(withoutLibx264, playback.HardwareAccelerationAuto); got != (playback.Encoder{}) {
		t.Errorf("without any encoder: got %+v, want the zero Encoder", got)
	}

	withoutLibx264.HardwareEncoders = []playback.HardwareEncoder{usableEncoder(playback.HardwareAPIQSV, encoderQSV)}

	want := hardwareEncoder(playback.HardwareAPIQSV, encoderQSV)
	if got := playback.ChooseVideoEncoder(withoutLibx264, playback.HardwareAccelerationAuto); got != want {
		t.Errorf("hardware only: got %+v, want %+v", got, want)
	}
}

func TestEncoderReasons(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		encoder playback.Encoder
		want    []playback.Reason
	}{
		{name: "software by choice", encoder: softwareEncoder()},
		{name: "hardware", encoder: hardwareEncoder(playback.HardwareAPINVENC, encoderNVENC)},
		{name: "software as fallback", encoder: softwareFallback(), want: []playback.Reason{
			{Code: playback.ReasonHardwareAccelerationUnavailable, Scope: playback.ScopeVideo},
		}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			got := test.encoder.Reasons()
			if len(got) != len(test.want) {
				t.Fatalf("Reasons() = %v, want %v", got, test.want)
			}

			for index := range got {
				if got[index] != test.want[index] {
					t.Errorf("Reasons()[%d] = %v, want %v", index, got[index], test.want[index])
				}
			}
		})
	}
}

func TestEncoderApply(t *testing.T) {
	t.Parallel()

	transcoded := playback.StreamDecision{
		Kind: media.StreamKindVideo, Action: playback.ActionTranscode,
		SourceCodec: "hevc", TargetCodec: playback.TranscodeVideoCodec, Encoder: playback.EncoderLibx264,
	}

	got := hardwareEncoder(playback.HardwareAPIVAAPI, encoderVAAPI).Apply(transcoded)
	if got.Encoder != encoderVAAPI {
		t.Errorf("Encoder = %q, want %q", got.Encoder, encoderVAAPI)
	}

	if got.SourceCodec != transcoded.SourceCodec || got.TargetCodec != transcoded.TargetCodec {
		t.Errorf("Apply changed the codecs: %+v", got)
	}

	copied := playback.StreamDecision{Kind: media.StreamKindVideo, Action: playback.ActionCopy, SourceCodec: "h264", TargetCodec: "h264"}
	if got := hardwareEncoder(playback.HardwareAPIVAAPI, encoderVAAPI).Apply(copied); !reflect.DeepEqual(got, copied) {
		t.Errorf("Apply changed a copied stream: %+v", got)
	}

	audio := playback.StreamDecision{Kind: media.StreamKindAudio, Action: playback.ActionTranscode, Encoder: playback.EncoderAAC}
	if got := hardwareEncoder(playback.HardwareAPIVAAPI, encoderVAAPI).Apply(audio); !reflect.DeepEqual(got, audio) {
		t.Errorf("Apply changed an audio stream: %+v", got)
	}
}

func TestHardwareEncoderAvailable(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		encoder playback.HardwareEncoder
		want    bool
	}{
		{name: "tested and healthy", encoder: playback.HardwareEncoder{Usable: true}, want: true},
		{name: "failed its test", encoder: playback.HardwareEncoder{}},
		{name: "degraded after a failed session", encoder: playback.HardwareEncoder{Usable: true, Degraded: true}},
		{name: "failed and degraded", encoder: playback.HardwareEncoder{Degraded: true}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			if got := test.encoder.Available(); got != test.want {
				t.Errorf("Available() = %v, want %v", got, test.want)
			}
		})
	}
}
