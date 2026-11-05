package recipecheck_test

import (
	"regexp"
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
	assertExactSet(t, "PROTOCOLS", recipeList(t, script, "PROTOCOLS"), []string{"file", "pipe", "concat", "subfile"})

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

// capabilitiesScript is the capability check every shipped FFmpeg build runs (task M3-16j).
const capabilitiesScript = "../../package/scripts/ffmpeg-capabilities.sh"

// TestRecipeReadsDiscImagesWithoutNetwork keeps the recipe and the shared capability check in
// step with ISO playback (M3-16g, task M3-16j-b): a title inside an image is read as a byte
// range through subfile,,start,A,end,B,,:file:<image>, so the static build has the subfile
// protocol, every shipped build is required to have it, and the build stays without network
// protocols.
func TestRecipeReadsDiscImagesWithoutNetwork(t *testing.T) {
	t.Parallel()

	script := readFile(t, recipeScript)
	if !strings.Contains(script, "--disable-network") {
		t.Errorf("%s no longer configures --disable-network", recipeScript)
	}

	match := regexp.MustCompile(`(?m)^readonly REQUIRED_PROTOCOLS="([^"]*)"`).
		FindStringSubmatch(readFile(t, capabilitiesScript))
	if match == nil {
		t.Fatalf("%s has no readonly REQUIRED_PROTOCOLS=\"...\" line", capabilitiesScript)
	}

	required := strings.Fields(match[1])
	for _, protocol := range []string{"file", "concat", "subfile"} {
		if !slices.Contains(required, protocol) {
			t.Errorf("REQUIRED_PROTOCOLS of %s = %v, lacks %q", capabilitiesScript, required, protocol)
		}
	}

	built := recipeList(t, script, "PROTOCOLS")
	for _, protocol := range required {
		if !slices.Contains(built, protocol) {
			t.Errorf("%s requires protocol %q, which PROTOCOLS of %s does not build",
				capabilitiesScript, protocol, recipeScript)
		}
	}
}
