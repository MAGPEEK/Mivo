package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func newTestGenerator(t *testing.T, ffmpeg *fakeFFmpeg, force bool) (*generator, *bytes.Buffer) {
	t.Helper()

	var progress bytes.Buffer

	return &generator{ffmpeg: ffmpeg, outDir: t.TempDir(), force: force, jobs: 2, progress: &progress}, &progress
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func readOutput(t *testing.T, dir, relative string) string {
	t.Helper()

	data, err := fs.ReadFile(os.DirFS(dir), relative)
	if err != nil {
		t.Fatalf("read %s: %v", relative, err)
	}

	return string(data)
}

// subtitleSample is a case with a subtitle input and two sidecar files.
func subtitleSample() mediaCase {
	c := sampleCase("subtitle-a", "subtitle/a.mkv")
	c.Category = categorySubtitle
	c.Encoders = []string{"libx264", "srt"}
	c.Inputs = []textInput{{FileName: "a.srt", Format: "srt", Role: roleSubtitle, Content: "input subtitle"}}
	c.Sidecars = []sidecarFile{
		{Output: "subtitle/a.srt", Content: "sidecar srt"},
		{Output: "subtitle/a.vtt", Content: "sidecar vtt"},
	}

	return c
}

func TestGenerateBuildsAndRenamesThePartialFile(t *testing.T) {
	t.Parallel()

	ffmpeg := newFixtureFFmpeg(t)
	gen, progress := newTestGenerator(t, ffmpeg, false)
	c := sampleCase("video-a", "video/a.mkv")
	c.Encoders = []string{"libx264", "aac"}

	entry := gen.generate(context.Background(), plannedCase{Case: c, Action: actionBuild})

	if entry.Status != statusGenerated || entry.Reason != "" || !slices.Equal(entry.Encoders, c.Encoders) {
		t.Fatalf("entry = %+v", entry)
	}

	if got := readOutput(t, gen.outDir, "video/a.mkv"); got != fakeMediaContent {
		t.Errorf("output content = %q", got)
	}

	outputs := ffmpeg.encodedOutputs()
	if len(outputs) != 1 || !strings.HasSuffix(outputs[0], filepath.FromSlash("video/a.mkv")+partialSuffix) {
		t.Errorf("ffmpeg wrote %v, want the partial file", outputs)
	}

	gen.report(entry)

	if !strings.Contains(progress.String(), "generated video-a") {
		t.Errorf("progress = %q", progress.String())
	}
}

func TestGenerateWritesTextInputsAndSidecars(t *testing.T) {
	t.Parallel()

	ffmpeg := newFixtureFFmpeg(t)
	gen, _ := newTestGenerator(t, ffmpeg, false)

	entry := gen.generate(context.Background(), plannedCase{Case: subtitleSample(), Action: actionBuild})
	if entry.Status != statusGenerated {
		t.Fatalf("entry = %+v", entry)
	}

	// The fake FFmpeg read the input while it existed; afterwards the directory is gone.
	inputs := ffmpeg.textInputs()
	if len(inputs) != 1 || inputs[0].content != "input subtitle" {
		t.Fatalf("ffmpeg saw text inputs %+v", inputs)
	}

	if _, err := os.Stat(filepath.Dir(inputs[0].path)); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("temporary input directory left behind: %v", err)
	}

	for file, want := range map[string]string{"subtitle/a.srt": "sidecar srt", "subtitle/a.vtt": "sidecar vtt"} {
		if got := readOutput(t, gen.outDir, file); got != want {
			t.Errorf("%s = %q, want %q", file, got, want)
		}
	}
}

func TestGenerateRestoresMissingSidecarsOfKeptFiles(t *testing.T) {
	t.Parallel()

	ffmpeg := newFixtureFFmpeg(t)
	gen, _ := newTestGenerator(t, ffmpeg, false)
	writeFile(t, filepath.Join(gen.outDir, "subtitle", "a.mkv"), "old")
	writeFile(t, filepath.Join(gen.outDir, "subtitle", "a.srt"), "edited by hand")

	entry := gen.generate(context.Background(), plannedCase{Case: subtitleSample(), Action: actionBuild})
	if entry.Status != statusGenerated || len(ffmpeg.encodedOutputs()) != 0 {
		t.Fatalf("entry = %+v, encodes = %v", entry, ffmpeg.encodedOutputs())
	}

	if got := readOutput(t, gen.outDir, "subtitle/a.srt"); got != "edited by hand" {
		t.Errorf("existing sidecar was replaced without -force: %q", got)
	}

	if got := readOutput(t, gen.outDir, "subtitle/a.vtt"); got != "sidecar vtt" {
		t.Errorf("missing sidecar was not restored: %q", got)
	}
}

