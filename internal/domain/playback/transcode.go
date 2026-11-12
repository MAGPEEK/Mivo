package playback

import (
	"fmt"
	"slices"
	"strings"

	"github.com/magpeek/mivo/internal/domain/media"
)

// Transcode targets of milestone 2 (ADR 0023, OD-11 A): H.264 High 8-bit 4:2:0 and AAC LC
// play in every supported browser. adapters/ffmpeg chooses the matching arguments.
const (
	TranscodeVideoCodec    = "h264"
	TranscodeVideoProfile  = "High"
	TranscodeVideoBitDepth = 8
	TranscodeAudioCodec    = "aac"
	// TranscodeStereoChannels is the target of stereo audio and of audio whose layout the
	// probe could not tell; mono stays mono.
	TranscodeStereoChannels = 2
	// TranscodeSurroundChannels (5.1) is the target of every layout with more than two
	// channels: Chrome, Edge and Firefox play AAC 5.1 and downmix it themselves.
	TranscodeSurroundChannels = 6
	// TranscodeLosslessAudioCodec is the target of DSD and of lossless audio above the
	// client's limits when the client plays it inside HLS (ADR 0030 section 4); FLAC is PCM
	// the browsers accept in fMP4.
	TranscodeLosslessAudioCodec = "flac"
	// EncoderFLAC is FFmpeg's FLAC encoder, reported in ServerCapabilities.FLACEncoder.
	EncoderFLAC = "flac"
	// EncoderWebVTT is FFmpeg's WebVTT subtitle encoder, reported in
	// ServerCapabilities.WebVTTEncoder.
	EncoderWebVTT = "webvtt"
)

// The reasons of the audio targets (ADR 0030 Notes (M4-02a), (M4-02b)).
const (
	// ReasonDSDToPCM: DSD audio is always decoded to PCM.
	ReasonDSDToPCM ReasonCode = "dsd_to_pcm"
	// ReasonHiResDownsampled: the audio exceeds ClientProfile.MaxAudioSampleRate or
	// MaxAudioBitDepth and is resampled.
	ReasonHiResDownsampled ReasonCode = "hires_downsampled"
)

// DefaultMaxAudioChannels (5.1) is the channel limit of a profile that names none. Firefox 159
// cannot decode AAC 7.1 ("no decoder found for audio/mp4a-latm") although canPlayType answers
// "probably", so more channels are only copied for a client that asks for them
// (ClientProfile.MaxAudioChannels).
const DefaultMaxAudioChannels = TranscodeSurroundChannels

// unknownChannelCount is what ffprobe reports for an audio stream without a layout.
const unknownChannelCount = 0

// dsdCodecPrefix starts FFmpeg's DSD codec names (dsd_lsbf, dsd_msbf and their planar forms).
const dsdCodecPrefix = "dsd_"

// pcmCodecPrefix starts FFmpeg's PCM codec names (pcm_s16le, pcm_s24be, pcm_f32le, ...).
const pcmCodecPrefix = "pcm_"

// Sample rates and sample depths of the audio targets.
const (
	// unknownSampleRate is a rate the probe could not tell; as a target it keeps the source's.
	unknownSampleRate = 0
	// maxDSDTargetSampleRate: DSD64 holds nothing audible above it, and a higher PCM rate
	// only stores more of DSD's ultrasonic noise (ADR 0030: "88.2/96 kHz max").
	maxDSDTargetSampleRate = 96000
	// maxAACSampleRate: FFmpeg's AAC encoder accepts 96 kHz, but browsers decode AAC LC
	// reliably only up to 48 kHz.
	maxAACSampleRate = 48000
	// assumedDSDSampleRate is DSD64 as ffprobe reports it (2.8224 MHz in bytes), the rate of
	// an SACD, used when the probe names none.
	assumedDSDSampleRate = 352800
	// lowestFamilyRate: below 44.1 kHz an integer ratio is worth less than the bandwidth.
	lowestFamilyRate = 44100
	// FLAC as FFmpeg writes it is 16 or 24 bit.
	losslessBitDepth       = 24
	losslessNarrowBitDepth = 16
)

// noHeightCap is the Policy value that keeps the source height.
const noHeightCap = 0

// unknownWidth is reported when the source width is unknown; FFmpeg still keeps the aspect
// ratio because it derives the width from the decoded frames.
const unknownWidth = 0

