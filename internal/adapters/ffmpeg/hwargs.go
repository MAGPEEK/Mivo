package ffmpeg

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/playback"
)

// Hardware H.264 encode options of ADR 0030 §5. Every builder produces the same stream as the
// software path: High profile, 8-bit 4:2:0, the same scaling target and an IDR frame every
// hlsSegmentSeconds, so a segment boundary is a keyframe whichever encoder runs.
const (
	// hardwareQuality is the constant-quality value of the uncapped hardware encodes; it
	// matches libx264's default CRF in spirit (lower is better) without being identical.
	hardwareQuality = "23"
	// nvencPreset p4 is NVENC's balanced preset; amfQuality speed AMF's fastest quality mode.
	nvencPreset = "p4"
	amfQuality  = "speed"
	// forbiddenDeviceCharacters would end the value of -init_hw_device, which reads
	// "type=name:device,key=value": the device must not be able to add options.
	forbiddenDeviceCharacters = ",:"
)

// hardwareVideo is a hardware encoder chosen for a plan, with the device it was tested on.
type hardwareVideo struct {
	encoder playback.HardwareEncoder
	// uploadedInGraph is set when the burn-in graph already ends with the upload tail, so the
	// encoder arguments add no -vf of their own.
	uploadedInGraph bool
}

// uploadTail is the filter that moves frames to the device, for the end of a burn-in graph;
// empty for software and for encoders that take system-memory frames. Safe on nil.
func (video *hardwareVideo) uploadTail() string {
	if video == nil {
		return ""
	}

	return strings.Join(video.uploadFilter(), ",")
}

// findHardwareVideo looks up the encoder a plan names among the tested ones. Only a usable
// encoder counts: an untested one may hang or fail, and the plan must never name it.
func findHardwareVideo(encoders []playback.HardwareEncoder, name string) (hardwareVideo, error) {
	index := slices.IndexFunc(encoders, func(encoder playback.HardwareEncoder) bool {
		return encoder.Name == name && encoder.Usable
	})
	if index < 0 {
		return hardwareVideo{}, fmt.Errorf("video: hardware encoder %q is not usable on this server: %w",
			name, domain.ErrUnsupported)
	}

	found := encoders[index]
	if found.Codec != playback.TranscodeVideoCodec {
		return hardwareVideo{}, fmt.Errorf("video: hardware encoder %q produces %q: %w",
			name, found.Codec, domain.ErrUnsupported)
	}

	needsDevice := found.API == playback.HardwareAPIVAAPI || found.API == playback.HardwareAPIQSV
	if needsDevice && found.Device == "" {
		return hardwareVideo{}, fmt.Errorf("video: hardware encoder %q has no device: %w", name, domain.ErrInvalid)
	}

	if err := validateHardwareDevice(found.Device); err != nil {
		return hardwareVideo{}, err
	}

	return hardwareVideo{encoder: found}, nil
}

// hardwareVideo returns the hardware encoder of a hardware video transcode, nil when the video
// is copied, absent or software.
func (job HLSJob) hardwareVideo(streams hlsStreams) (*hardwareVideo, error) {
	decision := streams.video
	if decision == nil || decision.Action != playback.ActionTranscode || !decision.Hardware {
		return nil, nil
	}

	video, err := findHardwareVideo(job.HardwareEncoders, decision.Encoder)
	if err != nil {
		return nil, err
	}

	return &video, nil
}

// hardwareInputArguments are the options the hardware encode needs before the input.
func (settings encoderSettings) hardwareInputArguments() []string {
	if settings.hardware == nil {
		return nil
	}

	return settings.hardware.inputArguments()
}

// validateHardwareDevice rejects a device that could inject options into FFmpeg's device
// syntax ("," and ":"). The config accepts any absolute path, so the check lives here.
func validateHardwareDevice(device string) error {
	if strings.ContainsAny(device, forbiddenDeviceCharacters) {
		return fmt.Errorf("hardware device %q contains one of %q: %w", device, forbiddenDeviceCharacters, domain.ErrInvalid)
	}

	return nil
}

// inputArguments are the options before -i: the device of the chain and, when the encoder's
// test run verified it, hardware decoding. Decoded frames come back to system memory, so the
// scaling and format filters of the software path stay in use.
func (video hardwareVideo) inputArguments() []string {
	device := video.encoder.Device

	var arguments []string

	switch video.encoder.API {
	case playback.HardwareAPIVAAPI:
		arguments = []string{"-vaapi_device", device}
		if video.encoder.Decode {
			arguments = append(arguments, "-hwaccel", "vaapi", "-hwaccel_device", device)
		}
	case playback.HardwareAPIQSV:
		// On Linux QSV sits on top of the VAAPI driver, derived from the render node.
		arguments = []string{
			"-init_hw_device", "vaapi=va:" + device, "-init_hw_device", "qsv=" + testHardwareDeviceName + "@va",
			"-filter_hw_device", testHardwareDeviceName,
		}
		if video.encoder.Decode {
			arguments = append(arguments, "-hwaccel", "qsv", "-hwaccel_device", testHardwareDeviceName)
		}
	case playback.HardwareAPINVENC:
		if video.encoder.Decode {
			arguments = []string{"-hwaccel", "cuda"}
		}
	case playback.HardwareAPIAMF:
		// AMF opens its own session and decodes in software.
	}

	return arguments
}

