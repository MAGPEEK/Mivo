package playback

import "github.com/magpeek/mivo/internal/domain/media"

// Mode is how a file is played, from cheapest to most expensive. The values are part of the
// API contract (PlaybackMode).
type Mode string

// The playback modes of ADR 0023.
const (
	// ModeDirectPlay serves the original file over HTTP with range requests.
	ModeDirectPlay Mode = "direct_play"
	// ModeRemux copies every selected stream into fMP4 HLS.
	ModeRemux Mode = "remux"
	// ModeTranscode re-encodes at least one selected stream, even if only the audio.
	ModeTranscode Mode = "transcode"
)

// Delivery is how the client loads the stream (API PlaybackDelivery).
type Delivery string

// The deliveries of ADR 0024.
const (
	// DeliveryFile is a progressive file with range support.
	DeliveryFile Delivery = "file"
	// DeliveryHLS is an HLS playlist with fMP4 segments.
	DeliveryHLS Delivery = "hls"
)

// Action is what happens to one selected source stream (API StreamAction).
type Action string

// The stream actions.
const (
	// ActionCopy passes the stream through unchanged.
	ActionCopy Action = "copy"
	// ActionTranscode re-encodes the stream.
	ActionTranscode Action = "transcode"
)

// Scope is the part of the file a reason or warning concerns (API PlaybackReasonScope).
type Scope string

// The scopes of reasons and warnings.
const (
	ScopeContainer Scope = "container"
	ScopeVideo     Scope = "video"
	ScopeAudio     Scope = "audio"
	ScopeSubtitle  Scope = "subtitle"
)

// ReasonCode explains why a cheaper mode was not possible. The codes are stable and
// translated by clients (ADR 0012); new codes may be added within API v1.
type ReasonCode string

// The reason codes of ADR 0023.
const (
	ReasonContainerNotSupported     ReasonCode = "container_not_supported"
	ReasonVideoCodecNotSupported    ReasonCode = "video_codec_not_supported"
	ReasonVideoProfileNotSupported  ReasonCode = "video_profile_not_supported"
	ReasonVideoBitDepthNotSupported ReasonCode = "video_bit_depth_not_supported"
	ReasonAudioCodecNotSupported    ReasonCode = "audio_codec_not_supported"
	// ReasonAudioChannelsNotSupported: the audio has more channels than the client plays
	// (ClientProfile.MaxAudioChannels, else DefaultMaxAudioChannels).
	ReasonAudioChannelsNotSupported ReasonCode = "audio_channels_not_supported"
	// ReasonAudioTrackNotFirst: the selected audio stream is not the first one, which is the
	// one browsers play from the original file whatever the default flag says.
	ReasonAudioTrackNotFirst ReasonCode = "audio_track_not_first"
	ReasonDiscSource         ReasonCode = "disc_source"
	// ReasonSubtitleBurnIn: the selected subtitle is drawn into the video, which is therefore
	// re-encoded (ADR 0030 §3). Plan.Subtitle names the track and its format.
	ReasonSubtitleBurnIn ReasonCode = "subtitle_burn_in"
)

// WarningCode describes a degraded result. Like reason codes, the values are stable.
type WarningCode string

// The warning codes of ADR 0023 and ADR 0030.
const (
	// WarningHDRNotToneMapped: an HDR source is transcoded without tone mapping until
	// milestone 4 and looks washed out.
	WarningHDRNotToneMapped WarningCode = "hdr_not_tone_mapped"
	// WarningSubtitleBurnInUnavailable: the requested subtitle needs burn-in, which the client
	// or the server cannot do; it stays off because the request allowed that.
	WarningSubtitleBurnInUnavailable WarningCode = "subtitle_burn_in_unavailable"
	// WarningVideoDownscaled: the transcoded video is smaller than the source (Policy).
	WarningVideoDownscaled WarningCode = "video_downscaled"
)

// Reason is one reason code with the part of the file it concerns.
type Reason struct {
	Code  ReasonCode
	Scope Scope
}

// Warning is one warning code with the part of the file it concerns.
type Warning struct {
	Code  WarningCode
	Scope Scope
}

// StreamDecision is what the plan does with one selected source stream.
type StreamDecision struct {
	Kind media.StreamKind
	// SourceIndex is ffprobe's stream index; FFmpeg maps the stream by it.
	SourceIndex int
	Action      Action
	// SourceCodec and TargetCodec are ffprobe codec names; they are equal when copied.
	SourceCodec string
	TargetCodec string
	// TargetWidth and TargetHeight are the video size the client receives; 0 when unknown
	// or for audio. A transcoded size is always even (H.264 4:2:0).
	TargetWidth  int
	TargetHeight int
	// Downscaled reports that a transcoded video is smaller than the source because of the
	// height cap (ADR 0023: only ever down, never up); it drives the video_downscaled warning.
	Downscaled bool
	// ScaleRequired reports that the encoder needs a scale filter to TargetWidth x
	// TargetHeight: after a downscale, or when odd source dimensions are rounded down to even
	// ones. A 0 dimension is derived by FFmpeg with the aspect ratio kept.
	ScaleRequired bool
	// MaxBitrate is the request's cap for the whole stream in bits per second on a transcoded
	// video (ADR 0030 section 6); 0 means constant quality without a cap. The adapter
	// subtracts the audio.
	MaxBitrate int64
	// TargetSampleRate is the sample rate in Hz a transcoded audio stream is resampled to; 0
	// keeps the rate of the source. TargetBitDepth is the sample depth of a lossless target;
	// 0 for a lossy target and for copies.
	TargetSampleRate int
	TargetBitDepth   int
	// TargetChannels is the channel count of an audio stream the client receives.
	TargetChannels int
	// Encoder is the FFmpeg encoder of a transcoded stream; empty when copied.
	Encoder string
	// Hardware reports that Encoder is a hardware video encoder (ChooseVideoEncoder); its API
	// and device are found by name in ServerCapabilities.HardwareEncoders.
	Hardware bool
}

// Plan is the result of Decide.
type Plan struct {
	Mode     Mode
	Delivery Delivery
	// SourceContainer is the container of the file; TargetContainer is what the client
	// receives: the source container for direct play, mp4 (fMP4 segments) for HLS.
	SourceContainer media.Container
	TargetContainer media.Container
	// Streams holds one decision per selected stream, video first, then audio.
	Streams []StreamDecision
	// Reasons explain why cheaper modes were not possible; empty for direct play.
	Reasons []Reason
	// Warnings describe degradations of the result.
	Warnings []Warning
	// Subtitle is how the selected subtitle is shown; its mode is never empty.
	Subtitle SubtitleDecision
}

// Video returns the decision for the selected video stream. The second result is false when
// the file has no video.
func (plan Plan) Video() (StreamDecision, bool) {
	return plan.streamOfKind(media.StreamKindVideo)
}

// Audio returns the decision for the selected audio stream. The second result is false when
// the file has no audio.
func (plan Plan) Audio() (StreamDecision, bool) {
	return plan.streamOfKind(media.StreamKindAudio)
}

func (plan Plan) streamOfKind(kind media.StreamKind) (StreamDecision, bool) {
	for _, decision := range plan.Streams {
		if decision.Kind == kind {
			return decision, true
		}
	}

	return StreamDecision{}, false
}
