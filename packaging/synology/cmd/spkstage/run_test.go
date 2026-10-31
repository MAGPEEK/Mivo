package main

import (
	"errors"
	"io"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/magpeek/mivo/packaging/synology/internal/spkmeta"
)

func validArgs(t *testing.T, family string) []string {
	t.Helper()

	inputs := writeInputs(t, family, fakeSourceSelected)

	return []string{
		"-family", family, "-version", testVersion, "-binary", inputs.binary,
		"-ffmpeg-dir", inputs.ffmpegDir, "-out", filepath.Join(t.TempDir(), "project"),
		"-repo", repoRoot(t), "-source-date-epoch", strconv.Itoa(testEpoch),
	}
}

func TestRunStagesAndPrintsASummary(t *testing.T) {
	t.Parallel()

	var stdout, stderr strings.Builder
	if err := run(validArgs(t, "armv7"), &stdout, &stderr); err != nil {
		t.Fatalf("run: %v (stderr: %s)", err, stderr.String())
	}

	if !strings.Contains(stdout.String(), "staged the armv7 project") || !strings.Contains(stdout.String(), "0.1.0-1020") {
		t.Errorf("stdout = %q", stdout.String())
	}
}

func TestRunReportsRefusals(t *testing.T) {
	t.Parallel()

	args := append(validArgs(t, "x86_64"), "-package-revision", "10")

	err := run(args, io.Discard, io.Discard)
	if !errors.Is(err, spkmeta.ErrInvalidVersion) || !strings.Contains(err.Error(), "-package-revision 10") {
		t.Errorf("error = %v, want an invalid package revision", err)
	}
}

func TestRunWithHelpSucceeds(t *testing.T) {
	t.Parallel()

	var stderr strings.Builder
	if err := run([]string{"-h"}, io.Discard, &stderr); err != nil {
		t.Errorf("run -h: %v", err)
	}

	if !strings.Contains(stderr.String(), "-ffmpeg-dir") {
		t.Errorf("usage lacks -ffmpeg-dir: %s", stderr.String())
	}
}

func TestRunWritesSummaryErrors(t *testing.T) {
	t.Parallel()

	err := run(validArgs(t, "x86_64"), failingWriter{}, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "write summary") {
		t.Errorf("error = %v, want a summary write error", err)
	}
}

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errors.New("broken pipe") }

func TestParseOptionsRefusals(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		args    []string
		want    string
		wantErr error
	}{
		{name: "no flags", args: nil, want: "-family", wantErr: errMissingFlag},
		{name: "no out", args: []string{"-family", "x86_64", "-version", "1.0.0", "-binary", "b", "-ffmpeg-dir", "f"}, want: "-out", wantErr: errMissingFlag},
		{name: "empty repo", args: []string{"-family", "x86_64", "-version", "1.0.0", "-binary", "b", "-ffmpeg-dir", "f", "-out", "o", "-repo", ""}, want: "-repo", wantErr: errMissingFlag},
		{name: "positional argument", args: []string{"-family", "x86_64", "extra"}, want: `"extra"`, wantErr: errInvalidFlag},
		{name: "negative epoch", args: []string{"-family", "x", "-version", "1", "-binary", "b", "-ffmpeg-dir", "f", "-out", "o", "-source-date-epoch", "-5"}, want: "-source-date-epoch -5", wantErr: errInvalidFlag},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			_, err := parseOptions(test.args, io.Discard, noEnvironment)
			if !errors.Is(err, test.wantErr) || !strings.Contains(err.Error(), test.want) {
				t.Errorf("error = %v, want %v naming %q", err, test.wantErr, test.want)
			}
		})
	}
}

func TestParseOptionsRejectsUnknownFlags(t *testing.T) {
	t.Parallel()

	if _, err := parseOptions([]string{"-colour"}, io.Discard, noEnvironment); err == nil {
		t.Error("unknown flag accepted")
	}
}

func noEnvironment(string) (string, bool) { return "", false }

func TestModificationTimeSources(t *testing.T) {
	t.Parallel()

	base := []string{"-family", "x86_64", "-version", "1.0.0", "-binary", "b", "-ffmpeg-dir", "f", "-out", "o"}

	tests := []struct {
		name  string
		extra []string
		env   map[string]string
		want  int64
	}{
		{name: "default", want: 0},
		{name: "environment", env: map[string]string{sourceDateEpochVariable: "1700000000"}, want: 1700000000},
		{name: "empty environment value", env: map[string]string{sourceDateEpochVariable: ""}, want: 0},
		{name: "flag wins", extra: []string{"-source-date-epoch", "42"}, env: map[string]string{sourceDateEpochVariable: "1700000000"}, want: 42},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			env := func(name string) (string, bool) {
				value, found := test.env[name]

				return value, found
			}

			opts, err := parseOptions(append(append([]string{}, base...), test.extra...), io.Discard, env)
			if err != nil {
				t.Fatalf("parseOptions: %v", err)
			}

			if !opts.modTime.Equal(time.Unix(test.want, 0)) {
				t.Errorf("modTime = %s, want %d", opts.modTime, test.want)
			}
		})
	}
}

func TestInvalidSourceDateEpochVariable(t *testing.T) {
	t.Parallel()

	env := func(string) (string, bool) { return "yesterday", true }
	args := []string{"-family", "x86_64", "-version", "1.0.0", "-binary", "b", "-ffmpeg-dir", "f", "-out", "o"}

	_, err := parseOptions(args, io.Discard, env)
	if !errors.Is(err, errInvalidFlag) || !strings.Contains(err.Error(), sourceDateEpochVariable) {
		t.Errorf("error = %v, want one naming %s", err, sourceDateEpochVariable)
	}
}
