package ffmpeg_test

import (
	"os/exec"
	"strings"
	"testing"
)

// ffmpegFeature is one entry of an FFmpeg listing a test depends on.
type ffmpegFeature struct {
	// listing is the option that prints the list: -encoders, -muxers, -filters or -devices.
	listing string
	name    string
	// reason says why the test needs it, for the skip message.
	reason string
}

// requireFFmpegFeature skips the test unless `ffmpeg -hide_banner <listing>` (-encoders,
// -filters, -devices, ...) lists name. The tests run against any FFmpeg, also the minimal static
// build of the Synology package (packaging/synology/ffmpeg), which has no libass, no lavfi
// device and no FLAC encoder; the server degrades without them, so the tests that need them skip
// there with the reason instead of failing.
func requireFFmpegFeature(t *testing.T, ffmpegPath string, feature ffmpegFeature) {
	t.Helper()

	// The executable is the configured test FFmpeg and the arguments are a slice; no shell is involved.
	output, err := exec.CommandContext(t.Context(), ffmpegPath, "-hide_banner", feature.listing).Output() //nolint:gosec // see above
	if err != nil {
		t.Fatalf("ffmpeg %s: %v", feature.listing, err)
	}

	for line := range strings.Lines(string(output)) {
		if fields := strings.Fields(line); len(fields) > 1 && fields[1] == feature.name {
			return
		}
	}

	t.Skipf("skipping: this FFmpeg lists no %s in %s (%s)", feature.name, feature.listing, feature.reason)
}
