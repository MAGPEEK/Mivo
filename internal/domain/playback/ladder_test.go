package playback_test

import (
	"slices"
	"testing"

	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/media"
	"github.com/magpeek/mivo/internal/domain/playback"
	"github.com/magpeek/mivo/internal/domain/playback/playbacktest"
)

// Bit rates of the capped sources and requests, in bits per second.
const (
	megabit         int64 = 1_000_000
	belowTheLadder        = megabit / 2
	betweenTheSteps       = 3 * megabit
	aboveTheLadder        = 50 * megabit
	fullHDBitrate         = 10 * megabit
	ultraHDBitrate        = 40 * megabit
	unknownBitrate  int64 = 0
)

// Frame sizes of the capped sources.
const (
	ultraHDWidth  = 3840
	ultraHDHeight = 2160
	fullHDWidth   = 1920
	fullHDHeight  = 1080
	hdWidth       = 1280
	hdHeight      = 720
	sdWidth       = 854
	sdHeight      = 480
	scopeHeight   = 800
	noPolicyCap   = 0
)

func bitrateCapReason() playback.Reason {
	return playback.Reason{Code: playback.ReasonBitrateCap, Scope: playback.ScopeVideo}
}

// sized is an H.264 High stream of the given size and bit rate, which Chromium plays as it is.
func sized(width, height int, bitRate int64) media.Stream {
	stream := h264High(0)
	stream.Width, stream.Height, stream.BitRate = width, height, bitRate

	return stream
}

func fullHD() media.Stream {
	return sized(fullHDWidth, fullHDHeight, fullHDBitrate)
}

func TestLadderListsTheFixedSteps(t *testing.T) {
	t.Parallel()

	want := []playback.LadderStep{
		{MaxBitrate: 20 * megabit, Height: ultraHDHeight},
		{MaxBitrate: 12 * megabit, Height: fullHDHeight},
		{MaxBitrate: 8 * megabit, Height: fullHDHeight},
		{MaxBitrate: 4 * megabit, Height: hdHeight},
		{MaxBitrate: 2 * megabit, Height: sdHeight},
		{MaxBitrate: 1 * megabit, Height: sdHeight},
	}

	if got := playback.Ladder(); !slices.Equal(got, want) {
		t.Errorf("Ladder() = %+v, want %+v", got, want)
	}

	// A caller that changes its copy must not change the ladder of the next caller.
	playback.Ladder()[0].Height = 0

	if got := playback.Ladder(); !slices.Equal(got, want) {
		t.Errorf("Ladder() after a caller changed its copy = %+v, want %+v", got, want)
	}
}

// TestDecideBitrateCapMode: only a cap below the bit rate of the source changes the mode; 0 is
// the original quality.
func TestDecideBitrateCapMode(t *testing.T) {
	t.Parallel()

	capped := transcode(transcoded, copied, bitrateCapReason())
	overall := func(bitRate int64, streams ...media.Stream) media.Description {
		description := clip(media.ContainerMP4, streams...)
		description.BitRate = bitRate

		return description
	}

	tests := []struct {
		name        string
		description media.Description
		layout      media.Layout
		maxBitrate  int64
		want        outcome
	}{
		{
			name: "no cap is the original quality", description: clip(media.ContainerMP4, fullHD(), audio(1, "aac")),
			maxBitrate: playback.OriginalQuality, want: directPlay(),
		},
		{
			name: "cap above the source", description: clip(media.ContainerMP4, fullHD(), audio(1, "aac")),
			maxBitrate: 12 * megabit, want: directPlay(),
		},
		{
			name: "cap equal to the source", description: clip(media.ContainerMP4, fullHD(), audio(1, "aac")),
			maxBitrate: fullHDBitrate, want: directPlay(),
		},
		{
			name: "cap below the source", description: clip(media.ContainerMP4, fullHD(), audio(1, "aac")),
			maxBitrate: 8 * megabit, want: capped,
		},
		{
			name: "cap between two steps below the source", description: clip(media.ContainerMP4, fullHD(), audio(1, "aac")),
			maxBitrate: fullHDBitrate - 1, want: capped,
		},
		{
			name:        "video bit rate unknown, overall bit rate above the cap",
			description: overall(fullHDBitrate, sized(fullHDWidth, fullHDHeight, unknownBitrate), audio(1, "aac")),
			maxBitrate:  8 * megabit, want: capped,
		},
		{
			name:        "video bit rate unknown, overall bit rate below the cap",
			description: overall(fullHDBitrate, sized(fullHDWidth, fullHDHeight, unknownBitrate), audio(1, "aac")),
			maxBitrate:  12 * megabit, want: directPlay(),
		},
		{
			name:        "video bit rate wins over the overall bit rate",
			description: overall(ultraHDBitrate, fullHD(), audio(1, "aac")),
			maxBitrate:  12 * megabit, want: directPlay(),
		},
		{
			// Nothing proves that the source fits, so the cap the user asked for is kept.
			name:        "no bit rate known at all",
			description: clip(media.ContainerMP4, sized(fullHDWidth, fullHDHeight, unknownBitrate), audio(1, "aac")),
			maxBitrate:  8 * megabit, want: capped,
		},
		{
			name:        "remux source below the cap stays a remux",
			description: clip(media.ContainerMKV, fullHD(), audio(1, "aac")),
			maxBitrate:  12 * megabit, want: remux(containerReason()),
		},
		{
			name:        "remux source above the cap",
			description: clip(media.ContainerMKV, fullHD(), audio(1, "aac")),
			maxBitrate:  8 * megabit, want: transcode(transcoded, copied, containerReason(), bitrateCapReason()),
		},
		{
			name:        "disc above the cap",
			description: clip(media.ContainerM2TS, fullHD(), audio(1, "aac")),
			layout:      media.LayoutBDMV,
			maxBitrate:  4 * megabit,
			want: transcode(transcoded, copied, discReason(), bitrateCapReason()).
				withWarnings(downscaleWarning()),
		},
		{
			// The ladder is about video; music is never re-encoded for a cap.
			name:        "audio only ignores the cap",
			description: overall(2*megabit, audio(0, "flac")),
			maxBitrate:  megabit,
			want:        outcome{mode: playback.ModeDirectPlay, audio: copied},
		},
		{
			name: "negative cap", description: clip(media.ContainerMP4, fullHD(), audio(1, "aac")),
			maxBitrate: -1, want: failure(domain.ErrInvalid),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			input := referenceInput(test.description)
			input.Layout = test.layout
			input.Request.MaxBitrate = test.maxBitrate

			plan, err := playback.Decide(input)

			assertOutcome(t, plan, err, test.want)
		})
	}
}

