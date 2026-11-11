package playback_test

import (
	"testing"

	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/media"
	"github.com/magpeek/mivo/internal/domain/playback"
)

func discReason() playback.Reason {
	return playback.Reason{Code: playback.ReasonDiscSource, Scope: playback.ScopeContainer}
}

// TestDecideDiscSources: a disc never plays directly, whatever its streams (ADR 0029); its
// reason is disc_source, the copy checks add why a stream is re-encoded, and bitmap subtitles
// stay unsupported until milestone 4.
func TestDecideDiscSources(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		layout      media.Layout
		description media.Description
		want        outcome
	}{
		{
			name: "file that plays directly", layout: media.LayoutFile,
			description: clip(media.ContainerM4V, h264High(0), audio(1, "aac")),
			want:        directPlay(),
		},
		{
			name: "image with directly playable streams is remuxed", layout: media.LayoutISO,
			description: clip(media.ContainerM2TS, h264High(0), audio(1, "aac")),
			want:        remux(discReason()),
		},
		{
			name: "Blu-ray folder with AC-3 audio", layout: media.LayoutBDMV,
			description: clip(media.ContainerM2TS, h264High(0), audio(1, "ac3")),
			want:        transcode(copied, transcoded, discReason(), audioCodecReason()),
		},
		{
			name: "DVD folder is transcoded", layout: media.LayoutVideoTS,
			description: clip(media.ContainerVOB, simpleVideo(0, "mpeg2video"), audio(1, "ac3")),
			want:        transcode(transcoded, transcoded, discReason(), videoCodecReason(), audioCodecReason()),
		},
		{
			name: "DVD bitmap subtitles stay off without a request", layout: media.LayoutVideoTS,
			description: clip(media.ContainerVOB, simpleVideo(0, "mpeg2video"), audio(1, "ac3"), subtitle(2, "dvd_subtitle")),
			want:        transcode(transcoded, transcoded, discReason(), videoCodecReason(), audioCodecReason()),
		},
		{
			name: "Blu-ray PGS subtitles stay off without a request", layout: media.LayoutBDMV,
			description: clip(media.ContainerM2TS, h264High(0), audio(1, "aac"), subtitle(2, "hdmv_pgs_subtitle")),
			want:        remux(discReason()),
		},
		{
			name: "unknown layout", layout: "dvd",
			description: clip(media.ContainerVOB, simpleVideo(0, "mpeg2video")),
			want:        failure(domain.ErrInvalid),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			input := referenceInput(test.description)
			input.Layout = test.layout

			plan, err := playback.Decide(input)

			assertOutcome(t, plan, err, test.want)
		})
	}
}
