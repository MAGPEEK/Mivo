package playback

// FFmpeg software encoders of the transcode targets. adapters/ffmpeg reports them in
// ServerCapabilities when FFmpeg lists them; hardware encoders are named by the FFmpeg build
// and arrive as HardwareEncoders.
const (
	EncoderLibx264 = "libx264"
	EncoderAAC     = "aac"
)

// ServerCapabilities is what the server's FFmpeg can do, detected once at startup by
// adapters/ffmpeg and never assumed (ADR 0015). The zero value is a server without FFmpeg:
// only direct play is possible.
type ServerCapabilities struct {
	// FFmpegVersion is shown in diagnostics; the decision does not depend on it.
	FFmpegVersion string
	// HLSMuxing reports that the hls and mp4 muxers exist, which remux and transcode need.
	HLSMuxing bool
	// FLACEncoder names the encoder of the lossless audio target (DSD and hi-res audio,
	// ADR 0030 section 4); when empty these sources are sent as AAC instead.
	FLACEncoder string
	// H264Encoder and AACEncoder name the encoders for the transcode targets; empty when
	// FFmpeg lacks them, for example a minimal build on a weak NAS.
	H264Encoder string
	AACEncoder  string
	// HardwareEncoders are the hardware encoders the FFmpeg build lists, each with the result
	// of its test encode (ADR 0030 §5). Empty on a server without a GPU, which is a normal
	// result and not a fault.
	HardwareEncoders []HardwareEncoder
}

// HardwareAPI is the hardware encoding API of an encoder (API HardwareAccelerationApi).
type HardwareAPI string

// The hardware encoding APIs of ADR 0030 §5.
const (
	HardwareAPINVENC HardwareAPI = "nvenc"
	HardwareAPIQSV   HardwareAPI = "qsv"
	HardwareAPIVAAPI HardwareAPI = "vaapi"
	HardwareAPIAMF   HardwareAPI = "amf"
)

// HardwareEncoder is one hardware encoder on one device, as tested by adapters/ffmpeg (API
// TranscoderDevice). Listing an encoder says nothing: a container without /dev/dri lists
// h264_vaapi too, so only Usable counts.
type HardwareEncoder struct {
	API HardwareAPI
	// Name is the FFmpeg encoder name, for example "h264_vaapi".
	Name string
	// Codec is the ffprobe codec name the encoder produces, "h264" or "hevc".
	Codec string
	// Device is the device the test ran on, for example a VAAPI render node; empty when the
	// API needs none.
	Device string
	// Usable reports that the test encode completed.
	Usable bool
	// Degraded reports that a session failed with this encoder before its first segment; it
	// stays set until the process restarts.
	Degraded bool
}

// Available reports whether sessions may use the encoder: it passed its test and no session
// has failed with it since.
func (encoder HardwareEncoder) Available() bool {
	return encoder.Usable && !encoder.Degraded
}