func TestGenerateFailsWhenASidecarCannotBeWritten(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		existing bool
	}{
		{name: "after building"},
		{name: "for a kept file", existing: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			gen, _ := newTestGenerator(t, newFixtureFFmpeg(t), false)
			if test.existing {
				writeFile(t, filepath.Join(gen.outDir, "subtitle", "a.mkv"), "old")
			}

			// A directory where the sidecar belongs makes the rename fail.
			if err := os.MkdirAll(filepath.Join(gen.outDir, "subtitle", "a.vtt", "blocker"), 0o750); err != nil {
				t.Fatal(err)
			}

			entry := gen.generate(context.Background(), plannedCase{Case: subtitleSample(), Action: actionBuild})
			if entry.Status != statusFailed || !strings.Contains(entry.Reason, "sidecar subtitle/a.vtt") {
				t.Errorf("entry = %+v", entry)
			}
		})
	}
}

func TestGenerateKeepsOrRebuildsExistingFiles(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		force       bool
		wantContent string
		wantEncodes int
	}{
		{name: "kept without force", force: false, wantContent: "old", wantEncodes: 0},
		{name: "rebuilt with force", force: true, wantContent: fakeMediaContent, wantEncodes: 1},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			ffmpeg := newFixtureFFmpeg(t)
			gen, _ := newTestGenerator(t, ffmpeg, test.force)
			writeFile(t, filepath.Join(gen.outDir, "video", "a.mkv"), "old")

			entry := gen.generate(context.Background(), plannedCase{Case: sampleCase("video-a", "video/a.mkv")})

			if entry.Status != statusGenerated {
				t.Errorf("status = %s", entry.Status)
			}

			if got := readOutput(t, gen.outDir, "video/a.mkv"); got != test.wantContent {
				t.Errorf("content = %q, want %q", got, test.wantContent)
			}

			if got := len(ffmpeg.encodedOutputs()); got != test.wantEncodes {
				t.Errorf("encodes = %d, want %d", got, test.wantEncodes)
			}
		})
	}
}

func TestGenerateReportsFailuresAndRemovesPartialFiles(t *testing.T) {
	t.Parallel()

	ffmpeg := newFixtureFFmpeg(t)
	ffmpeg.failOutputs["a.mkv"] = "Unknown encoder 'libx264'\nsecond line"
	gen, progress := newTestGenerator(t, ffmpeg, true)
	writeFile(t, filepath.Join(gen.outDir, "video", "a.mkv"), "old")

	entry := gen.generate(context.Background(), plannedCase{Case: sampleCase("video-a", "video/a.mkv")})

	if entry.Status != statusFailed || !strings.Contains(entry.Reason, "Unknown encoder 'libx264'") {
		t.Fatalf("entry = %+v", entry)
	}

	if got := readOutput(t, gen.outDir, "video/a.mkv"); got != "old" {
		t.Errorf("a failed -force run replaced the existing file: %q", got)
	}

	if _, err := os.Stat(filepath.Join(gen.outDir, "video", "a.mkv"+partialSuffix)); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("partial file left behind: %v", err)
	}

	gen.report(entry)

	if strings.Contains(progress.String(), "second line") {
		t.Errorf("progress should only show the first reason line: %q", progress.String())
	}
}

