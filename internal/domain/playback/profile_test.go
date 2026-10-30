package playback_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/media"
	"github.com/magpeek/mivo/internal/domain/playback"
	"github.com/magpeek/mivo/internal/domain/playback/playbacktest"
)

// Bounds of ADR 0023 and api/openapi.yaml, repeated here so the test fails when the code
// drifts from the contract.
const (
	maxNameLength       = 64
	maxDirectPlay       = 16
	maxCodecs           = 32
	maxCodecLimits      = 32
	maxLimitProfiles    = 16
	maxProfileLength    = 64
	maxIdentifierLength = 32
	maxBitDepth         = 16
)

func TestClientProfileValidateAcceptsValidProfiles(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		modify func(*playback.ClientProfile)
	}{
		{name: "reference profile", modify: func(*playback.ClientProfile) {}},
		{name: "empty name", modify: func(profile *playback.ClientProfile) { profile.Name = "" }},
		{name: "name at the limit in runes", modify: func(profile *playback.ClientProfile) {
			profile.Name = strings.Repeat("ü", maxNameLength)
		}},
		{name: "native HLS", modify: func(profile *playback.ClientProfile) {
			profile.HLS.Delivery = playback.HLSDeliveryNative
		}},
		{name: "no HLS and no lists", modify: func(profile *playback.ClientProfile) {
			profile.HLS = playback.HLSProfile{Delivery: playback.HLSDeliveryNone}
			profile.DirectPlay = nil
			profile.CodecLimits = nil
		}},
		{name: "identifier with inner punctuation", modify: func(profile *playback.ClientProfile) {
			profile.HLS.AudioCodecs = []string{"pcm_s16le", "mp4a.40-2", "9codec"}
		}},
		{name: "identifier at the length limit", modify: func(profile *playback.ClientProfile) {
			profile.HLS.AudioCodecs = []string{strings.Repeat("a", maxIdentifierLength)}
		}},
		{name: "unknown direct-play key is tolerated", modify: func(profile *playback.ClientProfile) {
			profile.DirectPlay = append(profile.DirectPlay, playback.DirectPlayProfile{Container: "avi"})
		}},
		{name: "lists at their limits", modify: fillToLimits},
		{name: "bit depth at the limit", modify: func(profile *playback.ClientProfile) {
			profile.CodecLimits = []playback.CodecLimit{{Codec: "hevc", MaxBitDepth: maxBitDepth}}
		}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			profile := playbacktest.ChromiumDesktop()
			test.modify(&profile)

			if err := profile.Validate(); err != nil {
				t.Errorf("Validate() = %v, want nil", err)
			}
		})
	}
}

func fillToLimits(profile *playback.ClientProfile) {
	codecs := repeated("aac", maxCodecs)
	profile.DirectPlay = make([]playback.DirectPlayProfile, maxDirectPlay)

	for index := range profile.DirectPlay {
		profile.DirectPlay[index] = playback.DirectPlayProfile{
			Container: media.DirectPlayMP4, VideoCodecs: codecs, AudioCodecs: codecs,
		}
	}

	profile.HLS.VideoCodecs = codecs
	profile.HLS.AudioCodecs = codecs
	profile.CodecLimits = make([]playback.CodecLimit, maxCodecLimits)

	for index := range profile.CodecLimits {
		profile.CodecLimits[index] = playback.CodecLimit{
			Codec: "h264", Profiles: repeated(strings.Repeat("p", maxProfileLength), maxLimitProfiles),
		}
	}
}

