package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/magpeek/mivo/internal/platform/brand"
)

func TestRun(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		args           []string
		wantCode       int
		wantStdoutHead string
		wantStderrPart string
	}{
		{
			name:           "version flag prints one line",
			args:           []string{"--version"},
			wantCode:       exitOK,
			wantStdoutHead: brand.DisplayName + " ",
		},
		{
			name:           "unknown flag fails with usage",
			args:           []string{"--unknown-flag"},
			wantCode:       exitUsage,
			wantStderrPart: "Usage",
		},
		{
			name:           "help flag prints usage and succeeds",
			args:           []string{"-h"},
			wantCode:       exitOK,
			wantStderrPart: "-version",
		},
		{
			name:           "unexpected positional argument fails with usage",
			args:           []string{"serve"},
			wantCode:       exitUsage,
			wantStderrPart: "Usage",
		},
		{
			name:     "no arguments succeeds",
			args:     nil,
			wantCode: exitOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var stdout, stderr bytes.Buffer

			code := run(tt.args, &stdout, &stderr)
			if code != tt.wantCode {
				t.Fatalf("exit code = %d, want %d (stderr: %q)", code, tt.wantCode, stderr.String())
			}

			assertStdout(t, stdout.String(), tt.wantStdoutHead)

			if !strings.Contains(stderr.String(), tt.wantStderrPart) {
				t.Errorf("stderr = %q, want it to contain %q", stderr.String(), tt.wantStderrPart)
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