// dimensionAlignment: H.264 with 4:2:0 chroma needs even widths and heights.
const dimensionAlignment = 2

// planVideo copies the video when the client plays it inside HLS, otherwise re-encodes it to
// the transcode target. It also returns the reasons a copy was not possible. Server
// capabilities are checked later, so that a client limit is reported before a server limit.
// A bit-rate cap below the source re-encodes a video the client would play, and every
// re-encoded video carries the cap, also when another reason forced the transcode.
func planVideo(stream media.Stream, input Input) (StreamDecision, []Reason, error) {
	hls := input.Profile.hlsCodecs()

	reasons := videoReasons(stream, hls.video, input.Profile.CodecLimits)
	reasons = append(reasons, bitrateCapReasons(input, stream)...)

	if len(reasons) == 0 {
		return copyDecision(stream), nil, nil
	}

	decision, encoderReasons, err := transcodeVideoDecision(stream, input)
	if err != nil {
		return StreamDecision{}, nil, err
	}

	return decision, append(reasons, encoderReasons...), nil
}

// transcodeVideoDecision re-encodes the video to the transcode target, for planVideo and for
// a burn-in alike, so both get the height of the ladder, the bit-rate cap and the encoder of
// the hardware setting. It fails when the client does not play the target. The reasons are
// those of the encoder choice only; why the video is re-encoded is the caller's to say.
func transcodeVideoDecision(stream media.Stream, input Input) (StreamDecision, []Reason, error) {
	if !input.Profile.playsTranscodedVideo() {
		return StreamDecision{}, nil, fmt.Errorf("video %s: %w", stream.Codec, errNoVideoTarget())
	}

	size := targetSize(stream.Width, stream.Height, transcodeHeightCap(input))
	encoder := chooseVideoEncoder(input)

	return StreamDecision{
		Kind: media.StreamKindVideo, SourceIndex: stream.Index, Action: ActionTranscode,
		SourceCodec: stream.Codec, TargetCodec: TranscodeVideoCodec,
		TargetWidth: size.width, TargetHeight: size.height,
		Downscaled: size.downscaled, ScaleRequired: size.scaleRequired,
		MaxBitrate: input.Request.MaxBitrate,
		Encoder:    encoder.Name, Hardware: encoder.Hardware,
	}, encoder.Reasons(), nil
}

// chooseVideoEncoder is ChooseVideoEncoder for the server and the setting of the input.
func chooseVideoEncoder(input Input) Encoder {
	return ChooseVideoEncoder(input.Capabilities, input.Policy.HardwareAcceleration)
}

// playsTranscodedVideo reports that the client plays the video transcode target inside HLS.
func (profile ClientProfile) playsTranscodedVideo() bool {
	return len(videoReasons(videoTarget(), profile.hlsCodecs().video, profile.CodecLimits)) == 0
}

func errNoVideoTarget() error {
	return fmt.Errorf("the client plays no %s %s inside HLS: %w",
		TranscodeVideoCodec, TranscodeVideoProfile, ErrNoCompatibleFormat)
}

// planAudio is planVideo for the selected audio stream.
func planAudio(stream media.Stream, input Input) (StreamDecision, []Reason, error) {
	hls := input.Profile.hlsCodecs()

	reasons := audioReasons(stream, hls.audio, input.Profile.audioLimits())
	if len(reasons) == 0 {
		return copyDecision(stream), nil, nil
	}

	target := chooseAudioTarget(stream, input)

	if !slices.Contains(hls.audio, target.codec) {
		return StreamDecision{}, nil, fmt.Errorf("audio %s: client plays no %s inside HLS: %w",
			stream.Codec, target.codec, ErrNoCompatibleFormat)
	}

	return StreamDecision{
		Kind: media.StreamKindAudio, SourceIndex: stream.Index, Action: ActionTranscode,
		SourceCodec: stream.Codec, TargetCodec: target.codec,
		TargetSampleRate: target.sampleRate, TargetBitDepth: target.bitDepth,
		TargetChannels: target.channels, Encoder: target.encoder,
	}, reasons, nil
}

// audioTarget is what a re-encoded audio stream becomes.
type audioTarget struct {
	codec, encoder string
	channels       int
	// sampleRate is unknownSampleRate when the source's rate is kept; bitDepth is 0 for AAC.
	sampleRate, bitDepth int
}

