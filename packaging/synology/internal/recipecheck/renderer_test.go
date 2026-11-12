package recipecheck_test

import (
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// imageRendererSource builds the ffmpeg arguments of the image renderer (M3-08).
const imageRendererSource = "../../../../internal/adapters/ffmpeg/image_args.go"

// TestRecipeBuildsWhatTheImageRendererNeeds keeps the MUXERS and ENCODERS lines of the recipe in
// step with image_args.go: the muxer and encoders it names must be built into the static ffmpeg.
func TestRecipeBuildsWhatTheImageRendererNeeds(t *testing.T) {
	t.Parallel()

	script := readFile(t, recipeScript)
	muxers := recipeList(t, script, "MUXERS")
	encoders := recipeList(t, script, "ENCODERS")

	assertExactSet(t, "MUXERS", muxers, []string{"hls", "mp4", "null", "image2pipe"})
	assertExactSet(t, "ENCODERS", encoders, []string{"libx264", "aac", "mjpeg", "png", "webvtt"})

	for _, muxer := range []string{"image2pipe", "hls", "mp4"} {
		if !slices.Contains(muxers, muxer) {
			t.Errorf("MUXERS of %s lacks %q", recipeScript, muxer)
		}
	}

	for _, encoder := range []string{"mjpeg", "png", "libx264", "aac", "webvtt"} {
		if !slices.Contains(encoders, encoder) {
			t.Errorf("ENCODERS of %s lacks %q", recipeScript, encoder)
		}
	}

	// The renderer source must still name what the recipe provides, or this test is stale.
	renderer := readFile(t, imageRendererSource)
	for _, name := range []string{`"image2pipe"`, `"mjpeg"`, `"png"`} {
		if !strings.Contains(renderer, name) {
			t.Errorf("%s no longer mentions %s; update the recipe and this test", imageRendererSource, name)
		}
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()

	content, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}

	return string(content)
}

// subtitleArgsSource builds the subtitle extraction arguments (M4-08a).
const subtitleArgsSource = "../../../../internal/adapters/ffmpeg/subtitle_args.go"

// TestRecipeBuildsTheWebVTTEncoder: the server extracts text subtitles with -c:s webvtt, so the
// ENCODERS line must keep webvtt (--disable-encoders drops everything not listed).
func TestRecipeBuildsTheWebVTTEncoder(t *testing.T) {
	t.Parallel()

	if !slices.Contains(recipeList(t, readFile(t, recipeScript), "ENCODERS"), "webvtt") {
		t.Errorf("ENCODERS of %s lacks webvtt", recipeScript)
	}

	if !strings.Contains(readFile(t, subtitleArgsSource), `"webvtt"`) {
		t.Errorf("%s no longer names the webvtt encoder; update the recipe and this test", subtitleArgsSource)
	}
}

// recipeList reads the comma separated value of a readonly <name>="..." line.
func recipeList(t *testing.T, script, name string) []string {
	t.Helper()

	match := regexp.MustCompile(`(?m)^readonly ` + name + `="([^"]*)"`).FindStringSubmatch(script)
	if match == nil {
		t.Fatalf("%s has no readonly %s=\"...\" line", recipeScript, name)
	}

	return strings.Split(match[1], ",")
}

// assertExactSet fails on any difference between the recipe's list and the expected set, so
// a new muxer or encoder (attack surface of the static binary) is a deliberate, reviewed change.
func assertExactSet(t *testing.T, name string, got, want []string) {
	t.Helper()

	got = slices.Sorted(slices.Values(got))
	want = slices.Sorted(slices.Values(want))

	if !slices.Equal(got, want) {
		t.Errorf("%s of %s = %v, want exactly %v", name, recipeScript, got, want)
	}
}