// TestDecideBitrateCapSize: a capped video is at most as high as the step of the cap, never
// higher than the source or the operator's limit, and always even.
func TestDecideBitrateCapSize(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		source     media.Stream
		maxBitrate int64
		policy     int
		want       targetSize
	}{
		{
			name: "20 Mbit/s keeps 2160p", source: sized(ultraHDWidth, ultraHDHeight, ultraHDBitrate),
			maxBitrate: 20 * megabit, policy: noPolicyCap,
			want: targetSize{width: ultraHDWidth, height: ultraHDHeight},
		},
		{
			name: "a cap above the ladder uses its top step", source: sized(ultraHDWidth, ultraHDHeight, 2*aboveTheLadder),
			maxBitrate: aboveTheLadder, policy: noPolicyCap,
			want: targetSize{width: ultraHDWidth, height: ultraHDHeight},
		},
		{
			name: "the operator's limit still applies", source: sized(ultraHDWidth, ultraHDHeight, ultraHDBitrate),
			maxBitrate: 20 * megabit, policy: referenceMaxHeight,
			want: targetSize{width: fullHDWidth, height: fullHDHeight, downscaled: true, scale: true},
		},
		{
			name: "the step applies below the operator's limit", source: sized(ultraHDWidth, ultraHDHeight, ultraHDBitrate),
			maxBitrate: 4 * megabit, policy: referenceMaxHeight,
			want: targetSize{width: hdWidth, height: hdHeight, downscaled: true, scale: true},
		},
		{
			name: "12 Mbit/s is 1080p", source: sized(ultraHDWidth, ultraHDHeight, ultraHDBitrate),
			maxBitrate: 12 * megabit, policy: noPolicyCap,
			want: targetSize{width: fullHDWidth, height: fullHDHeight, downscaled: true, scale: true},
		},
		{
			name: "8 Mbit/s keeps 1080p", source: fullHD(), maxBitrate: 8 * megabit, policy: referenceMaxHeight,
			want: targetSize{width: fullHDWidth, height: fullHDHeight},
		},
		{
			name: "4 Mbit/s is 720p", source: fullHD(), maxBitrate: 4 * megabit, policy: referenceMaxHeight,
			want: targetSize{width: hdWidth, height: hdHeight, downscaled: true, scale: true},
		},
		{
			name: "a cap between two steps uses the lower one", source: fullHD(), maxBitrate: betweenTheSteps,
			policy: referenceMaxHeight,
			want:   targetSize{width: sdWidth, height: sdHeight, downscaled: true, scale: true},
		},
		{
			name: "2 Mbit/s is 480p", source: fullHD(), maxBitrate: 2 * megabit, policy: referenceMaxHeight,
			want: targetSize{width: sdWidth, height: sdHeight, downscaled: true, scale: true},
		},
		{
			name: "1 Mbit/s is 480p", source: fullHD(), maxBitrate: megabit, policy: referenceMaxHeight,
			want: targetSize{width: sdWidth, height: sdHeight, downscaled: true, scale: true},
		},
		{
			name: "a cap below the ladder uses its lowest step", source: fullHD(), maxBitrate: belowTheLadder,
			policy: referenceMaxHeight,
			want:   targetSize{width: sdWidth, height: sdHeight, downscaled: true, scale: true},
		},
		{
			// 1920x800 (2.40:1) is not scaled to 720 for a 1080p step: nothing has to shrink.
			name: "a source below the step keeps its height", source: sized(fullHDWidth, scopeHeight, fullHDBitrate),
			maxBitrate: 8 * megabit, policy: referenceMaxHeight,
			want: targetSize{width: fullHDWidth, height: scopeHeight},
		},
		{
			name: "a small source is never scaled up", source: sized(clipWidth, clipHeight, 2*megabit),
			maxBitrate: megabit, policy: referenceMaxHeight,
			want: targetSize{width: clipWidth, height: clipHeight},
		},
		{
			name: "odd source below the step becomes even", source: sized(853, 479, 2*megabit),
			maxBitrate: megabit, policy: referenceMaxHeight,
			want: targetSize{width: 852, height: 478, scale: true},
		},
		{
			name: "odd source above the step gets an even width", source: sized(853, 481, 2*megabit),
			maxBitrate: megabit, policy: referenceMaxHeight,
			want: targetSize{width: 852, height: sdHeight, downscaled: true, scale: true},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			input := referenceInput(clip(media.ContainerMP4, test.source, audio(1, "aac")))
			input.Policy.MaxTranscodeHeight = test.policy
			input.Request.MaxBitrate = test.maxBitrate

			plan := mustDecide(t, input)
			video, _ := plan.Video()
			got := targetSize{
				width: video.TargetWidth, height: video.TargetHeight,
				downscaled: video.Downscaled, scale: video.ScaleRequired,
			}

			if video.Action != transcoded || got != test.want {
				t.Errorf("video = %s %+v, want %s %+v", video.Action, got, transcoded, test.want)
			}

			if video.MaxBitrate != test.maxBitrate {
				t.Errorf("MaxBitrate = %d, want %d", video.MaxBitrate, test.maxBitrate)
			}

			wantWarning := test.want.downscaled
			if gotWarning := slices.Contains(plan.Warnings, downscaleWarning()); gotWarning != wantWarning {
				t.Errorf("video_downscaled warning = %t, want %t (warnings %v)", gotWarning, wantWarning, plan.Warnings)
			}
		})
	}
}

