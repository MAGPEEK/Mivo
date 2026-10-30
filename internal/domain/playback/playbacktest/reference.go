// Package playbacktest provides the reference client profiles and server capabilities of
// ADR 0025 as Go values, so the decision engine, the use cases, the corpus integration test
// and the smoke test all decide against the same clients. Every function returns a fresh
// value the caller may modify.
package playbacktest

import (
	"github.com/magpeek/mivo/internal/domain/media"
	"github.com/magpeek/mivo/internal/domain/playback"
)

// Bit depths of the reference codec limits (ADR 0025).
const (
	h264MaxBitDepth = 8
	vp9MaxBitDepth  = 10
	av1MaxBitDepth  = 10
)

// referenceFFmpegVersion is the FFmpeg of the Docker image (Debian 7.1.x); the decision
// does not depend on it.
const referenceFFmpegVersion = "7.1.2"

// ChromiumDesktop is the profile player-core reports for Chrome and Edge on the desktop:
// H.264/AAC/VP9/AV1/Opus, HLS over MSE, no Matroska, no Theora, no HEVC.
func ChromiumDesktop() playback.ClientProfile {
	return playback.ClientProfile{
		Name:        "chromium-desktop",
		DirectPlay:  []playback.DirectPlayProfile{mp4DirectPlay(), webmDirectPlay()},
		HLS:         mseHLS(),
		CodecLimits: browserCodecLimits(),
	}
}

// FirefoxDesktop is the profile player-core reports for Firefox on the desktop: like
// ChromiumDesktop plus Ogg (Theora, Vorbis, Opus, FLAC) as a progressive file.
func FirefoxDesktop() playback.ClientProfile {
	return playback.ClientProfile{
		Name: "firefox-desktop",
		DirectPlay: []playback.DirectPlayProfile{
			mp4DirectPlay(),
			webmDirectPlay(),
			{
				Container:   media.DirectPlayOGG,
				VideoCodecs: []string{"theora"},
				AudioCodecs: []string{"vorbis", "opus", "flac"},
			},
		},
		HLS:         mseHLS(),
		CodecLimits: browserCodecLimits(),
	}
}

// FullCapabilities is a server whose FFmpeg muxes HLS and has the milestone 2 encoders.
func FullCapabilities() playback.ServerCapabilities {
	return playback.ServerCapabilities{
		FFmpegVersion: referenceFFmpegVersion,
		HLSMuxing:     true,
		H264Encoder:   playback.EncoderLibx264,
		AACEncoder:    playback.EncoderAAC,
	}
}

// NoEncoders is a server whose FFmpeg muxes HLS but cannot encode, like a minimal build on
// a weak NAS: direct play and remux work, every transcode fails with transcode_unavailable.
func NoEncoders() playback.ServerCapabilities {
	return playback.ServerCapabilities{
		FFmpegVersion: referenceFFmpegVersion,
		HLSMuxing:     true,
	}
}

func mp4DirectPlay() playback.DirectPlayProfile {
	return playback.DirectPlayProfile{
		Container:   media.DirectPlayMP4,
		VideoCodecs: []string{"h264", "av1", "vp9"},
		AudioCodecs: []string{"aac", "mp3", "opus", "flac"},
	}
}

func webmDirectPlay() playback.DirectPlayProfile {
	return playback.DirectPlayProfile{
		Container:   media.DirectPlayWebM,
		VideoCodecs: []string{"vp8", "vp9", "av1"},
		AudioCodecs: []string{"opus", "vorbis"},
	}
}

func mseHLS() playback.HLSProfile {
	return playback.HLSProfile{
		Delivery:    playback.HLSDeliveryMSE,
		VideoCodecs: []string{"h264", "av1", "vp9"},
		AudioCodecs: []string{"aac", "mp3", "opus", "flac"},
	}
}

func browserCodecLimits() []playback.CodecLimit {
	return []playback.CodecLimit{
		{
			Codec:       "h264",
			MaxBitDepth: h264MaxBitDepth,
			Profiles:    []string{"Constrained Baseline", "Baseline", "Main", "High"},
		},
		{Codec: "vp9", MaxBitDepth: vp9MaxBitDepth},
		{Codec: "av1", MaxBitDepth: av1MaxBitDepth},
	}
}