// chooseAudioTarget picks AAC, or FLAC when a lossless source would otherwise lose its
// quality only because of DSD or the client's limits. The sample rate honours the client's
// limit, and the limit of the target on top of it.
func chooseAudioTarget(stream media.Stream, input Input) audioTarget {
	limits := input.Profile.audioLimits()
	channels := transcodeChannels(stream.Channels, limits.channels)

	if !staysLossless(stream, input) {
		return audioTarget{
			codec: TranscodeAudioCodec, encoder: input.Capabilities.AACEncoder, channels: channels,
			sampleRate: resampledRate(stream, tighterRate(limits.sampleRate, maxAACSampleRate)),
		}
	}

	rateLimit := limits.sampleRate
	if isDSD(stream.Codec) {
		rateLimit = tighterRate(rateLimit, maxDSDTargetSampleRate)
	}

	return audioTarget{
		codec: TranscodeLosslessAudioCodec, encoder: input.Capabilities.FLACEncoder, channels: channels,
		sampleRate: resampledRate(stream, rateLimit), bitDepth: losslessTargetBitDepth(stream, limits.bitDepth),
	}
}

// staysLossless reports that the stream is re-encoded to FLAC: a lossless source that is DSD
// or above the client's limits, a client that plays FLAC inside HLS and accepts 16 bit, and a
// server that encodes it. Other lossless sources keep the AAC target of milestone 2, and a
// server without the encoder sends AAC instead of failing.
func staysLossless(stream media.Stream, input Input) bool {
	limits := input.Profile.audioLimits()

	needed := isDSD(stream.Codec) || (isLosslessAudio(stream.Codec) && limits.exceededBy(stream))
	clientPlays := slices.Contains(input.Profile.HLS.AudioCodecs, TranscodeLosslessAudioCodec) &&
		(limits.bitDepth == unlimitedAudioSamples || limits.bitDepth >= losslessNarrowBitDepth)

	return needed && clientPlays && input.Capabilities.FLACEncoder != ""
}

func isDSD(codec string) bool {
	return strings.HasPrefix(codec, dsdCodecPrefix)
}

// isLosslessAudio knows the lossless codecs FFmpeg decodes. DTS-HD MA and its like share a
// codec name with their lossy core and count as lossy.
func isLosslessAudio(codec string) bool {
	if isDSD(codec) || strings.HasPrefix(codec, pcmCodecPrefix) {
		return true
	}

	switch codec {
	case "flac", "alac", "truehd", "mlp", "wavpack", "ape", "tta":
		return true
	default:
		return false
	}
}

// tighterRate combines two sample-rate limits, where unlimitedAudioSamples is none.
func tighterRate(limit, other int) int {
	if limit == unlimitedAudioSamples || other < limit {
		return other
	}

	return limit
}

// resampledRate returns the rate the stream is resampled to, or unknownSampleRate when it
// fits the limit or the probe told no rate (FFmpeg then keeps what the decoder finds). The
// result is the highest common rate within the limit, preferring one the source rate is a
// multiple of (176.4 kHz becomes 88.2 kHz, not 96 kHz), which resamples without rounding.
func resampledRate(stream media.Stream, limit int) int {
	source := stream.SampleRate
	if source == unknownSampleRate && isDSD(stream.Codec) {
		source = assumedDSDSampleRate
	}

	if limit == unlimitedAudioSamples || source <= limit {
		return unknownSampleRate
	}

	fallback := unknownSampleRate

	for _, rate := range commonSampleRates() {
		if rate > limit {
			continue
		}

		if fallback == unknownSampleRate {
			fallback = rate
		}

		if rate >= lowestFamilyRate && source%rate == 0 {
			return rate
		}
	}

	return fallback
}

// commonSampleRates lists the rates every decoder plays, from the highest to the lowest
// limit a profile may name.
func commonSampleRates() []int {
	return []int{192000, 176400, 96000, 88200, 48000, 44100, 32000, 22050, 16000, 11025, 8000}
}

