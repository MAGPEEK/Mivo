package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"testing"
	"time"
)

const (
	// repositoryCorpus is the default output directory seen from this package directory.
	repositoryCorpus = "../../testdata/media"
	// durationTolerance allows for container and codec framing around the 2 s clips.
	durationTolerance = 0.25
	hdr10SideData     = "Mastering display metadata"
	contentLightLevel = "Content light level metadata"
)

// requireFFmpeg returns the paths of ffmpeg and ffprobe or skips the test.
func requireFFmpeg(t *testing.T) (ffmpegPath, ffprobePath string) {
	t.Helper()

	ffmpegPath, ffmpegErr := exec.LookPath("ffmpeg")
	ffprobePath, ffprobeErr := exec.LookPath("ffprobe")

	if ffmpegErr != nil || ffprobeErr != nil {
		t.Skip("skipping FFmpeg integration test: ffmpeg and ffprobe must be on PATH " +
			"(install FFmpeg, see docs/adr/0021-generated-test-media-corpus.md)")
	}

	return ffmpegPath, ffprobePath
}

func TestIntegrationGeneratesMKVCase(t *testing.T) {
	ffmpegPath, ffprobePath := requireFFmpeg(t)
	t.Parallel()

	outDir := t.TempDir()
	args := []string{"-out", outDir, "-ffmpeg", ffmpegPath, "-only", "video-mkv-h264-aac", "-jobs", "1"}

	var stdout, stderr bytes.Buffer
	if code := run(context.Background(), args, &stdout, &stderr); code != exitOK {
		t.Fatalf("exit code %d\nstdout:\n%s\nstderr:\n%s", code, stdout.String(), stderr.String())
	}

	entry := manifestEntryByName(t, readManifest(t, outDir), "video-mkv-h264-aac")
	if entry.Status != statusGenerated {
		t.Fatalf("entry = %+v", entry)
	}

	verifyEntry(t, ffprobePath, outDir, entry)

	// A second run keeps the file instead of encoding it again.
	path := filepath.Join(outDir, filepath.FromSlash(entry.File))
	before := modificationTime(t, path)

	if code := run(context.Background(), args, &stdout, &stderr); code != exitOK {
		t.Fatalf("second run exit code %d: %s", code, stderr.String())
	}

	if after := modificationTime(t, path); !after.Equal(before) {
		t.Errorf("existing file was rewritten without -force (%v -> %v)", before, after)
	}
}

// TestIntegrationCorpusMatchesManifest verifies every generated file of the repository corpus
// (`go run ./scripts/gen-testmedia`) with ffprobe against its expected properties.
func TestIntegrationCorpusMatchesManifest(t *testing.T) {
	_, ffprobePath := requireFFmpeg(t)
	t.Parallel()

	if _, err := os.Stat(filepath.Join(repositoryCorpus, manifestFileName)); err != nil {
		t.Skipf("skipping corpus check: no corpus at %s (run `go run ./scripts/gen-testmedia`)", repositoryCorpus)
	}

	for _, entry := range readManifest(t, repositoryCorpus).Cases {
		if entry.Status != statusGenerated {
			continue
		}

		t.Run(entry.Name, func(t *testing.T) {
			t.Parallel()
			verifyEntry(t, ffprobePath, repositoryCorpus, entry)
		})
	}
}

func manifestEntryByName(t *testing.T, result manifest, name string) manifestEntry {
	t.Helper()

	index := slices.IndexFunc(result.Cases, func(entry manifestEntry) bool { return entry.Name == name })
	if index < 0 {
		t.Fatalf("manifest has no entry %q", name)
	}

	return result.Cases[index]
}

func modificationTime(t *testing.T, path string) time.Time {
	t.Helper()

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}

	return info.ModTime()
}

// probeResult is the subset of `ffprobe -print_format json -show_format -show_streams`.
type probeResult struct {
	Format struct {
		FormatName string `json:"format_name"`
		Duration   string `json:"duration"`
	} `json:"format"`
	Streams []probeStream `json:"streams"`
}

type probeStream struct {
	CodecType      string `json:"codec_type"`
	CodecName      string `json:"codec_name"`
	Profile        string `json:"profile"`
	CodecTagString string `json:"codec_tag_string"`
	PixelFormat    string `json:"pix_fmt"`
	ColorPrimaries string `json:"color_primaries"`
	ColorTransfer  string `json:"color_transfer"`
	Width          int    `json:"width"`
	Height         int    `json:"height"`
	SampleRate     string `json:"sample_rate"`
	Channels       int    `json:"channels"`
}

func runFFprobe(t *testing.T, ffprobePath string, args ...string) []byte {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), capabilityTimeout)
	defer cancel()

	output, err := execRunner{executable: ffprobePath}.Run(ctx, append([]string{"-v", "error"}, args...))
	if err != nil {
		t.Fatalf("ffprobe %v: %v", args, err)
	}

	return output
}

