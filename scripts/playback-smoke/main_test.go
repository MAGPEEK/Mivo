package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"uuid"

	"github.com/magpeek/mivo/internal/adapters/corpustest"
	"github.com/magpeek/mivo/internal/adapters/http/apigen"
)

func apiRequestForTest() apigen.PlaybackRequest {
	return apigen.PlaybackRequest{FileID: uuid.New(), Profile: apigen.ClientProfile{Name: corpustest.ChromiumDesktop}}
}

func TestRunRejectsInvalidCommandLines(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "no base", args: nil, want: "-base must be an absolute http or https URL"},
		{name: "relative base", args: []string{"-base", "localhost:8460"}, want: "-base must be"},
		{name: "ftp base", args: []string{"-base", "ftp://nas"}, want: "-base must be"},
		{name: "extra argument", args: []string{"-base", "http://nas:8460", "now"}, want: `unexpected argument "now"`},
		{name: "zero timeout", args: []string{"-base", "http://nas:8460", "-case-timeout", "0s"}, want: "must be positive"},
		{name: "unknown flag", args: []string{"-bse", "http://nas:8460"}, want: "flag provided but not defined"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			var stdout, stderr bytes.Buffer

			if code := run(t.Context(), test.args, &stdout, &stderr); code != exitUsage {
				t.Errorf("run = %d, want %d", code, exitUsage)
			}

			if !strings.Contains(stderr.String(), test.want) || !strings.Contains(stderr.String(), "Usage of playback-smoke") {
				t.Errorf("stderr = %q, want %q and the usage", stderr.String(), test.want)
			}
		})
	}
}

func TestRunPrintsHelp(t *testing.T) {
	t.Parallel()

	var stderr bytes.Buffer

	if code := run(t.Context(), []string{"-h"}, &bytes.Buffer{}, &stderr); code != exitOK || !strings.Contains(stderr.String(), "-manifest") {
		t.Errorf("run -h = %d with %q, want %d and the flag list", code, stderr.String(), exitOK)
	}
}

func TestRunNamesAMissingFFprobe(t *testing.T) {
	t.Parallel()

	var stderr bytes.Buffer

	missing := filepath.Join(t.TempDir(), "no-ffprobe")
	code := run(t.Context(), []string{"-base", "http://127.0.0.1:1", "-ffprobe", missing}, &bytes.Buffer{}, &stderr)

	if code != exitFailure || !strings.Contains(stderr.String(), "not found; install FFmpeg") {
		t.Errorf("run = %d with %q, want %d and a hint to install FFmpeg", code, stderr.String(), exitFailure)
	}
}

func TestRunFailsWhenTheServerIsUnreachable(t *testing.T) {
	t.Parallel()

	_, server := newFakeServer(t)
	base := server.URL
	server.Close()

	var stderr bytes.Buffer

	// The ffprobe lookup uses this test binary, which always exists, so the run reaches the
	// server check without FFmpeg on the machine.
	executable := testExecutable(t)
	code := run(t.Context(), []string{"-base", base, "-manifest", passingManifest(t), "-ffprobe", executable}, &bytes.Buffer{}, &stderr)

	if code != exitFailure || !strings.Contains(stderr.String(), "reach the server") {
		t.Errorf("run = %d with %q, want %d and the unreachable server", code, stderr.String(), exitFailure)
	}
}

func testExecutable(t *testing.T) string {
	t.Helper()

	executable, err := os.Executable()
	if err != nil {
		t.Fatalf("locate the test binary: %v", err)
	}

	return executable
}