func TestGenerateWithoutBuilding(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		action     action
		existing   bool
		wantStatus entryStatus
		wantKind   skipKind
		wantReason string
	}{
		{name: "skip", action: actionSkip, wantStatus: statusSkipped, wantKind: skipMissingCapability, wantReason: "why"},
		{
			name: "skip ignores an old file", action: actionSkip, existing: true,
			wantStatus: statusSkipped, wantKind: skipMissingCapability, wantReason: "why",
		},
		{name: "gap", action: actionGap, wantStatus: statusSkipped, wantKind: skipKnownGap, wantReason: "why"},
		{name: "keep-only lists an existing file", action: actionKeepOnly, existing: true, wantStatus: statusGenerated},
		{
			name: "keep-only without a file", action: actionKeepOnly,
			wantStatus: statusSkipped, wantKind: skipNotSelected, wantReason: "why",
		},
		{name: "unknown action", action: action(99), wantStatus: statusFailed, wantReason: "unknown plan action 99"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			ffmpeg := newFixtureFFmpeg(t)
			gen, _ := newTestGenerator(t, ffmpeg, false)

			if test.existing {
				writeFile(t, filepath.Join(gen.outDir, "video", "a.mkv"), "old")
			}

			plan := plannedCase{Case: sampleCase("video-a", "video/a.mkv"), Action: test.action, Reason: "why"}
			entry := gen.generate(context.Background(), plan)

			if entry.Status != test.wantStatus || entry.SkipKind != test.wantKind || entry.Reason != test.wantReason {
				t.Errorf("entry = %s %q %q, want %s %q %q", entry.Status, entry.SkipKind, entry.Reason,
					test.wantStatus, test.wantKind, test.wantReason)
			}

			if len(entry.Encoders) != 0 {
				t.Errorf("encoders %v recorded for a case this run did not build", entry.Encoders)
			}

			if len(ffmpeg.encodedOutputs()) != 0 {
				t.Error("ffmpeg ran for a case that must not be built")
			}
		})
	}
}

func TestGenerateFailsWhenTheOutputDirectoryCannotBeCreated(t *testing.T) {
	t.Parallel()

	ffmpeg := newFixtureFFmpeg(t)
	gen, _ := newTestGenerator(t, ffmpeg, false)
	writeFile(t, filepath.Join(gen.outDir, "video"), "a file where a directory belongs")

	entry := gen.generate(context.Background(), plannedCase{Case: sampleCase("video-a", "video/a.mkv")})

	if entry.Status != statusFailed || !strings.Contains(entry.Reason, "create output directory") {
		t.Errorf("entry = %+v", entry)
	}
}

func TestGenerateAllKeepsPlanOrder(t *testing.T) {
	t.Parallel()

	const caseCount = 12

	ffmpeg := newFixtureFFmpeg(t)
	gen, _ := newTestGenerator(t, ffmpeg, false)

	plans := make([]plannedCase, 0, caseCount)
	for index := range caseCount {
		name := fmt.Sprintf("video-%02d", index)
		plans = append(plans, plannedCase{Case: sampleCase(name, "video/"+name+".mkv")})
	}

	// Zero jobs are raised to one worker.
	for _, jobs := range []int{0, 4} {
		gen.jobs = jobs
		gen.force = true

		entries := gen.generateAll(context.Background(), plans)
		for index, entry := range entries {
			if entry.Name != plans[index].Case.Name || entry.Status != statusGenerated {
				t.Errorf("jobs=%d entry %d = %+v", jobs, index, entry)
			}
		}
	}
}

func TestGenerateAllAfterCancellationDoesNotFailUntouchedCases(t *testing.T) {
	t.Parallel()

	ffmpeg := newFixtureFFmpeg(t)
	gen, _ := newTestGenerator(t, ffmpeg, true)
	writeFile(t, filepath.Join(gen.outDir, "video", "kept.mkv"), "old")

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	entries := gen.generateAll(ctx, []plannedCase{
		{Case: sampleCase("video-a", "video/a.mkv")},
		{Case: sampleCase("video-kept", "video/kept.mkv")},
	})

	if entries[0].Status != statusSkipped || entries[0].SkipKind != skipInterrupted || entries[0].Reason != interruptedReason {
		t.Errorf("untouched case = %+v", entries[0])
	}

	if entries[1].Status != statusGenerated {
		t.Errorf("a file from an earlier run must stay listed: %+v", entries[1])
	}

	if len(ffmpeg.encodedOutputs()) != 0 {
		t.Error("ffmpeg ran after the run was interrupted")
	}
}

