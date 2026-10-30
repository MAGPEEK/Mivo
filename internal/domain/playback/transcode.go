package playback

import (
	"fmt"

	"github.com/magpeek/mivo/internal/domain/media"
)

// Transcode targets of milestone 2 (ADR 0023, OD-11 A): H.264 High 8-bit 4:2:0 and AAC LC
// stereo play in every supported browser. adapters/ffmpeg chooses the matching arguments.
const (
	TranscodeVideoCodec    = "h264"
	TranscodeVideoProfile  = "High"
	TranscodeVideoBitDepth = 8
	TranscodeAudioCodec    = "aac"
	TranscodeAudioChannels = 2
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
func planVideo(stream media.Stream, input Input) (StreamDecision, []Reason, error) {
	hls := input.Profile.hlsCodecs()

	reasons := videoReasons(stream, hls.video, input.Profile.CodecLimits)
	if len(reasons) == 0 {
		return copyDecision(stream), nil, nil
	}

	if len(videoReasons(videoTarget(), hls.video, input.Profile.CodecLimits)) > 0 {
		return StreamDecision{}, nil, fmt.Errorf("video %s: client plays no %s %s inside HLS: %w",
			stream.Codec, TranscodeVideoCodec, TranscodeVideoProfile, ErrNoCompatibleFormat)
	}

	size := targetSize(stream.Width, stream.Height, input.Policy.MaxTranscodeHeight)

	return StreamDecision{
		Kind: media.StreamKindVideo, SourceIndex: stream.Index, Action: ActionTranscode,
		SourceCodec: stream.Codec, TargetCodec: TranscodeVideoCodec,
		TargetWidth: size.width, TargetHeight: size.height,
		Downscaled: size.downscaled, ScaleRequired: size.scaleRequired,
		Encoder: input.Capabilities.H264Encoder,
	}, reasons, nil
}

// planAudio is planVideo for the selected audio stream.
func planAudio(stream media.Stream, input Input) (StreamDecision, []Reason, error) {
	hls := input.Profile.hlsCodecs()

	reasons := audioReasons(stream, hls.audio)
	if len(reasons) == 0 {
		return copyDecision(stream), nil, nil
	}

	if len(audioReasons(media.Stream{Codec: TranscodeAudioCodec}, hls.audio)) > 0 {
		return StreamDecision{}, nil, fmt.Errorf("audio %s: client plays no %s inside HLS: %w",
			stream.Codec, TranscodeAudioCodec, ErrNoCompatibleFormat)
	}

	return StreamDecision{
		Kind: media.StreamKindAudio, SourceIndex: stream.Index, Action: ActionTranscode,
		SourceCodec: stream.Codec, TargetCodec: TranscodeAudioCodec,
		TargetChannels: TranscodeAudioChannels, Encoder: input.Capabilities.AACEncoder,
	}, reasons, nil
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
