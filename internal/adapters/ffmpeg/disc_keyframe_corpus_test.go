package ffmpeg_test

import (
	"testing"
	"time"

	"github.com/magpeek/mivo/internal/adapters/ffmpeg"
	appplayback "github.com/magpeek/mivo/internal/app/playback"
	"github.com/magpeek/mivo/internal/app/scan"
	"github.com/magpeek/mivo/internal/domain/media"
)

// The prober implements the disc ports of the scan and the playback service (M3-16h).
var (
	_ scan.DiscProber                 = (*ffmpeg.Prober)(nil)
	_ appplayback.DiscKeyframeLocator = (*ffmpeg.Prober)(nil)
)

// discKeyframeSlack bounds how far before the position the keyframe of the generated discs
// may lie: their GOPs are at most one second long.
const discKeyframeSlack = 1500 * time.Millisecond

func sourceOf(input ffmpeg.DiscInput) media.DiscSource {
	return media.DiscSource{RelativePath: input.DiscPath, Layout: input.Layout, Title: input.Title}
}

// TestDiscKeyframeAtOrBeforeSearchesTheTitle: a remux seek into a disc finds a keyframe of the
// joined title, past the first clip, in folders and images alike (BACKLOG m3-16f).
func TestDiscKeyframeAtOrBeforeSearchesTheTitle(t *testing.T) {
	fixture := requireDiscFixture(t)

	for name, input := range map[string]ffmpeg.DiscInput{
		"DVD": fixture.dvd(), "Blu-ray": fixture.bluray(), "DVD image": fixture.dvdImage(), "Blu-ray image": fixture.blurayImage(),
	} {
		t.Run(name, func(t *testing.T) {
			keyframe, err := fixture.tools.prober.DiscKeyframeAtOrBefore(t.Context(), fixture.library, sourceOf(input), discSeekPosition)
			if err != nil {
				t.Fatalf("DiscKeyframeAtOrBefore = %v", err)
			}

			if keyframe > discSeekPosition || keyframe < discSeekPosition-discKeyframeSlack {
				t.Errorf("keyframe = %s, want within %s before %s", keyframe, discKeyframeSlack, discSeekPosition)
			}

			start, err := fixture.tools.prober.DiscKeyframeAtOrBefore(t.Context(), fixture.library, sourceOf(input), 0)
			if err != nil || start != 0 {
				t.Errorf("keyframe at 0 = %s, %v, want 0", start, err)
			}
		})
	}
}

func TestProbeDiscTitleProbesLikeProbeDisc(t *testing.T) {
	fixture := requireDiscFixture(t)
	input := fixture.bluray()

	description, err := fixture.tools.prober.ProbeDiscTitle(t.Context(), fixture.library, sourceOf(input))
	if err != nil {
		t.Fatalf("ProbeDiscTitle = %v", err)
	}

	if description.Container != media.ContainerM2TS || len(description.Streams) != 2 {
		t.Errorf("description = %+v, want the Blu-ray title", description)
	}
}

func TestDiscKeyframeAtOrBeforeRejectsAnInvalidTitle(t *testing.T) {
	fixture := requireDiscFixture(t)
	input := fixture.bluray()
	input.Title = nil

	if _, err := fixture.tools.prober.DiscKeyframeAtOrBefore(t.Context(), fixture.library, sourceOf(input), discSeekPosition); err == nil {
		t.Error("DiscKeyframeAtOrBefore of an empty title succeeded")
	}
}
