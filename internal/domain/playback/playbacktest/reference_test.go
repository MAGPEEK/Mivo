package playbacktest_test

import (
	"slices"
	"testing"

	"github.com/magpeek/mivo/internal/domain/media"
	"github.com/magpeek/mivo/internal/domain/playback"
	"github.com/magpeek/mivo/internal/domain/playback/playbacktest"
)

// The expected values are copied from the ADR 0025 table on purpose: the table is the single
// source for Go and TypeScript tests, so a change to a profile must change this test too.
func TestReferenceProfilesMatchTheADRTable(t *testing.T) {
	t.Parallel()

	mp4 := playback.DirectPlayProfile{
		Container:   media.DirectPlayMP4,
		VideoCodecs: []string{"h264", "av1", "vp9"},
		AudioCodecs: []string{"aac", "mp3", "opus", "flac"},
	}
	webm := playback.DirectPlayProfile{
		Container:   media.DirectPlayWebM,
		VideoCodecs: []string{"vp8", "vp9", "av1"},
		AudioCodecs: []string{"opus", "vorbis"},
	}
	ogg := playback.DirectPlayProfile{
		Container:   media.DirectPlayOGG,
		VideoCodecs: []string{"theora"},
		AudioCodecs: []string{"vorbis", "opus", "flac"},
	}

	tests := []struct {
		name       string
		profile    playback.ClientProfile
		wantName   string
		directPlay []playback.DirectPlayProfile
	}{
		{
			name: "chromium", profile: playbacktest.ChromiumDesktop(), wantName: "chromium-desktop",
			directPlay: []playback.DirectPlayProfile{mp4, webm},
		},
		{
			name: "firefox", profile: playbacktest.FirefoxDesktop(), wantName: "firefox-desktop",
			directPlay: []playback.DirectPlayProfile{mp4, webm, ogg},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			if err := test.profile.Validate(); err != nil {
				t.Fatalf("Validate() = %v, want nil", err)
			}

			if test.profile.Name != test.wantName {
				t.Errorf("Name = %q, want %q", test.profile.Name, test.wantName)
			}

			assertDirectPlay(t, test.profile.DirectPlay, test.directPlay)
			assertHLS(t, test.profile.HLS)
			assertCodecLimits(t, test.profile.CodecLimits)
		})
	}
}

func assertDirectPlay(t *testing.T, got, want []playback.DirectPlayProfile) {
	t.Helper()

	if len(got) != len(want) {
		t.Fatalf("DirectPlay has %d entries, want %d", len(got), len(want))
	}

	for index := range want {
		if got[index].Container != want[index].Container ||
			!slices.Equal(got[index].VideoCodecs, want[index].VideoCodecs) ||
			!slices.Equal(got[index].AudioCodecs, want[index].AudioCodecs) {
			t.Errorf("DirectPlay[%d] = %+v, want %+v", index, got[index], want[index])
		}
	}
}

func assertHLS(t *testing.T, got playback.HLSProfile) {
	t.Helper()

	want := playback.HLSProfile{
		Delivery:    playback.HLSDeliveryMSE,
		VideoCodecs: []string{"h264", "av1", "vp9"},
		AudioCodecs: []string{"aac", "mp3", "opus", "flac"},
	}

	if got.Delivery != want.Delivery || !slices.Equal(got.VideoCodecs, want.VideoCodecs) ||
		!slices.Equal(got.AudioCodecs, want.AudioCodecs) {
		t.Errorf("HLS = %+v, want %+v", got, want)
	}
}

func assertCodecLimits(t *testing.T, got []playback.CodecLimit) {
	t.Helper()

	want := []playback.CodecLimit{
		{Codec: "h264", MaxBitDepth: 8, Profiles: []string{"Constrained Baseline", "Baseline", "Main", "High"}},
		{Codec: "vp9", MaxBitDepth: 10},
		{Codec: "av1", MaxBitDepth: 10},
	}

	if len(got) != len(want) {
		t.Fatalf("CodecLimits has %d entries, want %d", len(got), len(want))
	}

	for index := range want {
		if got[index].Codec != want[index].Codec || got[index].MaxBitDepth != want[index].MaxBitDepth ||
			!slices.Equal(got[index].Profiles, want[index].Profiles) {
			t.Errorf("CodecLimits[%d] = %+v, want %+v", index, got[index], want[index])
		}
	}
}

func TestReferenceProfilesAreIndependentCopies(t *testing.T) {
	t.Parallel()

	first := playbacktest.ChromiumDesktop()
	first.DirectPlay[0].VideoCodecs[0] = "changed"
	first.HLS.AudioCodecs[0] = "changed"
	first.CodecLimits[0].Profiles[0] = "changed"

	second := playbacktest.ChromiumDesktop()
	if second.DirectPlay[0].VideoCodecs[0] != "h264" || second.HLS.AudioCodecs[0] != "aac" ||
		second.CodecLimits[0].Profiles[0] != "Constrained Baseline" {
		t.Error("ChromiumDesktop() shares slices between calls; callers must be able to modify their copy")
	}
}

func TestServerCapabilities(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		capabilities playback.ServerCapabilities
		want         playback.ServerCapabilities
	}{
		{
			name:         "full",
			capabilities: playbacktest.FullCapabilities(),
			want: playback.ServerCapabilities{
				FFmpegVersion: "7.1.5", HLSMuxing: true,
				H264Encoder: playback.EncoderLibx264, AACEncoder: playback.EncoderAAC,
			},
		},
		{
			name:         "no encoders",
			capabilities: playbacktest.NoEncoders(),
			want:         playback.ServerCapabilities{FFmpegVersion: "7.1.5", HLSMuxing: true},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			if test.capabilities != test.want {
				t.Errorf("capabilities = %+v, want %+v", test.capabilities, test.want)
			}
		})
	}
}