func probeFile(t *testing.T, ffprobePath, path string) probeResult {
	t.Helper()

	output := runFFprobe(t, ffprobePath, "-print_format", "json", "-show_format", "-show_streams", path)

	var result probeResult
	if err := json.Unmarshal(output, &result); err != nil {
		t.Fatalf("decode ffprobe output for %s: %v", path, err)
	}

	return result
}

// verifyEntry checks a generated file against the expected properties of its entry.
func verifyEntry(t *testing.T, ffprobePath, dir string, entry manifestEntry) {
	t.Helper()

	path := filepath.Join(dir, filepath.FromSlash(entry.File))
	if _, err := fs.Stat(os.DirFS(dir), entry.File); err != nil {
		t.Fatalf("generated file missing: %v", err)
	}

	result := probeFile(t, ffprobePath, path)
	expected := entry.Expected

	check(t, "format", result.Format.FormatName, expected.ProbeFormat)
	checkDuration(t, result.Format.Duration)

	if expected.Video != nil {
		verifyVideo(t, streamOfType(t, result, "video"), *expected.Video)

		if expected.Video.HDR == "hdr10" {
			verifyHDR10SideData(t, ffprobePath, path)
		}
	}

	if expected.Audio != nil {
		verifyAudio(t, streamOfType(t, result, "audio"), *expected.Audio)
	}
}

func verifyVideo(t *testing.T, stream probeStream, want videoExpectation) {
	t.Helper()

	check(t, "video codec", stream.CodecName, want.Codec)
	checkOptional(t, "video profile", stream.Profile, want.Profile)
	checkOptional(t, "codec tag", stream.CodecTagString, want.CodecTag)
	checkOptional(t, "pixel format", stream.PixelFormat, want.PixelFormat)
	checkOptional(t, "color primaries", stream.ColorPrimaries, want.ColorPrimaries)
	checkOptional(t, "color transfer", stream.ColorTransfer, want.ColorTransfer)
	check(t, "size", fmt.Sprintf("%dx%d", stream.Width, stream.Height), fmt.Sprintf("%dx%d", want.Width, want.Height))
}

func verifyAudio(t *testing.T, stream probeStream, want audioExpectation) {
	t.Helper()

	check(t, "audio codec", stream.CodecName, want.Codec)
	check(t, "sample rate", stream.SampleRate, strconv.Itoa(want.SampleRate))
	check(t, "channels", strconv.Itoa(stream.Channels), strconv.Itoa(want.Channels))
}

// verifyHDR10SideData checks the first frame for HDR10 mastering display and light level SEI.
func verifyHDR10SideData(t *testing.T, ffprobePath, path string) {
	t.Helper()

	output := runFFprobe(t, ffprobePath, "-select_streams", "v:0", "-read_intervals", "%+#1",
		"-show_frames", "-show_entries", "frame=side_data_list", "-print_format", "json", path)

	var frames struct {
		Frames []struct {
			SideData []struct {
				Type string `json:"side_data_type"`
			} `json:"side_data_list"`
		} `json:"frames"`
	}
	if err := json.Unmarshal(output, &frames); err != nil {
		t.Fatalf("decode frame side data: %v", err)
	}

	found := map[string]bool{}

	for _, frame := range frames.Frames {
		for _, sideData := range frame.SideData {
			found[sideData.Type] = true
		}
	}

	if !found[hdr10SideData] || !found[contentLightLevel] {
		t.Errorf("first frame lacks HDR10 side data, found %v", found)
	}
}

func streamOfType(t *testing.T, result probeResult, codecType string) probeStream {
	t.Helper()

	index := slices.IndexFunc(result.Streams, func(stream probeStream) bool { return stream.CodecType == codecType })
	if index < 0 {
		t.Fatalf("no %s stream in %+v", codecType, result.Streams)
	}

	return result.Streams[index]
}

func checkDuration(t *testing.T, reported string) {
	t.Helper()

	seconds, err := strconv.ParseFloat(reported, 64)
	if err != nil {
		t.Errorf("duration %q is not a number", reported)

		return
	}

	if want := clipDuration.Seconds(); seconds < want-durationTolerance || seconds > want+durationTolerance {
		t.Errorf("duration = %.3f s, want %.1f s ± %.2f", seconds, want, durationTolerance)
	}
}

func check(t *testing.T, property, got, want string) {
	t.Helper()

	if got != want {
		t.Errorf("%s = %q, want %q", property, got, want)
	}
}

func checkOptional(t *testing.T, property, got, want string) {
	t.Helper()

	if want != "" {
		check(t, property, got, want)
	}
}