// losslessTargetBitDepth is 24 bit, or 16 bit for a 16-bit source and for a client limited
// below 24 bit. DSD has no PCM depth of its own and gets 24 bit (ADR 0030 section 4).
func losslessTargetBitDepth(stream media.Stream, limit int) int {
	sourceFits := !isDSD(stream.Codec) && stream.BitDepth != unknownBitDepth && stream.BitDepth <= losslessNarrowBitDepth
	limited := limit != unlimitedAudioSamples && limit < losslessBitDepth

	if sourceFits || limited {
		return losslessNarrowBitDepth
	}

	return losslessBitDepth
}

// transcodeChannels keeps mono and stereo, encodes every surround layout as 5.1 for a client
// that plays six channels (7.1 is downmixed, 3.0 to 5.0 are mapped onto 5.1) and as stereo
// for every other client. An unknown layout becomes stereo, which every source can be
// downmixed to. The result never exceeds the client's limit, so a mono client gets mono.
func transcodeChannels(sourceChannels, maxChannels int) int {
	target := TranscodeStereoChannels

	switch {
	case sourceChannels == unknownChannelCount:
		// Stays stereo.
	case sourceChannels <= TranscodeStereoChannels:
		target = sourceChannels
	case maxChannels >= TranscodeSurroundChannels:
		target = TranscodeSurroundChannels
	}

	return min(target, maxChannels)
}

// videoTarget describes the transcoded video, so the client's limits can be checked on it.
func videoTarget() media.Stream {
	return media.Stream{
		Kind: media.StreamKindVideo, Codec: TranscodeVideoCodec,
		Profile: TranscodeVideoProfile, BitDepth: TranscodeVideoBitDepth,
	}
}

// copyDecision passes a stream through; video keeps its size and audio its channels (the
// fields of the other kind are zero in the source).
func copyDecision(stream media.Stream) StreamDecision {
	return StreamDecision{
		Kind: stream.Kind, SourceIndex: stream.Index, Action: ActionCopy,
		SourceCodec: stream.Codec, TargetCodec: stream.Codec,
		TargetWidth: stream.Width, TargetHeight: stream.Height, TargetChannels: stream.Channels,
	}
}

// videoSize is the output size of a transcoded video.
type videoSize struct {
	width, height int
	downscaled    bool
	scaleRequired bool
}

// targetSize caps the height at maxHeight and never scales up (OD-5 B). The width keeps the
// aspect ratio rounded to an even number the way FFmpeg's scale=-2:<height> computes it, so
// the plan shows the size the client really receives. Without a downscale, odd dimensions
// are rounded down to even ones, because libx264 rejects odd sizes in 4:2:0.
func targetSize(width, height, maxHeight int) videoSize {
	if maxHeight == noHeightCap || height <= maxHeight {
		evenWidth, evenHeight := evenFloor(width), evenFloor(height)

		return videoSize{
			width: evenWidth, height: evenHeight,
			scaleRequired: evenWidth != width || evenHeight != height,
		}
	}

	return videoSize{
		width: scaledWidth(width, height, maxHeight), height: maxHeight,
		downscaled: true, scaleRequired: true,
	}
}

// scaledWidth is width*maxHeight/height rounded to the nearest even number, halves up like
// FFmpeg's av_rescale: dividing by 2*height counts pairs of pixels, and adding height (half
// that divisor) first turns the truncating division into rounding.
func scaledWidth(width, height, maxHeight int) int {
	if width <= unknownWidth {
		return unknownWidth
	}

	pairs := (width*maxHeight + height) / (height * dimensionAlignment)

	return pairs * dimensionAlignment
}

// evenFloor rounds a non-negative dimension down to the next even number.
func evenFloor(dimension int) int {
	return dimension - dimension%dimensionAlignment
}

// missingServerCapability returns an error when the server's FFmpeg cannot produce the
// planned HLS stream: no HLS muxer at all, or no encoder for a re-encoded stream.
func missingServerCapability(capabilities ServerCapabilities, streams []StreamDecision) error {
	if !capabilities.HLSMuxing {
		return fmt.Errorf("server cannot write HLS: %w", ErrTranscodeUnavailable)
	}

	for _, decision := range streams {
		if decision.Action == ActionTranscode && decision.Encoder == "" {
			return fmt.Errorf("server has no %s encoder for the %s stream: %w",
				decision.TargetCodec, decision.Kind, ErrTranscodeUnavailable)
		}
	}

	return nil
}
