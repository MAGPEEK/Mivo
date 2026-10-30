package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const messageFilePermissions = 0o600

func TestRun(t *testing.T) {
	t.Parallel()

	directory := t.TempDir()
	validFile := writeMessageFile(t, directory, "valid", "chore: hook test\n")
	invalidFile := writeMessageFile(t, directory, "invalid", "update stuff\n")

	tests := []struct {
		name       string
		args       []string
		wantCode   int
		wantStderr string
	}{
		{name: "valid message", args: []string{validFile}, wantCode: exitOK, wantStderr: ""},
		{
			name:       "invalid message",
			args:       []string{invalidFile},
			wantCode:   exitFailure,
			wantStderr: "type(scope)!: subject",
		},
		{
			name:       "missing file",
			args:       []string{filepath.Join(directory, "missing")},
			wantCode:   exitFailure,
			wantStderr: "read message file",
		},
		{name: "no argument", args: nil, wantCode: exitUsage, wantStderr: usageText},
		{name: "two arguments", args: []string{validFile, validFile}, wantCode: exitUsage, wantStderr: usageText},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			var stderr strings.Builder

			code := run(test.args, &stderr)
			if code != test.wantCode {
				t.Errorf("run(%q) = %d, want %d (stderr: %s)", test.args, code, test.wantCode, stderr.String())
			}

			if test.wantStderr == "" && stderr.Len() > 0 {
				t.Errorf("stderr = %q, want empty", stderr.String())
			}

			if !strings.Contains(stderr.String(), test.wantStderr) {
				t.Errorf("stderr = %q, want it to contain %q", stderr.String(), test.wantStderr)
			}
		})
	}
}

func writeMessageFile(t *testing.T, directory, name, content string) string {
	t.Helper()

	path := filepath.Join(directory, name)
	if err := os.WriteFile(path, []byte(content), messageFilePermissions); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}

	return path
}
