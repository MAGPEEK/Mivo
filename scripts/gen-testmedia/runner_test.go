package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// helperModeVariable switches the test binary into a fake external process, so execRunner is
// tested with a real child process but without FFmpeg.
const helperModeVariable = "GEN_TESTMEDIA_HELPER_MODE"

func TestHelperProcess(t *testing.T) {
	mode := os.Getenv(helperModeVariable)
	if mode == "" {
		t.Skip("helper process for execRunner tests, not a test on its own")
	}

	switch mode {
	case "stdout":
		fmt.Print("Encoders:\n ------\n")
		os.Exit(0)
	case "fail":
		fmt.Fprint(os.Stderr, strings.Repeat("noise ", maxStderrBytes)+"Unknown encoder 'libfoo'\n")
		os.Exit(1)
	case "hang":
		time.Sleep(time.Minute)
		os.Exit(0)
	}
}

func helperRunner(t *testing.T, mode string) execRunner {
	t.Helper()
	t.Setenv(helperModeVariable, mode)

	return execRunner{executable: os.Args[0]}
}

func helperArgs() []string { return []string{"-test.run=^TestHelperProcess$"} }

func TestExecRunnerReturnsStdout(t *testing.T) {
	output, err := helperRunner(t, "stdout").Run(context.Background(), helperArgs())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if !strings.HasPrefix(string(output), "Encoders:") {
		t.Errorf("stdout = %q", output)
	}
}

func TestExecRunnerPutsTruncatedStderrIntoTheError(t *testing.T) {
	_, err := helperRunner(t, "fail").Run(context.Background(), helperArgs())
	if err == nil {
		t.Fatal("Run succeeded, want an error")
	}

	message := err.Error()
	if !strings.Contains(message, "[truncated]") || !strings.HasSuffix(message, "Unknown encoder 'libfoo'") {
		t.Errorf("error does not end with the stderr tail: %q", message)
	}

	if len(message) > maxStderrBytes+len("exit status 1: [truncated] ") {
		t.Errorf("error is %d bytes long, stderr was not truncated", len(message))
	}
}

func TestExecRunnerKillsTheProcessOnCancel(t *testing.T) {
	runner := helperRunner(t, "hang")

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	started := time.Now()

	_, err := runner.Run(ctx, helperArgs())
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("error = %v, want context.DeadlineExceeded", err)
	}

	if elapsed := time.Since(started); elapsed > 30*time.Second {
		t.Errorf("Run returned after %v, the process was not killed", elapsed)
	}
}

func TestExecRunnerReportsMissingExecutable(t *testing.T) {
	t.Parallel()

	runner := execRunner{executable: filepath.Join(t.TempDir(), "no-such-ffmpeg")}
	if _, err := runner.Run(context.Background(), []string{"-version"}); err == nil {
		t.Error("Run succeeded with a missing executable")
	}
}

func TestTailBufferKeepsTheEnd(t *testing.T) {
	t.Parallel()

	buffer := newTailBuffer(5)
	for _, chunk := range []string{"abc", "defg", "hi"} {
		if _, err := buffer.Write([]byte(chunk)); err != nil {
			t.Fatal(err)
		}
	}

	if got := buffer.String(); got != "[truncated] efghi" {
		t.Errorf("String = %q", got)
	}

	short := newTailBuffer(10)
	if _, err := short.Write([]byte(" ok \n")); err != nil {
		t.Fatal(err)
	}

	if got := short.String(); got != "ok" {
		t.Errorf("String = %q", got)
	}
}

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

func TestGenerateBuildsAndRenamesThePartialFile(t *testing.T) {
	t.Parallel()

	ffmpeg := newFixtureFFmpeg(t)
	gen, progress := newTestGenerator(t, ffmpeg, false)
	c := sampleCase("video-a", "video/a.mkv")

	entry := gen.generate(context.Background(), plannedCase{Case: c, Action: actionBuild})

	if entry.Status != statusGenerated || entry.Reason != "" {
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
		wantReason string
	}{
		{name: "skip", action: actionSkip, wantStatus: statusSkipped, wantReason: "why"},
		{name: "skip ignores an old file", action: actionSkip, existing: true, wantStatus: statusSkipped, wantReason: "why"},
		{name: "keep-only lists an existing file", action: actionKeepOnly, existing: true, wantStatus: statusGenerated},
		{name: "keep-only without a file", action: actionKeepOnly, wantStatus: statusSkipped, wantReason: "why"},
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

			if entry.Status != test.wantStatus || entry.Reason != test.wantReason {
				t.Errorf("entry = %s %q, want %s %q", entry.Status, entry.Reason, test.wantStatus, test.wantReason)
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
	gen.jobs = 0 // raised to one worker

	plans := make([]plannedCase, 0, caseCount)
	for index := range caseCount {
		name := fmt.Sprintf("video-%02d", index)
		plans = append(plans, plannedCase{Case: sampleCase(name, "video/"+name+".mkv")})
	}

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

func TestGenerateAllFailsCasesAfterCancellation(t *testing.T) {
	t.Parallel()

	ffmpeg := newFixtureFFmpeg(t)
	gen, _ := newTestGenerator(t, ffmpeg, false)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	entries := gen.generateAll(ctx, []plannedCase{{Case: sampleCase("video-a", "video/a.mkv")}})
	if entries[0].Status != statusFailed || !strings.Contains(entries[0].Reason, context.Canceled.Error()) {
		t.Errorf("entry = %+v", entries[0])
	}
}
