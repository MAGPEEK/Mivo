package playback

// FFmpeg encoders milestone 2 transcodes with (software only; hardware encoders arrive in
// milestone 4). adapters/ffmpeg reports them in ServerCapabilities when FFmpeg lists them.
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
	// H264Encoder and AACEncoder name the encoders for the transcode targets; empty when
	// FFmpeg lacks them, for example a minimal build on a weak NAS.
	H264Encoder string
	AACEncoder  string
}
