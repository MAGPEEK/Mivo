package recipecheck_test

import (
	"slices"
	"strings"
	"testing"
)

// discInputSource builds the FFmpeg input of a folder disc (M3-16f, ADR 0029).
const discInputSource = "../../../../internal/adapters/ffmpeg/disc_input.go"

// TestRecipeBuildsWhatFolderDiscsNeed keeps the recipe in step with disc_input.go (task
// M3-16j): the concat demuxer reads a Blu-ray title's list, the concat protocol joins a DVD
// title's VOBs, and both open their parts as file:. concat stays off DEMUXERS, the list the
// server whitelists for library files.
func TestRecipeBuildsWhatFolderDiscsNeed(t *testing.T) {
	t.Parallel()

	script := readFile(t, recipeScript)

	assertExactSet(t, "DISC_DEMUXERS", recipeList(t, script, "DISC_DEMUXERS"), []string{"concat"})
	assertExactSet(t, "PROTOCOLS", recipeList(t, script, "PROTOCOLS"), []string{"file", "pipe", "concat"})

	whitelisted := recipeDemuxers(t)
	if slices.Contains(whitelisted, "concat") {
		t.Errorf("DEMUXERS of %s lists concat; it belongs to DISC_DEMUXERS only", recipeScript)
	}

	// The program stream (mpeg, configure name mpegps) and transport stream demuxers of the
	// clips must stay on the whitelisted list.
	for _, name := range []string{programStreamConfigureName, "mpegts"} {
		if !slices.Contains(whitelisted, name) {
			t.Errorf("DEMUXERS of %s lacks %q, which folder discs read", recipeScript, name)
		}
	}

	// The disc input source must still name what the recipe provides, or this test is stale.
	discInput := readFile(t, discInputSource)
	for _, name := range []string{`"concat"`, `"concat:"`, `",concat"`, `"mpeg"`, `"mpegts"`} {
		if !strings.Contains(discInput, name) {
			t.Errorf("%s no longer mentions %s; update the recipe and this test", discInputSource, name)
		}
	}
}
