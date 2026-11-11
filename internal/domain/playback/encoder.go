package playback

import "github.com/magpeek/mivo/internal/domain/media"

// HardwareAcceleration is the server setting transcode.hwaccel (API
// HardwareAccelerationSetting). The zero value means HardwareAccelerationAuto.
type HardwareAcceleration string

// The values of transcode.hwaccel (ADR 0030 §5).
const (
	// HardwareAccelerationAuto takes the first available API in the order of autoAPIOrder.
	HardwareAccelerationAuto HardwareAcceleration = "auto"
	// HardwareAccelerationOff encodes in software whatever the server has.
	HardwareAccelerationOff   HardwareAcceleration = "off"
	HardwareAccelerationVAAPI HardwareAcceleration = "vaapi"
	HardwareAccelerationQSV   HardwareAcceleration = "qsv"
	HardwareAccelerationNVENC HardwareAcceleration = "nvenc"
	HardwareAccelerationAMF   HardwareAcceleration = "amf"
)

// valid reports a value of transcode.hwaccel; the zero value counts as "auto".
func (setting HardwareAcceleration) valid() bool {
	switch setting {
	case "", HardwareAccelerationAuto, HardwareAccelerationOff, HardwareAccelerationVAAPI,
		HardwareAccelerationQSV, HardwareAccelerationNVENC, HardwareAccelerationAMF:
		return true
	default:
		return false
	}
}

// ReasonHardwareAccelerationUnavailable says that the administrator named a hardware API
// that has no available encoder, so the video is encoded in software. It informs and is not
// a fault.
const ReasonHardwareAccelerationUnavailable ReasonCode = "hwaccel_unavailable"

// Encoder is the video encoder of a transcode.
type Encoder struct {
	// Name is the FFmpeg encoder name, for example "libx264" or "h264_vaapi". It is empty
	// when the server has neither an available hardware encoder nor a software one.
	Name string
	// Hardware reports a hardware encoder; API and Device are set only then.
	Hardware bool
	API      HardwareAPI
	Device   string
	// FallbackReason is set when software stands in for the hardware API the setting names.
	FallbackReason ReasonCode
}

// Reasons returns the fallback reason for the plan, scoped to the video; none when the
// encoder is what the setting asked for.
func (encoder Encoder) Reasons() []Reason {
	if encoder.FallbackReason == "" {
		return nil
	}

	return []Reason{{Code: encoder.FallbackReason, Scope: ScopeVideo}}
}

// Apply names the encoder in the decision of a transcoded video stream. Copied streams and
// audio are returned unchanged, because they have no video encoder. Decide already applies
// the encoder of Policy.HardwareAcceleration; Apply is for a caller that replaces it later,
// for example after a hardware encoder failed.
func (encoder Encoder) Apply(decision StreamDecision) StreamDecision {
	if decision.Kind != media.StreamKindVideo || decision.Action != ActionTranscode {
		return decision
	}

	decision.Encoder = encoder.Name
	decision.Hardware = encoder.Hardware

	return decision
}

// autoAPIOrder is the preference of HardwareAccelerationAuto: dedicated GPUs first, then
// Intel's own API before the generic VAAPI, AMF last (ADR 0030 §5).
func autoAPIOrder() []HardwareAPI {
	return []HardwareAPI{HardwareAPINVENC, HardwareAPIQSV, HardwareAPIVAAPI, HardwareAPIAMF}
}

// ChooseVideoEncoder names the H.264 encoder of a video transcode. Software is the baseline
// (ADR 0030 §1): it is the answer for "off" and whenever no hardware encoder is available.
// That is silent for "auto", where a server without a GPU is the normal case, and carries
// ReasonHardwareAccelerationUnavailable when the setting names an API, because the
// administrator then expects hardware.
func ChooseVideoEncoder(capabilities ServerCapabilities, setting HardwareAcceleration) Encoder {
	software := Encoder{Name: capabilities.H264Encoder}

	if setting == HardwareAccelerationOff {
		return software
	}

	if setting == HardwareAccelerationAuto || setting == "" {
		for _, api := range autoAPIOrder() {
			if encoder, found := availableEncoder(capabilities.HardwareEncoders, api); found {
				return encoder
			}
		}

		return software
	}

	// The setting names one API. An unknown value finds no encoder and falls back like an API
	// without hardware; platform/config rejects such values before they get here.
	if encoder, found := availableEncoder(capabilities.HardwareEncoders, HardwareAPI(setting)); found {
		return encoder
	}

	software.FallbackReason = ReasonHardwareAccelerationUnavailable

	return software
}

// availableEncoder returns the first available encoder of the API that produces the transcode
// target. HEVC encoders are skipped: milestone 4 transcodes to H.264 only (ADR 0023).
func availableEncoder(encoders []HardwareEncoder, api HardwareAPI) (Encoder, bool) {
	for _, candidate := range encoders {
		if candidate.API != api || candidate.Codec != TranscodeVideoCodec || !candidate.Available() {
			continue
		}

		return Encoder{Name: candidate.Name, Hardware: true, API: candidate.API, Device: candidate.Device}, true
	}

	return Encoder{}, false
}