func TestClientProfileValidateRejectsInvalidProfiles(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		modify func(*playback.ClientProfile)
	}{
		{name: "name too long", modify: func(profile *playback.ClientProfile) {
			profile.Name = strings.Repeat("n", maxNameLength+1)
		}},
		{name: "name not UTF-8", modify: func(profile *playback.ClientProfile) { profile.Name = "bad\xff" }},
		// Control characters would let a client forge log lines or overlay text.
		{name: "name with line feed", modify: func(profile *playback.ClientProfile) { profile.Name = "a\nlevel=error" }},
		{name: "name with carriage return", modify: func(profile *playback.ClientProfile) { profile.Name = "a\rb" }},
		{name: "name with escape", modify: func(profile *playback.ClientProfile) { profile.Name = "\x1b[31mred" }},
		{name: "name with delete", modify: func(profile *playback.ClientProfile) { profile.Name = "a\x7f" }},
		{name: "name with tab", modify: func(profile *playback.ClientProfile) { profile.Name = "a\tb" }},
		{name: "name with C1 control", modify: func(profile *playback.ClientProfile) { profile.Name = "a\u0085b" }},
		{name: "limit profile with line feed", modify: func(profile *playback.ClientProfile) {
			profile.CodecLimits[0].Profiles = []string{"Main\n"}
		}},
		{name: "too many direct-play entries", modify: func(profile *playback.ClientProfile) {
			profile.DirectPlay = make([]playback.DirectPlayProfile, maxDirectPlay+1)
		}},
		{name: "direct-play container upper case", modify: func(profile *playback.ClientProfile) {
			profile.DirectPlay[0].Container = "MP4"
		}},
		{name: "direct-play container empty", modify: func(profile *playback.ClientProfile) {
			profile.DirectPlay[0].Container = ""
		}},
		{name: "direct-play video codec invalid", modify: func(profile *playback.ClientProfile) {
			profile.DirectPlay[0].VideoCodecs = []string{"H264"}
		}},
		{name: "direct-play too many audio codecs", modify: func(profile *playback.ClientProfile) {
			profile.DirectPlay[0].AudioCodecs = repeated("aac", maxCodecs+1)
		}},
		{name: "unknown HLS delivery", modify: func(profile *playback.ClientProfile) {
			profile.HLS.Delivery = "dash"
		}},
		{name: "missing HLS delivery", modify: func(profile *playback.ClientProfile) {
			profile.HLS.Delivery = ""
		}},
		{name: "HLS video codec starts with punctuation", modify: func(profile *playback.ClientProfile) {
			profile.HLS.VideoCodecs = []string{"-h264"}
		}},
		{name: "HLS audio codec too long", modify: func(profile *playback.ClientProfile) {
			profile.HLS.AudioCodecs = []string{strings.Repeat("a", maxIdentifierLength+1)}
		}},
		{name: "HLS audio codec with space", modify: func(profile *playback.ClientProfile) {
			profile.HLS.AudioCodecs = []string{"pcm s16le"}
		}},
		{name: "too many codec limits", modify: func(profile *playback.ClientProfile) {
			profile.CodecLimits = make([]playback.CodecLimit, maxCodecLimits+1)
		}},
		{name: "codec limit codec invalid", modify: func(profile *playback.ClientProfile) {
			profile.CodecLimits[0].Codec = "H.264"
		}},
		{name: "negative bit depth", modify: func(profile *playback.ClientProfile) {
			profile.CodecLimits[0].MaxBitDepth = -1
		}},
		{name: "bit depth above the limit", modify: func(profile *playback.ClientProfile) {
			profile.CodecLimits[0].MaxBitDepth = maxBitDepth + 1
		}},
		{name: "too many limit profiles", modify: func(profile *playback.ClientProfile) {
			profile.CodecLimits[0].Profiles = repeated("Main", maxLimitProfiles+1)
		}},
		{name: "limit profile too long", modify: func(profile *playback.ClientProfile) {
			profile.CodecLimits[0].Profiles = []string{strings.Repeat("p", maxProfileLength+1)}
		}},
		{name: "limit profile not UTF-8", modify: func(profile *playback.ClientProfile) {
			profile.CodecLimits[0].Profiles = []string{"\xc3"}
		}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			profile := playbacktest.ChromiumDesktop()
			test.modify(&profile)

			if err := profile.Validate(); !errors.Is(err, domain.ErrInvalid) {
				t.Errorf("Validate() = %v, want an error wrapping domain.ErrInvalid", err)
			}
		})
	}
}

func repeated(value string, count int) []string {
	values := make([]string, count)
	for index := range values {
		values[index] = value
	}

	return values
}
