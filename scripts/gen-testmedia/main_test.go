package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestParseOptions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		args []string
		want options
	}{
		{
			name: "defaults",
			want: options{outDir: defaultOutDir, ffmpeg: defaultFFmpeg, jobs: runtime.NumCPU()},
		},
		{
			name: "all flags",
			args: []string{"-out", "media", "-ffmpeg", "/opt/ffmpeg", "-force", "-only", "video-*", "-jobs", "3"},
			want: options{outDir: "media", ffmpeg: "/opt/ffmpeg", force: true, onlyGlob: "video-*", jobs: 3},
		},
		{
			name: "jobs below one are raised to one",
			args: []string{"-jobs", "-2"},
			want: options{outDir: defaultOutDir, ffmpeg: defaultFFmpeg, jobs: 1},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			got, err := parseOptions(test.args, io.Discard)
			if err != nil {
				t.Fatalf("parseOptions: %v", err)
			}

			if got != test.want {
				t.Errorf("options = %+v, want %+v", got, test.want)
			}
		})
	}
}

func TestParseOptionsRejectsInvalidCommandLines(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		args       []string
		wantStderr string
	}{
		{name: "malformed glob", args: []string{"-only", "video-["}, wantStderr: "invalid -only glob"},
		{name: "positional argument", args: []string{"extra"}, wantStderr: `unexpected argument "extra"`},
		{name: "unknown flag", args: []string{"-bogus"}, wantStderr: "flag provided but not defined"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			var stderr bytes.Buffer
			if _, err := parseOptions(test.args, &stderr); err == nil {
				t.Fatal("parseOptions accepted an invalid command line")
			}

			if !strings.Contains(stderr.String(), test.wantStderr) || !strings.Contains(stderr.String(), "-jobs") {
				t.Errorf("stderr lacks %q and the usage text:\n%s", test.wantStderr, stderr.String())
			}
		})
	}
}

func TestRunExitCodes(t *testing.T) {
	t.Parallel()

	missingFFmpeg := filepath.Join(t.TempDir(), "no-such-ffmpeg")

	tests := []struct {
		name       string
		args       []string
		wantCode   int
		wantStderr string
	}{
		{name: "help", args: []string{"-h"}, wantCode: exitOK},
		{name: "usage error", args: []string{"-jobs", "many"}, wantCode: exitUsage},
		{
			name:       "missing ffmpeg",
			args:       []string{"-ffmpeg", missingFFmpeg, "-out", t.TempDir()},
			wantCode:   exitFailure,
			wantStderr: "install FFmpeg or pass -ffmpeg",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			var stdout, stderr bytes.Buffer

			if code := run(context.Background(), test.args, &stdout, &stderr); code != test.wantCode {
				t.Fatalf("exit code = %d, want %d; stderr:\n%s", code, test.wantCode, stderr.String())
			}

			if !strings.Contains(stderr.String(), test.wantStderr) {
				t.Errorf("stderr lacks %q:\n%s", test.wantStderr, stderr.String())
			}
		})
	}
}

func TestParseOptionsHelpIsNotAnError(t *testing.T) {
	t.Parallel()

	if _, err := parseOptions([]string{"-help"}, io.Discard); !errors.Is(err, flag.ErrHelp) {
		t.Errorf("error = %v, want flag.ErrHelp", err)
	}
}

func readManifest(t *testing.T, dir string) manifest {
	t.Helper()

	data, err := fs.ReadFile(os.DirFS(dir), manifestFileName)
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}

	var result manifest
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("decode manifest: %v", err)
	}

	return result
}

func TestGenerateCorpusWithFixtureCapabilities(t *testing.T) {
	t.Parallel()

	ffmpeg := newFixtureFFmpeg(t)
	opts := options{outDir: t.TempDir(), ffmpeg: "fake", jobs: 4}

	var stdout bytes.Buffer
	if err := generateCorpus(context.Background(), opts, ffmpeg, &stdout); err != nil {
		t.Fatalf("generateCorpus: %v", err)
	}

	result := readManifest(t, opts.outDir)
	if result.FFmpegVersion != "9.0.2-full_build-www.gyan.dev" {
		t.Errorf("ffmpeg version = %q", result.FFmpegVersion)
	}

	// The fixture build has every encoder and muxer the cases need; only known gaps remain.
	gaps := knownGapNames()
	if got, want := result.count(statusGenerated), len(groupByName(allCases()))-len(gaps); got != want {
		t.Errorf("generated %d cases, want %d: %+v", got, want, result.Cases)
	}

	for _, want := range []string{"  audio ", "  image ", "  multitrack ", "  subtitle ", "  video ", "  total "} {
		if !strings.Contains(stdout.String(), want) {
			t.Errorf("summary lacks the line %q:\n%s", want, stdout.String())
		}
	}

	for _, entry := range result.Cases {
		if entry.Status != statusGenerated {
			continue
		}

		if got := readOutput(t, opts.outDir, entry.File); got != fakeMediaContent {
			t.Errorf("%s: content = %q", entry.File, got)
		}
	}

	if got := readOutput(t, opts.outDir, "subtitle/sidecar.srt"); got != formatSRT(englishCues()) {
		t.Errorf("sidecar SRT = %q", got)
	}
}

