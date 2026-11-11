package playback

import (
	"slices"
	"strings"

	"github.com/magpeek/mivo/internal/domain/media"
)

// unknownBitDepth is what media.BitDepth reports when neither ffprobe nor the pixel format
// tells the depth.
const unknownBitDepth = 0

// codecSet lists the codecs a client plays in one form of delivery.
type codecSet struct {
	video []string
	audio []string
}

// directPlayReasons returns why the client cannot play the original file; empty means
// direct play works. Codecs are only checked when the profile lists the container, because
// without an entry there is nothing to check them against. A bit-rate cap below the source
// rules the original out as well.
func directPlayReasons(input Input, chosen selection) []Reason {
	var reasons []Reason

	profile := input.Profile

	if codecs, found := profile.directPlayCodecs(input.Description.Container); found {
		reasons = append(reasons, chosen.codecReasons(codecs, profile)...)
	} else {
		reasons = append(reasons, Reason{Code: ReasonContainerNotSupported, Scope: ScopeContainer})
	}

	if chosen.hasVideo {
		reasons = append(reasons, bitrateCapReasons(input, chosen.video)...)
	}

	if !chosen.audioIsFirst {
		reasons = append(reasons, Reason{Code: ReasonAudioTrackNotFirst, Scope: ScopeAudio})
	}

	return reasons
}

// directPlayCodecs combines the codecs of every profile entry for the container's
// direct-play key. The second result is false when the container is never played directly
// or the profile does not list its key.
func (profile ClientProfile) directPlayCodecs(container media.Container) (codecSet, bool) {
	key, playable := media.DirectPlayKey(container)
	if !playable {
		return codecSet{}, false
	}

	var (
		codecs codecSet
		found  bool
	)

	for _, entry := range profile.DirectPlay {
		if entry.Container == key {
			codecs.video = append(codecs.video, entry.VideoCodecs...)
			codecs.audio = append(codecs.audio, entry.AudioCodecs...)
			found = true
		}
	}

	return codecs, found
}

func (profile ClientProfile) hlsCodecs() codecSet {
	return codecSet{video: profile.HLS.VideoCodecs, audio: profile.HLS.AudioCodecs}
}

// codecReasons checks the selected streams that exist against the codecs and the profile's
// limits.
func (chosen selection) codecReasons(codecs codecSet, profile ClientProfile) []Reason {
	var reasons []Reason

	if chosen.hasVideo {
		reasons = append(reasons, videoReasons(chosen.video, codecs.video, profile.CodecLimits)...)
	}

	if chosen.hasAudio {
		reasons = append(reasons, audioReasons(chosen.audio, codecs.audio, profile.audioLimits())...)
	}

	return reasons
}

// videoReasons returns why the client cannot decode the video stream; empty means it can.
func videoReasons(stream media.Stream, codecs []string, limits []CodecLimit) []Reason {
	if !slices.Contains(codecs, stream.Codec) {
		return []Reason{{Code: ReasonVideoCodecNotSupported, Scope: ScopeVideo}}
	}

	return limitReasons(stream, limits)
}

// audioReasons returns why the audio stream is not passed through. Codec limits are video
// only; audio has the profile's own limits (ADR 0030 Notes (M4-02b)). DSD is never passed
// through, whatever the client lists. The channel count is checked only for a codec the client
// plays, and an unknown count passes, like an unknown profile: it is a probe quirk, not a
// layout. Audio above the sample-rate or bit-depth limit is named in addition to an unsupported
// codec, because it explains the resampling the client hears.
func audioReasons(stream media.Stream, codecs []string, limits audioLimits) []Reason {
	if isDSD(stream.Codec) {
		return []Reason{{Code: ReasonDSDToPCM, Scope: ScopeAudio}}
	}

	var reasons []Reason

	switch {
	case !slices.Contains(codecs, stream.Codec):
		reasons = append(reasons, Reason{Code: ReasonAudioCodecNotSupported, Scope: ScopeAudio})
	case stream.Channels > limits.channels:
		reasons = append(reasons, Reason{Code: ReasonAudioChannelsNotSupported, Scope: ScopeAudio})
	}

	if limits.exceededBy(stream) {
		reasons = append(reasons, Reason{Code: ReasonHiResDownsampled, Scope: ScopeAudio})
	}

	return reasons
}

// limitReasons applies every limit entry of the stream's codec. An unknown profile passes,
// because ffprobe reports the profile of every codec that has limits and a missing value is
// a probe quirk. An unknown bit depth fails a bit-depth limit: a 10-bit stream the probe
// could not classify would otherwise reach an 8-bit decoder, while a transcode always
// produces the known 8-bit target.
func limitReasons(stream media.Stream, limits []CodecLimit) []Reason {
	var reasons []Reason

	depth := videoBitDepth(stream)

	for _, limit := range limits {
		if limit.Codec != stream.Codec {
			continue
		}

		if !limit.allowsProfile(stream.Profile) {
			reasons = appendUnique(reasons, Reason{Code: ReasonVideoProfileNotSupported, Scope: ScopeVideo})
		}

		if !limit.allowsBitDepth(depth) {
			reasons = appendUnique(reasons, Reason{Code: ReasonVideoBitDepthNotSupported, Scope: ScopeVideo})
		}
	}

	return reasons
}

// allowsProfile compares case-insensitively because clients type profile names by hand
// while ffprobe capitalises them.
func (limit CodecLimit) allowsProfile(profile string) bool {
	if profile == "" || len(limit.Profiles) == 0 {
		return true
	}

	return slices.ContainsFunc(limit.Profiles, func(allowed string) bool {
		return strings.EqualFold(allowed, profile)
	})
}

func (limit CodecLimit) allowsBitDepth(depth int) bool {
	if limit.MaxBitDepth == unlimitedBitDepth {
		return true
	}

	return depth != unknownBitDepth && depth <= limit.MaxBitDepth
}

// videoBitDepth uses the stored depth and falls back to the pixel format for descriptions
// that were stored without one.
func videoBitDepth(stream media.Stream) int {
	if stream.BitDepth > 0 {
		return stream.BitDepth
	}

	return media.BitDepth(0, stream.PixelFormat)
}

// isHDR uses the stored classification and falls back to the colour tags for descriptions
// that were stored without one.
func isHDR(stream media.Stream) bool {
	return stream.HDR.IsHDR() || media.ClassifyHDR(stream).IsHDR()
}

func appendUnique[T comparable](list []T, additions ...T) []T {
	for _, addition := range additions {
		if !slices.Contains(list, addition) {
			list = append(list, addition)
		}
	}

	return list
}