// arguments returns the video options of a hardware transcode of decision: encoder, rate
// control, keyframes and the filter chain.
func (video hardwareVideo) arguments(decision playback.StreamDecision, audioBitRate int64) ([]string, error) {
	rate, err := video.rateArguments(decision.MaxBitrate, audioBitRate)
	if err != nil {
		return nil, err
	}

	arguments := []string{"-c:v", video.encoder.Name}
	arguments = append(arguments, video.preset()...)
	arguments = append(arguments, rate...)
	arguments = append(arguments, "-profile:v", x264Profile)
	arguments = append(arguments, video.pixelFormat()...)
	arguments = append(arguments, "-force_key_frames", fmt.Sprintf("expr:gte(t,n_forced*%d)", hlsSegmentSeconds))
	arguments = append(arguments, video.keyframeArguments()...)

	filter, err := video.filter(decision)
	if err != nil || filter == "" {
		return arguments, err
	}

	return append(arguments, "-vf", filter), nil
}

// preset returns the speed setting of the encoder that has one.
func (video hardwareVideo) preset() []string {
	switch video.encoder.API {
	case playback.HardwareAPINVENC:
		return []string{"-preset", nvencPreset}
	case playback.HardwareAPIAMF:
		return []string{"-quality", amfQuality}
	case playback.HardwareAPIQSV, playback.HardwareAPIVAAPI:
		return nil
	}

	return nil
}

// pixelFormat pins the 8-bit 4:2:0 output of the encoders that take system-memory frames; QSV
// and VAAPI get it from the format filter before the upload.
func (video hardwareVideo) pixelFormat() []string {
	switch video.encoder.API {
	case playback.HardwareAPINVENC, playback.HardwareAPIAMF:
		return []string{"-pix_fmt", transcodePixelFormat}
	case playback.HardwareAPIQSV, playback.HardwareAPIVAAPI:
		return nil
	}

	return nil
}

// keyframeArguments make the forced keyframes real IDR frames and keep the encoder from adding
// scene-cut keyframes of its own (libx264's -sc_threshold 0), where the encoder has the option.
func (video hardwareVideo) keyframeArguments() []string {
	switch video.encoder.API {
	case playback.HardwareAPINVENC:
		return []string{"-forced-idr", "1", "-no-scenecut", "1"}
	case playback.HardwareAPIQSV, playback.HardwareAPIAMF:
		return []string{"-forced_idr", "1"}
	case playback.HardwareAPIVAAPI:
		// VAAPI turns a frame forced to be an I frame into an IDR frame by itself.
		return nil
	}

	return nil
}

// rateArguments applies the bit-rate cap through the shared bitrateArguments/videoBudget: the
// hardware encoder gets the budget as its target and bound, and keeps constant quality
// underneath where the API has one. An uncapped job runs at constant quality alone.
func (video hardwareVideo) rateArguments(capBitsPerSecond, audioBitsPerSecond int64) ([]string, error) {
	limits, err := bitrateArguments(capBitsPerSecond, audioBitsPerSecond)
	if err != nil {
		return nil, err
	}

	if len(limits) == 0 {
		return video.uncappedRate(), nil
	}

	budget := strconv.FormatInt(videoBudget(capBitsPerSecond, audioBitsPerSecond)/bitsPerKilobit, 10) + "k"
	target := append([]string{"-b:v", budget}, limits...)

	switch video.encoder.API {
	case playback.HardwareAPINVENC:
		return append([]string{"-rc", "vbr", "-cq", hardwareQuality}, target...), nil
	case playback.HardwareAPIQSV:
		return append([]string{"-global_quality", hardwareQuality}, target...), nil
	case playback.HardwareAPIVAAPI:
		// The constant-quality mode of VAAPI ignores a rate limit, so a cap needs VBR.
		return append([]string{"-rc_mode", "VBR"}, target...), nil
	case playback.HardwareAPIAMF:
		return append([]string{"-rc", "vbr_peak"}, target...), nil
	}

	return target, nil
}

// uncappedRate is the constant-quality setting of each API.
func (video hardwareVideo) uncappedRate() []string {
	switch video.encoder.API {
	case playback.HardwareAPINVENC:
		// -b:v 0 lets -cq decide; NVENC otherwise limits the rate to its default target.
		return []string{"-rc", "vbr", "-cq", hardwareQuality, "-b:v", "0"}
	case playback.HardwareAPIQSV:
		return []string{"-global_quality", hardwareQuality}
	case playback.HardwareAPIVAAPI:
		return []string{"-qp", hardwareQuality}
	case playback.HardwareAPIAMF:
		return []string{"-rc", "cqp", "-qp_i", hardwareQuality, "-qp_p", hardwareQuality, "-qp_b", hardwareQuality}
	}

	return nil
}

// filter returns the video filter chain: the software scale of the plan, then the upload to
// the device for the encoders that read device frames. Empty when nothing is needed.
func (video hardwareVideo) filter(decision playback.StreamDecision) (string, error) {
	// The burn-in graph scales and uploads by itself (-filter_complex excludes a second -vf).
	if video.uploadedInGraph {
		return "", nil
	}

	var chain []string

	if decision.ScaleRequired {
		scale, err := scaleFilter(decision.TargetWidth, decision.TargetHeight)
		if err != nil {
			return "", err
		}

		chain = append(chain, scale)
	}

	chain = append(chain, video.uploadFilter()...)

	return strings.Join(chain, ","), nil
}

// uploadFilter converts to 8-bit 4:2:0 and moves the frames to the device.
func (video hardwareVideo) uploadFilter() []string {
	switch video.encoder.API {
	case playback.HardwareAPIVAAPI:
		return []string{testSurfaceFilter}
	case playback.HardwareAPIQSV:
		return []string{testSurfaceFilterQSV}
	case playback.HardwareAPINVENC, playback.HardwareAPIAMF:
		return nil
	}

	return nil
}