// knownGapNames returns the names of all known gap cases.
func knownGapNames() []string {
	var names []string

	for _, c := range allCases() {
		if c.isGap() {
			names = append(names, c.Name)
		}
	}

	return names
}

func TestGenerateCorpusListsKnownGapsInEveryRun(t *testing.T) {
	t.Parallel()

	// Even a run that selects nothing lists the gaps, with a reason naming the missing piece.
	opts := options{outDir: t.TempDir(), ffmpeg: "fake", onlyGlob: "nothing", jobs: 1}
	if err := generateCorpus(context.Background(), opts, newFixtureFFmpeg(t), io.Discard); err != nil {
		t.Fatalf("generateCorpus: %v", err)
	}

	result := readManifest(t, opts.outDir)
	for _, name := range knownGapNames() {
		entry := manifestEntryByName(t, result, name)
		if entry.Status != statusSkipped || entry.SkipKind != skipKnownGap ||
			!strings.HasPrefix(entry.Reason, gapReasonPrefix+"ffmpeg ") {
			t.Errorf("gap entry = %+v", entry)
		}
	}
}

func TestGenerateCorpusFiltersAndReportsFailures(t *testing.T) {
	t.Parallel()

	ffmpeg := newFixtureFFmpeg(t)
	ffmpeg.failOutputs["h264_aac.mkv"] = "Conversion failed!"
	opts := options{outDir: t.TempDir(), ffmpeg: "fake", onlyGlob: "video-mkv-*", jobs: 1}

	err := generateCorpus(context.Background(), opts, ffmpeg, io.Discard)
	if !errors.Is(err, errCasesFailed) {
		t.Fatalf("error = %v, want errCasesFailed", err)
	}

	result := readManifest(t, opts.outDir)
	statuses := map[string]entryStatus{}

	for _, entry := range result.Cases {
		statuses[entry.Name] = entry.Status
	}

	want := map[string]entryStatus{
		"video-mkv-h264-aac":           statusFailed,
		"video-mkv-hevc10-ac3":         statusGenerated,
		"video-mkv-hevc10-hdr10-ac3":   statusGenerated,
		"video-mp4-h264-aac-faststart": statusSkipped,
	}
	for name, status := range want {
		if statuses[name] != status {
			t.Errorf("%s: status = %q, want %q", name, statuses[name], status)
		}
	}

	if len(result.Cases) != len(groupByName(allCases())) {
		t.Errorf("a filtered run must still list every case, got %d", len(result.Cases))
	}
}

func TestGenerateCorpusReportsInterruption(t *testing.T) {
	t.Parallel()

	t.Run("before capability detection", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		err := generateCorpus(ctx, options{outDir: t.TempDir(), ffmpeg: "fake", jobs: 1}, newFixtureFFmpeg(t), io.Discard)
		if !errors.Is(err, errInterrupted) || !errors.Is(err, context.Canceled) {
			t.Errorf("error = %v, want errInterrupted wrapping context.Canceled", err)
		}
	})

	t.Run("during generation", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithCancel(context.Background())
		ffmpeg := newFixtureFFmpeg(t)
		ffmpeg.onEncode = cancel
		opts := options{outDir: t.TempDir(), ffmpeg: "fake", onlyGlob: "audio-*", jobs: 1}

		var stdout bytes.Buffer

		err := generateCorpus(ctx, opts, ffmpeg, &stdout)
		if !errors.Is(err, errInterrupted) {
			t.Fatalf("error = %v, want errInterrupted", err)
		}

		// The manifest still matches the disk, and nothing counts as failed.
		result := readManifest(t, opts.outDir)
		if result.count(statusFailed) != 0 || !strings.Contains(stdout.String(), "interrupted") {
			t.Errorf("failed = %d, summary:\n%s", result.count(statusFailed), stdout.String())
		}
	})
}

func TestGenerateCorpusFailsWhenTheManifestCannotBeWritten(t *testing.T) {
	t.Parallel()

	blocker := filepath.Join(t.TempDir(), "file")
	writeFile(t, blocker, "x")

	opts := options{outDir: blocker, ffmpeg: "fake", onlyGlob: "none", jobs: 1}
	if err := generateCorpus(context.Background(), opts, newFixtureFFmpeg(t), io.Discard); err == nil {
		t.Error("generateCorpus succeeded although the manifest could not be written")
	}
}
