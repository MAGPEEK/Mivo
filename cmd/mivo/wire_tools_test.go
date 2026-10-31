package main

import (
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/playback"
	"github.com/magpeek/mivo/internal/platform/config"
)

func TestResolveTool(t *testing.T) {
	t.Parallel()

	directory := t.TempDir()
	executable := writeToolFile(t, directory, "ffmpeg.exe")
	script := writeToolFile(t, directory, "ffmpeg.bat")
	upperScript := writeToolFile(t, directory, "ffprobe.CMD")
	vanished := filepath.Join(directory, "vanished.exe")

	found := func(path string) func(string) (string, error) {
		return func(string) (string, error) { return path, nil }
	}
	failing := func(err error) func(string) (string, error) {
		return func(file string) (string, error) { return "", &exec.Error{Name: file, Err: err} }
	}

	tests := []struct {
		name     string
		goos     string
		lookPath func(string) (string, error)
		want     string
		wantErr  error
	}{
		{name: "found executable", goos: windowsOS, lookPath: found(executable), want: executable},
		{name: "not on PATH", goos: "linux", lookPath: failing(exec.ErrNotFound), wantErr: errToolMissing},
		{name: "configured path missing", goos: "linux", lookPath: failing(fs.ErrNotExist), wantErr: errToolMissing},
		{name: "vanished after the lookup", goos: "linux", lookPath: found(vanished), wantErr: errToolMissing},
		{name: "lookup refused", goos: "linux", lookPath: failing(fs.ErrPermission), wantErr: fs.ErrPermission},
		{name: "found only in the working directory", goos: windowsOS, lookPath: failing(exec.ErrDot), wantErr: exec.ErrDot},
		{name: "directory", goos: "linux", lookPath: found(directory), wantErr: errToolNotRegularFile},
		{name: "batch script on windows", goos: windowsOS, lookPath: found(script), wantErr: errToolIsBatchScript},
		{name: "upper-case cmd script on windows", goos: windowsOS, lookPath: found(upperScript), wantErr: errToolIsBatchScript},
		{name: "batch script with a trailing dot", goos: windowsOS, lookPath: found(script + "."), wantErr: errToolIsBatchScript},
		{name: "batch script with trailing spaces", goos: windowsOS, lookPath: found(script + "  "), wantErr: errToolIsBatchScript},
		{name: "bat name elsewhere is a regular file", goos: "linux", lookPath: found(script), want: script},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			got, err := resolveTool(ffmpegSetting, "ffmpeg", toolHost{goos: test.goos, lookPath: test.lookPath})

			if test.wantErr == nil {
				if err != nil || got != test.want {
					t.Fatalf("resolveTool = %q, %v; want %q", got, err, test.want)
				}

				return
			}

			assertToolError(t, got, err, test.wantErr)
		})
	}
}

// assertToolError checks a failed resolution: it wraps want, names the setting, and counts
// as missing (a degraded start) only when want is errToolMissing.
func assertToolError(t *testing.T, got string, err, want error) {
	t.Helper()

	if !errors.Is(err, want) || got != "" {
		t.Fatalf("resolveTool = %q, %v; want an error wrapping %v", got, err, want)
	}

	if !strings.Contains(err.Error(), ffmpegSetting) {
		t.Errorf("error %q does not name the setting %s", err, ffmpegSetting)
	}

	if errors.Is(err, errToolMissing) != errors.Is(want, errToolMissing) {
		t.Errorf("error %v: counts as missing = %t, want %t", err, errors.Is(err, errToolMissing), errors.Is(want, errToolMissing))
	}
}

func TestResolveToolMakesRelativeResultsAbsolute(t *testing.T) {
	t.Parallel()

	directory := t.TempDir()
	writeToolFile(t, directory, "ffprobe")

	relative, err := filepath.Rel(mustWorkingDirectory(t), filepath.Join(directory, "ffprobe"))
	if err != nil {
		t.Skipf("temp dir has no path relative to the working directory: %v", err)
	}

	got, err := resolveTool(ffprobeSetting, "ffprobe", toolHost{
		goos:     "linux",
		lookPath: func(string) (string, error) { return relative, nil },
	})
	if err != nil || !filepath.IsAbs(got) {
		t.Fatalf("resolveTool = %q, %v; want an absolute path", got, err)
	}
}

// TestRunRefusesABatchScriptAsTool goes through the real exec.LookPath, which on Windows
// resolves a configured path with PATHEXT.
func TestRunRefusesABatchScriptAsTool(t *testing.T) {
	t.Parallel()

	if runtime.GOOS != windowsOS {
		t.Skip("batch scripts are a Windows hazard; TestResolveTool covers the rule on every OS")
	}

	environment := isolatedEnvironment(t)
	environment[envToolsFFmpeg] = writeToolFile(t, t.TempDir(), "ffmpeg.bat")

	assertStartFails(t, environment, "batch script")
}

func writeToolFile(t *testing.T, directory, name string) string {
	t.Helper()

	path := filepath.Join(directory, name)
	// Not executable: lookPath is faked, and Windows does not check the mode.
	if err := os.WriteFile(path, []byte("@echo off\n"), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}

	return path
}

func mustWorkingDirectory(t *testing.T) string {
	t.Helper()

	directory, err := os.Getwd()
	if err != nil {
		t.Fatalf("working directory: %v", err)
	}

	return directory
}

