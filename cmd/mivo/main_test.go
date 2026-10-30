package main

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/magpeek/mivo/internal/platform/brand"
)

func TestRunParsesCommandLine(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		args            []string
		wantCode        int
		wantStdoutHead  string
		wantEmptyStderr bool
		wantStderrPart  string
	}{
		{
			name:            "version flag prints one line",
			args:            []string{"--version"},
			wantCode:        exitOK,
			wantStdoutHead:  brand.DisplayName + " ",
			wantEmptyStderr: true,
		},
		{
			name:           "unknown flag fails with the reason",
			args:           []string{"--unknown-flag"},
			wantCode:       exitUsage,
			wantStderrPart: "flag provided but not defined: -unknown-flag",
		},
		{
			name:           "help flag prints usage and succeeds",
			args:           []string{"-h"},
			wantCode:       exitOK,
			wantStderrPart: "-config-dir",
		},
		{
			name:           "unexpected positional argument fails with the reason",
			args:           []string{"serve"},
			wantCode:       exitUsage,
			wantStderrPart: `unexpected argument "serve"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var stdout, stderr bytes.Buffer

			env := runEnv{stdout: &stdout, stderr: &stderr, lookupEnv: lookupIn(nil)}

			code := run(t.Context(), tt.args, env, runHooks{})
			if code != tt.wantCode {
				t.Fatalf("exit code = %d, want %d (stderr: %q)", code, tt.wantCode, stderr.String())
			}

			assertStdout(t, stdout.String(), tt.wantStdoutHead)
			assertStderr(t, stderr.String(), tt.wantEmptyStderr, tt.wantStderrPart)
		})
	}
}

func TestRunReportsVersionWriteFailure(t *testing.T) {
	t.Parallel()

	var stderr bytes.Buffer

	env := runEnv{stdout: failingWriter{}, stderr: &stderr, lookupEnv: lookupIn(nil)}

	if code := run(t.Context(), []string{"--version"}, env, runHooks{}); code != exitFailure {
		t.Fatalf("exit code = %d, want %d", code, exitFailure)
	}

	if !strings.Contains(stderr.String(), "write version") {
		t.Errorf("stderr = %q, want the write error", stderr.String())
	}
}

func TestSingleLineJoinsLineBreaks(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  error
		want string
	}{
		{name: "plain error", err: errors.New("broken"), want: "broken"},
		{name: "joined errors", err: errors.Join(errors.New("first"), errors.New("second")), want: "first; second"},
		{name: "windows line break", err: errors.New("first\r\nsecond\rthird"), want: "first; second; third"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := singleLine(tt.err); got != tt.want {
				t.Errorf("singleLine() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestStoppedWhileStartingOnlyAcceptsTheShutdownCause(t *testing.T) {
	t.Parallel()

	cancelled, cancel := context.WithCancel(t.Context())
	cancel()

	tests := []struct {
		name string
		ctx  context.Context
		err  error
		want bool
	}{
		{name: "cancelled context and its error", ctx: cancelled, err: context.Canceled, want: true},
		{name: "cancelled context and another error", ctx: cancelled, err: errors.New("disk full"), want: false},
		{name: "running context", ctx: t.Context(), err: context.Canceled, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := stoppedWhileStarting(tt.ctx, tt.err); got != tt.want {
				t.Errorf("stoppedWhileStarting() = %t, want %t", got, tt.want)
			}
		})
	}
}

func assertStdout(t *testing.T, stdout, wantHead string) {
	t.Helper()

	if wantHead == "" {
		return
	}

	if !strings.HasPrefix(stdout, wantHead) {
		t.Errorf("stdout = %q, want prefix %q", stdout, wantHead)
	}

	if lines := strings.Count(stdout, "\n"); lines != 1 {
		t.Errorf("stdout has %d lines, want 1", lines)
	}
}

func assertStderr(t *testing.T, stderr string, wantEmpty bool, wantPart string) {
	t.Helper()

	if wantEmpty && stderr != "" {
		t.Errorf("stderr = %q, want it empty", stderr)
	}

	if !strings.Contains(stderr, wantPart) {
		t.Errorf("stderr = %q, want it to contain %q", stderr, wantPart)
	}

	if wantPart != "" && strings.Count(stderr, wantPart) != 1 {
		t.Errorf("stderr = %q, want %q exactly once", stderr, wantPart)
	}
}

// lookupIn returns an environment lookup over vars only, never the host's variables.
func lookupIn(vars map[string]string) func(key string) (string, bool) {
	return func(key string) (string, bool) {
		value, ok := vars[key]

		return value, ok
	}
}

// failingWriter rejects every write.
type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) {
	return 0, errors.New("stream closed")
}