func TestGenerateTreatsAFailureDuringInterruptionAsInterrupted(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	ffmpeg := newFixtureFFmpeg(t)
	// The fake cancels the run while FFmpeg works, like Ctrl+C during an encode.
	ffmpeg.onEncode = cancel
	ffmpeg.failOutputs["a.mkv"] = "killed"
	gen, _ := newTestGenerator(t, ffmpeg, false)

	entry := gen.generate(ctx, plannedCase{Case: sampleCase("video-a", "video/a.mkv")})
	if entry.Status != statusSkipped || entry.SkipKind != skipInterrupted {
		t.Errorf("entry = %+v", entry)
	}
}

func TestWriteTextInputsReportsUnwritableFiles(t *testing.T) {
	t.Parallel()

	// A file name with a directory that does not exist inside the temporary directory.
	dir, err := writeTextInputs([]textInput{{FileName: filepath.Join("missing", "a.srt"), Content: "x"}})
	if err == nil || dir != "" {
		t.Errorf("writeTextInputs = %q, %v; want an error", dir, err)
	}
}

// keptEncodersTest is one row of TestGenerateListsKeptFilesWithTheirPreviousEncoders.
type keptEncodersTest struct {
	name        string
	action      action
	force       bool
	interrupted bool
	// previousFile is the file of the earlier manifest entry; "" means there is none.
	previousFile string
	wantEncoders []string
	wantLabel    string
}

// The earlier run built the file with another alternative than this run would choose.
func earlierEncoders() []string { return []string{"libaom-av1", "libopus"} }

func chosenEncoders() []string { return []string{"libsvtav1", "libopus"} }

func TestGenerateListsKeptFilesWithTheirPreviousEncoders(t *testing.T) {
	t.Parallel()

	tests := []keptEncodersTest{
		{name: "kept without force", action: actionBuild, previousFile: "video/a.mkv", wantEncoders: earlierEncoders(), wantLabel: "kept"},
		{name: "kept by -only", action: actionKeepOnly, previousFile: "video/a.mkv", wantEncoders: earlierEncoders(), wantLabel: "kept"},
		{
			name: "kept by an interrupted force run", action: actionBuild, force: true, interrupted: true,
			previousFile: "video/a.mkv", wantEncoders: earlierEncoders(), wantLabel: "kept",
		},
		{name: "kept without an earlier entry", action: actionBuild, wantLabel: "kept"},
		{name: "earlier entry for another file", action: actionBuild, previousFile: "video/renamed.mkv", wantLabel: "kept"},
		{
			name: "rebuilt with force", action: actionBuild, force: true,
			previousFile: "video/a.mkv", wantEncoders: chosenEncoders(), wantLabel: "generated",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			checkKeptEncoders(t, test)
		})
	}
}

func checkKeptEncoders(t *testing.T, test keptEncodersTest) {
	t.Helper()

	gen, progress := newTestGenerator(t, newFixtureFFmpeg(t), test.force)
	writeFile(t, filepath.Join(gen.outDir, "video", "a.mkv"), "old")

	previous := earlierEncoders()
	if test.previousFile != "" {
		gen.previous = previousEncoders{"video-a": {file: test.previousFile, encoders: previous}}
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if test.interrupted {
		cancel()
	}

	c := sampleCase("video-a", "video/a.mkv")
	c.Encoders = chosenEncoders()
	entry := gen.generate(ctx, plannedCase{Case: c, Action: test.action, Reason: "why"})

	if entry.Status != statusGenerated || !slices.Equal(entry.Encoders, test.wantEncoders) {
		t.Errorf("entry = %s %v, want generated %v", entry.Status, entry.Encoders, test.wantEncoders)
	}

	gen.report(entry)

	if want := fmt.Sprintf("%-9s video-a\n", test.wantLabel); progress.String() != want {
		t.Errorf("progress = %q, want %q", progress.String(), want)
	}

	// The entry owns its encoder list.
	if len(entry.Encoders) == 0 {
		return
	}

	entry.Encoders[0] = "changed"
	if !slices.Equal(c.Encoders, chosenEncoders()) || !slices.Equal(previous, earlierEncoders()) {
		t.Error("the entry shares its encoder list with the case or the previous manifest")
	}
}