// assertStartFails runs the server with environment and expects a startup error containing
// wantPart.
func assertStartFails(t *testing.T, environment map[string]string, wantPart string) string {
	t.Helper()

	env, stdout, stderr := newTestEnv(environment)

	if code := run(t.Context(), nil, env, runHooks{}); code != exitFailure {
		t.Fatalf("exit code = %d, want %d (stderr: %q)", code, exitFailure, stderr.String())
	}

	assertStartupError(t, stderr.String(), wantPart)
	assertNoOutput(t, stdout)

	return stderr.String()
}

// missingToolPath is an absolute path where no tool exists.
func missingToolPath(t *testing.T, name string) string {
	t.Helper()

	return filepath.Join(t.TempDir(), "missing", fmt.Sprintf("%s-not-installed", name))
}

func TestLogCapabilitiesNamesTheMissingMode(t *testing.T) {
	t.Parallel()

	full := playback.ServerCapabilities{FFmpegVersion: "9.0.2", HLSMuxing: true, H264Encoder: "libx264", AACEncoder: "aac"}
	noEncoder := full
	noEncoder.H264Encoder = ""
	noMuxer := full
	noMuxer.HLSMuxing = false

	tests := []struct {
		name         string
		capabilities playback.ServerCapabilities
		want         []string
	}{
		{name: "everything", capabilities: full, want: []string{"level=INFO", "direct play, remux and transcode", "version=9.0.2", "h264_encoder=libx264"}},
		{name: "no encoder", capabilities: noEncoder, want: []string{"level=WARN", "no transcoding", `h264_encoder=""`}},
		{name: "no hls muxer", capabilities: noMuxer, want: []string{"level=WARN", "only direct play available", "hls_muxing=false"}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			logs := &syncBuffer{}
			app := application{logger: slog.New(slog.NewTextHandler(logs, nil))}

			app.logCapabilities(t.Context(), test.capabilities)
			assertLogLine(t, logs.String(), test.want...)
		})
	}
}

// TestRunStartsWithAnFFmpegThatCannotAnswer: a file that is no working ffmpeg fails the
// capability detection, which only degrades the server to direct play.
func TestRunStartsWithAnFFmpegThatCannotAnswer(t *testing.T) {
	t.Parallel()

	environment := isolatedEnvironment(t)
	environment[envToolsFFmpeg] = writeBrokenTool(t)

	server := startTestServer(t, environment, runHooks{})
	assertHealthy(t, server.baseURL)
	server.stop(t)

	const failure = "ffmpeg capability detection failed: only direct play available"

	logs := server.stderr.String()
	assertLogLine(t, logs, "level=WARN", failure)

	// The adapter logs the failure; the composition root must not repeat it (M2-39).
	if count := strings.Count(logs, failure); count != 1 {
		t.Errorf("the capability failure is logged %d times, want once:\n%s", count, logs)
	}
}

// TestDetectCapabilitiesFailsTheStartOnAWiringError: ErrInvalid from the adapter is a
// programming error (here a missing logger), not an FFmpeg that cannot answer, so it must not
// degrade the server silently to direct play.
func TestDetectCapabilitiesFailsTheStartOnAWiringError(t *testing.T) {
	t.Parallel()

	app := application{}

	_, err := app.detectCapabilities(t.Context(), filepath.Join(t.TempDir(), "ffmpeg.exe"), true)
	if !errors.Is(err, domain.ErrInvalid) {
		t.Errorf("detectCapabilities error = %v, want ErrInvalid", err)
	}
}

// writeBrokenTool writes an executable that fails at once: a script exiting with an error on
// unix, a text file the loader refuses on Windows.
func writeBrokenTool(t *testing.T) string {
	t.Helper()

	name, content := "ffmpeg", "#!/bin/sh\necho broken >&2\nexit 3\n"
	if runtime.GOOS == windowsOS {
		name, content = "ffmpeg.exe", "not a program"
	}

	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}

	// The PATH lookup only accepts executable files on unix.
	if err := os.Chmod(path, 0o700); err != nil { //nolint:gosec // a test tool must be executable
		t.Fatalf("make %s executable: %v", path, err)
	}

	return path
}

// TestMissingToolIsNeverLookedUpAgain: a missing tool must not reach the adapters as a bare
// name, which os/exec would look up on PATH again at every run.
func TestMissingToolIsNeverLookedUpAgain(t *testing.T) {
	t.Parallel()

	dataDir := t.TempDir()
	app := application{
		config: config.Config{Paths: config.Paths{Data: dataDir}},
		logger: slog.New(slog.DiscardHandler),
		tools: toolHost{goos: runtime.GOOS, lookPath: func(file string) (string, error) {
			return "", &exec.Error{Name: file, Err: exec.ErrNotFound}
		}},
	}

	path, found, err := app.resolveConfiguredTool(t.Context(), ffprobeSetting, "ffprobe")
	if err != nil || found {
		t.Fatalf("resolveConfiguredTool = %q, %t, %v; want a missing tool without an error", path, found, err)
	}

	if want := filepath.Join(dataDir, missingToolsDirectory, "ffprobe"); path != want || !filepath.IsAbs(path) {
		t.Errorf("path = %q, want the absolute %q", path, want)
	}

	if _, err := os.Stat(filepath.Dir(path)); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("stat %s = %v, want the directory to stay absent", filepath.Dir(path), err)
	}
}