// TestDecideBitrateCapOnOtherTranscodes: a video that is re-encoded anyway is capped too, but
// the cap is only named as a reason when it is what forced the transcode.
func TestDecideBitrateCapOnOtherTranscodes(t *testing.T) {
	t.Parallel()

	hevc := hevcMain10(0)
	hevc.Width, hevc.Height, hevc.BitRate = ultraHDWidth, ultraHDHeight, 6*megabit

	tests := []struct {
		name       string
		maxBitrate int64
		wantHeight int
		want       outcome
	}{
		{
			name: "cap above the source", maxBitrate: 8 * megabit, wantHeight: fullHDHeight,
			want: transcode(transcoded, copied, videoCodecReason()).withWarnings(downscaleWarning()),
		},
		{
			name: "cap below the source", maxBitrate: 4 * megabit, wantHeight: hdHeight,
			want: transcode(transcoded, copied, videoCodecReason(), bitrateCapReason()).withWarnings(downscaleWarning()),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			input := referenceInput(clip(media.ContainerMP4, hevc, audio(1, "aac")))
			input.Policy.MaxTranscodeHeight = noPolicyCap
			input.Request.MaxBitrate = test.maxBitrate

			plan, err := playback.Decide(input)

			assertOutcome(t, plan, err, test.want)

			video, _ := plan.Video()
			if video.TargetHeight != test.wantHeight || video.MaxBitrate != test.maxBitrate {
				t.Errorf("video = height %d cap %d, want height %d cap %d",
					video.TargetHeight, video.MaxBitrate, test.wantHeight, test.maxBitrate)
			}
		})
	}
}

// TestDecideBitrateCapLeavesCopiesUncapped: a copied stream cannot be capped, so the plan must
// not claim it.
func TestDecideBitrateCapLeavesCopiesUncapped(t *testing.T) {
	t.Parallel()

	input := referenceInput(clip(media.ContainerMKV, fullHD(), audio(1, "aac")))
	input.Request.MaxBitrate = 12 * megabit

	plan := mustDecide(t, input)

	for _, decision := range plan.Streams {
		if decision.MaxBitrate != playback.OriginalQuality {
			t.Errorf("%s MaxBitrate = %d, want none on a copied stream", decision.Kind, decision.MaxBitrate)
		}
	}
}

// TestDecideBitrateCapOnAServerWithoutEncoders: a weak NAS that only remuxes must say that it
// cannot honour the cap instead of sending the original.
func TestDecideBitrateCapOnAServerWithoutEncoders(t *testing.T) {
	t.Parallel()

	input := referenceInput(clip(media.ContainerMP4, fullHD(), audio(1, "aac")))
	input.Capabilities = playbacktest.NoEncoders()
	input.Request.MaxBitrate = 4 * megabit

	plan, err := playback.Decide(input)

	assertOutcome(t, plan, err, failure(playback.ErrTranscodeUnavailable))
}
